package pages

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"pi-ntop/internal/monitor"
	"pi-ntop/internal/ui/layout"

	"github.com/a-h/templ"
)

type DashboardViewModel struct {
	AppName        string
	Environment    string
	HTTPAddr       string
	DatabasePath   string
	DatabaseOnline bool
	DatabaseStatus string
	Snapshot       monitor.DashboardSnapshot
}

func Dashboard(vm DashboardViewModel) templ.Component {
	body := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		latestSample := latestActivityAt(vm.Snapshot)
		if _, err := fmt.Fprintf(w, "<main class=\"page-shell\"><section class=\"hero\"><div class=\"hero-copy\"><p class=\"eyebrow\">Phase 5 alerts and retention</p><h1>%s</h1><p class=\"hero-summary\" id=\"phase-summary\">%s</p><div class=\"hero-actions\"><a class=\"hero-link\" href=\"#alerts-retention\">Jump to alerts</a><a class=\"hero-link hero-link-secondary\" href=\"#interfaces-monitor\">Jump to live interfaces</a><a class=\"hero-link hero-link-secondary\" href=\"#path-discovery\">Jump to path discovery</a><a class=\"hero-link hero-link-secondary\" href=\"#speed-tests\">Jump to speed tests</a><a class=\"hero-link hero-link-secondary\" href=\"/healthz\">Check health JSON</a>", templ.EscapeString(vm.AppName), templ.EscapeString(vmSummaryText(vm))); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</div></div><div class=\"hero-panel\"><p class=\"panel-kicker\">Collector state</p><ul class=\"status-list\"><li><span>Environment</span><strong>"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(vm.Environment)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>HTTP listener</span><strong>"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(vm.HTTPAddr)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>SQLite file</span><strong>"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(vm.DatabasePath)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Collector cadence</span><strong>"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(formatInterval(vm.Snapshot.SampleIntervalSeconds))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Active interfaces</span><strong id=\"active-interfaces-value\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.ActiveInterfaceCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Latest sample</span><strong id=\"last-sample-panel\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(formatClock(latestSample))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Trace targets</span><strong id=\"monitored-targets-panel\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.MonitoredTargetCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Speed targets</span><strong id=\"speed-targets-panel\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.SpeedTargetCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Degraded paths</span><strong id=\"degraded-paths-panel\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.DegradedPathCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Failed speed tests</span><strong id=\"speed-failures-panel\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.FailedSpeedTestCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li><li><span>Active alerts</span><strong id=\"active-alerts-value\">"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(fmt.Sprintf("%d", vm.Snapshot.ActiveAlertCount))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</strong></li></ul></div></section><section class=\"stats-grid\">"); err != nil {
			return err
		}

		cards := []metricCard{
			{Label: "Current download", ValueID: "total-rx-value", Value: formatBitrate(vm.Snapshot.TotalRXBps), DetailID: "total-rx-detail", Detail: "Aggregated receive throughput across active interfaces."},
			{Label: "Current upload", ValueID: "total-tx-value", Value: formatBitrate(vm.Snapshot.TotalTXBps), DetailID: "total-tx-detail", Detail: "Aggregated transmit throughput across active interfaces."},
			{Label: "Monitored interfaces", ValueID: "card-active-interfaces", Value: fmt.Sprintf("%d", vm.Snapshot.ActiveInterfaceCount), DetailID: "card-active-detail", Detail: "Active interfaces with persisted per-second samples."},
			{Label: "Trace targets", ValueID: "monitored-targets-value", Value: fmt.Sprintf("%d", vm.Snapshot.MonitoredTargetCount), DetailID: "monitored-targets-detail", Detail: "Targets with scheduled traceroute discovery and persisted hop metrics."},
			{Label: "Speed test targets", ValueID: "speed-targets-value", Value: fmt.Sprintf("%d", vm.Snapshot.SpeedTargetCount), DetailID: "speed-targets-detail", Detail: "Endpoints with scheduled HTTP download and upload measurements."},
			{Label: "Failed speed tests", ValueID: "speed-failures-value", Value: fmt.Sprintf("%d", vm.Snapshot.FailedSpeedTestCount), DetailID: "speed-failures-detail", Detail: "Targets whose latest speed test run failed completely."},
			{Label: "Active alerts", ValueID: "card-active-alerts", Value: fmt.Sprintf("%d", vm.Snapshot.ActiveAlertCount), DetailID: "card-active-alerts-detail", Detail: "Threshold breaches that are still active against the latest persisted measurements."},
			{Label: "Avg test download", ValueID: "avg-speed-download-value", Value: formatBitrate(vm.Snapshot.AverageDownloadBps), DetailID: "avg-speed-download-detail", Detail: "Average of the latest persisted download measurements across speed-test targets."},
			{Label: "Avg test upload", ValueID: "avg-speed-upload-value", Value: formatBitrate(vm.Snapshot.AverageUploadBps), DetailID: "avg-speed-upload-detail", Detail: "Average of the latest persisted upload measurements across speed-test targets that support uploads."},
		}
		for _, card := range cards {
			if err := renderMetricCard(w, card); err != nil {
				return err
			}
		}

		if _, err := io.WriteString(w, "</section><section id=\"alerts-retention\" class=\"monitor-section\"><div class=\"monitor-section-header\"><div><p class=\"section-label\">Threshold alerts and automatic cleanup</p><h2>Alerts and retention</h2></div><p class=\"section-meta\">Current alert queue and the latest retention pass from <span class=\"mono\">/api/interfaces/live</span>.</p></div><div id=\"alerts-grid\" class=\"monitor-grid\">"); err != nil {
			return err
		}

		if err := renderAlertGrid(w, vm.Snapshot.Alerts, vm.Snapshot.ActiveAlertCount, vm.Snapshot.Retention); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</div></section><section id=\"interfaces-monitor\" class=\"monitor-section\"><div class=\"monitor-section-header\"><div><p class=\"section-label\">Live throughput by interface</p><h2>Local interface activity</h2></div><p class=\"section-meta\">Dashboard refreshes automatically every few seconds from <span class=\"mono\">/api/interfaces/live</span>.</p></div><div id=\"interfaces-grid\" class=\"monitor-grid\">"); err != nil {
			return err
		}

		if err := renderInterfaceGrid(w, vm.Snapshot.Interfaces); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</div></section>"); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "<section id=\"path-discovery\" class=\"monitor-section\"><div class=\"monitor-section-header\"><div><p class=\"section-label\">Traceroute snapshots and hop health</p><h2>Path discovery</h2></div><p class=\"section-meta\">Current route, route changes, and estimated per-hop loss and jitter from <span class=\"mono\">/api/paths/live</span>.</p></div><div id=\"paths-grid\" class=\"monitor-grid\">"); err != nil {
			return err
		}

		if err := renderPathGrid(w, vm.Snapshot.PathTargets); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</div></section>"); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "<section id=\"speed-tests\" class=\"monitor-section\"><div class=\"monitor-section-header\"><div><p class=\"section-label\">HTTP download and upload throughput</p><h2>End-to-end speed tests</h2></div><p class=\"section-meta\">Latest HTTP latency and throughput history for configured speed-test endpoints from <span class=\"mono\">/api/interfaces/live</span>.</p></div><div id=\"speed-grid\" class=\"monitor-grid\">"); err != nil {
			return err
		}

		if err := renderSpeedGrid(w, vm.Snapshot.SpeedTargets); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</div></section>"); err != nil {
			return err
		}

		if _, err := io.WriteString(w, liveDashboardScript); err != nil {
			return err
		}

		if _, err := io.WriteString(w, "</main>"); err != nil {
			return err
		}

		return nil
	})

	return layout.AppShell(vm.AppName+" local monitoring", body)
}

