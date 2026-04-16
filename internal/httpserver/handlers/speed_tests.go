package handlers

import (
	"net/http"

	"pi-ntop/internal/monitor"
	speedtests "pi-ntop/internal/ui/speed_tests"

	"github.com/a-h/templ"
)

func NewSpeedTestsPageHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if _, ok := defaultPeriods[period]; !ok {
			period = defaultPeriod
		}

		since := resolveSince(period)

		targets, err := monitorService.SpeedTestHistory(r.Context(), since)
		if err != nil {
			http.Error(w, "load speed test history", http.StatusInternalServerError)
			return
		}

		pageData := speedtests.PageData{
			Period:  period,
			Targets: buildTargetCharts(targets, period),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templ.Handler(speedtests.SpeedTestsPage(pageData)).ServeHTTP(w, r)
	})
}

func buildTargetCharts(targets []monitor.SpeedTargetSnapshot, period string) []speedtests.TargetChartData {
	labelFmt := labelFormatForPeriod(period)
	result := make([]speedtests.TargetChartData, 0, len(targets))
	for _, t := range targets {
		history := t.History
		labels := make([]string, len(history))
		downloadMbps := make([]float64, len(history))
		uploadMbps := make([]float64, len(history))
		latencyMs := make([]float64, len(history))

		failCount := 0
		for i, pt := range history {
			labels[i] = pt.StartedAt.Format(labelFmt)
			downloadMbps[i] = bpsToMbps(pt.DownloadBps)
			uploadMbps[i] = bpsToMbps(pt.UploadBps)
			latencyMs[i] = pt.LatencyMs
			if pt.Status != "completed" {
				failCount++
			}
		}

		var latestDown, latestUp, latestLat float64
		if t.LatestTest != nil {
			latestDown = bpsToMbps(t.LatestTest.DownloadBps)
			latestUp = bpsToMbps(t.LatestTest.UploadBps)
			latestLat = t.LatestTest.LatencyMs
		}

		result = append(result, speedtests.TargetChartData{
			Name:           t.Name,
			IsHealthy:      t.IsHealthy,
			HasUpload:      t.HasUpload,
			Labels:         labels,
			DownloadMbps:   downloadMbps,
			UploadMbps:     uploadMbps,
			LatencyMs:      latencyMs,
			LatestDownMbps: latestDown,
			LatestUpMbps:   latestUp,
			LatestLatMs:    latestLat,
			TestCount:      len(history),
			FailCount:      failCount,
		})
	}
	return result
}

func bpsToMbps(bps float64) float64 {
	return bps / 1_000_000
}
