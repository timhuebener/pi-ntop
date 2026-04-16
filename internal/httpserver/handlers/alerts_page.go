package handlers

import (
	"net/http"
	"time"

	"pi-ntop/internal/monitor"
	alertsui "pi-ntop/internal/ui/alerts"

	"github.com/a-h/templ"
)

func NewAlertsPageHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if _, ok := defaultPeriods[period]; !ok {
			period = defaultPeriod
		}

		dur := defaultPeriods[period]
		since := time.Now().UTC().Add(-dur)

		alerts, total, err := monitorService.AlertHistorySince(r.Context(), since)
		if err != nil {
			http.Error(w, "load alert history", http.StatusInternalServerError)
			return
		}

		pageData := buildAlertsPageData(period, alerts, total)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templ.Handler(alertsui.AlertsPage(pageData)).ServeHTTP(w, r)
	})
}

func buildAlertsPageData(period string, alerts []monitor.AlertSnapshot, total int) alertsui.PageData {
	data := alertsui.PageData{
		Period:     period,
		TotalCount: total,
		Alerts:     make([]alertsui.AlertData, 0, len(alerts)),
	}

	for _, a := range alerts {
		if a.Status == "active" {
			data.ActiveCount++
			if a.Severity == "critical" {
				data.CriticalCount++
			} else if a.Severity == "warning" {
				data.WarningCount++
			}
		}

		data.Alerts = append(data.Alerts, alertsui.AlertData{
			ID:             a.ID,
			Severity:       a.Severity,
			SourceType:     a.SourceType,
			SourceName:     a.SourceName,
			MetricKey:      a.MetricKey,
			Status:         a.Status,
			Message:        a.Message,
			ThresholdValue: a.ThresholdValue,
			CurrentValue:   a.CurrentValue,
			CreatedAt:      a.CreatedAt,
			UpdatedAt:      a.UpdatedAt,
			LastSeenAt:     a.LastSeenAt,
			ResolvedAt:     a.ResolvedAt,
		})
	}

	return data
}