type metricCard struct {
	Label    string
	ValueID  string
	Value    string
	DetailID string
	Detail   string
}

func vmSummaryText(vm DashboardViewModel) string {
	if vm.Snapshot.ActiveInterfaceCount == 0 && vm.Snapshot.MonitoredTargetCount == 0 && vm.Snapshot.SpeedTargetCount == 0 {
		return "Collectors are active, but the dashboard is still waiting for the first persisted interface, path discovery, and speed test samples. Leave the app running for a few seconds and the initial telemetry plus alert state will appear."
	}
	return fmt.Sprintf("Sampling %d active interfaces every %s, tracing %d target paths, measuring %d end-to-end speed targets, and tracking %d active alert(s) with automatic retention and one-minute rollups in SQLite.", vm.Snapshot.ActiveInterfaceCount, formatInterval(vm.Snapshot.SampleIntervalSeconds), vm.Snapshot.MonitoredTargetCount, vm.Snapshot.SpeedTargetCount, vm.Snapshot.ActiveAlertCount)
}

func renderMetricCard(w io.Writer, card metricCard) error {
	if _, err := io.WriteString(w, "<article class=\"stat-card\"><div>"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<p class=\"stat-label\">%s</p><p class=\"stat-value\" id=\"%s\">%s</p></div><p class=\"stat-detail\" id=\"%s\">%s</p></article>", templ.EscapeString(card.Label), templ.EscapeString(card.ValueID), templ.EscapeString(card.Value), templ.EscapeString(card.DetailID), templ.EscapeString(card.Detail)); err != nil {
		return err
	}
	return nil
}

