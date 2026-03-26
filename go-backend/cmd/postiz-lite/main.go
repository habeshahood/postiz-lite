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
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "auth", "showorg"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve uploaded files from disk
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "/uploads"
	}
	r.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir(uploadDir))))

	// Public API v1 — API key auth (used by Rust postiz_client)
	r.Route("/api/public/v1", func(r chi.Router) {
		r.Use(middleware.APIKeyAuth(store))
		api.RegisterPublicV1(r, store)
	})

	// Internal API — JWT auth (used by Next.js frontend)
	r.Route("/api", func(r chi.Router) {
		api.RegisterAuth(r, store, cfg.JWTSecret)
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(cfg.JWTSecret, store))
			api.RegisterInternal(r, store)
		})
	})

	// Reverse proxy: everything else → Next.js frontend on FRONTEND_INTERNAL_PORT
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
		slog.Info("postiz-lite starting", "port", cfg.Port, "bind", cfg.BindAddr)
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

type config struct {
	DatabaseURL         string
	RedisURL            string
	Port                int
	BindAddr            string
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
	frontendInternal := os.Getenv("FRONTEND_INTERNAL_URL")
	if frontendInternal == "" {
		frontendInternal = "http://localhost:4200"
	}
	return config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		Port:                port,
		BindAddr:            bindAddr,
		FrontendURL:         os.Getenv("FRONTEND_URL"),
		FrontendInternalURL: frontendInternal,
		JWTSecret:           os.Getenv("JWT_SECRET"),
	}
}
