package handlers

import (
	"net/http"
	"time"

	"pi-ntop/internal/monitor"
	interfaces "pi-ntop/internal/ui/interfaces"

	"github.com/a-h/templ"
)

func NewInterfacesPageHandler(monitorService *monitor.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if _, ok := defaultPeriods[period]; !ok {
			period = defaultPeriod
		}

		dur := defaultPeriods[period]
		since := time.Now().UTC().Add(-dur)

		ifaces, err := monitorService.InterfaceHistorySince(r.Context(), since)
		if err != nil {
			http.Error(w, "load interface history", http.StatusInternalServerError)
			return
		}

		pageData := interfaces.PageData{
			Period:     period,
			Interfaces: buildInterfaceCharts(ifaces),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templ.Handler(interfaces.InterfacesPage(pageData)).ServeHTTP(w, r)
	})
}

func buildInterfaceCharts(ifaces []monitor.InterfaceSnapshot) []interfaces.InterfaceChartData {
	result := make([]interfaces.InterfaceChartData, 0, len(ifaces))
	for _, iface := range ifaces {
		history := iface.History
		labels := make([]string, len(history))
		rxBps := make([]float64, len(history))
		txBps := make([]float64, len(history))

		for i, pt := range history {
			labels[i] = pt.CapturedAt.Format("01/02 15:04")
			rxBps[i] = pt.RXBps
			txBps[i] = pt.TXBps
		}

		result = append(result, interfaces.InterfaceChartData{
			Name:        iface.Name,
			DisplayName: iface.DisplayName,
			IsActive:    iface.IsActive,
			IsLoopback:  iface.IsLoopback,
			Labels:      labels,
			RXBps:       rxBps,
			TXBps:       txBps,
			LatestRXBps: iface.RXBps,
			LatestTXBps: iface.TXBps,
			PeakBps:     iface.CombinedPeakBps,
			SampleCount: len(history),
		})
	}
	return result
}
