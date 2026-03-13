// Run from the workspace root: go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
//go:generate go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml

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
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	middleware "github.com/oapi-codegen/gin-middleware"
	"github.com/wikiSubmission/ws-backend/api/handlers"
	"github.com/wikiSubmission/ws-backend/db"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

func StartServer(database *db.DB) {
	swagger, _ := handlers.GetSwagger()
	r := gin.Default()

	r.Use(gin.Recovery())
	r.Use(SupportHEAD)

	p := ginprometheus.NewPrometheus("gin")
	p.Use(r)
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	proxiesEnv := os.Getenv("TRUSTED_PROXIES")
	if proxiesEnv != "" {
		proxyList := strings.Split(proxiesEnv, ",")
		r.SetTrustedProxies(proxyList)
		slog.Info("Trusted proxies configured", "proxies", proxyList)
	} else {
		r.SetTrustedProxies(nil)
		slog.Warn("No trusted proxies set; trusting all (standard Gin default)")
	}

	// Rate Limiting (5 req/sec per IP)
	// limiter := tollbooth.NewLimiter(5, nil)

	// API Routes

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})

	// r.GET("/favicon.ico", func (c *gin.Context)  {
	// 	logo_key := "wikisubmission/media/images/logo.png"
	// })

	r.GET("/health", func(c *gin.Context) {
		// Deep health check: verify DB connectivity
		if err := database.Pool.Ping(c.Request.Context()); err != nil {
			slog.Error("Health check failed: DB unreachable", slog.Any("error", err))
			c.JSON(http.StatusInternalServerError, gin.H{"status": "unhealthy", "error": "db error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
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

	// 4. Register handlers using the BaseURL
	serverImpl := &handlers.Server{DB: database}
	strictHandler := handlers.NewStrictHandler(serverImpl, nil)

	// Register handlers to the root 'r' but with the BaseURL prefix
	handlers.RegisterHandlersWithOptions(r, strictHandler, handlers.GinServerOptions{
		BaseURL: apiPrefix,
	})

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
		Addr:    fmt.Sprintf(":%v", port),
		Handler: r,
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
