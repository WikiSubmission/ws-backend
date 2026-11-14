package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// PostgresVersion is the bundled server version. The release pipeline must
// ship the matching embedded-postgres-binaries-<os>-<arch>-<version>.txz.
const PostgresVersion = embeddedpostgres.V18

const (
	dbName       = "wikisubmission"
	dbUser       = "postgres"
	passwordFile = "pg-password"
	// noDownloadURL makes any attempt by the library to fetch binaries fail
	// immediately. The desktop app must never download executables at runtime;
	// a missing archive is a packaging bug and should surface as an error.
	noDownloadURL = "http://127.0.0.1:9/embedded-postgres-download-disabled"
)

// PostgresOptions configures StartPostgres.
type PostgresOptions struct {
	Layout Layout
	// ArchiveDir holds the bundled Postgres .txz archive. It may be inside the
	// application install, which does not need to be an ASCII path because the
	// archive is only read, never executed from.
	ArchiveDir string
	// Log receives Postgres server output. Defaults to stderr.
	Log io.Writer
}

// Postgres is a running embedded server.
type Postgres struct {
	URL  string
	Port uint32
	ep   *embeddedpostgres.EmbeddedPostgres
}

// StartPostgres extracts the bundled binaries if needed, initializes a UTF-8
// cluster on first run, recovers from a previous crash, and starts the server
// on a free loopback port.
func StartPostgres(ctx context.Context, opts PostgresOptions) (*Postgres, error) {
	l := opts.Layout
	if opts.Log == nil {
		opts.Log = os.Stderr
	}
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return nil, err
	}

	password, err := loadOrCreatePassword(l)
	if err != nil {
		return nil, err
	}
	if err := recoverStaleServer(ctx, l); err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	cfg := embeddedpostgres.DefaultConfig().
		Version(PostgresVersion).
		Port(port).
		Username(dbUser).
		Password(password).
		Database(dbName).
		CachePath(opts.ArchiveDir).
		BinaryRepositoryURL(noDownloadURL).
		BinariesPath(l.Binaries).
		RuntimePath(l.Runtime).
		DataPath(l.Data).
		// UTF-8 is required for Arabic text; Windows initdb otherwise picks the
		// system code page (WIN1252). The C locale avoids depending on which OS
		// locales are installed.
		Encoding("UTF8").
		Locale("C").
		StartTimeout(90 * time.Second).
		StartParameters(map[string]string{
			"listen_addresses": "127.0.0.1",
			// Match the hosted server, so timestamps serialize the same way
			// (UTC) whatever the user's system time zone is.
			"timezone":                "UTC",
			"max_connections":         "30",
			"unix_socket_directories": "",
		}).
		Logger(opts.Log)

	ep := embeddedpostgres.NewDatabase(cfg)
	start := time.Now()
	if err := ep.Start(); err != nil {
		return nil, fmt.Errorf("desktop: start postgres: %w", err)
	}
	slog.Info("Embedded Postgres started", "port", port, "data", l.Data, "elapsed", time.Since(start).String())

	return &Postgres{
		URL:  fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", dbUser, password, port, dbName),
		Port: port,
		ep:   ep,
	}, nil
}

// Stop shuts the server down. It is safe to call more than once.
func (p *Postgres) Stop() error {
	if p == nil || p.ep == nil {
		return nil
	}
	err := p.ep.Stop()
	if errors.Is(err, embeddedpostgres.ErrServerNotStarted) {
		return nil
	}
	return err
}

// loadOrCreatePassword returns the cluster's superuser password. It is created
// with the cluster and must survive with it: an existing cluster without its
// password file cannot be opened, so that case is an error rather than a
// silent re-init that would discard user data.
func loadOrCreatePassword(l Layout) (string, error) {
	path := filepath.Join(l.Root, passwordFile)
	b, err := os.ReadFile(path)
	if err == nil {
		pw := strings.TrimSpace(string(b))
		if pw == "" {
			return "", fmt.Errorf("desktop: %s is empty", path)
		}
		return pw, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, statErr := os.Stat(filepath.Join(l.Data, "PG_VERSION")); statErr == nil {
		return "", fmt.Errorf("desktop: database exists at %s but %s is missing", l.Data, path)
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	pw := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(pw), 0o600); err != nil {
		return "", err
	}
	return pw, nil
}

// recoverStaleServer handles a cluster left behind by a crash. If a server is
// still running on the data directory (an orphan from a previous launch), it is
// stopped. If only the postmaster.pid file remains, it is removed so pg_ctl
// does not refuse to start.
//
// This assumes one app instance per data directory, which the Electron shell
// enforces with a single-instance lock.
func recoverStaleServer(ctx context.Context, l Layout) error {
	pidFile := filepath.Join(l.Data, "postmaster.pid")
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return nil
	}
	pgCtl := filepath.Join(l.Binaries, "bin", "pg_ctl")
	if _, err := os.Stat(pgCtl + exeSuffix()); err != nil {
		// No binaries yet, so no server of ours can be running.
		return os.Remove(pidFile)
	}

	status := exec.CommandContext(ctx, pgCtl, "status", "-D", l.Data)
	if err := status.Run(); err == nil {
		slog.Warn("Stopping Postgres left running by a previous launch", "data", l.Data)
		stop := exec.CommandContext(ctx, pgCtl, "stop", "-D", l.Data, "-m", "fast", "-w", "-t", "30")
		if out, err := stop.CombinedOutput(); err != nil {
			return fmt.Errorf("desktop: stop orphaned postgres: %w: %s", err, out)
		}
		return nil
	}

	slog.Warn("Removing stale postmaster.pid", "path", pidFile)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func freePort() (uint32, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return uint32(ln.Addr().(*net.TCPAddr).Port), nil
}
