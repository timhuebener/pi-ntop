package handlers

import (
	"fmt"
	"net/http"

	"pi-ntop/internal/monitor"
	tracehops "pi-ntop/internal/ui/trace_hops"

	"github.com/a-h/templ"
)

func NewTraceHopsPageHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if _, ok := defaultPeriods[period]; !ok {
			period = defaultPeriod
		}

		since := resolveSince(period)

		targets, err := monitorService.TraceHopHistorySince(r.Context(), since)
		if err != nil {
			http.Error(w, "load trace hop history", http.StatusInternalServerError)
			return
		}

		pageData := tracehops.PageData{
			Period:  period,
			Targets: buildTraceHopData(targets),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templ.Handler(tracehops.TraceHopsPage(pageData)).ServeHTTP(w, r)
	})
}

func buildTraceHopData(targets []monitor.TargetPathSnapshot) []tracehops.TargetHopData {
	result := make([]tracehops.TargetHopData, 0, len(targets))
	for _, t := range targets {
		td := tracehops.TargetHopData{
			Name:           t.Name,
			Host:           t.Host,
			Status:         t.Status,
			HasDegradedHop: t.HasDegradedHop,
			RunCount:       len(t.RecentRuns),
		}

		if t.LatestRun != nil && len(t.LatestRun.Hops) > 0 {
			hops := t.LatestRun.Hops
			labels := make([]string, len(hops))
			avgRTT := make([]float64, len(hops))
			jitter := make([]float64, len(hops))
			lossPct := make([]float64, len(hops))
			hopData := make([]tracehops.HopData, len(hops))

			for i, h := range hops {
				label := fmt.Sprintf("Hop %d", h.HopIndex)
				if h.Hostname != "" && h.Hostname != h.Address {
					label = h.Hostname
				} else if h.Address != "" {
					label = h.Address
				}
				labels[i] = label
				avgRTT[i] = h.AvgRTTMs
				jitter[i] = h.JitterMs
				lossPct[i] = h.LossPct
				hopData[i] = tracehops.HopData{
					HopIndex:  h.HopIndex,
					Address:   h.Address,
					Hostname:  h.Hostname,
					AvgRTTMs:  h.AvgRTTMs,
					JitterMs:  h.JitterMs,
					LossPct:   h.LossPct,
					IsTimeout: h.IsTimeout,
				}
			}

			td.Labels = labels
			td.AvgRTTMs = avgRTT
			td.JitterMs = jitter
			td.LossPct = lossPct
			td.Hops = hopData
		}

		result = append(result, td)
	}
	return result
}
