package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"

	"pi-ntop/internal/config"
)

const (
	defaultSampleInterval = time.Second
	defaultHistoryLimit   = 30
	defaultPathHistory    = 6
	defaultSpeedHistory   = 12
)

type Service struct {
	db             *sql.DB
	config         config.Config
	collector      *Collector
	pathCollector  *PathCollector
	speedCollector *SpeedCollector
	sampleInterval time.Duration
	historyLimit   int
	pathHistory    int
	speedHistory   int
	alertLimit     int
	retentionMu    sync.RWMutex
	retentionState RetentionSnapshot
}

type DashboardSnapshot struct {
	GeneratedAt           time.Time             `json:"generatedAt"`
	SampleIntervalSeconds int                   `json:"sampleIntervalSeconds"`
	ActiveInterfaceCount  int                   `json:"activeInterfaceCount"`
	MonitoredTargetCount  int                   `json:"monitoredTargetCount"`
	DegradedPathCount     int                   `json:"degradedPathCount"`
	RouteChangeCount      int                   `json:"routeChangeCount"`
	SpeedTargetCount      int                   `json:"speedTargetCount"`
	FailedSpeedTestCount  int                   `json:"failedSpeedTestCount"`
	ActiveAlertCount      int                   `json:"activeAlertCount"`
	AverageDownloadBps    float64               `json:"averageDownloadBps"`
	AverageUploadBps      float64               `json:"averageUploadBps"`
	TotalRXBps            float64               `json:"totalRxBps"`
	TotalTXBps            float64               `json:"totalTxBps"`
	Interfaces            []InterfaceSnapshot   `json:"interfaces"`
	PathTargets           []TargetPathSnapshot  `json:"pathTargets"`
	SpeedTargets          []SpeedTargetSnapshot `json:"speedTargets"`
	Alerts                []AlertSnapshot       `json:"alerts"`
	Retention             RetentionSnapshot     `json:"retention"`
}

