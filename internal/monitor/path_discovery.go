package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTraceIntervalSeconds = 45
	degradedRTTThresholdMs      = 100.0
	degradedLossThresholdPct    = 1.0
)

var (
	tracerouteHopPattern      = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)
	tracerouteRTTPattern      = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*ms`)
	tracerouteEndpointPattern = regexp.MustCompile(`^(.*?)\s*\(([^)]+)\)$`)
)

type PathCollector struct {
	db                   *sql.DB
	configuredTargets    []string
	defaultProbeInterval time.Duration
	commandTimeout       time.Duration
	maxHops              int
	lastRun              map[int64]time.Time
	commandPath          string
}

type TargetPathSnapshot struct {
	ID                   int64             `json:"id"`
	Name                 string            `json:"name"`
	Host                 string            `json:"host"`
	ProbeIntervalSeconds int               `json:"probeIntervalSeconds"`
	LatestRun            *TraceRunSnapshot `json:"latestRun"`
	RecentRuns           []TraceRunSummary `json:"recentRuns"`
	Status               string            `json:"status"`
	RouteChanged         bool              `json:"routeChanged"`
	HasDegradedHop       bool              `json:"hasDegradedHop"`
}

type TraceRunSnapshot struct {
	ID               int64              `json:"id"`
	StartedAt        time.Time          `json:"startedAt"`
	CompletedAt      time.Time          `json:"completedAt"`
	HopCount         int                `json:"hopCount"`
	Status           string             `json:"status"`
	RouteFingerprint string             `json:"routeFingerprint"`
	RouteChanged     bool               `json:"routeChanged"`
	DegradedHopCount int                `json:"degradedHopCount"`
	ErrorMessage     string             `json:"errorMessage"`
	Hops             []TraceHopSnapshot `json:"hops"`
}

type TraceHopSnapshot struct {
	HopIndex   int     `json:"hopIndex"`
	Address    string  `json:"address"`
	Hostname   string  `json:"hostname"`
	AvgRTTMs   float64 `json:"avgRttMs"`
	JitterMs   float64 `json:"jitterMs"`
	LossPct    float64 `json:"lossPct"`
	IsTimeout  bool    `json:"isTimeout"`
	IsDegraded bool    `json:"isDegraded"`
}

type TraceRunSummary struct {
	StartedAt        time.Time `json:"startedAt"`
	HopCount         int       `json:"hopCount"`
	Status           string    `json:"status"`
	RouteChanged     bool      `json:"routeChanged"`
	DegradedHopCount int       `json:"degradedHopCount"`
}

type traceTarget struct {
	ID                   int64
	Name                 string
	Host                 string
	ProbeIntervalSeconds int
}

type traceHopMeasurement struct {
	HopIndex   int
	Address    string
	Hostname   string
	AvgRTTMs   float64
	JitterMs   float64
	LossPct    float64
	IsTimeout  bool
	IsDegraded bool
}

type latestTraceRow struct {
	TargetID             int64
	TargetName           string
	TargetHost           string
	ProbeIntervalSeconds int
	RunID                sql.NullInt64
	StartedAt            sql.NullString
	CompletedAt          sql.NullString
	HopCount             sql.NullInt64
	Status               sql.NullString
	RouteFingerprint     sql.NullString
	RouteChanged         sql.NullInt64
	DegradedHopCount     sql.NullInt64
	ErrorMessage         sql.NullString
}

func NewPathCollector(database *sql.DB, targets []string, defaultProbeInterval, commandTimeout time.Duration, maxHops int) *PathCollector {
	commandPath, _ := exec.LookPath("traceroute")
	if defaultProbeInterval <= 0 {
		defaultProbeInterval = defaultTraceIntervalSeconds * time.Second
	}
	if commandTimeout <= 0 {
		commandTimeout = 20 * time.Second
	}
	if maxHops <= 0 {
		maxHops = 16
	}

	return &PathCollector{
		db:                   database,
		configuredTargets:    append([]string(nil), targets...),
		defaultProbeInterval: defaultProbeInterval,
		commandTimeout:       commandTimeout,
		maxHops:              maxHops,
		lastRun:              make(map[int64]time.Time),
		commandPath:          commandPath,
	}
}

func (c *PathCollector) Run(ctx context.Context) error {
	if err := c.syncConfiguredTargets(ctx); err != nil {
		return err
	}

	if len(c.configuredTargets) == 0 {
		return nil
	}

	if c.commandPath == "" {
		log.Printf("path discovery disabled: traceroute executable not found")
		return nil
	}

	c.collect(ctx)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *PathCollector) collect(ctx context.Context) {
	if err := c.collectDue(ctx); err != nil {
		log.Printf("collect path discovery: %v", err)
	}
}

func (c *PathCollector) collectDue(ctx context.Context) error {
	targets, err := c.listEnabledTargets(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, target := range targets {
		interval := time.Duration(target.ProbeIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = c.defaultProbeInterval
		}

		lastRunAt := c.lastRun[target.ID]
		if !lastRunAt.IsZero() && now.Sub(lastRunAt) < interval {
			continue
		}

		c.lastRun[target.ID] = now
		if err := c.collectTarget(ctx, target); err != nil {
			log.Printf("trace %s: %v", target.Host, err)
		}
	}

	return nil
}

func (c *PathCollector) collectTarget(ctx context.Context, target traceTarget) error {
	startedAt := time.Now().UTC()
	traceCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	hops, execErr := c.executeTraceroute(traceCtx, target.Host)
	status := "completed"
	errorMessage := ""
	if execErr != nil {
		errorMessage = execErr.Error()
		if len(hops) == 0 {
			status = "failed"
		} else {
			status = "partial"
		}
	}

	fingerprint := buildRouteFingerprint(hops)
	previousFingerprint, err := c.previousSuccessfulFingerprint(ctx, target.ID)
	if err != nil {
		return fmt.Errorf("load previous fingerprint: %w", err)
	}

	routeChanged := fingerprint != "" && previousFingerprint != "" && fingerprint != previousFingerprint
	degradedHopCount := countDegradedHops(hops)

	if err := c.persistTraceRun(
		ctx,
		target,
		startedAt,
		time.Now().UTC(),
		status,
		errorMessage,
		fingerprint,
		routeChanged,
		degradedHopCount,
		hops,
	); err != nil {
		return fmt.Errorf("persist trace run: %w", err)
	}

	if execErr != nil && len(hops) == 0 {
		return execErr
	}

	return nil
}

func (c *PathCollector) syncConfiguredTargets(ctx context.Context) error {
	if len(c.configuredTargets) == 0 {
		return nil
	}

	defaultIntervalSeconds := int(c.defaultProbeInterval / time.Second)
	if defaultIntervalSeconds <= 0 {
		defaultIntervalSeconds = defaultTraceIntervalSeconds
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin target sync transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const query = `
		INSERT INTO targets (name, host, enabled, probe_interval_seconds)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(host) DO UPDATE SET
			name = excluded.name,
			enabled = 1,
			updated_at = CURRENT_TIMESTAMP;
	`

	for _, host := range c.configuredTargets {
		if _, err := tx.ExecContext(ctx, query, host, host, defaultIntervalSeconds); err != nil {
			return fmt.Errorf("upsert trace target %s: %w", host, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit target sync: %w", err)
	}
	committed = true
	return nil
}

func (c *PathCollector) listEnabledTargets(ctx context.Context) ([]traceTarget, error) {
	const query = `
		SELECT id, name, host, probe_interval_seconds
		FROM targets
		WHERE enabled = 1
		ORDER BY name ASC, host ASC;
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query trace targets: %w", err)
	}
	defer rows.Close()

	var targets []traceTarget
	for rows.Next() {
		var target traceTarget
		if err := rows.Scan(&target.ID, &target.Name, &target.Host, &target.ProbeIntervalSeconds); err != nil {
			return nil, fmt.Errorf("scan trace target: %w", err)
		}
		targets = append(targets, target)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace targets: %w", err)
	}

	return targets, nil
}

