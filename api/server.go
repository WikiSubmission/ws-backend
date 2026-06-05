// Run from the repo root: go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml
//go:generate go tool oapi-codegen -config ../oapi-codegen.yaml ../openapi.yaml

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	middleware "github.com/oapi-codegen/gin-middleware"
	apitembed "github.com/wikiSubmission/ws-backend/api/embed"
	"github.com/wikiSubmission/ws-backend/api/handlers"
	"github.com/wikiSubmission/ws-backend/api/health"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/sanity"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

func StartServer(database *db.DB) {
	swagger, _ := handlers.GetSwagger()

	// ── Auth setup ───────────────────────────────────────────────────────────
	jwtSecret := os.Getenv("WS_BACKEND_JWT_SECRET")
	verifier, err := auth.NewAuthJSVerifier(jwtSecret)
	if err != nil {
		slog.Warn("Auth.js verifier init failed, auth endpoints will return errors", "error", err)
		verifier = &auth.AuthJSVerifier{}
	}
	userHandler := handlers.UserHandler{DB: database}

	// gin.New() instead of gin.Default() to avoid double-adding Recovery.
	r := gin.New()

	originsEnv := os.Getenv("CORS_ALLOWED_ORIGINS")
	allowedOrigins := []string{"*"} // Default fallback

	// If the env var is set and not empty, split it into a slice
	if originsEnv != "" {
		allowedOrigins = strings.Split(originsEnv, ",")
		// Clean up whitespace in case of "https://a.com, https://b.com"
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
		slog.Info("CORS configured with specific origins", "origins", allowedOrigins)
	} else {
		slog.Warn("CORS_ALLOWED_ORIGINS not set; defaulting to '*' (Permissive)")
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.Use(gin.Logger())
	r.Use(func(c *gin.Context) {
		c.Next()
		for _, e := range c.Errors {
			slog.Error("gin request error",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"status", c.Writer.Status(),
				"error", e.Error(),
			)
		}
	})
	r.Use(gin.Recovery())
	r.Use(SupportHEAD)

	p := ginprometheus.NewPrometheus("gin")
	p.Use(r)
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// Trusted proxies — must be set explicitly; default to none (no IP spoofing).
	proxiesEnv := os.Getenv("TRUSTED_PROXIES")
	if proxiesEnv != "" {
		proxyList := strings.Split(proxiesEnv, ",")
		r.SetTrustedProxies(proxyList)
		slog.Info("Trusted proxies configured", "proxies", proxyList)
	} else {
		r.SetTrustedProxies(nil)
		slog.Warn("No TRUSTED_PROXIES set; all proxy headers will be ignored")
	}

	// Rate limiting — 20 req/s per IP across all API routes.
	// INTERNAL_BYPASS_TOKEN, when set, lets trusted internal callers skip
	// the limiter via the X-Internal-Auth header (e.g. SSR frontends).
	r.Use(RateLimitMiddleware(20, os.Getenv("INTERNAL_BYPASS_TOKEN")))

	// API Routes

	// r.GET("/favicon.ico", func (c *gin.Context)  {
	// 	logo_key := "wikisubmission/media/images/logo.png"
	// })

	// Health endpoints — see wikisubmission-infra/runbooks/health-endpoints.md
	healthDeps := health.New("ws-backend",
		health.WithCheck("postgres", true, func(ctx context.Context) error {
			return database.Pool.Ping(ctx)
		}),
		health.WithEnv("APP_ENV", os.Getenv("APP_ENV")),
	)
	healthDeps.Register(r, healthSecretsFromEnv)

	// Legacy aliases — keep during cutover, remove after all monitors updated.
	r.GET("/health", healthDeps.Liveness())
	r.GET("/ping", healthDeps.Liveness())
	r.GET("/robots.txt", func(c *gin.Context) {
		c.String(http.StatusOK,
			"User-agent: *\n"+
				"\n"+
				"# Public Quran reader API\n"+
				"Allow: /api/v1/quran\n"+
				"\n"+
				"# Block all other API paths (editor, history, auth)\n"+
				"Disallow: /api/\n"+
				"\n"+
				"# Block utility endpoints\n"+
				"Disallow: /health\n"+
				"Disallow: /healthz\n"+
				"Disallow: /ping\n"+
				"\n"+
				"# Allow everything else (UI routes)\n"+
				"Allow: /\n",
		)
	})

	r.GET("/sitemap.xml", func(c *gin.Context) {
		c.Header("Content-Type", "application/xml")
		c.Header("Cache-Control", "public, max-age=86400")
		c.String(http.StatusOK,
			"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
				"<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n"+
				"  <url>\n"+
				"    <loc>https://library.wikisubmission.org/</loc>\n"+
				"    <changefreq>daily</changefreq>\n"+
				"    <priority>1.0</priority>\n"+
				"  </url>\n"+
				"  <url>\n"+
				"    <loc>https://library.wikisubmission.org/explorer</loc>\n"+
				"    <changefreq>hourly</changefreq>\n"+
				"    <priority>0.8</priority>\n"+
				"  </url>\n"+
				"  <url>\n"+
				"    <loc>https://library.wikisubmission.org/library</loc>\n"+
				"    <changefreq>weekly</changefreq>\n"+
				"    <priority>0.7</priority>\n"+
				"  </url>\n"+
				"  <url>\n"+
				"    <loc>https://library.wikisubmission.org/search</loc>\n"+
				"    <changefreq>monthly</changefreq>\n"+
				"    <priority>0.5</priority>\n"+
				"  </url>\n"+
				"</urlset>\n",
		)
	})
	if len(swagger.Servers) <= 0 {
		slog.Error("No servers block set in openapi.yaml, please set at least one.")
	}

	serverURL := swagger.Servers[0].URL

	// Example logic to pick the right one:
	appEnv := os.Getenv("APP_ENV") // e.g., "production" or "development"
	for _, s := range swagger.Servers {
		if strings.Contains(strings.ToLower(s.Description), appEnv) {
			serverURL = s.URL
			break
		}
	}

	// 2. Safely parse the path for Gin
	u, _ := url.Parse(serverURL)
	apiPrefix := u.Path
	apiPrefix = "/" + strings.Trim(apiPrefix, "/")

	slog.Info("Mounting API", "prefix", apiPrefix)

	// Reconstruct scheme+host on the request URL so the OpenAPI validator can
	// match against the servers block. Behind a reverse proxy (Railway, etc.)
	// c.Request.URL.Scheme and .Host are empty — kin-openapi returns 404 if it
	// can't match any server URL.
	//
	// After setting scheme/host from headers we scan the spec's servers list and
	// snap the URL to whichever entry shares the same hostname (port-insensitive).
	// This means every server in the servers: block works automatically — adding a
	// new domain only requires an entry in openapi.yaml, no code change needed.
	isProd := appEnv == "production"
	r.Use(func(c *gin.Context) {
		if c.Request.URL.Scheme == "" {
			if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
				// Honour whatever the proxy (Cloudflare, Railway public proxy) says.
				c.Request.URL.Scheme = proto
			} else if isProd && !strings.Contains(c.Request.Host, ".railway.internal") {
				// Public prod requests without a proxy header are still HTTPS.
				c.Request.URL.Scheme = "https"
			} else {
				// Railway internal traffic (no proxy) and local dev are plain HTTP.
				c.Request.URL.Scheme = "http"
			}
		}
		if c.Request.URL.Host == "" {
			c.Request.URL.Host = c.Request.Host
		}

		// Snap scheme+host to the matching spec server so kin-openapi always finds
		// an exact match. Comparison is hostname-only (ignores port) so that e.g.
		// localhost:8080 matches the spec entry localhost:8082.
		reqHostname := c.Request.URL.Hostname()
		for _, s := range swagger.Servers {
			u, err := url.Parse(s.URL)
			if err != nil {
				continue
			}
			if u.Hostname() == reqHostname {
				c.Request.URL.Scheme = u.Scheme
				c.Request.URL.Host = u.Host
				break
			}
		}

		c.Next()
	})

	// 2. Define validator options
	validatorOptions := &middleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: func(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
				if input.SecuritySchemeName != "bearerAuth" {
					return nil
				}
				req := input.RequestValidationInput.Request
				header := req.Header.Get("Authorization")
				if !strings.HasPrefix(header, "Bearer ") {
					return fmt.Errorf("missing bearer token")
				}
				token := strings.TrimPrefix(header, "Bearer ")
				authID, email, name, err := verifier.VerifyToken(token)
				if err != nil {
					return fmt.Errorf("invalid token")
				}

				// Propagate auth identity into the live Gin request/context so strict
				// handlers can read it at runtime.
				if gc := middleware.GetGinContext(ctx); gc != nil {
					reqCtx := context.WithValue(gc.Request.Context(), auth.ContextKeyUID, authID)
					reqCtx = context.WithValue(reqCtx, auth.ContextKeyEmail, email)
					reqCtx = context.WithValue(reqCtx, auth.ContextKeyName, name)
					gc.Request = gc.Request.WithContext(reqCtx)
					gc.Set(auth.ContextKeyUID, authID)
					gc.Set(auth.ContextKeyEmail, email)
					gc.Set(auth.ContextKeyName, name)
				}

				reqCtx := context.WithValue(req.Context(), auth.ContextKeyUID, authID)
				reqCtx = context.WithValue(reqCtx, auth.ContextKeyEmail, email)
				reqCtx = context.WithValue(reqCtx, auth.ContextKeyName, name)
				input.RequestValidationInput.Request = req.WithContext(reqCtx)
				return nil
			},
		},
		SilenceServersWarning: true,
	}

	// Sanity proxy: webhook + image passthrough live OUTSIDE the OpenAPI surface.
	// Register before the validator middleware so the validator does not 404 them.
	sanityClient, err := sanity.NewFromEnv()
	if err != nil {
		slog.Error("Sanity client init failed", slog.Any("error", err))
		os.Exit(1)
	}
	registerSanityRoutes(r, sanityClient)

	// Editor routes (Quran Games catalog) live OUTSIDE the OpenAPI surface, so
	// like the sanity routes they must be registered before the validator or it
	// would 404 them. Gated by the DB-backed games_editor permission (or admin
	// role); see auth.RequireEditor. Replaced the GAMES_EDITOR_EMAILS env
	// allowlist so editors can be granted through the admin access page.
	gamesAdmin := handlers.GamesAdminHandler{DB: database}
	editor := r.Group("/api/v1/admin/games")
	editor.Use(auth.RequireAuth(verifier))
	editor.Use(auth.RequireEditor(database))
	editor.GET("/passages", gamesAdmin.ListPassages)
	editor.GET("/passages/:id", gamesAdmin.GetPassage)
	editor.POST("/passages/:id/status", gamesAdmin.SetStatus)
	editor.GET("/maintenance/frequency-stats", gamesAdmin.GetFrequencyStats)
	editor.GET("/maintenance/lemma-stats", gamesAdmin.GetLemmaStats)
	editor.POST("/maintenance/seed-frequency", gamesAdmin.SeedFrequency)
	editor.POST("/maintenance/load-lemmas", gamesAdmin.LoadLemmas)
	editor.POST("/curate", gamesAdmin.Curate)

	// In-play check endpoint (instant per-blank feedback). Lives outside the
	// OpenAPI surface like the editor routes, so it must be registered before
	// the validator. Returns only correct/incorrect, never the answer.
	gamesPlay := handlers.GamesPlayHandler{DB: database}
	play := r.Group("/api/v1/games/fill-blank")
	play.Use(auth.RequireAuth(verifier))
	play.POST("/check", gamesPlay.CheckBlank)

	// Activity feed + consent. Out-of-contract /me routes that record search /
	// browse events and expose a per-user consent toggle. Drops writes when
	// consent is off, so the client need not check first.
	meActivity := handlers.MeActivityHandler{DB: database}
	activity := r.Group("/api/v1/me")
	activity.Use(auth.RequireAuth(verifier))
	activity.GET("/activity", meActivity.List)
	activity.POST("/activity", meActivity.Record)
	activity.DELETE("/activity", meActivity.Clear)
	activity.GET("/consent", meActivity.GetConsent)
	activity.PUT("/consent", meActivity.SetConsent)
	meExport := handlers.MeExportHandler{
		DB:       database,
		Uploader: buildExportUploader(),
	}
	activity.POST("/export", meExport.PostExport)
	activity.GET("/export/status", meExport.GetStatus)
	meDelete := handlers.MeDeleteHandler{DB: database}
	activity.POST("/delete-content", meDelete.PostDelete)
	activity.GET("/delete-content/status", meDelete.GetDeleteStatus)
	pushHandler := handlers.PushHandler{DB: database, Sender: handlers.NewWebPushSender()}
	activity.POST("/push/subscribe", pushHandler.Subscribe)
	activity.DELETE("/push/subscribe", pushHandler.Unsubscribe)
	activity.POST("/push/test", pushHandler.SendTest)

	// 3. APPLY VALIDATOR TO THE MAIN ROUTER
	// This ensures it sees "/api/v1/quran" and can match it against the swagger servers
	r.Use(middleware.OapiRequestValidatorWithOptions(swagger, validatorOptions))

	// Cache-Control for public reader endpoints. Quran/search responses are
	// deterministic and immutable enough for a 1-hour CDN/browser cache.
	r.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasSuffix(p, "/quran") || strings.HasSuffix(p, "/search") {
			c.Header("Cache-Control", "public, max-age=3600")
		} else if strings.HasSuffix(p, "/languages") || strings.HasSuffix(p, "/chapters") || strings.HasSuffix(p, "/appendices") {
			c.Header("Cache-Control", "public, max-age=86400")
		}
		c.Next()
	})

	// 4. Register handlers using the BaseURL
	serverImpl := &handlers.Server{DB: database, Sanity: sanityClient}
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		serverImpl.Embedder = apitembed.NewOpenAIEmbedder(apiKey)
		slog.Info("Semantic search enabled", "model", "text-embedding-3-small", "dims", 512)
	} else {
		slog.Warn("OPENAI_API_KEY not set — semantic search disabled, using FTS only")
	}
	strictHandler := handlers.NewStrictHandler(serverImpl, nil)

	// Register handlers to the root 'r' but with the BaseURL prefix
	handlers.RegisterHandlersWithOptions(r, strictHandler, handlers.GinServerOptions{
		BaseURL: apiPrefix,
	})

	// ── Authenticated routes ─────────────────────────────────────────────────
	// /users/me, GET /users, and PATCH /users/{id} are served by the strict
	// handlers generated from openapi.yaml (see users_strict.go) and registered
	// via RegisterHandlersWithOptions above. Auth and authID propagation happen
	// in the validator's AuthenticationFunc. Only out-of-contract /users routes
	// are registered manually below.
	authed := r.Group("/api/v1")
	authed.Use(auth.RequireAuth(verifier))
	authed.POST("/users", userHandler.UpsertUser)
	authed.GET("/users/:id", userHandler.GetUserByID)

	if err := serverImpl.InitializeCaches(context.Background()); err != nil {
		slog.Error("Cache initialization failed — server cannot serve requests", slog.Any("error", err))
		os.Exit(1)
	}

	// Debug registered routes
	for _, route := range r.Routes() {
		slog.Info("Route registered", "method", route.Method, "path", route.Path)
	}

	// Server Configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	slog.Info("Starting API Server", "port", port)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%v", port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second, // prevent Slowloris attacks
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second, // generous for large /quran responses
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("API Server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server listen error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Channel to listen for interrupt signals (SIGINT, SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit // Block here until a signal is received
	slog.Info("Shutdown signal received; shutting down gracefully...")

	// Create a deadline for the shutdown process
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", slog.Any("error", err))
	}

	slog.Info("Server exited cleanly")
}

