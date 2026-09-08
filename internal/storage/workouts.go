package storage

import (
	"database/sql"
	"fmt"
)

// WorkoutStatRow is one <WorkoutStatistics> child.
type WorkoutStatRow struct {
	Type      string
	StartDate string
	EndDate   string
	Sum       any
	Average   any
	Minimum   any
	Maximum   any
	Unit      any
}

// WorkoutEventRow is one <WorkoutEvent> child.
type WorkoutEventRow struct {
	Type         string
	Date         string
	Duration     any
	DurationUnit any
	Metadata     any
}

// WorkoutZoneRow is one <WorkoutZone> inside a <WorkoutZoneGroup>.
type WorkoutZoneRow struct {
	GroupType    string
	GroupUnit    any
	ZoneIndex    int
	Minimum      any
	Maximum      any
	Duration     any
	DurationUnit any
}

// RouteRow is a <WorkoutRoute> header; points are inserted separately.
type RouteRow struct {
	WorkoutID      any // int64 or nil for an orphan route
	SourceName     string
	SourceVersion  any
	DeviceID       any
	CreationDate   any
	StartDate      string
	EndDate        string
	FilePath       any
	Metadata       any
	PointCount     int
	DistanceM      any
	ElevationGainM any
}

// RoutePoint is one GPX track point.
type RoutePoint struct {
	Seq    int     `json:"seq"`
	Time   string  `json:"t"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Ele    any     `json:"ele"`
	Speed  any     `json:"speed"`
	Course any     `json:"course"`
	HAcc   any     `json:"hacc"`
	VAcc   any     `json:"vacc"`
}

// UpsertWorkout inserts a workout row (INSERT OR IGNORE) and returns its id,
// whether it was just inserted or already existed. columns must match the
// hk.WorkoutColumns() order and row must start with
// activity_type, source_name, start_date, end_date.
func (db *DB) UpsertWorkout(columns []string, row []any) (int64, error) {
	if len(row) < 4 || len(row) != len(columns) {
		return 0, fmt.Errorf("workout row has %d values for %d columns", len(row), len(columns))
	}
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	q := fmt.Sprintf("INSERT OR IGNORE INTO workouts (%s) VALUES (%s)", join(columns), join(placeholders))
	if _, err := db.conn.Exec(q, row...); err != nil {
		return 0, fmt.Errorf("inserting workout: %w", err)
	}
	var id int64
	err := db.conn.QueryRow(
		`SELECT id FROM workouts WHERE activity_type = ? AND source_name = ? AND start_date = ? AND end_date = ?`,
		row[0], row[1], row[2], row[3],
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("looking up workout id: %w", err)
	}
	return id, nil
}

// ReplaceWorkoutChildren replaces the statistics, events and zones of a
// workout in one transaction. Re-importing the same export is therefore
// idempotent: children are never duplicated.
func (db *DB) ReplaceWorkoutChildren(workoutID int64, stats []WorkoutStatRow, events []WorkoutEventRow, zones []WorkoutZoneRow) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range []string{"workout_statistics", "workout_events", "workout_zones"} {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE workout_id = ?", t), workoutID); err != nil {
			return fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	for _, s := range stats {
		if _, err := tx.Exec(`INSERT INTO workout_statistics (workout_id, type, start_date, end_date, sum, average, minimum, maximum, unit) VALUES (?,?,?,?,?,?,?,?,?)`,
			workoutID, s.Type, s.StartDate, s.EndDate, s.Sum, s.Average, s.Minimum, s.Maximum, s.Unit); err != nil {
			return fmt.Errorf("inserting workout statistic: %w", err)
		}
	}
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO workout_events (workout_id, type, date, duration, duration_unit, metadata) VALUES (?,?,?,?,?,?)`,
			workoutID, e.Type, e.Date, e.Duration, e.DurationUnit, e.Metadata); err != nil {
			return fmt.Errorf("inserting workout event: %w", err)
		}
	}
	for _, z := range zones {
		if _, err := tx.Exec(`INSERT INTO workout_zones (workout_id, group_type, group_unit, zone_index, minimum, maximum, duration, duration_unit) VALUES (?,?,?,?,?,?,?,?)`,
			workoutID, z.GroupType, z.GroupUnit, z.ZoneIndex, z.Minimum, z.Maximum, z.Duration, z.DurationUnit); err != nil {
			return fmt.Errorf("inserting workout zone: %w", err)
		}
	}
	return tx.Commit()
}

// ReplaceRoute upserts a route header and replaces all of its points.
// Returns the route id.
func (db *DB) ReplaceRoute(r RouteRow, points []RoutePoint) (int64, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO workout_routes
		(workout_id, source_name, source_version, device_id, creation_date, start_date, end_date, file_path, metadata, point_count, distance_m, elevation_gain_m)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source_name, start_date, end_date) DO UPDATE SET
			workout_id = COALESCE(excluded.workout_id, workout_routes.workout_id),
			source_version = excluded.source_version,
			device_id = excluded.device_id,
			creation_date = excluded.creation_date,
			file_path = excluded.file_path,
			metadata = excluded.metadata,
			point_count = excluded.point_count,
			distance_m = excluded.distance_m,
			elevation_gain_m = excluded.elevation_gain_m`,
		r.WorkoutID, r.SourceName, r.SourceVersion, r.DeviceID, r.CreationDate, r.StartDate, r.EndDate, r.FilePath, r.Metadata, len(points), r.DistanceM, r.ElevationGainM)
	if err != nil {
		return 0, fmt.Errorf("upserting route: %w", err)
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM workout_routes WHERE source_name = ? AND start_date = ? AND end_date = ?`, r.SourceName, r.StartDate, r.EndDate).Scan(&id); err != nil {
		return 0, fmt.Errorf("looking up route id: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM workout_route_points WHERE route_id = ?`, id); err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO workout_route_points (route_id, seq, time, lat, lon, ele, speed, course, h_acc, v_acc) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, p := range points {
		if _, err := stmt.Exec(id, p.Seq, p.Time, p.Lat, p.Lon, p.Ele, p.Speed, p.Course, p.HAcc, p.VAcc); err != nil {
			return 0, fmt.Errorf("inserting route point: %w", err)
		}
	}
	return id, tx.Commit()
}