func (c *PathCollector) executeTraceroute(ctx context.Context, host string) ([]traceHopMeasurement, error) {
	args := []string{
		"-q", "3",
		"-m", strconv.Itoa(c.maxHops),
		"-w", "2",
		host,
	}

	output, err := exec.CommandContext(ctx, c.commandPath, args...).CombinedOutput()
	hops, parseErr := parseTracerouteOutput(string(output))
	if parseErr != nil {
		if err != nil {
			return nil, fmt.Errorf("%w; parse traceroute output: %v", err, parseErr)
		}
		return nil, parseErr
	}
	if ctx.Err() != nil {
		return hops, ctx.Err()
	}
	if err != nil {
		return hops, err
	}
	return hops, nil
}

func (c *PathCollector) previousSuccessfulFingerprint(ctx context.Context, targetID int64) (string, error) {
	const query = `
		SELECT route_fingerprint
		FROM trace_runs
		WHERE target_id = ?
		  AND hop_count > 0
		  AND route_fingerprint <> ''
		  AND status IN ('completed', 'partial')
		ORDER BY started_at DESC
		LIMIT 1;
	`

	var fingerprint string
	err := c.db.QueryRowContext(ctx, query, targetID).Scan(&fingerprint)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	return fingerprint, nil
}

func (c *PathCollector) persistTraceRun(ctx context.Context, target traceTarget, startedAt, completedAt time.Time, status, errorMessage, fingerprint string, routeChanged bool, degradedHopCount int, hops []traceHopMeasurement) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin trace transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const runQuery = `
		INSERT INTO trace_runs (
			target_id,
			started_at,
			completed_at,
			hop_count,
			status,
			route_fingerprint,
			route_changed,
			degraded_hop_count,
			error_message
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id;
	`

	var traceRunID int64
	if err := tx.QueryRowContext(
		ctx,
		runQuery,
		target.ID,
		formatTimestamp(startedAt),
		formatTimestamp(completedAt),
		len(hops),
		status,
		fingerprint,
		boolToInt(routeChanged),
		degradedHopCount,
		errorMessage,
	).Scan(&traceRunID); err != nil {
		return fmt.Errorf("insert trace run: %w", err)
	}

	const hopQuery = `
		INSERT INTO trace_hops (
			trace_run_id,
			hop_index,
			address,
			hostname,
			avg_rtt_ms,
			jitter_ms,
			loss_pct,
			is_timeout
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`

	for _, hop := range hops {
		if _, err := tx.ExecContext(
			ctx,
			hopQuery,
			traceRunID,
			hop.HopIndex,
			hop.Address,
			hop.Hostname,
			hop.AvgRTTMs,
			hop.JitterMs,
			hop.LossPct,
			boolToInt(hop.IsTimeout),
		); err != nil {
			return fmt.Errorf("insert trace hop %d: %w", hop.HopIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit trace run: %w", err)
	}
	committed = true
	return nil
}

func (s *Service) listPathTargets(ctx context.Context, recentLimit int) ([]TargetPathSnapshot, error) {
	latestRows, err := s.listLatestTraceRows(ctx)
	if err != nil {
		return nil, err
	}

	runIDs := make([]int64, 0, len(latestRows))
	for _, row := range latestRows {
		if row.RunID.Valid {
			runIDs = append(runIDs, row.RunID.Int64)
		}
	}

	hopsByRun, err := s.listTraceHopsByRun(ctx, runIDs)
	if err != nil {
		return nil, err
	}

	recentRunsByTarget, err := s.listRecentTraceRuns(ctx, recentLimit)
	if err != nil {
		return nil, err
	}

	targets := make([]TargetPathSnapshot, 0, len(latestRows))
	for _, row := range latestRows {
		target := TargetPathSnapshot{
			ID:                   row.TargetID,
			Name:                 row.TargetName,
			Host:                 row.TargetHost,
			ProbeIntervalSeconds: row.ProbeIntervalSeconds,
			RecentRuns:           recentRunsByTarget[row.TargetID],
			Status:               "pending",
		}

		if row.RunID.Valid {
			startedAt, err := parseTimestamp(row.StartedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse trace run start: %w", err)
			}

			completedAt := startedAt
			if row.CompletedAt.Valid {
				completedAt, err = parseTimestamp(row.CompletedAt.String)
				if err != nil {
					return nil, fmt.Errorf("parse trace run completion: %w", err)
				}
			}

			latestRun := &TraceRunSnapshot{
				ID:               row.RunID.Int64,
				StartedAt:        startedAt,
				CompletedAt:      completedAt,
				HopCount:         int(row.HopCount.Int64),
				Status:           row.Status.String,
				RouteFingerprint: row.RouteFingerprint.String,
				RouteChanged:     row.RouteChanged.Int64 == 1,
				DegradedHopCount: int(row.DegradedHopCount.Int64),
				ErrorMessage:     row.ErrorMessage.String,
				Hops:             hopsByRun[row.RunID.Int64],
			}

			target.LatestRun = latestRun
			target.Status = latestRun.Status
			target.RouteChanged = latestRun.RouteChanged
			target.HasDegradedHop = latestRun.DegradedHopCount > 0
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func (s *Service) listLatestTraceRows(ctx context.Context) ([]latestTraceRow, error) {
	const query = `
		SELECT
			t.id,
			t.name,
			t.host,
			t.probe_interval_seconds,
			r.id,
			r.started_at,
			r.completed_at,
			r.hop_count,
			r.status,
			r.route_fingerprint,
			r.route_changed,
			r.degraded_hop_count,
			r.error_message
		FROM targets t
		LEFT JOIN trace_runs r ON r.id = (
			SELECT r2.id
			FROM trace_runs r2
			WHERE r2.target_id = t.id
			ORDER BY r2.started_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1
		ORDER BY t.name ASC, t.host ASC;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latest trace runs: %w", err)
	}
	defer rows.Close()

	var latest []latestTraceRow
	for rows.Next() {
		var row latestTraceRow
		if err := rows.Scan(
			&row.TargetID,
			&row.TargetName,
			&row.TargetHost,
			&row.ProbeIntervalSeconds,
			&row.RunID,
			&row.StartedAt,
			&row.CompletedAt,
			&row.HopCount,
			&row.Status,
			&row.RouteFingerprint,
			&row.RouteChanged,
			&row.DegradedHopCount,
			&row.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan latest trace run: %w", err)
		}
		latest = append(latest, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest trace runs: %w", err)
	}

	return latest, nil
}

func (s *Service) listTraceHopsByRun(ctx context.Context, runIDs []int64) (map[int64][]TraceHopSnapshot, error) {
	if len(runIDs) == 0 {
		return map[int64][]TraceHopSnapshot{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, 0, len(runIDs))
	for _, runID := range runIDs {
		args = append(args, runID)
	}

	query := fmt.Sprintf(`
		SELECT trace_run_id, hop_index, address, hostname, avg_rtt_ms, jitter_ms, loss_pct, is_timeout
		FROM trace_hops
		WHERE trace_run_id IN (%s)
		ORDER BY trace_run_id ASC, hop_index ASC;
	`, placeholders)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trace hops: %w", err)
	}
	defer rows.Close()

	hopsByRun := make(map[int64][]TraceHopSnapshot, len(runIDs))
	for rows.Next() {
		var (
			runID     int64
			hop       TraceHopSnapshot
			isTimeout int
		)

		if err := rows.Scan(&runID, &hop.HopIndex, &hop.Address, &hop.Hostname, &hop.AvgRTTMs, &hop.JitterMs, &hop.LossPct, &isTimeout); err != nil {
			return nil, fmt.Errorf("scan trace hop: %w", err)
		}

		hop.IsTimeout = isTimeout == 1
		hop.IsDegraded = hop.LossPct >= degradedLossThresholdPct || hop.AvgRTTMs >= degradedRTTThresholdMs
		hopsByRun[runID] = append(hopsByRun[runID], hop)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace hops: %w", err)
	}

	return hopsByRun, nil
}

func (s *Service) listRecentTraceRuns(ctx context.Context, limit int) (map[int64][]TraceRunSummary, error) {
	if limit <= 0 {
		return map[int64][]TraceRunSummary{}, nil
	}

	const query = `
		SELECT target_id, started_at, hop_count, status, route_changed, degraded_hop_count
		FROM (
			SELECT
				target_id,
				started_at,
				hop_count,
				status,
				route_changed,
				degraded_hop_count,
				ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY started_at DESC) AS row_num
			FROM trace_runs
		)
		WHERE row_num <= ?
		ORDER BY target_id ASC, started_at DESC;
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent trace runs: %w", err)
	}
	defer rows.Close()

	history := make(map[int64][]TraceRunSummary)
	for rows.Next() {
		var (
			targetID         int64
			startedAtRaw     string
			hopCount         int
			status           string
			routeChangedRaw  int
			degradedHopCount int
		)

		if err := rows.Scan(&targetID, &startedAtRaw, &hopCount, &status, &routeChangedRaw, &degradedHopCount); err != nil {
			return nil, fmt.Errorf("scan recent trace run: %w", err)
		}

		startedAt, err := parseTimestamp(startedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse recent trace timestamp: %w", err)
		}

		history[targetID] = append(history[targetID], TraceRunSummary{
			StartedAt:        startedAt,
			HopCount:         hopCount,
			Status:           status,
			RouteChanged:     routeChangedRaw == 1,
			DegradedHopCount: degradedHopCount,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent trace runs: %w", err)
	}

	return history, nil
}

func parseTracerouteOutput(output string) ([]traceHopMeasurement, error) {
	lines := strings.Split(output, "\n")
	hops := make([]traceHopMeasurement, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := tracerouteHopPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		hopIndex, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		rest := matches[2]
		hop := traceHopMeasurement{HopIndex: hopIndex}
		hop.Hostname, hop.Address = parseHopEndpoint(rest)

		rttMatches := tracerouteRTTPattern.FindAllStringSubmatch(rest, -1)
		rtts := make([]float64, 0, len(rttMatches))
		for _, match := range rttMatches {
			value, err := strconv.ParseFloat(match[1], 64)
			if err != nil {
				continue
			}
			rtts = append(rtts, value)
		}

		timeoutCount := strings.Count(rest, "*")
		if len(rtts) == 0 {
			timeoutCount = 3
			hop.IsTimeout = true
		}

		totalProbes := len(rtts) + timeoutCount
		if totalProbes == 0 {
			totalProbes = 3
		}

		hop.AvgRTTMs = average(rtts)
		hop.JitterMs = jitter(rtts)
		hop.LossPct = float64(timeoutCount) * 100 / float64(totalProbes)
		hop.IsDegraded = hop.LossPct >= degradedLossThresholdPct || hop.AvgRTTMs >= degradedRTTThresholdMs

		hops = append(hops, hop)
	}

	if len(hops) == 0 {
		return nil, fmt.Errorf("no hop lines parsed")
	}

	return hops, nil
}

func parseHopEndpoint(rest string) (string, string) {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" || strings.HasPrefix(trimmed, "*") {
		return "No reply", ""
	}

	endpointPart := rest
	if idx := tracerouteRTTPattern.FindStringIndex(rest); idx != nil {
		endpointPart = rest[:idx[0]]
	}
	if starIndex := strings.Index(endpointPart, "*"); starIndex >= 0 {
		endpointPart = endpointPart[:starIndex]
	}
	endpointPart = strings.TrimSpace(endpointPart)
	if endpointPart == "" {
		return "No reply", ""
	}

	if matches := tracerouteEndpointPattern.FindStringSubmatch(endpointPart); len(matches) == 3 {
		hostname := strings.TrimSpace(matches[1])
		address := strings.TrimSpace(matches[2])
		if hostname == "" {
			hostname = address
		}
		return hostname, address
	}

	fields := strings.Fields(endpointPart)
	if len(fields) == 0 {
		return "No reply", ""
	}

	if net.ParseIP(fields[0]) != nil {
		return fields[0], fields[0]
	}

	return fields[0], ""
}

func buildRouteFingerprint(hops []traceHopMeasurement) string {
	if len(hops) == 0 {
		return ""
	}

	parts := make([]string, 0, len(hops))
	for _, hop := range hops {
		identity := hop.Address
		if identity == "" {
			identity = hop.Hostname
		}
		if identity == "" {
			identity = "*"
		}
		parts = append(parts, fmt.Sprintf("%d:%s", hop.HopIndex, identity))
	}

	return strings.Join(parts, "|")
}

func countDegradedHops(hops []traceHopMeasurement) int {
	count := 0
	for _, hop := range hops {
		if hop.IsDegraded {
			count++
		}
	}
	return count
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func jitter(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	deltaTotal := 0.0
	for index := 1; index < len(values); index++ {
		deltaTotal += math.Abs(values[index] - values[index-1])
	}

	return deltaTotal / float64(len(values)-1)
}