// buildExportUploader wires the S3-backed data-export uploader from the
// environment (EXPORTS_BUCKET_NAME + the CloudFront signing vars). When the
// required configuration is absent or invalid it falls back to a no-op uploader
// so the service still boots in environments without AWS access (e.g. local
// dev); export requests then fail gracefully as before.
// buildExportUploader wires the data-export uploader. All S3 + CloudFront access
// lives in ws-lib: ws-backend stores exports via ws-lib's authenticated
// /private/store endpoint over the internal network and holds no AWS credentials
// or signing keys. When WSLIB_BASE_URL or STORE_AUTH_SECRET is absent it falls
// back to a no-op uploader so the service still boots (feature disabled).
func buildExportUploader() handlers.ExportUploader {
	baseURL := strings.TrimRight(os.Getenv("WSLIB_BASE_URL"), "/")
	secret := os.Getenv("STORE_AUTH_SECRET")
	if baseURL == "" || secret == "" {
		slog.Warn("WSLIB_BASE_URL or STORE_AUTH_SECRET not set; user data export disabled")
		return handlers.NoopExportUploader{}
	}

	slog.Info("User data export uploader configured (ws-lib)", "base_url", baseURL)
	return handlers.WSLibExportUploader{
		BaseURL: baseURL,
		Secret:  []byte(secret),
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}
