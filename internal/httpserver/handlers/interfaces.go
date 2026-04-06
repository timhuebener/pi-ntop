package handlers

import (
	"encoding/json"
	"net/http"

	"pi-ntop/internal/monitor"
)

func NewInterfaceSnapshotHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := monitorService.Dashboard(r.Context())
		if err != nil {
			http.Error(w, "load interface snapshot", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(snapshot); err != nil {
			http.Error(w, "encode interface snapshot", http.StatusInternalServerError)
		}
	})
}