func renderInterfaceGrid(w io.Writer, interfaces []monitor.InterfaceSnapshot) error {
	if len(interfaces) == 0 {
		_, err := io.WriteString(w, "<article class=\"empty-state\"><p class=\"section-label\">Waiting for samples</p><h3>No active interface data yet</h3><p>The collector writes an initial baseline immediately and computes rates on the next sample. Leave the app running for a couple of seconds and reload if this state persists.</p></article>")
		return err
	}

	ordered := append([]monitor.InterfaceSnapshot(nil), interfaces...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].RXBps + ordered[i].TXBps
		right := ordered[j].RXBps + ordered[j].TXBps
		if left == right {
			return ordered[i].Name < ordered[j].Name
		}
		return left > right
	})

	for _, iface := range ordered {
		if err := renderInterfaceCard(w, iface); err != nil {
			return err
		}
	}

	return nil
}

func renderInterfaceCard(w io.Writer, iface monitor.InterfaceSnapshot) error {
	if _, err := fmt.Fprintf(w, "<article class=\"monitor-card\"><header class=\"monitor-header\"><div><p class=\"interface-label\">Interface</p><h3 class=\"interface-name\">%s</h3></div><div class=\"interface-badges\">", templ.EscapeString(iface.DisplayName)); err != nil {
		return err
	}
	if iface.IsLoopback {
		if _, err := io.WriteString(w, "<span class=\"interface-badge\">Loopback</span>"); err != nil {
			return err
		}
	}
	if iface.IsActive {
		if _, err := io.WriteString(w, "<span class=\"interface-badge interface-badge-live\">Live</span>"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</div></header><div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">Download</p><p class=\"metric-value metric-download\">"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, templ.EscapeString(formatBitrate(iface.RXBps))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</p><p class=\"metric-total\">Total received "); err != nil {
		return err
	}
	if _, err := io.WriteString(w, templ.EscapeString(formatBytes(iface.RXBytesTotal))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</p></div><div class=\"metric-block\"><p class=\"metric-caption\">Upload</p><p class=\"metric-value metric-upload\">"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, templ.EscapeString(formatBitrate(iface.TXBps))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</p><p class=\"metric-total\">Total transmitted "); err != nil {
		return err
	}
	if _, err := io.WriteString(w, templ.EscapeString(formatBytes(iface.TXBytesTotal))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</p></div></div><div class=\"chart-grid\">"); err != nil {
		return err
	}
	if err := renderChartPanel(w, "Download history", formatBitrate(iface.RXBps), sparklinePoints(iface.History, func(point monitor.ThroughputPoint) float64 { return point.RXBps }), "sparkline-rx"); err != nil {
		return err
	}
	if err := renderChartPanel(w, "Upload history", formatBitrate(iface.TXBps), sparklinePoints(iface.History, func(point monitor.ThroughputPoint) float64 { return point.TXBps }), "sparkline-tx"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "</div><footer class=\"monitor-footer\"><span>Last persisted sample %s</span><span>Recent window %d points</span></footer></article>", templ.EscapeString(formatClock(iface.CapturedAt)), len(iface.History)); err != nil {
		return err
	}
	return nil
}

func renderChartPanel(w io.Writer, label, value, points, lineClass string) error {
	if _, err := fmt.Fprintf(w, "<section class=\"chart-panel\"><div class=\"chart-header\"><span>%s</span><strong>%s</strong></div><svg class=\"sparkline %s\" viewBox=\"0 0 240 72\" preserveAspectRatio=\"none\" aria-hidden=\"true\"><polyline points=\"%s\"></polyline></svg></section>", templ.EscapeString(label), templ.EscapeString(value), templ.EscapeString(lineClass), templ.EscapeString(points)); err != nil {
		return err
	}
	return nil
}

func sparklinePoints(points []monitor.ThroughputPoint, metric func(monitor.ThroughputPoint) float64) string {
	const (
		width  = 240.0
		height = 72.0
		padX   = 8.0
		padY   = 8.0
	)

	if len(points) == 0 {
		baseline := height - padY
		return fmt.Sprintf("%.0f,%.0f %.0f,%.0f", padX, baseline, width-padX, baseline)
	}

	maxValue := 0.0
	for _, point := range points {
		value := metric(point)
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue <= 0 {
		maxValue = 1
	}

	if len(points) == 1 {
		value := metric(points[0])
		y := scaledY(value, maxValue, height, padY)
		return fmt.Sprintf("%.0f,%.2f %.0f,%.2f", padX, y, width-padX, y)
	}

	stepX := (width - (padX * 2)) / float64(len(points)-1)
	coords := make([]string, 0, len(points))
	for index, point := range points {
		x := padX + (float64(index) * stepX)
		y := scaledY(metric(point), maxValue, height, padY)
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x, y))
	}

	return strings.Join(coords, " ")
}