type AlertSnapshot struct {
	ID             int64     `json:"id"`
	Severity       string    `json:"severity"`
	SourceType     string    `json:"sourceType"`
	SourceID       int64     `json:"sourceId"`
	SourceName     string    `json:"sourceName"`
	MetricKey      string    `json:"metricKey"`
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	ThresholdValue float64   `json:"thresholdValue"`
	CurrentValue   float64   `json:"currentValue"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	ResolvedAt     time.Time `json:"resolvedAt"`
}

type RetentionSnapshot struct {
	JobIntervalSeconds       int       `json:"jobIntervalSeconds"`
	InterfaceRawSeconds      int       `json:"interfaceRawSeconds"`
	InterfaceRollupSeconds   int       `json:"interfaceRollupSeconds"`
	TraceRetentionSeconds    int       `json:"traceRetentionSeconds"`
	SpeedRetentionSeconds    int       `json:"speedRetentionSeconds"`
	ResolvedAlertSeconds     int       `json:"resolvedAlertSeconds"`
	LastRunAt                time.Time `json:"lastRunAt"`
	LastSuccessAt            time.Time `json:"lastSuccessAt"`
	LastError                string    `json:"lastError"`
	UpsertedInterfaceRollups int64     `json:"upsertedInterfaceRollups"`
	DeletedInterfaceSamples  int64     `json:"deletedInterfaceSamples"`
	DeletedInterfaceRollups  int64     `json:"deletedInterfaceRollups"`
	DeletedTraceRuns         int64     `json:"deletedTraceRuns"`
	DeletedSpeedTests        int64     `json:"deletedSpeedTests"`
	DeletedResolvedAlerts    int64     `json:"deletedResolvedAlerts"`
}

type InterfaceSnapshot struct {
	Name            string            `json:"name"`
	DisplayName     string            `json:"displayName"`
	IsActive        bool              `json:"isActive"`
	IsLoopback      bool              `json:"isLoopback"`
	CapturedAt      time.Time         `json:"capturedAt"`
	RXBytesTotal    uint64            `json:"rxBytesTotal"`
	TXBytesTotal    uint64            `json:"txBytesTotal"`
	RXBps           float64           `json:"rxBps"`
	TXBps           float64           `json:"txBps"`
	History         []ThroughputPoint `json:"history"`
	CombinedPeakBps float64           `json:"combinedPeakBps"`
}

type ThroughputPoint struct {
	CapturedAt time.Time `json:"capturedAt"`
	RXBps      float64   `json:"rxBps"`
	TXBps      float64   `json:"txBps"`
}

type Collector struct {
	db             *sql.DB
	sampleInterval time.Duration
	previous       map[string]counterSample
	interfaceIDs   map[string]int64
}

type counterSample struct {
	CapturedAt time.Time
	RXBytes    uint64
	TXBytes    uint64
}

type interfaceMeta struct {
	DisplayName string
	IsLoopback  bool
}

type latestRow struct {
	ID           int64
	Name         string
	DisplayName  string
	IsActive     bool
	IsLoopback   bool
	CapturedAt   time.Time
	RXBytesTotal uint64
	TXBytesTotal uint64
	RXBps        float64
	TXBps        float64
}

func New(database *sql.DB, cfg config.Config) *Service {
	return &Service{
		db:             database,
		config:         cfg,
		collector:      NewCollector(database, defaultSampleInterval),
		pathCollector:  NewPathCollector(database, cfg.TraceTargets, cfg.TraceInterval, cfg.TraceTimeout, cfg.TraceMaxHops),
		speedCollector: NewSpeedCollector(database, cfg.SpeedTargets, cfg.SpeedInterval, cfg.SpeedTimeout, cfg.SpeedDownBytes, cfg.SpeedUpBytes),
		sampleInterval: defaultSampleInterval,
		historyLimit:   defaultHistoryLimit,
		pathHistory:    defaultPathHistory,
		speedHistory:   defaultSpeedHistory,
		alertLimit:     6,
	}
}

func (s *Service) Run(ctx context.Context) {
	go func() {
		if err := s.collector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("interface monitor stopped: %v", err)
		}
	}()

	go func() {
		if err := s.pathCollector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("path discovery stopped: %v", err)
		}
	}()

	go func() {
		if err := s.speedCollector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("speed tests stopped: %v", err)
		}
	}()

	go func() {
		if err := s.runAlertLoop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("alert evaluator stopped: %v", err)
		}
	}()

	go func() {
		if err := s.runRetentionLoop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("retention worker stopped: %v", err)
		}
	}()
}

func (s *Service) Dashboard(ctx context.Context) (DashboardSnapshot, error) {
	rows, err := s.listLatest(ctx)
	if err != nil {
		return DashboardSnapshot{}, err
	}

	historyByInterface, err := s.listHistory(ctx, s.historyLimit)
	if err != nil {
		return DashboardSnapshot{}, err
	}

	snapshot := DashboardSnapshot{
		GeneratedAt:           time.Now().UTC(),
		SampleIntervalSeconds: int(s.sampleInterval / time.Second),
		Interfaces:            make([]InterfaceSnapshot, 0, len(rows)),
	}

	for _, row := range rows {
		interfaceHistory := historyByInterface[row.ID]
		peak := row.RXBps + row.TXBps
		for _, point := range interfaceHistory {
			if total := point.RXBps + point.TXBps; total > peak {
				peak = total
			}
		}

		iface := InterfaceSnapshot{
			Name:            row.Name,
			DisplayName:     row.DisplayName,
			IsActive:        row.IsActive,
			IsLoopback:      row.IsLoopback,
			CapturedAt:      row.CapturedAt,
			RXBytesTotal:    row.RXBytesTotal,
			TXBytesTotal:    row.TXBytesTotal,
			RXBps:           row.RXBps,
			TXBps:           row.TXBps,
			History:         interfaceHistory,
			CombinedPeakBps: peak,
		}

		snapshot.TotalRXBps += iface.RXBps
		snapshot.TotalTXBps += iface.TXBps
		snapshot.Interfaces = append(snapshot.Interfaces, iface)
	}

	snapshot.ActiveInterfaceCount = len(snapshot.Interfaces)

	pathTargets, err := s.listPathTargets(ctx, s.pathHistory)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	snapshot.PathTargets = pathTargets
	snapshot.MonitoredTargetCount = len(pathTargets)
	for _, target := range pathTargets {
		if target.RouteChanged {
			snapshot.RouteChangeCount++
		}
		if target.HasDegradedHop {
			snapshot.DegradedPathCount++
		}
	}

	speedTargets, err := s.listSpeedTargets(ctx, s.speedHistory)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	snapshot.SpeedTargets = speedTargets
	snapshot.SpeedTargetCount = len(speedTargets)

	measuredDownloads := 0
	measuredUploads := 0
	for _, target := range speedTargets {
		if target.LatestTest == nil {
			continue
		}
		if target.Status == "failed" {
			snapshot.FailedSpeedTestCount++
		}
		if target.LatestTest.DownloadBps > 0 {
			snapshot.AverageDownloadBps += target.LatestTest.DownloadBps
			measuredDownloads++
		}
		if target.LatestTest.UploadBps > 0 {
			snapshot.AverageUploadBps += target.LatestTest.UploadBps
			measuredUploads++
		}
	}
	if measuredDownloads > 0 {
		snapshot.AverageDownloadBps /= float64(measuredDownloads)
	}
	if measuredUploads > 0 {
		snapshot.AverageUploadBps /= float64(measuredUploads)
	}

	alerts, alertCount, err := s.listActiveAlerts(ctx, s.alertLimit)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	snapshot.Alerts = alerts
	snapshot.ActiveAlertCount = alertCount
	snapshot.Retention = s.currentRetentionSnapshot()

	return snapshot, nil
}

func (s *Service) listLatest(ctx context.Context) ([]latestRow, error) {
	const query = `
		SELECT
			i.id,
			i.name,
			i.display_name,
			i.is_active,
			i.is_loopback,
			s.captured_at,
			s.rx_bytes_total,
			s.tx_bytes_total,
			s.rx_bps,
			s.tx_bps
		FROM interfaces i
		JOIN interface_samples s ON s.id = (
			SELECT s2.id
			FROM interface_samples s2
			WHERE s2.interface_id = i.id
			ORDER BY s2.captured_at DESC
			LIMIT 1
		)
		WHERE i.is_active = 1
		ORDER BY (s.rx_bps + s.tx_bps) DESC, i.name ASC;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latest interface samples: %w", err)
	}
	defer rows.Close()

	var latest []latestRow
	for rows.Next() {
		var (
			row           latestRow
			capturedAtRaw string
			isActiveRaw   int
			isLoopbackRaw int
		)

		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.DisplayName,
			&isActiveRaw,
			&isLoopbackRaw,
			&capturedAtRaw,
			&row.RXBytesTotal,
			&row.TXBytesTotal,
			&row.RXBps,
			&row.TXBps,
		); err != nil {
			return nil, fmt.Errorf("scan latest interface sample: %w", err)
		}

		capturedAt, err := parseTimestamp(capturedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse latest interface sample timestamp: %w", err)
		}

		row.CapturedAt = capturedAt
		row.IsActive = isActiveRaw == 1
		row.IsLoopback = isLoopbackRaw == 1
		latest = append(latest, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest interface samples: %w", err)
	}

	return latest, nil
}

