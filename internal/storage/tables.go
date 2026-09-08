package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BRO3886/healthsync/internal/hk"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// TableAvailability describes one populated table.
type TableAvailability struct {
	Table     string `json:"table"`
	MetricKey string `json:"metric_key,omitempty"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Unit      string `json:"unit,omitempty"`
	Rows      int64  `json:"rows"`
	First     string `json:"first,omitempty"`
	Last      string `json:"last,omitempty"`
}

// OtherType is an HK identifier stored in the generic fallback tables.
type OtherType struct {
	Type  string `json:"hk_type"`
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
	First string `json:"first,omitempty"`
	Last  string `json:"last,omitempty"`
}

// Availability is what the UI uses to hide sections that have no data.
type Availability struct {
	FirstDate    string              `json:"first_date,omitempty"`
	LastDate     string              `json:"last_date,omitempty"`
	TotalRows    int64               `json:"total_rows"`
	Tables       []TableAvailability `json:"tables"`
	OtherTypes   []OtherType         `json:"other_types"`
	Workouts     int64               `json:"workouts"`
	Routes       int64               `json:"routes"`
	ECGs         int64               `json:"ecgs"`
	ActivityDays int64               `json:"activity_days"`
}

// Availability scans every table for row counts and date bounds.
func (db *DB) Availability() (*Availability, error) {
	out := &Availability{Tables: []TableAvailability{}, OtherTypes: []OtherType{}}

	scan := func(table, dateCol string) (int64, string, string, error) {
		var n int64
		var first, last sql.NullString
		err := db.conn.QueryRow(fmt.Sprintf(`SELECT COUNT(*), MIN(%s), MAX(%s) FROM %s`, dateCol, dateCol, table)).Scan(&n, &first, &last)
		return n, trimDay(first.String), trimDay(last.String), err
	}
	consider := func(first, last string) {
		if first != "" && (out.FirstDate == "" || first < out.FirstDate) {
			out.FirstDate = first
		}
		if last != "" && last > out.LastDate {
			out.LastDate = last
		}
	}

	for _, m := range hk.Metrics {
		if m.Paired {
			continue
		}
		n, first, last, err := scan(m.Table, "start_date")
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		out.TotalRows += n
		consider(first, last)
		out.Tables = append(out.Tables, TableAvailability{Table: m.Table, MetricKey: m.Key, Name: m.Name, Group: m.Group, Kind: m.Agg.String(), Unit: m.Unit, Rows: n, First: first, Last: last})
	}
	if n, first, last, err := scan(hk.BloodPressureTable, "start_date"); err == nil && n > 0 {
		out.TotalRows += n
		consider(first, last)
		out.Tables = append(out.Tables, TableAvailability{Table: hk.BloodPressureTable, MetricKey: "blood-pressure", Name: "Blood Pressure", Group: hk.GroupHeart, Kind: "paired", Unit: "mmHg", Rows: n, First: first, Last: last})
	}
	if n, first, last, err := scan("workouts", "start_date"); err == nil && n > 0 {
		out.Workouts = n
		consider(first, last)
		out.Tables = append(out.Tables, TableAvailability{Table: "workouts", MetricKey: "workouts", Name: "Workouts", Group: hk.GroupActivity, Kind: "workouts", Rows: n, First: first, Last: last})
	}
	if n, first, last, err := scan("activity_summary", "date"); err == nil && n > 0 {
		out.ActivityDays = n
		consider(first, last)
		out.Tables = append(out.Tables, TableAvailability{Table: "activity_summary", Name: "Activity Rings", Group: hk.GroupActivity, Kind: "rings", Rows: n, First: first, Last: last})
	}
	if n, first, last, err := scan("ecg", "recorded_date"); err == nil && n > 0 {
		out.ECGs = n
		consider(first, last)
		out.Tables = append(out.Tables, TableAvailability{Table: "ecg", Name: "Electrocardiograms", Group: hk.GroupHeart, Kind: "ecg", Rows: n, First: first, Last: last})
	}
	if n, _, _, err := scan("workout_routes", "start_date"); err == nil {
		out.Routes = n
	}

	for _, t := range []string{hk.OtherQuantityTable, hk.OtherCategoryTable} {
		rows, err := db.conn.Query(fmt.Sprintf(`SELECT type, COUNT(*), MIN(start_date), MAX(start_date) FROM %s GROUP BY type ORDER BY type`, t))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var o OtherType
			var first, last sql.NullString
			if err := rows.Scan(&o.Type, &o.Rows, &first, &last); err != nil {
				rows.Close()
				return nil, err
			}
			o.Table = t
			o.First, o.Last = trimDay(first.String), trimDay(last.String)
			out.TotalRows += o.Rows
			consider(o.First, o.Last)
			out.OtherTypes = append(out.OtherTypes, o)
		}
		rows.Close()
	}
	return out, nil
}

func trimDay(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// TableQuery selects rows from any table for the Explore page.
type TableQuery struct {
	Table  string // resolved real table name
	Type   string // for other_* tables
	From   string
	To     string
	Limit  int
	Offset int
	Desc   bool
}

// dateColumn returns the column used for range filtering and ordering.
func dateColumn(table string) string {
	switch table {
	case "activity_summary":
		return "date"
	case "ecg":
		return "recorded_date"
	case "workout_events":
		return "date"
	case "imports":
		return "started_at"
	case "devices", "profile", "workout_statistics", "workout_zones", "hrv_beats":
		return ""
	}
	return "start_date"
}

// TableRows returns a page of rows and the total count. Column names come
// from the table itself so new columns appear without code changes.
func (db *DB) TableRows(q TableQuery) ([]string, []map[string]any, int64, error) {
	if _, ok := ResolveTable(q.Table); !ok {
		return nil, nil, 0, fmt.Errorf("unknown table %q", q.Table)
	}
	var args []any
	where := " WHERE 1=1"
	if q.Type != "" {
		where += " AND type = ?"
		args = append(args, q.Type)
	}
	dc := dateColumn(q.Table)
	if dc != "" {
		where += rangeClause(dc, q.From, q.To, &args)
	}

	var total int64
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM "+q.Table+where, args...).Scan(&total); err != nil {
		return nil, nil, 0, err
	}

	sqlq := "SELECT * FROM " + q.Table + where
	if dc != "" {
		if q.Desc {
			sqlq += " ORDER BY " + dc + " DESC"
		} else {
			sqlq += " ORDER BY " + dc + " ASC"
		}
	}
	if q.Limit > 0 {
		sqlq += fmt.Sprintf(" LIMIT %d OFFSET %d", q.Limit, q.Offset)
	}
	rows, err := db.conn.Query(sqlq, args...)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	// Never ship the raw ECG waveform through the generic table view.
	if q.Table == "ecg" {
		filtered := make([]string, 0, len(cols))
		for _, c := range cols {
			if c != "samples" {
				filtered = append(filtered, c)
			}
		}
		cols = filtered
	}
	out, err := scanRows(rows)
	if err != nil {
		return nil, nil, 0, err
	}
	if q.Table == "ecg" {
		for _, r := range out {
			delete(r, "samples")
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	return cols, out, total, nil
}

// StreamTableCSV writes every matching row as CSV via the callback, in
// chunks, so an unbounded export never materialises in memory.
func (db *DB) StreamTableCSV(q TableQuery, write func(cols []string, row []any) error) error {
	if _, ok := ResolveTable(q.Table); !ok {
		return fmt.Errorf("unknown table %q", q.Table)
	}
	var args []any
	where := " WHERE 1=1"
	if q.Type != "" {
		where += " AND type = ?"
		args = append(args, q.Type)
	}
	dc := dateColumn(q.Table)
	if dc != "" {
		where += rangeClause(dc, q.From, q.To, &args)
	}
	sqlq := "SELECT * FROM " + q.Table + where
	if dc != "" {
		sqlq += " ORDER BY " + dc
	}
	rows, err := db.conn.Query(sqlq, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	skip := -1
	if q.Table == "ecg" {
		for i, c := range cols {
			if c == "samples" {
				skip = i
			}
		}
	}
	outCols := make([]string, 0, len(cols))
	for i, c := range cols {
		if i != skip {
			outCols = append(outCols, c)
		}
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		row := make([]any, 0, len(outCols))
		for i, v := range vals {
			if i == skip {
				continue
			}
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row = append(row, v)
		}
		if err := write(outCols, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// EventRows returns category event rows (heart events, audio exposure events)
// with their metadata decoded, for the dashboards.
func (db *DB) EventRows(table, from, to string) ([]map[string]any, error) {
	var args []any
	q := fmt.Sprintf(`SELECT id, source_name, start_date, end_date, value, metadata FROM %s WHERE 1=1`, table)
	q += rangeClause("start_date", from, to, &args)
	q += " ORDER BY start_date DESC"
	rows, err := db.rowsAsMaps(q, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if s, ok := r["metadata"].(string); ok {
			r["metadata"] = parseMetadata(s)
		}
		if v, ok := r["value"].(string); ok {
			r["value"] = strings.TrimPrefix(v, "HKCategoryValue")
		}
	}
	return rows, nil
}

// BloodPressureRows returns readings in range, newest first.
func (db *DB) BloodPressureRows(from, to string) ([]map[string]any, error) {
	var args []any
	q := `SELECT id, source_name, start_date, systolic, diastolic, unit, metadata FROM blood_pressure WHERE 1=1`
	q += rangeClause("start_date", from, to, &args)
	q += " ORDER BY start_date DESC"
	return db.rowsAsMaps(q, args...)
}

// HRVBeats returns the beat-to-beat samples behind one HRV reading.
func (db *DB) HRVBeats(hrvID int64) ([]map[string]any, error) {
	var src, start string
	if err := db.conn.QueryRow(`SELECT source_name, start_date FROM hrv WHERE id = ?`, hrvID).Scan(&src, &start); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return db.rowsAsMaps(`SELECT seq, time, bpm FROM hrv_beats WHERE source_name = ? AND start_date = ? ORDER BY seq`, src, start)
}

// HRVReadings lists HRV readings in range with their beat counts.
func (db *DB) HRVReadings(from, to string) ([]map[string]any, error) {
	var args []any
	q := `SELECT h.id, h.start_date, h.value, h.unit, h.source_name,
	        (SELECT COUNT(*) FROM hrv_beats b WHERE b.source_name = h.source_name AND b.start_date = h.start_date) AS beats
	      FROM hrv h WHERE 1=1`
	q += rangeClause("h.start_date", from, to, &args)
	q += " ORDER BY h.start_date DESC"
	return db.rowsAsMaps(q, args...)
}
