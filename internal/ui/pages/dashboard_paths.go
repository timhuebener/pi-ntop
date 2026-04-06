package pages

import (
	"fmt"
	"io"
	"strings"

	"pi-ntop/internal/monitor"

	"github.com/a-h/templ"
)

func renderPathGrid(w io.Writer, targets []monitor.TargetPathSnapshot) error {
	if len(targets) == 0 {
		_, err := io.WriteString(w, "<article class=\"empty-state\"><p class=\"section-label\">No trace targets yet</p><h3>Path discovery is waiting for configured targets</h3><p>Set <span class=\"mono\">PI_NTOP_TRACE_TARGETS</span> to one or more hosts or IPs and restart the app to begin traceroute snapshots.</p></article>")
		return err
	}

	for _, target := range targets {
		if err := renderPathCard(w, target); err != nil {
			return err
		}
	}

	return nil
}

func renderPathCard(w io.Writer, target monitor.TargetPathSnapshot) error {
	if _, err := fmt.Fprintf(w, "<article class=\"monitor-card\"><header class=\"monitor-header\"><div><p class=\"interface-label\">Trace target</p><h3 class=\"interface-name\">%s</h3><p class=\"metric-total\">%s</p></div><div class=\"interface-badges\">%s</div></header>", templ.EscapeString(target.Name), templ.EscapeString(target.Host), targetBadges(target)); err != nil {
		return err
	}

	if target.LatestRun == nil {
		_, err := io.WriteString(w, "<div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">Status</p><p class=\"metric-value\">Pending</p><p class=\"metric-total\">Waiting for the first traceroute run to complete.</p></div><div class=\"metric-block\"><p class=\"metric-caption\">Probe cadence</p><p class=\"metric-value\">"+templ.EscapeString(formatInterval(target.ProbeIntervalSeconds))+"</p><p class=\"metric-total\">Configured discovery interval for this target.</p></div></div></article>")
		return err
	}

	latestLatency := latestHopLatency(target.LatestRun.Hops)
	latestLatencyText := "No reply"
	if latestLatency > 0 {
		latestLatencyText = formatMilliseconds(latestLatency)
	}

	if _, err := fmt.Fprintf(w, "<div class=\"metric-pair\"><div class=\"metric-block\"><p class=\"metric-caption\">End-to-end RTT</p><p class=\"metric-value\">%s</p><p class=\"metric-total\">Last reachable hop from the most recent path snapshot.</p></div><div class=\"metric-block\"><p class=\"metric-caption\">Hop health</p><p class=\"metric-value\">%d / %d</p><p class=\"metric-total\">Degraded hops in the current path.</p></div></div>", templ.EscapeString(latestLatencyText), target.LatestRun.DegradedHopCount, target.LatestRun.HopCount); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "<div class=\"chart-grid\">"); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "<section class=\"chart-panel\"><div class=\"chart-header\"><span>Latest trace</span><strong>%s</strong></div><p class=\"stat-detail\" style=\"margin-top:0.75rem;\">Completed %s with %d discovered hops.</p></section>", templ.EscapeString(strings.Title(target.LatestRun.Status)), templ.EscapeString(formatClock(target.LatestRun.CompletedAt)), target.LatestRun.HopCount); err != nil {
		return err
	}

	historyText := fmt.Sprintf("%d recent runs persisted for this target.", len(target.RecentRuns))
	if _, err := fmt.Fprintf(w, "<section class=\"chart-panel\"><div class=\"chart-header\"><span>Route history</span><strong>%d runs</strong></div><p class=\"stat-detail\" style=\"margin-top:0.75rem;\">%s</p></section></div>", len(target.RecentRuns), templ.EscapeString(historyText)); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "<section style=\"margin-top:1.5rem;\"><p class=\"section-label\">Current path</p><ol style=\"display:grid; gap:0.75rem; margin-top:0.75rem;\">"); err != nil {
		return err
	}
	for _, hop := range target.LatestRun.Hops {
		if err := renderHopRow(w, hop); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</ol></section>"); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "<section style=\"margin-top:1.5rem;\"><p class=\"section-label\">Recent route history</p><ul style=\"display:grid; gap:0.6rem; margin-top:0.75rem;\">"); err != nil {
		return err
	}
	for _, run := range target.RecentRuns {
		if err := renderRecentRun(w, run); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</ul></section>"); err != nil {
		return err
	}

	if target.LatestRun.ErrorMessage != "" {
		if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Collector note</span><span>%s</span></footer>", templ.EscapeString(target.LatestRun.ErrorMessage)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "<footer class=\"monitor-footer\"><span>Probe cadence %s</span><span>Last updated %s</span></footer>", templ.EscapeString(formatInterval(target.ProbeIntervalSeconds)), templ.EscapeString(formatClock(target.LatestRun.CompletedAt))); err != nil {
		return err
	}

	_, err := io.WriteString(w, "</article>")
	return err
}

