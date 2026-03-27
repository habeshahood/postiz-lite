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
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/habeshahood/postiz-lite/internal/api"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
	"github.com/habeshahood/postiz-lite/internal/scheduler"
	"github.com/habeshahood/postiz-lite/internal/tenant"
	"github.com/redis/go-redis/v9"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	tenantsFile := os.Getenv("TENANTS_FILE")
	if tenantsFile == "" {
		tenantsFile = "/config/tenants.json"
	}

	configs, err := tenant.LoadConfigs(tenantsFile)
	if err != nil {
		slog.Warn("no tenants.json, falling back to single-tenant mode", "error", err)
		runSingleTenant()
		return
	}

	runMultiTenant(configs, tenantsFile)
}

func runMultiTenant(configs []tenant.Config, tenantsFile string) {
	ctx := context.Background()

	tm := tenant.NewTenantManager()

	for _, cfg := range configs {
		cfg := cfg // capture

		pool, err := db.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("failed to connect to database", "tenant", cfg.ID, "error", err)
			os.Exit(1)
		}

		var rdb *redis.Client
		redisURL := cfg.RedisURL
		if redisURL == "" {
			redisURL = "redis://localhost:6379"
		}
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("invalid redis_url", "tenant", cfg.ID, "error", err)
			os.Exit(1)
		}
		rdb = redis.NewClient(opts)
		if err := rdb.Ping(ctx).Err(); err != nil {
			slog.Error("failed to ping Redis", "tenant", cfg.ID, "error", err)
			os.Exit(1)
		}

		store := db.NewStore(pool)
		router := buildTenantRouter(store, cfg, rdb)

		// Build a context with this tenant's social keys for the scheduler's token refresh job.
		socialKeys := cfg.Social
		schedCtx := context.Background()
		schedCtx = tenant.WithSocialKeys(schedCtx, &socialKeys)
		schedCtx = tenant.WithFrontendURL(schedCtx, cfg.FrontendURL)

		sched := scheduler.New(store, schedCtx)
		sched.Start()

		lt := &tenant.LiveTenant{
			Config: cfg,
			Router: router,
			Pool:   pool,
			Redis:  rdb,
			StopFunc: func() {
				sched.Stop()
				rdb.Close()
				pool.Close()
			},
		}
		tm.Add(lt)

		slog.Info("tenant initialized", "id", cfg.ID, "hosts", cfg.Hosts)
	}

	// Admin router wraps the tenant dispatcher and exposes admin endpoints.
	adminRouter := chi.NewRouter()
	api.RegisterAdmin(adminRouter, tm, tenantsFile, buildTenantRouter)
	// Wrap: admin routes take priority, everything else falls through to TenantManager.
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the path starts with /admin/ — route to admin router.
		if strings.HasPrefix(r.URL.Path, "/admin/") || r.URL.Path == "/admin" {
			adminRouter.ServeHTTP(w, r)
			return
		}
		tm.ServeHTTP(w, r)
	})

	port := 3000
	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", bindAddr, port),
		Handler:      mainHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("postiz-lite multi-tenant starting", "port", port, "bind", bindAddr, "tenants", len(configs))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)

	for _, lt := range tm.Tenants() {
		lt.StopFunc()
	}
}

// buildTenantRouter constructs the chi.Router for a single tenant.
// It is used at startup (for each config) and by the admin API (for hot-add).
func buildTenantRouter(store *db.Store, cfg tenant.Config, rdb *redis.Client) chi.Router {
	uploadDir := cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "/uploads"
	}

	socialKeys := cfg.Social // copy
	tenantID := cfg.ID
	frontendURL := cfg.FrontendURL

	r := chi.NewRouter()
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

	// Inject tenant context for every request
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			c := req.Context()
			c = tenant.WithTenantID(c, tenantID)
			c = tenant.WithSocialKeys(c, &socialKeys)
			c = tenant.WithFrontendURL(c, frontendURL)
			c = tenant.WithUploadDir(c, uploadDir)
			next.ServeHTTP(w, req.WithContext(c))
		})
	})

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if err := store.Ping(req.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"error","tenant":"%s","error":"db ping failed"}`, tenantID)
			return
		}
		fmt.Fprintf(w, `{"status":"ok","tenant":"%s"}`, tenantID)
	})

	r.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir(uploadDir))))

	mountAPI(r, "", store, cfg.JWTSecret, rdb)

	return r
}

func runSingleTenant() {
	cfg := loadConfig()

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := db.NewStore(pool)

	// Connect to Redis (for OAuth state management)
	rdb := api.NewRedisClient()

	// Start the post scheduler (replaces Temporal).
	// Single-tenant mode has no tenant context for refresh; pass background ctx.
	sched := scheduler.New(store, context.Background())
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
	prefix := cfg.APIPrefix
	mountAPI(r, prefix, store, cfg.JWTSecret, rdb)

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
func mountAPI(r chi.Router, prefix string, store *db.Store, jwtSecret string, rdb *redis.Client) {
	mount := func(r chi.Router) {
		// Public API v1 (Rust client uses /public/v1/*)
		r.Route("/public/v1", func(r chi.Router) {
			r.Use(middleware.APIKeyAuth(store))
			api.RegisterPublicV1(r, store)
		})

		// Auth routes (no JWT required)
		api.RegisterAuth(r, store, jwtSecret)

		// Public routes (no auth) — provider catalog + OAuth callbacks
		api.RegisterPublic(r, store, rdb)

		// JWT-protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(jwtSecret, store))
			api.RegisterInternal(r, store, rdb)
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
