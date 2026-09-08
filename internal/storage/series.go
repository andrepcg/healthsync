package storage

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
)

// Bucket is the aggregation granularity of a series.
type Bucket string

const (
	BucketDay   Bucket = "day"
	BucketWeek  Bucket = "week"
	BucketMonth Bucket = "month"
)

// ParseBucket validates a bucket string, defaulting to day.
func ParseBucket(s string) (Bucket, error) {
	switch Bucket(s) {
	case "", BucketDay:
		return BucketDay, nil
	case BucketWeek, BucketMonth:
		return Bucket(s), nil
	}
	return "", fmt.Errorf("invalid bucket %q (day|week|month)", s)
}

// SeriesPoint is one bucket. V is the bucket's headline value (sum for
// cumulative metrics, mean for samples, hours for durations, count for
// events). Min/Max are present for sample metrics. N is the number of
// underlying rows (or nights, for sleep).
type SeriesPoint struct {
	T   string   `json:"t"`
	V   float64  `json:"v"`
	N   int64    `json:"n"`
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// Series is the result of QuerySeries.
type Series struct {
	Metric string        `json:"metric"`
	Name   string        `json:"name"`
	Unit   string        `json:"unit"`
	Bucket Bucket        `json:"bucket"`
	Agg    string        `json:"agg"`
	Points []SeriesPoint `json:"points"`
}

const dateLayout = "2006-01-02"

// dateRange turns inclusive YYYY-MM-DD bounds into the [from, toExclusive)
// pair used in SQL. Empty bounds are left open.
func dateRange(from, to string) (string, string) {
	lo := from
	hi := ""
	if to != "" {
		if t, err := time.Parse(dateLayout, to[:min(len(to), 10)]); err == nil {
			hi = t.AddDate(0, 0, 1).Format(dateLayout)
		}
	}
	return lo, hi
}

func rangeClause(col, from, to string, args *[]any) string {
	lo, hi := dateRange(from, to)
	q := ""
	if lo != "" {
		q += " AND " + col + " >= ?"
		*args = append(*args, lo)
	}
	if hi != "" {
		q += " AND " + col + " < ?"
		*args = append(*args, hi)
	}
	return q
}

// bucketKey maps a YYYY-MM-DD day to its bucket label: the day itself, the
// Monday of its ISO week, or the first of its month.
func bucketKey(day string, b Bucket) string {
	switch b {
	case BucketWeek:
		t, err := time.Parse(dateLayout, day)
		if err != nil {
			return day
		}
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return t.AddDate(0, 0, -(wd - 1)).Format(dateLayout)
	case BucketMonth:
		if len(day) >= 7 {
			return day[:7] + "-01"
		}
	}
	return day
}

// QuerySeries aggregates a metric over [from, to] into buckets.
func (db *DB) QuerySeries(m *hk.Metric, from, to string, bucket Bucket) (*Series, error) {
	if m == nil {
		return nil, fmt.Errorf("nil metric")
	}
	if m.Paired {
		return nil, fmt.Errorf("blood pressure is not a series metric; use the blood_pressure table")
	}
	s := &Series{Metric: m.Key, Name: m.Name, Unit: m.Unit, Bucket: bucket, Agg: m.Agg.String()}
	var err error
	switch m.Agg {
	case hk.Cumulative:
		s.Points, s.Unit, err = db.cumulativeSeries(m.Table, "", from, to, bucket)
	case hk.Sample:
		s.Points, s.Unit, err = db.sampleSeries(m.Table, "", from, to, bucket)
	case hk.Duration:
		if m.Table == "sleep" {
			s.Points, err = db.sleepSeries(from, to, bucket)
			s.Unit = "hr"
		} else {
			s.Points, err = db.durationSeries(m.Table, from, to, bucket)
			s.Unit = "min"
		}
	case hk.Event:
		s.Points, err = db.eventSeries(m.Table, from, to, bucket)
		s.Unit = "count"
	}
	if err != nil {
		return nil, err
	}
	if s.Unit == "" {
		s.Unit = m.Unit
	}
	normalizePercent(s)
	return s, nil
}

// normalizePercent rescales percentage metrics that Apple stores as fractions
// (SpO2 0.97 with unit "%", body fat 0.21) to 0–100 so every consumer sees
// the same scale. Only applied when every value is <= 1.
func normalizePercent(s *Series) {
	if s.Unit != "%" || len(s.Points) == 0 {
		return
	}
	for _, p := range s.Points {
		if p.V > 1 || (p.Max != nil && *p.Max > 1) {
			return
		}
	}
	for i := range s.Points {
		p := &s.Points[i]
		p.V *= 100
		if p.Min != nil {
			v := *p.Min * 100
			p.Min = &v
		}
		if p.Max != nil {
			v := *p.Max * 100
			p.Max = &v
		}
	}
}

// percentScale returns 100 when a "%" metric's values are stored as fractions.
func percentScale(unit string, sample float64) float64 {
	if unit == "%" && sample <= 1 {
		return 100
	}
	return 1
}

// QueryOtherSeries aggregates a type from the generic fallback tables.
func (db *DB) QueryOtherSeries(hkType, from, to string, bucket Bucket) (*Series, error) {
	s := &Series{Metric: "other:" + hkType, Name: hkType, Bucket: bucket}
	var n int64
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM `+hk.OtherQuantityTable+` WHERE type = ?`, hkType).Scan(&n); err != nil {
		return nil, err
	}
	var err error
	if n > 0 {
		s.Agg = hk.Sample.String()
		s.Points, s.Unit, err = db.sampleSeries(hk.OtherQuantityTable, hkType, from, to, bucket)
	} else {
		s.Agg = hk.Event.String()
		s.Unit = "count"
		s.Points, err = db.eventSeriesTyped(hk.OtherCategoryTable, hkType, from, to, bucket)
	}
	return s, err
}

// cumulativeSeries loads every row in range, removes overlapping rows from
// lower-priority sources, and sums per bucket.
func (db *DB) cumulativeSeries(table, typ, from, to string, bucket Bucket) ([]SeriesPoint, string, error) {
	q := fmt.Sprintf("SELECT source_name, start_date, end_date, value, unit FROM %s WHERE 1=1", table)
	var args []any
	if typ != "" {
		q += " AND type = ?"
		args = append(args, typ)
	}
	q += rangeClause("start_date", from, to, &args)
	q += " ORDER BY start_date ASC"

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("querying %s: %w", table, err)
	}
	defer rows.Close()

	var records []stepsRecord
	unit := ""
	for rows.Next() {
		var r stepsRecord
		if err := rows.Scan(&r.source, &r.startDate, &r.endDate, &r.value, &r.unit); err != nil {
			return nil, "", err
		}
		if unit == "" {
			unit = r.unit
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	totals := map[string]*SeriesPoint{}
	for _, r := range dedupOverlaps(records) {
		if len(r.startDate) < 10 {
			continue
		}
		k := bucketKey(r.startDate[:10], bucket)
		p, ok := totals[k]
		if !ok {
			p = &SeriesPoint{T: k}
			totals[k] = p
		}
		p.V += r.value
		p.N++
	}
	return sortPoints(totals), unit, nil
}

// sampleSeries aggregates per day in SQL, then re-buckets in Go so weekly and
// monthly means are weighted by sample count.
func (db *DB) sampleSeries(table, typ, from, to string, bucket Bucket) ([]SeriesPoint, string, error) {
	q := fmt.Sprintf(`SELECT substr(start_date, 1, 10) AS d, AVG(value), MIN(value), MAX(value), COUNT(*), MIN(unit) FROM %s WHERE 1=1`, table)
	var args []any
	if typ != "" {
		q += " AND type = ?"
		args = append(args, typ)
	}
	q += rangeClause("start_date", from, to, &args)
	q += " GROUP BY d ORDER BY d"

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("querying %s: %w", table, err)
	}
	defer rows.Close()

	type acc struct {
		sum, min, max float64
		n             int64
	}
	buckets := map[string]*acc{}
	unit := ""
	for rows.Next() {
		var d, u string
		var avg, mn, mx float64
		var n int64
		if err := rows.Scan(&d, &avg, &mn, &mx, &n, &u); err != nil {
			return nil, "", err
		}
		if unit == "" {
			unit = u
		}
		k := bucketKey(d, bucket)
		a, ok := buckets[k]
		if !ok {
			a = &acc{min: mn, max: mx}
			buckets[k] = a
		}
		a.sum += avg * float64(n)
		a.n += n
		if mn < a.min {
			a.min = mn
		}
		if mx > a.max {
			a.max = mx
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	pts := make(map[string]*SeriesPoint, len(buckets))
	for k, a := range buckets {
		mn, mx := a.min, a.max
		pts[k] = &SeriesPoint{T: k, V: a.sum / float64(a.n), N: a.n, Min: &mn, Max: &mx}
	}
	return sortPoints(pts), unit, nil
}

// durationSeries sums interval lengths (minutes) per bucket of the start day.
func (db *DB) durationSeries(table, from, to string, bucket Bucket) ([]SeriesPoint, error) {
	q := fmt.Sprintf(`SELECT substr(start_date,1,10), SUM((julianday(end_date) - julianday(start_date)) * 1440.0), COUNT(*) FROM %s WHERE 1=1`, table)
	var args []any
	q += rangeClause("start_date", from, to, &args)
	q += " GROUP BY 1 ORDER BY 1"
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", table, err)
	}
	defer rows.Close()
	pts := map[string]*SeriesPoint{}
	for rows.Next() {
		var d string
		var minutes float64
		var n int64
		if err := rows.Scan(&d, &minutes, &n); err != nil {
			return nil, err
		}
		k := bucketKey(d, bucket)
		p, ok := pts[k]
		if !ok {
			p = &SeriesPoint{T: k}
			pts[k] = p
		}
		p.V += minutes
		p.N += n
	}
	return sortPoints(pts), rows.Err()
}

// eventSeries counts rows per bucket.
func (db *DB) eventSeries(table, from, to string, bucket Bucket) ([]SeriesPoint, error) {
	return db.eventSeriesTyped(table, "", from, to, bucket)
}

func (db *DB) eventSeriesTyped(table, typ, from, to string, bucket Bucket) ([]SeriesPoint, error) {
	q := fmt.Sprintf(`SELECT substr(start_date,1,10), COUNT(*) FROM %s WHERE 1=1`, table)
	var args []any
	if typ != "" {
		q += " AND type = ?"
		args = append(args, typ)
	}
	q += rangeClause("start_date", from, to, &args)
	q += " GROUP BY 1 ORDER BY 1"
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", table, err)
	}
	defer rows.Close()
	pts := map[string]*SeriesPoint{}
	for rows.Next() {
		var d string
		var n int64
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		k := bucketKey(d, bucket)
		p, ok := pts[k]
		if !ok {
			p = &SeriesPoint{T: k}
			pts[k] = p
		}
		p.V += float64(n)
		p.N += n
	}
	return sortPoints(pts), rows.Err()
}

// sleepSeries returns hours of night sleep per night (day bucket) or the mean
// per night present (week/month). Nights without data are never zero-filled.
func (db *DB) sleepSeries(from, to string, bucket Bucket) ([]SeriesPoint, error) {
	nights, err := db.sleepNightTotals(QueryParams{From: from, To: to})
	if err != nil {
		return nil, err
	}
	type acc struct {
		sum float64
		n   int64
	}
	buckets := map[string]*acc{}
	for _, t := range nights {
		if t.Onset.IsZero() {
			continue // nap-only night: no night sleep recorded
		}
		k := bucketKey(t.Night, bucket)
		a, ok := buckets[k]
		if !ok {
			a = &acc{}
			buckets[k] = a
		}
		a.sum += t.Hours
		a.n++
	}
	pts := map[string]*SeriesPoint{}
	for k, a := range buckets {
		pts[k] = &SeriesPoint{T: k, V: a.sum / float64(a.n), N: a.n}
	}
	return sortPoints(pts), nil
}

func sortPoints(m map[string]*SeriesPoint) []SeriesPoint {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]SeriesPoint, 0, len(keys))
	for _, k := range keys {
		out = append(out, *m[k])
	}
	return out
}

// LatestValue returns the most recent row of a sample metric, optionally
// bounded by an inclusive upper date.
func (db *DB) LatestValue(table, to string) (*SeriesPoint, string, error) {
	q := fmt.Sprintf(`SELECT start_date, value, unit FROM %s WHERE 1=1`, table)
	var args []any
	q += rangeClause("start_date", "", to, &args)
	q += " ORDER BY start_date DESC LIMIT 1"
	var d, u string
	var v float64
	err := db.conn.QueryRow(q, args...).Scan(&d, &v, &u)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, "", nil
		}
		return nil, "", err
	}
	return &SeriesPoint{T: d[:min(len(d), 10)], V: v, N: 1}, u, nil
}

// FirstValue returns the earliest row of a sample metric at or after from.
func (db *DB) FirstValue(table, from string) (*SeriesPoint, error) {
	q := fmt.Sprintf(`SELECT start_date, value FROM %s WHERE 1=1`, table)
	var args []any
	q += rangeClause("start_date", from, "", &args)
	q += " ORDER BY start_date ASC LIMIT 1"
	var d string
	var v float64
	err := db.conn.QueryRow(q, args...).Scan(&d, &v)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	return &SeriesPoint{T: d[:min(len(d), 10)], V: v, N: 1}, nil
}
