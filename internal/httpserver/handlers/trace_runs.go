package handlers

import (
	"net/http"
	"time"

	"pi-ntop/internal/monitor"
	traceruns "pi-ntop/internal/ui/trace_runs"

	"github.com/a-h/templ"
)

func NewTraceRunsPageHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if _, ok := defaultPeriods[period]; !ok {
			period = defaultPeriod
		}

		dur := defaultPeriods[period]
		since := time.Now().UTC().Add(-dur)

		targets, err := monitorService.TraceRunHistorySince(r.Context(), since)
		if err != nil {
			http.Error(w, "load trace run history", http.StatusInternalServerError)
			return
		}

		pageData := traceruns.PageData{
			Period:  period,
			Targets: buildTraceRunCharts(targets),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templ.Handler(traceruns.TraceRunsPage(pageData)).ServeHTTP(w, r)
	})
}

func buildTraceRunCharts(targets []monitor.TargetPathSnapshot) []traceruns.TraceRunChartData {
	result := make([]traceruns.TraceRunChartData, 0, len(targets))
	for _, t := range targets {
		runs := t.RecentRuns
		labels := make([]string, len(runs))
		hopCounts := make([]float64, len(runs))
		degradedCounts := make([]float64, len(runs))

		failCount := 0
		routeChanges := 0
		for i, r := range runs {
			labels[i] = r.StartedAt.Format("01/02 15:04")
			hopCounts[i] = float64(r.HopCount)
			degradedCounts[i] = float64(r.DegradedHopCount)
			if r.Status != "completed" {
				failCount++
			}
			if r.RouteChanged {
				routeChanges++
			}
		}

		result = append(result, traceruns.TraceRunChartData{
			Name:           t.Name,
			Host:           t.Host,
			Status:         t.Status,
			RouteChanged:   t.RouteChanged,
			HasDegradedHop: t.HasDegradedHop,
			Labels:         labels,
			HopCounts:      hopCounts,
			DegradedCounts: degradedCounts,
			RunCount:       len(runs),
			FailCount:      failCount,
			RouteChanges:   routeChanges,
		})
	}
	return result
}