func (s *Service) listHistory(ctx context.Context, limit int) (map[int64][]ThroughputPoint, error) {
	const query = `
		SELECT interface_id, captured_at, rx_bps, tx_bps
		FROM (
			SELECT
				s.interface_id,
				s.captured_at,
				s.rx_bps,
				s.tx_bps,
				ROW_NUMBER() OVER (PARTITION BY s.interface_id ORDER BY s.captured_at DESC) AS row_num
			FROM interface_samples s
			JOIN interfaces i ON i.id = s.interface_id
			WHERE i.is_active = 1
		)
		WHERE row_num <= ?
		ORDER BY interface_id ASC, captured_at ASC;
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query interface history: %w", err)
	}
	defer rows.Close()

	history := make(map[int64][]ThroughputPoint)
	for rows.Next() {
		var (
			interfaceID   int64
			capturedAtRaw string
			point         ThroughputPoint
		)

		if err := rows.Scan(&interfaceID, &capturedAtRaw, &point.RXBps, &point.TXBps); err != nil {
			return nil, fmt.Errorf("scan interface history: %w", err)
		}

		capturedAt, err := parseTimestamp(capturedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse interface history timestamp: %w", err)
		}

		point.CapturedAt = capturedAt
		history[interfaceID] = append(history[interfaceID], point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface history: %w", err)
	}

	return history, nil
}

func NewCollector(database *sql.DB, sampleInterval time.Duration) *Collector {
	return &Collector{
		db:             database,
		sampleInterval: sampleInterval,
		previous:       make(map[string]counterSample),
		interfaceIDs:   make(map[string]int64),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	c.collect(ctx)

	ticker := time.NewTicker(c.sampleInterval)
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

func (c *Collector) collect(ctx context.Context) {
	if err := c.collectOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("collect interface counters: %v", err)
	}
}

func (c *Collector) collectOnce(ctx context.Context) error {
	committed := false
	metadata, err := activeInterfaces()
	if err != nil {
		return fmt.Errorf("list active interfaces: %w", err)
	}

	counters, err := gopsnet.IOCounters(true)
	if err != nil {
		return fmt.Errorf("read interface counters: %w", err)
	}

	now := time.Now().UTC()
	seenNames := make([]string, 0, len(counters))

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin interface sample transaction: %w", err)
	}
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, counter := range counters {
		meta, ok := metadata[counter.Name]
		if !ok {
			continue
		}

		interfaceID, err := c.upsertInterface(ctx, tx, counter.Name, meta, now)
		if err != nil {
			return err
		}

		rxBps, txBps := c.computeRates(counter.Name, now, counter.BytesRecv, counter.BytesSent)
		if err := c.insertSample(ctx, tx, interfaceID, now, counter.BytesRecv, counter.BytesSent, rxBps, txBps); err != nil {
			return err
		}

		c.previous[counter.Name] = counterSample{
			CapturedAt: now,
			RXBytes:    counter.BytesRecv,
			TXBytes:    counter.BytesSent,
		}
		seenNames = append(seenNames, counter.Name)
	}

	if err := c.markInactiveInterfaces(ctx, tx, seenNames); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit interface samples: %w", err)
	}
	committed = true

	return nil
}

func (c *Collector) upsertInterface(ctx context.Context, tx *sql.Tx, name string, meta interfaceMeta, now time.Time) (int64, error) {
	if id, ok := c.interfaceIDs[name]; ok {
		const updateQuery = `
			UPDATE interfaces
			SET display_name = ?, is_active = 1, is_loopback = ?, last_seen_at = ?, updated_at = ?
			WHERE id = ?;
		`

		if _, err := tx.ExecContext(ctx, updateQuery, meta.DisplayName, boolToInt(meta.IsLoopback), formatTimestamp(now), formatTimestamp(now), id); err != nil {
			return 0, fmt.Errorf("update interface %s: %w", name, err)
		}
		return id, nil
	}

	const query = `
		INSERT INTO interfaces (name, display_name, is_active, is_loopback, last_seen_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			display_name = excluded.display_name,
			is_active = excluded.is_active,
			is_loopback = excluded.is_loopback,
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at
		RETURNING id;
	`

	var interfaceID int64
	if err := tx.QueryRowContext(ctx, query, name, meta.DisplayName, boolToInt(meta.IsLoopback), formatTimestamp(now), formatTimestamp(now)).Scan(&interfaceID); err != nil {
		return 0, fmt.Errorf("upsert interface %s: %w", name, err)
	}

	c.interfaceIDs[name] = interfaceID
	return interfaceID, nil
}

func (c *Collector) insertSample(ctx context.Context, tx *sql.Tx, interfaceID int64, capturedAt time.Time, rxBytesTotal, txBytesTotal uint64, rxBps, txBps float64) error {
	const query = `
		INSERT INTO interface_samples (interface_id, captured_at, rx_bytes_total, tx_bytes_total, rx_bps, tx_bps)
		VALUES (?, ?, ?, ?, ?, ?);
	`

	if _, err := tx.ExecContext(ctx, query, interfaceID, formatTimestamp(capturedAt), rxBytesTotal, txBytesTotal, rxBps, txBps); err != nil {
		return fmt.Errorf("insert interface sample for id %d: %w", interfaceID, err)
	}

	return nil
}

func (c *Collector) markInactiveInterfaces(ctx context.Context, tx *sql.Tx, seenNames []string) error {
	if len(seenNames) == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE interfaces SET is_active = 0, updated_at = ?;`, formatTimestamp(time.Now().UTC())); err != nil {
			return fmt.Errorf("mark interfaces inactive: %w", err)
		}
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seenNames)), ",")
	args := make([]any, 0, len(seenNames)+1)
	args = append(args, formatTimestamp(time.Now().UTC()))
	for _, name := range seenNames {
		args = append(args, name)
	}

	query := fmt.Sprintf(`UPDATE interfaces SET is_active = 0, updated_at = ? WHERE name NOT IN (%s);`, placeholders)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("mark unseen interfaces inactive: %w", err)
	}

	return nil
}

func (c *Collector) computeRates(name string, capturedAt time.Time, rxBytesTotal, txBytesTotal uint64) (float64, float64) {
	previous, ok := c.previous[name]
	if !ok {
		return 0, 0
	}

	elapsed := capturedAt.Sub(previous.CapturedAt).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}

	rxDelta := counterDelta(previous.RXBytes, rxBytesTotal)
	txDelta := counterDelta(previous.TXBytes, txBytesTotal)

	return float64(rxDelta) * 8 / elapsed, float64(txDelta) * 8 / elapsed
}

func activeInterfaces() (map[string]interfaceMeta, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]interfaceMeta, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		metadata[iface.Name] = interfaceMeta{
			DisplayName: iface.Name,
			IsLoopback:  iface.Flags&net.FlagLoopback != 0,
		}
	}

	return metadata, nil
}

func counterDelta(previous, current uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTimestamp(value string) (time.Time, error) {
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, layout := range formats {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format %q", value)
}