func renderHopRow(w io.Writer, hop monitor.TraceHopSnapshot) error {
	status := "Stable"
	if hop.IsTimeout {
		status = "Timeout"
	} else if hop.IsDegraded {
		status = "Degraded"
	}

	address := hop.Address
	if address == "" {
		address = hop.Hostname
	}
	if address == "" {
		address = "No reply"
	}

	if _, err := fmt.Fprintf(w, "<li style=\"display:flex; justify-content:space-between; gap:1rem; padding:0.85rem 0 0.85rem; border-top:1px solid rgba(120,131,152,0.18);\"><div><p class=\"metric-caption\">Hop %d</p><p class=\"metric-value\" style=\"font-size:1.05rem;\">%s</p><p class=\"metric-total\">%s</p></div><div style=\"text-align:right; min-width:12rem;\"><p class=\"metric-caption\">%s</p><p class=\"metric-total\">RTT %s · jitter %s · loss %s</p></div></li>", hop.HopIndex, templ.EscapeString(address), templ.EscapeString(hop.Hostname), templ.EscapeString(status), templ.EscapeString(formatMilliseconds(hop.AvgRTTMs)), templ.EscapeString(formatMilliseconds(hop.JitterMs)), templ.EscapeString(formatPercent(hop.LossPct))); err != nil {
		return err
	}
	return nil
}

func renderRecentRun(w io.Writer, run monitor.TraceRunSummary) error {
	changeText := "Stable route"
	if run.RouteChanged {
		changeText = "Route changed"
	}

	degradedText := "healthy"
	if run.DegradedHopCount > 0 {
		degradedText = fmt.Sprintf("%d degraded hop(s)", run.DegradedHopCount)
	}

	_, err := fmt.Fprintf(w, "<li style=\"display:flex; justify-content:space-between; gap:1rem; padding:0.75rem 0; border-top:1px solid rgba(120,131,152,0.18);\"><span>%s</span><span>%s · %d hops · %s</span></li>", templ.EscapeString(formatClock(run.StartedAt)), templ.EscapeString(changeText), run.HopCount, templ.EscapeString(degradedText))
	return err
}

func targetBadges(target monitor.TargetPathSnapshot) string {
	badges := []string{fmt.Sprintf("<span class=\"interface-badge\">%s</span>", templ.EscapeString(strings.Title(target.Status)))}
	if target.RouteChanged {
		badges = append(badges, "<span class=\"interface-badge\">Route change</span>")
	}
	if target.HasDegradedHop {
		badges = append(badges, "<span class=\"interface-badge interface-badge-live\">Degraded</span>")
	}
	return strings.Join(badges, "")
}

func latestHopLatency(hops []monitor.TraceHopSnapshot) float64 {
	for index := len(hops) - 1; index >= 0; index-- {
		if hops[index].IsTimeout {
			continue
		}
		return hops[index].AvgRTTMs
	}
	return 0
}

func formatMilliseconds(value float64) string {
	if value <= 0 {
		return "0.0 ms"
	}
	return fmt.Sprintf("%.1f ms", value)
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.0f%%", value)
}

