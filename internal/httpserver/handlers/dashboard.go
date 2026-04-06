package handlers

import (
	"database/sql"
	"net/http"
	"path/filepath"

	"pi-ntop/internal/config"
	"pi-ntop/internal/monitor"
	"pi-ntop/internal/ui/pages"

	"github.com/a-h/templ"
)

func NewDashboardHandler(cfg config.Config, database *sql.DB, monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		databaseOnline := database.PingContext(r.Context()) == nil
		statusText := "Connected and migrated"
		if !databaseOnline {
			statusText = "Unavailable"
		}

		snapshot, err := monitorService.Dashboard(r.Context())
		if err != nil {
			http.Error(w, "load dashboard data", http.StatusInternalServerError)
			return
		}

		viewModel := pages.DashboardViewModel{
			AppName:        cfg.AppName,
			Environment:    cfg.Environment,
			HTTPAddr:       cfg.HTTPAddr,
			DatabasePath:   filepath.Clean(cfg.DatabasePath),
			DatabaseOnline: databaseOnline,
			DatabaseStatus: statusText,
			Snapshot:       snapshot,
		}

		templ.Handler(pages.Dashboard(viewModel)).ServeHTTP(w, r)
	})
}
