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
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

func StartServer(database *db.DB) {
	swagger, _ := handlers.GetSwagger()

	// ── Auth setup ───────────────────────────────────────────────────────────
	jwksURL := os.Getenv("SUPABASE_JWKS_URL")
	jwksCache, err := auth.NewJWKSCache(context.Background(), jwksURL)
	if err != nil {
		slog.Warn("JWKS cache init failed, auth endpoints will return errors", "error", err)
		jwksCache = &auth.JWKSCache{} // no-op
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
	r.Use(RateLimitMiddleware(20))

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
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		SilenceServersWarning: true,
	}

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
	serverImpl := &handlers.Server{DB: database}
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
	authed := r.Group("/api/v1")
	authed.Use(auth.RequireAuth(jwksCache))
	authed.POST("/users", userHandler.UpsertUser)
	authed.GET("/users/:id", userHandler.GetUserByID)

	// ── Admin routes ─────────────────────────────────────────────────────────
	admin := r.Group("/api/v1")
	admin.Use(auth.RequireAuth(jwksCache))
	admin.Use(auth.RequireAdmin(database))
	admin.GET("/users", userHandler.GetUsers)
	admin.PATCH("/users/:id", userHandler.UpdateUser)

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
