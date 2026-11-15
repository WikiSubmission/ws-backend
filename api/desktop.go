package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/desktop"
)

// desktopOrigin is the origin the Electron shell serves the app from.
const desktopOrigin = "app://wikisubmission"

const (
	desktopAuthID      = "local:desktop"
	desktopEmail       = "local@desktop.invalid"
	desktopDisplayName = "Local user"
	// The signing secret is regenerated on every launch, so a long token
	// lifetime only spans one session of the app.
	desktopTokenTTL = 365 * 24 * time.Hour
)

// desktopConfig is the single JSON line the Electron shell writes to stdin.
// Keeping it off the command line and out of the environment keeps the paths
// and any future secrets out of process listings.
type desktopConfig struct {
	// DataDir is where the database lives, normally the app's userData folder.
	DataDir string `json:"dataDir"`
	// PgArchiveDir holds the bundled embedded-postgres-binaries-*.txz.
	PgArchiveDir string `json:"pgArchiveDir"`
	// SnapshotDir holds the bundled Quran content (manifest.json plus one
	// file per table, from cmd/build-desktop-snapshot). It is loaded on first
	// run and again only when its content version changes.
	SnapshotDir string `json:"snapshotDir"`
}

// desktopReady is the single JSON line written to stdout once the server
// accepts connections. Logs go to stderr so stdout carries nothing else.
type desktopReady struct {
	Ready bool   `json:"ready"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// desktopEnvDenylist is cleared on startup so a stray .env or an inherited
// environment can never point the desktop sidecar at hosted services.
var desktopEnvDenylist = []string{
	"DATABASE_URL", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DOMAIN",
	"DATABASE_PORT", "DATABASE_NAME", "DATABASE_SSL_MODE",
	"OPENAI_API_KEY", "GROQ_API_KEY", "GROQ_BASE_URL", "GROQ_MODEL",
	"VAPID_PRIVATE_KEY", "VAPID_PUBLIC_KEY", "VAPID_SUBJECT",
	"WSLIB_BASE_URL", "STORE_AUTH_SECRET", "INTERNAL_BYPASS_TOKEN",
	"HEALTH_AUTH_SECRET", "HEALTH_AUTH_SECRET_PREVIOUS",
	"CORS_ALLOWED_ORIGINS", "TRUSTED_PROXIES", "PORT",
	"OFFLINE_BUNDLES_BASE_URL", "GOOGLE_APPLICATION_CREDENTIALS",
}

// runDesktop runs the offline desktop sidecar and returns the process exit
// code. Postgres is stopped on every path out of this function, including a
// panic, so a crash in the server never leaves an orphaned database behind.
func runDesktop(stdin io.Reader, stdout io.Writer) (code int) {
	for _, k := range desktopEnvDenylist {
		os.Unsetenv(k)
	}
	os.Setenv("APP_ENV", "desktop")
	// The hosted server runs in UTC, and pgx decodes timestamptz into Go's
	// local zone, so match it to serialize timestamps the same way.
	time.Local = time.UTC
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = os.Stderr
	gin.DefaultErrorWriter = os.Stderr

	in := bufio.NewReader(stdin)
	line, err := in.ReadBytes('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		slog.Error("Desktop: reading config from stdin failed", slog.Any("error", err))
		return 2
	}
	var cfg desktopConfig
	if err := json.Unmarshal(line, &cfg); err != nil {
		slog.Error("Desktop: config line is not valid JSON", slog.Any("error", err))
		return 2
	}

	root, err := desktop.ResolveRoot(cfg.DataDir)
	if err != nil {
		slog.Error("Desktop: resolving data directory failed", slog.Any("error", err))
		return 1
	}
	if root != cfg.DataDir {
		slog.Info("Desktop: using ASCII data directory", "requested", cfg.DataDir, "root", root)
	}

	ctx := context.Background()
	pg, err := desktop.StartPostgres(ctx, desktop.PostgresOptions{
		Layout:     desktop.NewLayout(root, string(desktop.PostgresVersion)),
		ArchiveDir: cfg.PgArchiveDir,
		Log:        os.Stderr,
	})
	if err != nil {
		slog.Error("Desktop: Postgres failed to start", slog.Any("error", err))
		return 1
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Desktop: panic, stopping Postgres", "panic", fmt.Sprint(r))
			code = 1
		}
		if err := pg.Stop(); err != nil {
			slog.Error("Desktop: stopping Postgres failed", slog.Any("error", err))
		}
	}()

	os.Setenv("DATABASE_URL", pg.URL)
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Desktop: database setup failed", slog.Any("error", err))
		return 1
	}
	defer database.Close()

	if cfg.SnapshotDir == "" {
		slog.Warn("Desktop: no snapshotDir configured; content tables stay as they are")
	} else if _, err := desktop.LoadSnapshot(ctx, database.Pool, cfg.SnapshotDir); err != nil {
		slog.Error("Desktop: loading the content snapshot failed", slog.Any("error", err))
		return 1
	}

	secret, err := randomSecret()
	if err != nil {
		slog.Error("Desktop: generating signing secret failed", slog.Any("error", err))
		return 1
	}
	os.Setenv("WS_BACKEND_JWT_SECRET", secret)
	token, err := provisionLocalUser(ctx, database, secret)
	if err != nil {
		slog.Error("Desktop: provisioning local user failed", slog.Any("error", err))
		return 1
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		slog.Error("Desktop: listen failed", slog.Any("error", err))
		return 1
	}

	// The shell closes stdin to ask for shutdown, and stdin also closes if the
	// shell dies, so the sidecar never outlives it.
	stop := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, in)
		close(stop)
	}()

	err = runServer(database, serverOptions{
		Desktop:  true,
		Listener: ln,
		Stop:     stop,
		OnReady: func() {
			msg, _ := json.Marshal(desktopReady{
				Ready: true,
				Port:  ln.Addr().(*net.TCPAddr).Port,
				Token: token,
			})
			fmt.Fprintln(stdout, string(msg))
		},
	})
	if err != nil {
		slog.Error("Desktop: server stopped with error", slog.Any("error", err))
		return 1
	}
	return 0
}

// provisionLocalUser creates the single desktop user on first run and mints
// the access token the shell attaches to every proxied request.
func provisionLocalUser(ctx context.Context, database *db.DB, secret string) (string, error) {
	name := desktopDisplayName
	if _, err := database.UpsertUser(ctx, desktopAuthID, desktopEmail, &name); err != nil {
		return "", err
	}
	minter, err := auth.NewMinter(secret)
	if err != nil {
		return "", err
	}
	return minter.MintAccessToken(auth.Identity{
		AuthID: desktopAuthID,
		Email:  desktopEmail,
		Name:   desktopDisplayName,
	}, desktopTokenTTL, time.Now())
}

func randomSecret() (string, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// desktopBackupMaxBytes bounds a restore upload. Real backups are far smaller;
// the cap keeps a wrong file from being read into memory whole.
const desktopBackupMaxBytes = 256 << 20

// registerDesktopRoutes adds the desktop-only backup endpoints. They sit
// outside the OpenAPI contract, so they are registered before the validator.
func registerDesktopRoutes(r *gin.Engine, database *db.DB, verifier *auth.AuthJSVerifier) {
	g := r.Group("/api/v1/desktop")
	g.Use(auth.RequireAuth(verifier))

	g.GET("/backup", func(c *gin.Context) {
		userID, ok := desktopUserID(c, database)
		if !ok {
			return
		}
		b, err := desktop.ExportBackup(c.Request.Context(), database.Pool, userID)
		if err != nil {
			slog.Error("Desktop backup failed", slog.Any("error", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "backup failed"})
			return
		}
		name := "wikisubmission-backup-" + b.CreatedAt.Format("2006-01-02") + ".json"
		c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
		c.JSON(http.StatusOK, b)
	})

	g.POST("/restore", func(c *gin.Context) {
		userID, ok := desktopUserID(c, database)
		if !ok {
			return
		}
		var b desktop.Backup
		dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, desktopBackupMaxBytes))
		if err := dec.Decode(&b); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the file is not a valid backup"})
			return
		}
		res, err := desktop.RestoreBackup(c.Request.Context(), database.Pool, userID, &b)
		if err != nil {
			slog.Error("Desktop restore failed", slog.Any("error", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "restore failed; your data was not changed", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	})
}

func desktopUserID(c *gin.Context, database *db.DB) (int, bool) {
	authID, _ := c.Get(auth.ContextKeyUID)
	id, _ := authID.(string)
	user, err := database.FetchUserByAuthID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unknown user"})
		return 0, false
	}
	return user.ID, true
}

// desktopHostGuard rejects requests whose Host header is not the loopback
// address the server listens on. A web page open in the user's browser can
// reach 127.0.0.1, and DNS rebinding can make it look same-origin; it cannot
// make the browser send Host: 127.0.0.1:<port>.
func desktopHostGuard(ln net.Listener) gin.HandlerFunc {
	want := ""
	if ln != nil {
		want = ln.Addr().String()
	}
	return func(c *gin.Context) {
		if c.Request.Host != want {
			c.AbortWithStatus(http.StatusMisdirectedRequest)
			return
		}
		c.Next()
	}
}

// desktopRoutes are the API paths the offline Quran browser uses. A path is
// allowed if it equals an entry or continues it with "/". Everything else,
// including blog, Bible, music, games, editorial writes and admin, returns 404.
var desktopRoutes = []string{
	"/health", "/healthz", "/livez", "/readyz", "/ping",

	"/api/v1/languages",
	"/api/v1/chapters",
	"/api/v1/appendices",
	"/api/v1/quran",
	"/api/v1/search",
	"/api/v1/roots",
	"/api/v1/topic-index",
	"/api/v1/editorial/public/appendices",

	"/api/v1/users/me",
	"/api/v1/me/bookmark-categories",
	"/api/v1/me/bookmark-entries",
	"/api/v1/me/cover-to-cover",
	"/api/v1/me/scripture-state",
	"/api/v1/me/notes",
	"/api/v1/me/search",
	"/api/v1/me/reading-progress",
	"/api/v1/me/streak",
	"/api/v1/me/reading-stats",
	"/api/v1/me/reading-log",
	"/api/v1/me/preferences",
	"/api/v1/me/collections",
	"/api/v1/me/activity",

	"/api/v1/desktop/backup",
	"/api/v1/desktop/restore",
}

func desktopRouteAllowed(path string) bool {
	for _, p := range desktopRoutes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func desktopAllowlist() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !desktopRouteAllowed(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not available in the desktop app"})
			return
		}
		c.Next()
	}
}