const liveDashboardScript = `<script>
(() => {
  const endpoint = '/api/interfaces/live';
  const interfacesGrid = document.getElementById('interfaces-grid');
  const pathsGrid = document.getElementById('paths-grid');
  const speedGrid = document.getElementById('speed-grid');
  const alertsGrid = document.getElementById('alerts-grid');
  const totalRxValue = document.getElementById('total-rx-value');
  const totalTxValue = document.getElementById('total-tx-value');
  const activeInterfacesValue = document.getElementById('active-interfaces-value');
  const activeAlertsValue = document.getElementById('active-alerts-value');
  const cardActiveInterfaces = document.getElementById('card-active-interfaces');
  const cardActiveAlerts = document.getElementById('card-active-alerts');
  const monitoredTargetsValue = document.getElementById('monitored-targets-value');
  const monitoredTargetsPanel = document.getElementById('monitored-targets-panel');
  const speedTargetsValue = document.getElementById('speed-targets-value');
  const speedTargetsPanel = document.getElementById('speed-targets-panel');
  const speedFailuresValue = document.getElementById('speed-failures-value');
  const speedFailuresPanel = document.getElementById('speed-failures-panel');
  const avgSpeedDownloadValue = document.getElementById('avg-speed-download-value');
  const avgSpeedUploadValue = document.getElementById('avg-speed-upload-value');
  const degradedPathsPanel = document.getElementById('degraded-paths-panel');
  const lastSamplePanel = document.getElementById('last-sample-panel');
  const phaseSummary = document.getElementById('phase-summary');

  function formatBitrate(bitsPerSecond) {
    const units = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps'];
    let value = Number(bitsPerSecond) || 0;
    let unitIndex = 0;
    while (value >= 1000 && unitIndex < units.length - 1) {
      value /= 1000;
      unitIndex += 1;
    }
    if (unitIndex === 0) {
      return Math.round(value) + ' ' + units[unitIndex];
    }
    return value.toFixed(1) + ' ' + units[unitIndex];
  }

  function formatBytes(bytes) {
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let value = Number(bytes) || 0;
    let unitIndex = 0;
    while (value >= 1000 && unitIndex < units.length - 1) {
      value /= 1000;
      unitIndex += 1;
    }
    if (unitIndex === 0) {
      return Math.round(value) + ' ' + units[unitIndex];
    }
    return value.toFixed(1) + ' ' + units[unitIndex];
  }

  function formatClock(value) {
    if (!value) {
      return 'Waiting for samples';
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return 'Waiting for samples';
    }
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }

  function formatInterval(seconds) {
    if (!seconds || seconds <= 1) {
      return '1 second';
    }
    return String(seconds) + ' seconds';
  }

  function formatMilliseconds(value) {
    const numeric = Number(value) || 0;
    return numeric.toFixed(1) + ' ms';
  }

  function formatPercent(value) {
    return Math.round(Number(value) || 0) + '%';
  }

  function escapeHTML(value) {
    return String(value)
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#39;');
  }

  function sparklinePoints(history, selector) {
    const width = 240;
    const height = 72;
    const padX = 8;
    const padY = 8;
    if (!Array.isArray(history) || history.length === 0) {
      const baseline = height - padY;
      return padX + ',' + baseline + ' ' + (width - padX) + ',' + baseline;
    }

    const values = history.map((point) => {
      const selected = selector(point);
      return Number.isFinite(selected) ? selected : 0;
    });
    let max = Math.max(...values);
    if (max <= 0) {
      max = 1;
    }

    if (values.length === 1) {
      const y = scaledY(values[0], max, height, padY);
      return padX + ',' + y.toFixed(2) + ' ' + (width - padX) + ',' + y.toFixed(2);
    }

    const stepX = (width - (padX * 2)) / (values.length - 1);
    return values.map((value, index) => {
      const x = padX + (index * stepX);
      const y = scaledY(value, max, height, padY);
      return x.toFixed(2) + ',' + y.toFixed(2);
    }).join(' ');
  }

  function scaledY(value, max, height, padY) {
    const usableHeight = height - (padY * 2);
    if (usableHeight <= 0) {
      return height / 2;
    }
    return padY + (usableHeight * (1 - Math.min(value / max, 1)));
  }

  function latestCapturedAt(snapshot) {
    const interfaces = Array.isArray(snapshot.interfaces) ? snapshot.interfaces : [];
    const withInterfaces = interfaces.reduce((current, iface) => {
      if (!iface || !iface.capturedAt) {
        return current;
      }
      if (!current) {
        return iface.capturedAt;
      }
      return new Date(iface.capturedAt) > new Date(current) ? iface.capturedAt : current;
    }, snapshot.generatedAt || '');

    const pathTargets = Array.isArray(snapshot.pathTargets) ? snapshot.pathTargets : [];
    const withPaths = pathTargets.reduce((current, target) => {
      const completedAt = target && target.latestRun ? target.latestRun.completedAt : '';
      if (!completedAt) {
        return current;
      }
      if (!current) {
        return completedAt;
      }
      return new Date(completedAt) > new Date(current) ? completedAt : current;
    }, withInterfaces);

    const speedTargets = Array.isArray(snapshot.speedTargets) ? snapshot.speedTargets : [];
    return speedTargets.reduce((current, target) => {
      const completedAt = target && target.latestTest ? target.latestTest.completedAt : '';
      if (!completedAt) {
        return current;
      }
      if (!current) {
        return completedAt;
      }
      return new Date(completedAt) > new Date(current) ? completedAt : current;
    }, withPaths);
  }

  function renderInterfaceEmptyState() {
    return '<article class="empty-state"><p class="section-label">Waiting for samples</p><h3>No active interface data yet</h3><p>The collector writes an initial baseline immediately and computes rates on the next sample. Leave the app running for a couple of seconds and refresh if this state persists.</p></article>';
  }

  function renderPathEmptyState() {
    return '<article class="empty-state"><p class="section-label">No trace targets yet</p><h3>Path discovery is waiting for configured targets</h3><p>Set <span class="mono">PI_NTOP_TRACE_TARGETS</span> to one or more hosts or IPs and restart the app to begin traceroute snapshots.</p></article>';
  }

  function renderSpeedEmptyState() {
    return '<article class="empty-state"><p class="section-label">No speed targets yet</p><h3>Speed testing is waiting for configured endpoints</h3><p>Set <span class="mono">PI_NTOP_SPEED_TEST_TARGETS</span> to one or more HTTP endpoints and restart the app to begin download and upload measurements.</p></article>';
  }

  function alertMetricTitle(metricKey) {
    switch (metricKey) {
      case 'download_bps':
        return 'Low throughput';
      case 'latency_ms':
        return 'High latency';
      case 'loss_pct':
        return 'Packet loss';
      case 'route_changed':
        return 'Route change';
      case 'availability':
        return 'Collector failure';
      default:
        return 'Alert';
    }
  }

  function alertValueText(metricKey, value) {
    switch (metricKey) {
      case 'download_bps':
        return formatBitrate(value);
      case 'latency_ms':
        return formatMilliseconds(value);
      case 'loss_pct':
        return (Number(value) || 0).toFixed(1) + '%';
      case 'availability':
        return (Number(value) || 0) > 0 ? 'up' : 'down';
      default:
        return String(value ?? '');
    }
  }

  function formatRetentionWindow(seconds) {
    const value = Number(seconds) || 0;
    if (value <= 0) {
      return 'Disabled';
    }
    if (value % 86400 === 0) {
      return String(value / 86400) + ' days';
    }
    if (value % 3600 === 0) {
      return String(value / 3600) + ' hours';
    }
    if (value % 60 === 0) {
      return String(value / 60) + ' minutes';
    }
    return String(value) + ' seconds';
  }

  function formatDateTime(value) {
    if (!value) {
      return 'Pending';
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return 'Pending';
    }
    return date.toLocaleString([], { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }

  function renderAlertsCard(alerts, total) {
    const activeAlerts = Array.isArray(alerts) ? alerts : [];
    if (!total) {
      return '<article class="monitor-card"><header class="monitor-header"><div><p class="section-label">Alert queue</p><h3 class="interface-name">Active alerts</h3><p class="metric-total" id="alerts-summary">0 active issue(s)</p></div><div class="interface-badges"><span class="interface-badge" id="active-alerts-panel">0</span></div></header><p class="stat-detail">Threshold checks are healthy. Alerts here resolve automatically when the latest measurements return inside policy.</p></article>';
    }

    const rows = activeAlerts.map((alert) => {
      const severityClass = alert.severity === 'critical' ? ' interface-badge-live' : '';
      const severityText = String(alert.severity || 'warning');
      const sourceName = String(alert.sourceName || 'target').replaceAll('_', ' ');
      return [
        '<li style="display:grid; gap:0.45rem; padding:0.85rem 0; border-top:1px solid rgba(120,131,152,0.18);">',
        '<div style="display:flex; justify-content:space-between; gap:1rem; align-items:center;"><div><p class="metric-caption">', escapeHTML(sourceName), '</p><p class="metric-value" style="font-size:1.05rem;">', escapeHTML(alertMetricTitle(alert.metricKey)), '</p></div><span class="interface-badge', severityClass, '">', escapeHTML(severityText.slice(0, 1).toUpperCase() + severityText.slice(1)), '</span></div>',
        '<p class="stat-detail">', escapeHTML(alert.message || ''), '</p>',
        '<p class="metric-total">Current ', escapeHTML(alertValueText(alert.metricKey, alert.currentValue)), ' · threshold ', escapeHTML(alertValueText(alert.metricKey, alert.thresholdValue)), ' · last seen ', escapeHTML(formatDateTime(alert.lastSeenAt)), '</p>',
        '</li>'
      ].join('');
    }).join('');

    const footer = total > activeAlerts.length ? '<footer class="monitor-footer"><span>Showing ' + String(activeAlerts.length) + ' of ' + String(total) + ' alerts</span><span>Refreshes automatically</span></footer>' : '';
    return '<article class="monitor-card"><header class="monitor-header"><div><p class="section-label">Alert queue</p><h3 class="interface-name">Active alerts</h3><p class="metric-total" id="alerts-summary">' + String(total) + ' active issue(s)</p></div><div class="interface-badges"><span class="interface-badge" id="active-alerts-panel">' + String(total) + '</span></div></header><ul id="alerts-list" style="display:grid; gap:0.75rem; margin-top:0.5rem;">' + rows + '</ul>' + footer + '</article>';
  }

  function retentionRunLabel(retention) {
    if (retention && retention.lastSuccessAt) {
      return formatDateTime(retention.lastSuccessAt);
    }
    if (retention && retention.lastRunAt) {
      return formatDateTime(retention.lastRunAt);
    }
    return 'Pending';
  }

  function retentionStatusText(retention) {
    if (retention && retention.lastError) {
      return String(retention.lastError);
    }
    if (!retention || !retention.lastSuccessAt) {
      return 'Waiting for the first retention pass.';
    }
    return 'Last pass wrote ' + String(retention.upsertedInterfaceRollups || 0) + ' rollup row(s) and cleaned up expired records.';
  }

  function renderRetentionCard(retention) {
    const snapshot = retention || {};
    return [
      '<article class="monitor-card"><header class="monitor-header"><div><p class="section-label">Retention policies</p><h3 class="interface-name">Downsampling and cleanup</h3><p class="metric-total">One-minute interface rollups plus expiry windows for traces, speed tests, and resolved alerts.</p></div><div class="interface-badges"><span class="interface-badge">1m rollups</span></div></header>',
      '<div class="metric-pair"><div class="metric-block"><p class="metric-caption">Raw samples</p><p class="metric-value">', escapeHTML(formatRetentionWindow(snapshot.interfaceRawSeconds)), '</p><p class="metric-total">Per-second interface samples stay raw for this long.</p></div><div class="metric-block"><p class="metric-caption">Rollup retention</p><p class="metric-value">', escapeHTML(formatRetentionWindow(snapshot.interfaceRollupSeconds)), '</p><p class="metric-total">One-minute interface aggregates remain queryable for this window.</p></div></div>',
      '<div class="chart-grid"><section class="chart-panel"><div class="chart-header"><span>Retention job</span><strong id="retention-last-run">', escapeHTML(retentionRunLabel(snapshot)), '</strong></div><p class="stat-detail" id="retention-summary" style="margin-top:0.75rem;">', escapeHTML(retentionStatusText(snapshot)), '</p></section><section class="chart-panel"><div class="chart-header"><span>Cleanup counts</span><strong>', String(snapshot.deletedInterfaceSamples || 0), ' / ', String(snapshot.deletedTraceRuns || 0), ' / ', String(snapshot.deletedSpeedTests || 0), '</strong></div><p class="stat-detail" id="retention-counts" style="margin-top:0.75rem;">Deleted ', String(snapshot.deletedInterfaceSamples || 0), ' interface samples, ', String(snapshot.deletedTraceRuns || 0), ' trace runs, ', String(snapshot.deletedSpeedTests || 0), ' speed tests, and ', String(snapshot.deletedResolvedAlerts || 0), ' resolved alerts in the last successful pass.</p></section></div>',
      '<footer class="monitor-footer"><span>Trace retention ', escapeHTML(formatRetentionWindow(snapshot.traceRetentionSeconds)), ' · speed retention ', escapeHTML(formatRetentionWindow(snapshot.speedRetentionSeconds)), '</span><span>Resolved alerts ', escapeHTML(formatRetentionWindow(snapshot.resolvedAlertSeconds)), ' · cadence ', escapeHTML(formatRetentionWindow(snapshot.jobIntervalSeconds)), '</span></footer></article>'
    ].join('');
  }

  function renderAlertsGrid(alerts, total, retention) {
    return renderAlertsCard(alerts, total) + renderRetentionCard(retention);
  }

  function renderInterfaceCard(iface) {
    const history = Array.isArray(iface.history) ? iface.history : [];
    const badges = [];
    if (iface.isLoopback) {
      badges.push('<span class="interface-badge">Loopback</span>');
    }
    if (iface.isActive) {
      badges.push('<span class="interface-badge interface-badge-live">Live</span>');
    }

    return [
      '<article class="monitor-card">',
      '<header class="monitor-header"><div><p class="interface-label">Interface</p><h3 class="interface-name">', escapeHTML(iface.displayName || iface.name || 'unknown'), '</h3></div><div class="interface-badges">', badges.join(''), '</div></header>',
      '<div class="metric-pair">',
      '<div class="metric-block"><p class="metric-caption">Download</p><p class="metric-value metric-download">', escapeHTML(formatBitrate(iface.rxBps)), '</p><p class="metric-total">Total received ', escapeHTML(formatBytes(iface.rxBytesTotal)), '</p></div>',
      '<div class="metric-block"><p class="metric-caption">Upload</p><p class="metric-value metric-upload">', escapeHTML(formatBitrate(iface.txBps)), '</p><p class="metric-total">Total transmitted ', escapeHTML(formatBytes(iface.txBytesTotal)), '</p></div>',
      '</div>',
      '<div class="chart-grid">',
      '<section class="chart-panel"><div class="chart-header"><span>Download history</span><strong>', escapeHTML(formatBitrate(iface.rxBps)), '</strong></div><svg class="sparkline sparkline-rx" viewBox="0 0 240 72" preserveAspectRatio="none" aria-hidden="true"><polyline points="', escapeHTML(sparklinePoints(history, (point) => Number(point.rxBps) || 0)), '"></polyline></svg></section>',
      '<section class="chart-panel"><div class="chart-header"><span>Upload history</span><strong>', escapeHTML(formatBitrate(iface.txBps)), '</strong></div><svg class="sparkline sparkline-tx" viewBox="0 0 240 72" preserveAspectRatio="none" aria-hidden="true"><polyline points="', escapeHTML(sparklinePoints(history, (point) => Number(point.txBps) || 0)), '"></polyline></svg></section>',
      '</div>',
      '<footer class="monitor-footer"><span>Last persisted sample ', escapeHTML(formatClock(iface.capturedAt)), '</span><span>Recent window ', String(history.length), ' points</span></footer>',
      '</article>'
    ].join('');
  }

  function renderInterfaceGrid(interfaces) {
    if (!Array.isArray(interfaces) || interfaces.length === 0) {
      return renderInterfaceEmptyState();
    }

    return interfaces
      .slice()
      .sort((left, right) => {
        const leftTotal = (Number(left.rxBps) || 0) + (Number(left.txBps) || 0);
        const rightTotal = (Number(right.rxBps) || 0) + (Number(right.txBps) || 0);
        if (leftTotal === rightTotal) {
          return String(left.name || '').localeCompare(String(right.name || ''));
        }
        return rightTotal - leftTotal;
      })
      .map(renderInterfaceCard)
      .join('');
  }

  function latestHopLatency(hops) {
    if (!Array.isArray(hops)) {
      return 0;
    }
    for (let index = hops.length - 1; index >= 0; index -= 1) {
      const hop = hops[index];
      if (!hop || hop.isTimeout) {
        continue;
      }
      return Number(hop.avgRttMs) || 0;
    }
    return 0;
  }

  function renderHop(hop) {
    const address = hop.address || hop.hostname || 'No reply';
    const status = hop.isTimeout ? 'Timeout' : (hop.isDegraded ? 'Degraded' : 'Stable');
    return [
      '<li style="display:flex; justify-content:space-between; gap:1rem; padding:0.85rem 0 0.85rem; border-top:1px solid rgba(120,131,152,0.18);">',
      '<div><p class="metric-caption">Hop ', String(hop.hopIndex || 0), '</p><p class="metric-value" style="font-size:1.05rem;">', escapeHTML(address), '</p><p class="metric-total">', escapeHTML(hop.hostname || ''), '</p></div>',
      '<div style="text-align:right; min-width:12rem;"><p class="metric-caption">', escapeHTML(status), '</p><p class="metric-total">RTT ', escapeHTML(formatMilliseconds(hop.avgRttMs)), ' · jitter ', escapeHTML(formatMilliseconds(hop.jitterMs)), ' · loss ', escapeHTML(formatPercent(hop.lossPct)), '</p></div>',
      '</li>'
    ].join('');
  }

  function renderRecentRun(run) {
    const changeText = run.routeChanged ? 'Route changed' : 'Stable route';
    const degradedText = (Number(run.degradedHopCount) || 0) > 0 ? String(run.degradedHopCount) + ' degraded hop(s)' : 'healthy';
    return [
      '<li style="display:flex; justify-content:space-between; gap:1rem; padding:0.75rem 0; border-top:1px solid rgba(120,131,152,0.18);">',
      '<span>', escapeHTML(formatClock(run.startedAt)), '</span>',
      '<span>', escapeHTML(changeText), ' · ', String(run.hopCount || 0), ' hops · ', escapeHTML(degradedText), '</span>',
      '</li>'
    ].join('');
  }

  function renderTarget(target) {
    const latestRun = target.latestRun;
    const badges = ['<span class="interface-badge">' + escapeHTML(String(target.status || 'pending')) + '</span>'];
    if (target.routeChanged) {
      badges.push('<span class="interface-badge">Route change</span>');
    }
    if (target.hasDegradedHop) {
      badges.push('<span class="interface-badge interface-badge-live">Degraded</span>');
    }

    if (!latestRun) {
      return [
        '<article class="monitor-card">',
        '<header class="monitor-header"><div><p class="interface-label">Trace target</p><h3 class="interface-name">', escapeHTML(target.name || target.host || 'unknown'), '</h3><p class="metric-total">', escapeHTML(target.host || ''), '</p></div><div class="interface-badges">', badges.join(''), '</div></header>',
        '<div class="metric-pair"><div class="metric-block"><p class="metric-caption">Status</p><p class="metric-value">Pending</p><p class="metric-total">Waiting for the first traceroute run to complete.</p></div><div class="metric-block"><p class="metric-caption">Probe cadence</p><p class="metric-value">', escapeHTML(formatInterval(target.probeIntervalSeconds)), '</p><p class="metric-total">Configured discovery interval for this target.</p></div></div>',
        '</article>'
      ].join('');
    }

    const hops = Array.isArray(latestRun.hops) ? latestRun.hops : [];
    const recentRuns = Array.isArray(target.recentRuns) ? target.recentRuns : [];
    const latestLatency = latestHopLatency(hops);
    const latestLatencyText = latestLatency > 0 ? formatMilliseconds(latestLatency) : 'No reply';

    return [
      '<article class="monitor-card">',
      '<header class="monitor-header"><div><p class="interface-label">Trace target</p><h3 class="interface-name">', escapeHTML(target.name || target.host || 'unknown'), '</h3><p class="metric-total">', escapeHTML(target.host || ''), '</p></div><div class="interface-badges">', badges.join(''), '</div></header>',
      '<div class="metric-pair"><div class="metric-block"><p class="metric-caption">End-to-end RTT</p><p class="metric-value">', escapeHTML(latestLatencyText), '</p><p class="metric-total">Last reachable hop from the most recent path snapshot.</p></div><div class="metric-block"><p class="metric-caption">Hop health</p><p class="metric-value">', String(latestRun.degradedHopCount || 0), ' / ', String(latestRun.hopCount || 0), '</p><p class="metric-total">Degraded hops in the current path.</p></div></div>',
      '<div class="chart-grid">',
      '<section class="chart-panel"><div class="chart-header"><span>Latest trace</span><strong>', escapeHTML(String(latestRun.status || 'completed')), '</strong></div><p class="stat-detail" style="margin-top:0.75rem;">Completed ', escapeHTML(formatClock(latestRun.completedAt)), ' with ', String(latestRun.hopCount || 0), ' discovered hops.</p></section>',
      '<section class="chart-panel"><div class="chart-header"><span>Route history</span><strong>', String(recentRuns.length), ' runs</strong></div><p class="stat-detail" style="margin-top:0.75rem;">', String(recentRuns.length), ' recent runs persisted for this target.</p></section>',
      '</div>',
      '<section style="margin-top:1.5rem;"><p class="section-label">Current path</p><ol style="display:grid; gap:0.75rem; margin-top:0.75rem;">', hops.map(renderHop).join(''), '</ol></section>',
      '<section style="margin-top:1.5rem;"><p class="section-label">Recent route history</p><ul style="display:grid; gap:0.6rem; margin-top:0.75rem;">', recentRuns.map(renderRecentRun).join(''), '</ul></section>',
      latestRun.errorMessage ? '<footer class="monitor-footer"><span>Collector note</span><span>' + escapeHTML(latestRun.errorMessage) + '</span></footer>' : '<footer class="monitor-footer"><span>Probe cadence ' + escapeHTML(formatInterval(target.probeIntervalSeconds)) + '</span><span>Last updated ' + escapeHTML(formatClock(latestRun.completedAt)) + '</span></footer>',
      '</article>'
    ].join('');
  }

  function renderPathGrid(targets) {
    if (!Array.isArray(targets) || targets.length === 0) {
      return renderPathEmptyState();
    }

    return targets.map(renderTarget).join('');
  }

  function renderSpeedCard(target) {
    const latestTest = target.latestTest;
    const history = Array.isArray(target.history) ? target.history : [];
    const badges = ['<span class="interface-badge">' + escapeHTML(String(target.status || 'pending')) + '</span>'];
    if (target.hasUpload) {
      badges.push('<span class="interface-badge">Upload</span>');
    }
    if (target.isHealthy) {
      badges.push('<span class="interface-badge interface-badge-live">Healthy</span>');
    }

    if (!latestTest) {
      return [
        '<article class="monitor-card">',
        '<header class="monitor-header"><div><p class="interface-label">Speed target</p><h3 class="interface-name">', escapeHTML(target.name || target.downloadUrl || 'unknown'), '</h3><p class="metric-total">', escapeHTML(target.downloadUrl || ''), '</p></div><div class="interface-badges">', badges.join(''), '</div></header>',
        '<div class="metric-pair"><div class="metric-block"><p class="metric-caption">Status</p><p class="metric-value">Pending</p><p class="metric-total">Waiting for the first HTTP speed test to complete.</p></div><div class="metric-block"><p class="metric-caption">Probe cadence</p><p class="metric-value">', escapeHTML(formatInterval(target.intervalSeconds)), '</p><p class="metric-total">Configured speed-test interval for this target.</p></div></div>',
        '</article>'
      ].join('');
    }

    const secondaryLabel = target.hasUpload ? 'Upload' : 'Latency';
    const secondaryValue = target.hasUpload ? formatBitrate(latestTest.uploadBps) : formatMilliseconds(latestTest.latencyMs);
    const secondaryDetail = target.hasUpload ? 'Latest persisted upload throughput.' : 'HTTP time-to-first-byte from the most recent download measurement.';
    const secondaryChartLabel = target.hasUpload ? 'Upload history' : 'Latency history';
    const secondaryChartValue = target.hasUpload ? formatBitrate(latestTest.uploadBps) : formatMilliseconds(latestTest.latencyMs);
    const secondaryChartPoints = sparklinePoints(history, (point) => target.hasUpload ? (Number(point.uploadBps) || 0) : (Number(point.latencyMs) || 0));
    const secondaryChartClass = target.hasUpload ? 'sparkline-tx' : 'sparkline-rx';

    return [
      '<article class="monitor-card">',
      '<header class="monitor-header"><div><p class="interface-label">Speed target</p><h3 class="interface-name">', escapeHTML(target.name || target.downloadUrl || 'unknown'), '</h3><p class="metric-total">', escapeHTML(target.downloadUrl || ''), '</p></div><div class="interface-badges">', badges.join(''), '</div></header>',
      '<div class="metric-pair"><div class="metric-block"><p class="metric-caption">Download</p><p class="metric-value metric-download">', escapeHTML(formatBitrate(latestTest.downloadBps)), '</p><p class="metric-total">Transferred ', escapeHTML(formatBytes(latestTest.downloadBytes)), ' in the latest test.</p></div><div class="metric-block"><p class="metric-caption">', escapeHTML(secondaryLabel), '</p><p class="metric-value metric-upload">', escapeHTML(secondaryValue), '</p><p class="metric-total">', escapeHTML(secondaryDetail), '</p></div></div>',
      '<div class="chart-grid">',
      '<section class="chart-panel"><div class="chart-header"><span>Download history</span><strong>', escapeHTML(formatBitrate(latestTest.downloadBps)), '</strong></div><svg class="sparkline sparkline-rx" viewBox="0 0 240 72" preserveAspectRatio="none" aria-hidden="true"><polyline points="', escapeHTML(sparklinePoints(history, (point) => Number(point.downloadBps) || 0)), '"></polyline></svg></section>',
      '<section class="chart-panel"><div class="chart-header"><span>', escapeHTML(secondaryChartLabel), '</span><strong>', escapeHTML(secondaryChartValue), '</strong></div><svg class="sparkline ', escapeHTML(secondaryChartClass), '" viewBox="0 0 240 72" preserveAspectRatio="none" aria-hidden="true"><polyline points="', escapeHTML(secondaryChartPoints), '"></polyline></svg></section>',
      '</div>',
      latestTest.errorMessage ? '<footer class="monitor-footer"><span>Latest latency ' + escapeHTML(formatMilliseconds(latestTest.latencyMs)) + '</span><span>' + escapeHTML(latestTest.errorMessage) + '</span></footer>' : '<footer class="monitor-footer"><span>Latest latency ' + escapeHTML(formatMilliseconds(latestTest.latencyMs)) + '</span><span>Last updated ' + escapeHTML(formatClock(latestTest.completedAt)) + '</span></footer>',
      '</article>'
    ].join('');
  }

  function renderSpeedGrid(targets) {
    if (!Array.isArray(targets) || targets.length === 0) {
      return renderSpeedEmptyState();
    }

    return targets.map(renderSpeedCard).join('');
  }

  async function refresh() {
    try {
      const response = await fetch(endpoint, {
        headers: { 'Accept': 'application/json' },
        cache: 'no-store'
      });
      if (!response.ok) {
        throw new Error('request failed with status ' + response.status);
      }

      const snapshot = await response.json();
      totalRxValue.textContent = formatBitrate(snapshot.totalRxBps);
      totalTxValue.textContent = formatBitrate(snapshot.totalTxBps);
      activeInterfacesValue.textContent = String(snapshot.activeInterfaceCount || 0);
      activeAlertsValue.textContent = String(snapshot.activeAlertCount || 0);
      cardActiveInterfaces.textContent = String(snapshot.activeInterfaceCount || 0);
      cardActiveAlerts.textContent = String(snapshot.activeAlertCount || 0);
      monitoredTargetsValue.textContent = String(snapshot.monitoredTargetCount || 0);
      monitoredTargetsPanel.textContent = String(snapshot.monitoredTargetCount || 0);
      speedTargetsValue.textContent = String(snapshot.speedTargetCount || 0);
      speedTargetsPanel.textContent = String(snapshot.speedTargetCount || 0);
      speedFailuresValue.textContent = String(snapshot.failedSpeedTestCount || 0);
      speedFailuresPanel.textContent = String(snapshot.failedSpeedTestCount || 0);
      avgSpeedDownloadValue.textContent = formatBitrate(snapshot.averageDownloadBps);
      avgSpeedUploadValue.textContent = formatBitrate(snapshot.averageUploadBps);
      degradedPathsPanel.textContent = String(snapshot.degradedPathCount || 0);

      const latest = latestCapturedAt(snapshot);
      lastSamplePanel.textContent = formatClock(latest);
      alertsGrid.innerHTML = renderAlertsGrid(snapshot.alerts, snapshot.activeAlertCount || 0, snapshot.retention || {});
      interfacesGrid.innerHTML = renderInterfaceGrid(snapshot.interfaces);
      pathsGrid.innerHTML = renderPathGrid(snapshot.pathTargets);
      speedGrid.innerHTML = renderSpeedGrid(snapshot.speedTargets);

      if ((snapshot.activeInterfaceCount || 0) === 0 && (snapshot.monitoredTargetCount || 0) === 0 && (snapshot.speedTargetCount || 0) === 0) {
        phaseSummary.textContent = 'Collectors are active, but the dashboard is still waiting for the first persisted interface, path discovery, and speed test samples. Leave the app running for a few seconds and the initial telemetry plus alert state will appear.';
      } else {
        phaseSummary.textContent = 'Sampling ' + String(snapshot.activeInterfaceCount || 0) + ' active interfaces every ' + formatInterval(snapshot.sampleIntervalSeconds) + ', tracing ' + String(snapshot.monitoredTargetCount || 0) + ' target paths, measuring ' + String(snapshot.speedTargetCount || 0) + ' end-to-end speed targets, and tracking ' + String(snapshot.activeAlertCount || 0) + ' active alert(s) with automatic retention and one-minute rollups in SQLite.';
      }
    } catch (error) {
      console.error('refresh dashboard', error);
    }
  }

  window.setInterval(refresh, 3000);
})();
</script>`
