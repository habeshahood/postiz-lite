package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/habeshahood/postiz-lite/internal/api"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
	"github.com/habeshahood/postiz-lite/internal/scheduler"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := loadConfig()

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := db.NewStore(pool)

	// Start the post scheduler (replaces Temporal)
	sched := scheduler.New(store)
	sched.Start()
	defer sched.Stop()

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(chimw.Compress(5))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.FrontendURL, "http://localhost:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "auth", "showorg", "impersonate", "reload", "onboarding"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health (always at root, regardless of prefix)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve uploaded files
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "/uploads"
	}
	r.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir(uploadDir))))

	// Mount API routes at the configured prefix.
	//
	// Behind nginx (Docker): nginx strips /api/ prefix, so routes arrive at /auth/login etc.
	//   → API_PREFIX="" (default), PORT=3000
	//
	// Standalone (no nginx): routes arrive as /api/auth/login etc.
	//   → API_PREFIX="/api", PORT=5000
	prefix := cfg.APIPrefix
	mountAPI(r, prefix, store, cfg.JWTSecret)

	// Reverse proxy to Next.js frontend (standalone mode only)
	if cfg.FrontendInternalURL != "" {
		frontendURL, err := url.Parse(cfg.FrontendInternalURL)
		if err != nil {
			slog.Error("invalid FRONTEND_INTERNAL_URL", "url", cfg.FrontendInternalURL, "error", err)
			os.Exit(1)
		}
		proxy := httputil.NewSingleHostReverseProxy(frontendURL)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("frontend proxy error", "path", r.URL.Path, "error", err)
			http.Error(w, "Frontend unavailable", http.StatusBadGateway)
		}
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			proxy.ServeHTTP(w, r)
		})
		slog.Info("reverse proxy enabled", "frontendURL", cfg.FrontendInternalURL)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("postiz-lite starting", "port", cfg.Port, "bind", cfg.BindAddr, "apiPrefix", prefix)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

// mountAPI registers all API routes under the given prefix.
// prefix="" → routes at /auth/login, /posts, etc. (behind nginx)
// prefix="/api" → routes at /api/auth/login, /api/posts, etc. (standalone)
func mountAPI(r chi.Router, prefix string, store *db.Store, jwtSecret string) {
	mount := func(r chi.Router) {
		// Public API v1 (Rust client uses /public/v1/*)
		r.Route("/public/v1", func(r chi.Router) {
			r.Use(middleware.APIKeyAuth(store))
			api.RegisterPublicV1(r, store)
		})

		// Auth routes (no JWT required)
		api.RegisterAuth(r, store, jwtSecret)

		// JWT-protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(jwtSecret, store))
			api.RegisterInternal(r, store)
		})
	}

	if prefix == "" {
		mount(r)
	} else {
		r.Route(prefix, mount)
	}
}

type config struct {
	DatabaseURL         string
	RedisURL            string
	Port                int
	BindAddr            string
	APIPrefix           string
	FrontendURL         string
	FrontendInternalURL string
	JWTSecret           string
}

func loadConfig() config {
	port := 5000
	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	// API_PREFIX controls route mounting:
	//   "/api"     = standalone mode, routes include /api/ prefix
	//   "none"     = behind nginx, routes arrive without /api/ prefix
	//   ""         = auto-detect: if PORT <= 3100 assume behind nginx, else standalone
	apiPrefix := os.Getenv("API_PREFIX")
	if apiPrefix == "none" {
		apiPrefix = ""
	} else if apiPrefix == "" {
		if port <= 3100 {
			apiPrefix = "" // Behind nginx (port 3000)
		} else {
			apiPrefix = "/api" // Standalone (port 5000+)
		}
	}

	// FRONTEND_INTERNAL_URL enables the reverse proxy to Next.js (standalone mode only).
	// In Docker (behind nginx), leave empty — nginx handles frontend proxying.
	frontendInternal := os.Getenv("FRONTEND_INTERNAL_URL")

	return config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		Port:                port,
		BindAddr:            bindAddr,
		APIPrefix:           apiPrefix,
		FrontendURL:         os.Getenv("FRONTEND_URL"),
		FrontendInternalURL: frontendInternal,
		JWTSecret:           os.Getenv("JWT_SECRET"),
	}
}
