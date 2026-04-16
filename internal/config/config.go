package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSpeedDownloadURL = "https://speed.cloudflare.com/__down?bytes=5000000"
	defaultSpeedUploadURL   = "https://speed.cloudflare.com/__up"
)

type SpeedTestTarget struct {
	Name          string
	InterfaceName string
	DownloadURL   string
	UploadURL     string
}

type AlertThresholds struct {
	EvaluateInterval time.Duration
	MinDownloadBps   float64
	MaxLatencyMs     float64
	MaxLossPct       float64
}

type RetentionPolicy struct {
	JobInterval     time.Duration
	InterfaceRaw    time.Duration
	InterfaceRollup time.Duration
	InterfaceDaily  time.Duration
	InterfaceWeekly time.Duration
	TraceRuns       time.Duration
	TraceDaily      time.Duration
	TraceWeekly     time.Duration
	SpeedTests      time.Duration
	SpeedDaily      time.Duration
	SpeedWeekly     time.Duration
	ResolvedAlerts  time.Duration
}

type Config struct {
	AppName         string
	Environment     string
	HTTPAddr        string
	DatabasePath    string
	TraceTargets    []string
	TraceInterval   time.Duration
	TraceTimeout    time.Duration
	TraceMaxHops    int
	SpeedTargets    []SpeedTestTarget
	SpeedInterval   time.Duration
	SpeedTimeout    time.Duration
	SpeedDownBytes  int64
	SpeedUpBytes    int64
	Alerts          AlertThresholds
	Retention       RetentionPolicy
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		AppName:        "pi-ntop",
		Environment:    getenv("GO_ENV", "development"),
		HTTPAddr:       getenv("PI_NTOP_HTTP_ADDR", ":8090"),
		DatabasePath:   getenv("PI_NTOP_DB_PATH", "var/pi-ntop.sqlite"),
		TraceTargets:   parseTargets(getenv("PI_NTOP_TRACE_TARGETS", "1.1.1.1")),
		TraceInterval:  45 * time.Second,
		TraceTimeout:   20 * time.Second,
		TraceMaxHops:   16,
		SpeedTargets:   parseSpeedTestTargets(getenv("PI_NTOP_SPEED_TEST_TARGETS", "Cloudflare|"+defaultSpeedDownloadURL+"|"+defaultSpeedUploadURL)),
		SpeedInterval:  3 * time.Minute,
		SpeedTimeout:   45 * time.Second,
		SpeedDownBytes: 5_000_000,
		SpeedUpBytes:   512_000,
		Alerts: AlertThresholds{
			EvaluateInterval: 15 * time.Second,
			MinDownloadBps:   5_000_000,
			MaxLatencyMs:     150,
			MaxLossPct:       5,
		},
		Retention: RetentionPolicy{
			JobInterval:     10 * time.Minute,
			InterfaceRaw:    30 * 24 * time.Hour,
			InterfaceRollup: 90 * 24 * time.Hour,
			InterfaceDaily:  365 * 24 * time.Hour,
			InterfaceWeekly: 2 * 365 * 24 * time.Hour,
			TraceRuns:       30 * 24 * time.Hour,
			TraceDaily:      365 * 24 * time.Hour,
			TraceWeekly:     2 * 365 * 24 * time.Hour,
			SpeedTests:      30 * 24 * time.Hour,
			SpeedDaily:      365 * 24 * time.Hour,
			SpeedWeekly:     2 * 365 * 24 * time.Hour,
			ResolvedAlerts:  7 * 24 * time.Hour,
		},
		ShutdownTimeout: 10 * time.Second,
	}

	if raw := os.Getenv("PI_NTOP_TRACE_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_TRACE_INTERVAL: %w", err)
		}
		cfg.TraceInterval = interval
	}

	if raw := os.Getenv("PI_NTOP_TRACE_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_TRACE_TIMEOUT: %w", err)
		}
		cfg.TraceTimeout = timeout
	}

	if raw := os.Getenv("PI_NTOP_TRACE_MAX_HOPS"); raw != "" {
		maxHops, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_TRACE_MAX_HOPS: %w", err)
		}
		cfg.TraceMaxHops = maxHops
	}

	if raw := os.Getenv("PI_NTOP_SPEED_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_SPEED_INTERVAL: %w", err)
		}
		cfg.SpeedInterval = interval
	}

	if raw := os.Getenv("PI_NTOP_SPEED_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_SPEED_TIMEOUT: %w", err)
		}
		cfg.SpeedTimeout = timeout
	}

	if raw := os.Getenv("PI_NTOP_SPEED_DOWNLOAD_BYTES"); raw != "" {
		size, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_SPEED_DOWNLOAD_BYTES: %w", err)
		}
		cfg.SpeedDownBytes = size
	}

	if raw := os.Getenv("PI_NTOP_SPEED_UPLOAD_BYTES"); raw != "" {
		size, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_SPEED_UPLOAD_BYTES: %w", err)
		}
		cfg.SpeedUpBytes = size
	}

	if raw := os.Getenv("PI_NTOP_ALERT_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_ALERT_INTERVAL: %w", err)
		}
		cfg.Alerts.EvaluateInterval = interval
	}

	if raw := os.Getenv("PI_NTOP_ALERT_MIN_DOWNLOAD_BPS"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_ALERT_MIN_DOWNLOAD_BPS: %w", err)
		}
		cfg.Alerts.MinDownloadBps = value
	}

	if raw := os.Getenv("PI_NTOP_ALERT_MAX_LATENCY_MS"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_ALERT_MAX_LATENCY_MS: %w", err)
		}
		cfg.Alerts.MaxLatencyMs = value
	}

	if raw := os.Getenv("PI_NTOP_ALERT_MAX_LOSS_PCT"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_ALERT_MAX_LOSS_PCT: %w", err)
		}
		cfg.Alerts.MaxLossPct = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_INTERVAL"); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_INTERVAL: %w", err)
		}
		cfg.Retention.JobInterval = interval
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_INTERFACE_RAW"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_INTERFACE_RAW: %w", err)
		}
		cfg.Retention.InterfaceRaw = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_INTERFACE_ROLLUP"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_INTERFACE_ROLLUP: %w", err)
		}
		cfg.Retention.InterfaceRollup = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_INTERFACE_DAILY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_INTERFACE_DAILY: %w", err)
		}
		cfg.Retention.InterfaceDaily = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_INTERFACE_WEEKLY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_INTERFACE_WEEKLY: %w", err)
		}
		cfg.Retention.InterfaceWeekly = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_TRACE_RUNS"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_TRACE_RUNS: %w", err)
		}
		cfg.Retention.TraceRuns = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_TRACE_DAILY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_TRACE_DAILY: %w", err)
		}
		cfg.Retention.TraceDaily = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_TRACE_WEEKLY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_TRACE_WEEKLY: %w", err)
		}
		cfg.Retention.TraceWeekly = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_SPEED_TESTS"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_SPEED_TESTS: %w", err)
		}
		cfg.Retention.SpeedTests = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_SPEED_DAILY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_SPEED_DAILY: %w", err)
		}
		cfg.Retention.SpeedDaily = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_SPEED_WEEKLY"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_SPEED_WEEKLY: %w", err)
		}
		cfg.Retention.SpeedWeekly = value
	}

	if raw := os.Getenv("PI_NTOP_RETENTION_RESOLVED_ALERTS"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_RETENTION_RESOLVED_ALERTS: %w", err)
		}
		cfg.Retention.ResolvedAlerts = value
	}

	if raw := os.Getenv("PI_NTOP_SHUTDOWN_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PI_NTOP_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = timeout
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseTargets(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})

	targets := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		target := strings.TrimSpace(part)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func parseSpeedTestTargets(raw string) []SpeedTestTarget {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})

	targets := make([]SpeedTestTarget, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		target, ok := parseSpeedTestTarget(strings.TrimSpace(part))
		if !ok {
			continue
		}
		// Dedup on download_url + interface_name combo.
		key := target.DownloadURL + "\x00" + target.InterfaceName
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func parseSpeedTestTarget(raw string) (SpeedTestTarget, bool) {
	if raw == "" {
		return SpeedTestTarget{}, false
	}

	segments := strings.Split(raw, "|")
	for index := range segments {
		segments[index] = strings.TrimSpace(segments[index])
	}

	// Supported formats (backward compatible):
	//   DownloadURL
	//   DownloadURL|UploadURL
	//   Name|DownloadURL
	//   Name|DownloadURL|UploadURL
	//   Name|@InterfaceName|DownloadURL
	//   Name|@InterfaceName|DownloadURL|UploadURL
	//
	// An interface name is indicated by a leading '@' character, e.g. @wlan0.

	switch len(segments) {
	case 1:
		if !looksLikeURL(segments[0]) {
			return SpeedTestTarget{}, false
		}
		return SpeedTestTarget{
			Name:        speedTargetName("", segments[0]),
			DownloadURL: segments[0],
		}, true
	case 2:
		if looksLikeURL(segments[0]) && looksLikeURL(segments[1]) {
			return SpeedTestTarget{
				Name:        speedTargetName("", segments[0]),
				DownloadURL: segments[0],
				UploadURL:   segments[1],
			}, true
		}
		if !looksLikeURL(segments[1]) {
			return SpeedTestTarget{}, false
		}
		return SpeedTestTarget{
			Name:        speedTargetName(segments[0], segments[1]),
			DownloadURL: segments[1],
		}, true
	default:
		// 3+ segments. Check if segments[1] is an @interface specifier.
		ifaceName := ""
		urlStart := 1 // index of first URL segment
		if looksLikeInterface(segments[1]) {
			ifaceName = segments[1][1:] // strip leading '@'
			urlStart = 2
		}

		if urlStart >= len(segments) || !looksLikeURL(segments[urlStart]) {
			return SpeedTestTarget{}, false
		}

		target := SpeedTestTarget{
			Name:          speedTargetName(segments[0], segments[urlStart]),
			InterfaceName: ifaceName,
			DownloadURL:   segments[urlStart],
		}
		if urlStart+1 < len(segments) && looksLikeURL(segments[urlStart+1]) {
			target.UploadURL = segments[urlStart+1]
		}
		return target, true
	}
}

func looksLikeInterface(s string) bool {
	return len(s) > 1 && s[0] == '@' && !looksLikeURL(s)
}

func looksLikeURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func speedTargetName(name, rawURL string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	return parsed.Host
}
