package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Tier is one resolution of stored history.
type Tier int

const (
	// TierRaw holds every Report.
	TierRaw Tier = iota
	// TierMinute holds 1-minute averages.
	TierMinute
	// TierQuarter holds 15-minute averages.
	TierQuarter
)

var historyTables = []string{"metrics_raw", "metrics_1m", "metrics_15m"}

// bucket is the width of each tier's buckets, in milliseconds (0: none).
var bucket = []int64{0, int64(time.Minute / time.Millisecond), int64(15 * time.Minute / time.Millisecond)}

// metricColumns are the charted metrics, in the order Sample and History use.
const metricColumns = `cpu, memory, swap, disk, rx_rate, tx_rate, load1`

// Sample is one Report's charted metrics. Nil means the Report lacked it.
type Sample struct {
	HostID string
	// T is when the Dashboard received the Report, in Unix milliseconds.
	T                       int64
	CPU, Memory, Swap, Disk *float64
	RxRate, TxRate, Load1   *float64
}

func (s Sample) values() []any {
	return []any{s.CPU, s.Memory, s.Swap, s.Disk, s.RxRate, s.TxRate, s.Load1}
}

// WriteBatch stores samples and each Host's latest Report (as JSON) in one transaction.
func (s *Store) WriteBatch(ctx context.Context, samples []Sample, latest map[string][]byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	insert, err := tx.PrepareContext(ctx, `INSERT INTO metrics_raw (host_id, t, `+metricColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for _, smp := range samples {
		if _, err := insert.ExecContext(ctx, append([]any{smp.HostID, smp.T}, smp.values()...)...); err != nil {
			return err
		}
	}
	for id, report := range latest {
		if _, err := tx.ExecContext(ctx, `UPDATE hosts SET latest_report = ? WHERE id = ?`, string(report), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LatestReports returns each Host's last stored Report (as JSON).
func (s *Store) LatestReports(ctx context.Context) (map[string][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, latest_report FROM hosts WHERE latest_report IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var id, report string
		if err := rows.Scan(&id, &report); err != nil {
			return nil, err
		}
		out[id] = []byte(report)
	}
	return out, rows.Err()
}

// Retention is how long each tier is kept.
type Retention struct {
	Raw, Minute, Quarter time.Duration
}

// Downsample averages raw samples into 1-minute buckets and those into
// 15-minute buckets, for every complete bucket from since (Unix ms) up to now,
// then deletes data older than each tier's retention. Re-running it over the
// same period is harmless: buckets are recomputed, not added to.
func (s *Store) Downsample(ctx context.Context, since int64, now time.Time, keep Retention) error {
	nowMS := now.UnixMilli()
	for _, step := range []struct {
		from, to string
		width    int64
	}{
		{"metrics_raw", "metrics_1m", bucket[TierMinute]},
		{"metrics_1m", "metrics_15m", bucket[TierQuarter]},
	} {
		end := nowMS / step.width * step.width // start of the current, incomplete bucket
		start := since / step.width * step.width
		_, err := s.db.ExecContext(ctx, fmt.Sprintf(
			`INSERT OR REPLACE INTO %s (host_id, t, %s)
			 SELECT host_id, t / %d * %d AS b, AVG(cpu), AVG(memory), AVG(swap), AVG(disk), AVG(rx_rate), AVG(tx_rate), AVG(load1)
			 FROM %s WHERE t >= ? AND t < ? GROUP BY host_id, b`,
			step.to, metricColumns, step.width, step.width, step.from), start, end)
		if err != nil {
			return fmt.Errorf("downsample into %s: %w", step.to, err)
		}
	}
	for i, d := range []time.Duration{keep.Raw, keep.Minute, keep.Quarter} {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+historyTables[i]+` WHERE t < ?`, now.Add(-d).UnixMilli()); err != nil {
			return fmt.Errorf("purge %s: %w", historyTables[i], err)
		}
	}
	return nil
}

// History is a Host's charted metrics over time, column by column.
type History struct {
	// T holds bucket (or sample) times in Unix milliseconds.
	T                                              []int64
	CPU, Memory, Swap, Disk, RxRate, TxRate, Load1 []*float64
}

// History returns a Host's samples from tier at or after from (Unix ms), oldest first.
func (s *Store) History(ctx context.Context, hostID string, tier Tier, from int64) (History, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t, `+metricColumns+` FROM `+historyTables[tier]+` WHERE host_id = ? AND t >= ? ORDER BY t`, hostID, from)
	if err != nil {
		return History{}, err
	}
	defer rows.Close()
	var h History
	for rows.Next() {
		var t int64
		var v [7]sql.NullFloat64
		if err := rows.Scan(&t, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6]); err != nil {
			return History{}, err
		}
		h.T = append(h.T, t)
		for i, col := range []*[]*float64{&h.CPU, &h.Memory, &h.Swap, &h.Disk, &h.RxRate, &h.TxRate, &h.Load1} {
			var p *float64
			if v[i].Valid {
				f := v[i].Float64
				p = &f
			}
			*col = append(*col, p)
		}
	}
	return h, rows.Err()
}

// ParseRetention parses a duration that may also use days, e.g. "7d" or "36h".
func ParseRetention(v string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(v, "d"); ok {
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err != nil || n <= 0 || fmt.Sprint(n) != days {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q", v)
	}
	return d, nil
}
