package handlers

import (
	"database/sql"
	"net/http"

	"pi-ntop/internal/config"
	"pi-ntop/internal/monitor"
	templates "pi-ntop/internal/ui/home"

	"github.com/a-h/templ"
)

func NewDashboardHandler(cfg config.Config, database *sql.DB, monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = cfg
		_ = database
		_ = monitorService

		templ.Handler(templates.Home()).ServeHTTP(w, r)
	})
}
