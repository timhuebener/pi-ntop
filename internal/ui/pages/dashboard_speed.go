package pages

import (
	"fmt"
	"io"
	"strings"

	"pi-ntop/internal/monitor"

	"github.com/a-h/templ"
)

func renderSpeedGrid(w io.Writer, targets []monitor.SpeedTargetSnapshot) error {
	if len(targets) == 0 {
		_, err := io.WriteString(w, "<article class=\"empty-state\"><p class=\"section-label\">No speed targets yet</p><h3>Speed testing is waiting for configured endpoints</h3><p>Set <span class=\"mono\">PI_NTOP_SPEED_TEST_TARGETS</span> to one or more HTTP endpoints and restart the app to begin download and upload measurements.</p></article>")
		return err
	}

	for _, target := range targets {
		if err := renderSpeedCard(w, target); err != nil {
			return err
		}
	}

	return nil
}

func renderSpeedCard(w io.Writer, target monitor.SpeedTargetSnapshot) error {
	if _, err := fmt.Fprintf(w, "<article class=\"monitor-card\"><header class=\"monitor-header\"><div><p class=\"interface-label\">Speed target</p><h3 class=\"interface-name\">%s</h3><p class=\"metric-total\">%s</p></div><div class=\"interface-badges\">%s</div></header>", templ.EscapeString(target.Name), templ.EscapeString(target.DownloadURL), speedTargetBadges(target)); err != nil {
		return err
	}

	if target.LatestTest == nil {
		_, err := io.WriteString(w, "<div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">Status</p><p class=\"metric-value\">Pending</p><p class=\"metric-total\">Waiting for the first HTTP speed test to complete.</p></div><div class=\"metric-block\"><p class=\"metric-caption\">Probe cadence</p><p class=\"metric-value\">"+templ.EscapeString(formatInterval(target.IntervalSeconds))+"</p><p class=\"metric-total\">Configured speed-test interval for this target.</p></div></div></article>")
		return err
	}

	secondaryCaption := "Upload"
	secondaryValue := formatBitrate(target.LatestTest.UploadBps)
	secondaryDetail := "Latest persisted upload throughput."
	secondaryChartLabel := "Upload history"
	secondaryChartValue := formatBitrate(target.LatestTest.UploadBps)
	secondaryChartClass := "sparkline-tx"
	secondaryChartPoints := speedSparklinePoints(target.History, func(point monitor.SpeedTestPoint) float64 { return point.UploadBps })
	if !target.HasUpload {
		secondaryCaption = "Latency"
		secondaryValue = formatMilliseconds(target.LatestTest.LatencyMs)
		secondaryDetail = "HTTP time-to-first-byte from the most recent download measurement."
		secondaryChartLabel = "Latency history"
		secondaryChartValue = formatMilliseconds(target.LatestTest.LatencyMs)
		secondaryChartClass = "sparkline-rx"
		secondaryChartPoints = speedSparklinePoints(target.History, func(point monitor.SpeedTestPoint) float64 { return point.LatencyMs })
	}

	if _, err := fmt.Fprintf(w, "<div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">Download</p><p class=\"metric-value metric-download\">%s</p><p class=\"metric-total\">Transferred %s in the latest test.</p></div><div class=\"metric-block\"><p class=\"metric-caption\">%s</p><p class=\"metric-value metric-upload\">%s</p><p class=\"metric-total\">%s</p></div></div>", templ.EscapeString(formatBitrate(target.LatestTest.DownloadBps)), templ.EscapeString(formatBytes(uint64(target.LatestTest.DownloadBytes))), templ.EscapeString(secondaryCaption), templ.EscapeString(secondaryValue), templ.EscapeString(secondaryDetail)); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "<div class=\"chart-grid\">"); err != nil {
		return err
	}
	if err := renderChartPanel(w, "Download history", formatBitrate(target.LatestTest.DownloadBps), speedSparklinePoints(target.History, func(point monitor.SpeedTestPoint) float64 { return point.DownloadBps }), "sparkline-rx"); err != nil {
		return err
	}
	if err := renderChartPanel(w, secondaryChartLabel, secondaryChartValue, secondaryChartPoints, secondaryChartClass); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</div>"); err != nil {
		return err
	}

	if target.LatestTest.ErrorMessage != "" {
		if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Latest latency %s</span><span>%s</span></footer>", templ.EscapeString(formatMilliseconds(target.LatestTest.LatencyMs)), templ.EscapeString(target.LatestTest.ErrorMessage)); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</article>")
		return err
	}

	if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Latest latency %s</span><span>Last updated %s</span></footer></article>", templ.EscapeString(formatMilliseconds(target.LatestTest.LatencyMs)), templ.EscapeString(formatClock(target.LatestTest.CompletedAt))); err != nil {
		return err
	}

	return nil
}

func speedSparklinePoints(points []monitor.SpeedTestPoint, metric func(monitor.SpeedTestPoint) float64) string {
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

func speedTargetBadges(target monitor.SpeedTargetSnapshot) string {
	badges := []string{fmt.Sprintf("<span class=\"interface-badge\">%s</span>", templ.EscapeString(speedStatusText(target.Status)))}
	if target.HasUpload {
		badges = append(badges, "<span class=\"interface-badge\">Upload</span>")
	}
	if target.IsHealthy {
		badges = append(badges, "<span class=\"interface-badge interface-badge-live\">Healthy</span>")
	}
	return strings.Join(badges, "")
}

func speedStatusText(status string) string {
	if status == "" {
		return "Pending"
	}
	return strings.ToUpper(status[:1]) + status[1:]
}