func scaledY(value, maxValue, height, padY float64) float64 {
	usableHeight := height - (padY * 2)
	if usableHeight <= 0 {
		return height / 2
	}
	return padY + (usableHeight * (1 - math.Min(value/maxValue, 1)))
}

func latestActivityAt(snapshot monitor.DashboardSnapshot) time.Time {
	latest := snapshot.GeneratedAt
	for _, iface := range snapshot.Interfaces {
		if iface.CapturedAt.After(latest) {
			latest = iface.CapturedAt
		}
	}
	for _, target := range snapshot.PathTargets {
		if target.LatestRun != nil && target.LatestRun.CompletedAt.After(latest) {
			latest = target.LatestRun.CompletedAt
		}
	}
	for _, target := range snapshot.SpeedTargets {
		if target.LatestTest != nil && target.LatestTest.CompletedAt.After(latest) {
			latest = target.LatestTest.CompletedAt
		}
	}
	return latest
}

func formatInterval(seconds int) string {
	if seconds <= 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", seconds)
}

func formatClock(value time.Time) string {
	if value.IsZero() {
		return "Waiting for samples"
	}
	return value.Local().Format("15:04:05")
}

func formatBitrate(bitsPerSecond float64) string {
	units := []string{"bps", "Kbps", "Mbps", "Gbps", "Tbps"}
	value := bitsPerSecond
	unitIndex := 0
	for value >= 1000 && unitIndex < len(units)-1 {
		value /= 1000
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%.0f %s", value, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}

func formatBytes(bytes uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	unitIndex := 0
	for value >= 1000 && unitIndex < len(units)-1 {
		value /= 1000
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}