// ECGRow is one electrocardiogram recording.
type ECGRow struct {
	ID              int64    `json:"id"`
	RecordedDate    string   `json:"recorded_at"`
	Classification  string   `json:"classification"`
	Symptoms        string   `json:"symptoms"`
	SoftwareVersion string   `json:"software_version"`
	Device          string   `json:"device"`
	SampleRateHz    float64  `json:"sample_rate"`
	Lead            string   `json:"lead"`
	Unit            string   `json:"unit"`
	SampleCount     int      `json:"sample_count"`
	AverageHR       *float64 `json:"avg_hr,omitempty"`
	FileName        string   `json:"file_name"`
	Samples         []byte   `json:"-"`
}

// InsertECG stores a recording; a recording with the same recorded_date is
// skipped. Returns true when a row was inserted.
func (db *DB) InsertECG(e ECGRow) (bool, error) {
	res, err := db.conn.Exec(`INSERT OR IGNORE INTO ecg
		(recorded_date, classification, symptoms, software_version, device, sample_rate_hz, lead, unit, sample_count, average_hr, file_name, samples)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.RecordedDate, e.Classification, e.Symptoms, e.SoftwareVersion, e.Device, e.SampleRateHz, e.Lead, e.Unit, e.SampleCount, e.AverageHR, e.FileName, e.Samples)
	if err != nil {
		return false, fmt.Errorf("inserting ecg: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetECGAverageHR stores a derived average heart rate for a recording.
func (db *DB) SetECGAverageHR(id int64, hr float64) error {
	_, err := db.conn.Exec(`UPDATE ecg SET average_hr = ? WHERE id = ?`, hr, id)
	return err
}

// ImportRow is one row of the imports table.
type ImportRow struct {
	ID           int64  `json:"id"`
	Filename     string `json:"filename"`
	SizeBytes    int64  `json:"size_bytes"`
	ExportDate   string `json:"export_date"`
	Locale       string `json:"locale"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Status       string `json:"status"`
	Error        string `json:"error"`
	Records      int64  `json:"records"`
	Workouts     int64  `json:"workouts"`
	Routes       int64  `json:"routes"`
	ECGs         int64  `json:"ecgs"`
	ActivityDays int64  `json:"activity_days"`
	HRVBeats     int64  `json:"hrv_beats"`
	Errors       int64  `json:"errors"`
	TableStats   string `json:"table_stats"`
}

// BeginImport records the start of an import and returns its id.
func (db *DB) BeginImport(filename string, sizeBytes int64, startedAt string) (int64, error) {
	res, err := db.conn.Exec(`INSERT INTO imports (filename, size_bytes, started_at, status) VALUES (?,?,?, 'running')`, filename, sizeBytes, startedAt)
	if err != nil {
		return 0, fmt.Errorf("recording import: %w", err)
	}
	return res.LastInsertId()
}

// FinishImport completes an import row with its outcome.
func (db *DB) FinishImport(id int64, r ImportRow) error {
	_, err := db.conn.Exec(`UPDATE imports SET export_date=?, locale=?, finished_at=?, status=?, error=?, records=?, workouts=?, routes=?, ecgs=?, activity_days=?, hrv_beats=?, errors=?, table_stats=? WHERE id=?`,
		nullIfEmpty(r.ExportDate), nullIfEmpty(r.Locale), r.FinishedAt, r.Status, nullIfEmpty(r.Error), r.Records, r.Workouts, r.Routes, r.ECGs, r.ActivityDays, r.HRVBeats, r.Errors, nullIfEmpty(r.TableStats), id)
	return err
}

// Imports lists import history, newest first.
func (db *DB) Imports() ([]ImportRow, error) {
	rows, err := db.conn.Query(`SELECT id, filename, size_bytes, export_date, locale, started_at, finished_at, status, error, records, workouts, routes, ecgs, activity_days, hrv_beats, errors, table_stats FROM imports ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportRow
	for rows.Next() {
		var r ImportRow
		var filename, exportDate, locale, finishedAt, errStr, tableStats sql.NullString
		var size sql.NullInt64
		if err := rows.Scan(&r.ID, &filename, &size, &exportDate, &locale, &r.StartedAt, &finishedAt, &r.Status, &errStr, &r.Records, &r.Workouts, &r.Routes, &r.ECGs, &r.ActivityDays, &r.HRVBeats, &r.Errors, &tableStats); err != nil {
			return nil, err
		}
		r.Filename, r.SizeBytes, r.ExportDate, r.Locale = filename.String, size.Int64, exportDate.String, locale.String
		r.FinishedAt, r.Error, r.TableStats = finishedAt.String, errStr.String, tableStats.String
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
