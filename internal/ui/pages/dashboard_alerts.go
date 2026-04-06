package pages

import (
	"fmt"
	"io"
	"strings"
	"time"

	"pi-ntop/internal/monitor"

	"github.com/a-h/templ"
)

func renderAlertGrid(w io.Writer, alerts []monitor.AlertSnapshot, total int, retention monitor.RetentionSnapshot) error {
	if err := renderAlertsCard(w, alerts, total); err != nil {
		return err
	}
	return renderRetentionCard(w, retention)
}

func renderAlertsCard(w io.Writer, alerts []monitor.AlertSnapshot, total int) error {
	if _, err := fmt.Fprintf(w, "<article class=\"monitor-card\"><header class=\"monitor-header\"><div><p class=\"section-label\">Alert queue</p><h3 class=\"interface-name\">Active alerts</h3><p class=\"metric-total\" id=\"alerts-summary\">%d active issue(s)</p></div><div class=\"interface-badges\"><span class=\"interface-badge\" id=\"active-alerts-panel\">%d</span></div></header>", total, total); err != nil {
		return err
	}

	if total == 0 {
		_, err := io.WriteString(w, "<p class=\"stat-detail\">Threshold checks are healthy. Alerts here resolve automatically when the latest measurements return inside policy.</p></article>")
		return err
	}

	if _, err := io.WriteString(w, "<ul id=\"alerts-list\" style=\"display:grid; gap:0.75rem; margin-top:0.5rem;\">"); err != nil {
		return err
	}
	for _, alert := range alerts {
		if err := renderAlertRow(w, alert); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</ul>"); err != nil {
		return err
	}

	if total > len(alerts) {
		if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Showing %d of %d alerts</span><span>Refreshes automatically</span></footer>", len(alerts), total); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "</article>")
	return err
}

func renderAlertRow(w io.Writer, alert monitor.AlertSnapshot) error {
	if _, err := fmt.Fprintf(w, "<li style=\"display:grid; gap:0.45rem; padding:0.85rem 0; border-top:1px solid rgba(120,131,152,0.18);\"><div style=\"display:flex; justify-content:space-between; gap:1rem; align-items:center;\"><div><p class=\"metric-caption\">%s</p><p class=\"metric-value\" style=\"font-size:1.05rem;\">%s</p></div><span class=\"interface-badge%s\">%s</span></div><p class=\"stat-detail\">%s</p><p class=\"metric-total\">Current %s · threshold %s · last seen %s</p></li>", templ.EscapeString(strings.ReplaceAll(alert.SourceName, "_", " ")), templ.EscapeString(alertMetricTitle(alert.MetricKey)), alertSeverityClass(alert.Severity), templ.EscapeString(titleCase(alert.Severity)), templ.EscapeString(alert.Message), templ.EscapeString(alertValueText(alert.MetricKey, alert.CurrentValue)), templ.EscapeString(alertValueText(alert.MetricKey, alert.ThresholdValue)), templ.EscapeString(formatDateTime(alert.LastSeenAt))); err != nil {
		return err
	}
	return nil
}

func renderRetentionCard(w io.Writer, retention monitor.RetentionSnapshot) error {
	if _, err := io.WriteString(w, "<article class=\"monitor-card\"><header class=\"monitor-header\"><div><p class=\"section-label\">Retention policies</p><h3 class=\"interface-name\">Downsampling and cleanup</h3><p class=\"metric-total\">One-minute interface rollups plus expiry windows for traces, speed tests, and resolved alerts.</p></div><div class=\"interface-badges\"><span class=\"interface-badge\">1m rollups</span></div></header>"); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "<div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">Raw samples</p><p class=\"metric-value\">%s</p><p class=\"metric-total\">Per-second interface samples stay raw for this long.</p></div><div class=\"metric-block\"><p class=\"metric-caption\">Rollup retention</p><p class=\"metric-value\">%s</p><p class=\"metric-total\">One-minute interface aggregates remain queryable for this window.</p></div></div>", templ.EscapeString(formatRetentionWindow(retention.InterfaceRawSeconds)), templ.EscapeString(formatRetentionWindow(retention.InterfaceRollupSeconds))); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "<div class=\"chart-grid\"><section class=\"chart-panel\"><div class=\"chart-header\"><span>Retention job</span><strong id=\"retention-last-run\">%s</strong></div><p class=\"stat-detail\" id=\"retention-summary\" style=\"margin-top:0.75rem;\">%s</p></section><section class=\"chart-panel\"><div class=\"chart-header\"><span>Cleanup counts</span><strong>%d / %d / %d</strong></div><p class=\"stat-detail\" id=\"retention-counts\" style=\"margin-top:0.75rem;\">Deleted %d interface samples, %d trace runs, %d speed tests, and %d resolved alerts in the last successful pass.</p></section></div>", templ.EscapeString(retentionRunLabel(retention)), templ.EscapeString(retentionStatusText(retention)), retention.DeletedInterfaceSamples, retention.DeletedTraceRuns, retention.DeletedSpeedTests, retention.DeletedInterfaceSamples, retention.DeletedTraceRuns, retention.DeletedSpeedTests, retention.DeletedResolvedAlerts); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Trace retention %s · speed retention %s</span><span>Resolved alerts %s · cadence %s</span></footer></article>", templ.EscapeString(formatRetentionWindow(retention.TraceRetentionSeconds)), templ.EscapeString(formatRetentionWindow(retention.SpeedRetentionSeconds)), templ.EscapeString(formatRetentionWindow(retention.ResolvedAlertSeconds)), templ.EscapeString(formatRetentionWindow(retention.JobIntervalSeconds))); err != nil {
		return err
	}

	return nil
}

