package httpserver

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	assetsfs "pi-ntop/assets"
	"pi-ntop/internal/config"
	"pi-ntop/internal/httpserver/handlers"
	"pi-ntop/internal/monitor"
)

func New(cfg config.Config, database *sql.DB, monitorService *monitor.Service) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /", handlers.NewDashboardHandler(cfg, database, monitorService))
	mux.Handle("GET /api/interfaces/live", handlers.NewInterfaceSnapshotHandler(monitorService))
	mux.Handle("GET /api/paths/live", handlers.NewInterfaceSnapshotHandler(monitorService))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusOK
		response := map[string]string{
			"status":      "ok",
			"environment": cfg.Environment,
		}

		if err := database.PingContext(r.Context()); err != nil {
			statusCode = http.StatusServiceUnavailable
			response["status"] = "degraded"
			response["database"] = err.Error()
		} else {
			response["database"] = "connected"
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("encode health response: %v", err)
		}
	})

	assetsSubtree, err := fs.Sub(assetsfs.FS, ".")
	if err == nil {
		mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsSubtree))))
	}

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       time.Minute,
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