func alertMetricTitle(metricKey string) string {
	switch metricKey {
	case "download_bps":
		return "Low throughput"
	case "latency_ms":
		return "High latency"
	case "loss_pct":
		return "Packet loss"
	case "route_changed":
		return "Route change"
	case "availability":
		return "Collector failure"
	default:
		return "Alert"
	}
}

func alertValueText(metricKey string, value float64) string {
	switch metricKey {
	case "download_bps":
		return formatBitrate(value)
	case "latency_ms":
		return formatMilliseconds(value)
	case "loss_pct":
		return fmt.Sprintf("%.1f%%", value)
	case "availability":
		if value > 0 {
			return "up"
		}
		return "down"
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func alertSeverityClass(severity string) string {
	if severity == "critical" {
		return " interface-badge-live"
	}
	return ""
}

func titleCase(value string) string {
	if value == "" {
		return "Alert"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func retentionRunLabel(retention monitor.RetentionSnapshot) string {
	if !retention.LastSuccessAt.IsZero() {
		return formatDateTime(retention.LastSuccessAt)
	}
	if !retention.LastRunAt.IsZero() {
		return formatDateTime(retention.LastRunAt)
	}
	return "Pending"
}

func retentionStatusText(retention monitor.RetentionSnapshot) string {
	if retention.LastError != "" {
		return retention.LastError
	}
	if retention.LastSuccessAt.IsZero() {
		return "Waiting for the first retention pass."
	}
	return fmt.Sprintf("Last pass wrote %d rollup row(s) and cleaned up expired records.", retention.UpsertedInterfaceRollups)
}

func formatRetentionWindow(seconds int) string {
	if seconds <= 0 {
		return "Disabled"
	}
	duration := time.Duration(seconds) * time.Second
	if duration%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d days", int(duration/(24*time.Hour)))
	}
	if duration%time.Hour == 0 {
		return fmt.Sprintf("%d hours", int(duration/time.Hour))
	}
	if duration%time.Minute == 0 {
		return fmt.Sprintf("%d minutes", int(duration/time.Minute))
	}
	return duration.String()
}

func formatDateTime(value time.Time) string {
	if value.IsZero() {
		return "Pending"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
