package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// WorkoutFilter narrows ListWorkouts.
type WorkoutFilter struct {
	From, To string
	Type     string
	Limit    int
	Offset   int
}

// WorkoutItem is a workout list entry with the most useful statistics joined.
type WorkoutItem struct {
	ID           int64    `json:"id"`
	ActivityType string   `json:"activity_type"`
	SourceName   string   `json:"source"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	DurationMin  *float64 `json:"duration_min"`
	DistanceKm   *float64 `json:"distance_km,omitempty"`
	DistanceUnit string   `json:"distance_unit,omitempty"`
	EnergyKcal   *float64 `json:"energy_kcal,omitempty"`
	AvgHR        *float64 `json:"avg_hr,omitempty"`
	MaxHR        *float64 `json:"max_hr,omitempty"`
	EffortScore  *float64 `json:"effort_score,omitempty"`
	HasRoute     bool     `json:"has_route"`
	Indoor       *bool    `json:"indoor,omitempty"`
}

// WorkoutTypeCount is one activity type with its frequency.
type WorkoutTypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

const workoutSelect = `
SELECT w.id, w.activity_type, w.source_name, w.start_date, w.end_date,
       w.duration, w.duration_unit, w.total_distance, w.total_distance_unit,
       w.total_energy_burned, w.total_energy_burned_unit, w.metadata,
       (SELECT average FROM workout_statistics s WHERE s.workout_id = w.id AND s.type = 'HKQuantityTypeIdentifierHeartRate') AS avg_hr,
       (SELECT maximum FROM workout_statistics s WHERE s.workout_id = w.id AND s.type = 'HKQuantityTypeIdentifierHeartRate') AS max_hr,
       (SELECT value FROM workout_effort_score e WHERE e.start_date >= w.start_date AND e.start_date <= w.end_date ORDER BY e.start_date LIMIT 1) AS effort,
       (SELECT value FROM estimated_workout_effort_score e WHERE e.start_date >= w.start_date AND e.start_date <= w.end_date ORDER BY e.start_date LIMIT 1) AS est_effort,
       EXISTS(SELECT 1 FROM workout_routes r WHERE r.workout_id = w.id AND r.point_count > 0) AS has_route
FROM workouts w WHERE 1=1`

func (db *DB) scanWorkoutItem(rows interface {
	Scan(dest ...any) error
}) (*WorkoutItem, error) {
	var (
		it                                  WorkoutItem
		dur, dist, energy, avgHR, maxHR     sql.NullFloat64
		effort, estEffort                   sql.NullFloat64
		durUnit, distUnit, energyUnit, meta sql.NullString
		hasRoute                            int
	)
	if err := rows.Scan(&it.ID, &it.ActivityType, &it.SourceName, &it.Start, &it.End,
		&dur, &durUnit, &dist, &distUnit, &energy, &energyUnit, &meta,
		&avgHR, &maxHR, &effort, &estEffort, &hasRoute); err != nil {
		return nil, err
	}
	it.DurationMin = toMinutes(dur, durUnit.String)
	if dist.Valid {
		v := dist.Float64
		switch distUnit.String {
		case "m":
			v /= 1000
		case "mi":
			v *= 1.609344
		}
		it.DistanceKm = &v
		it.DistanceUnit = "km"
	}
	it.EnergyKcal = nf(energy)
	it.AvgHR, it.MaxHR = nf(avgHR), nf(maxHR)
	if effort.Valid {
		it.EffortScore = nf(effort)
	} else if estEffort.Valid {
		it.EffortScore = nf(estEffort)
	}
	it.HasRoute = hasRoute == 1
	if strings.Contains(meta.String, `"HKIndoorWorkout":"1"`) {
		t := true
		it.Indoor = &t
	} else if strings.Contains(meta.String, `"HKIndoorWorkout":"0"`) {
		f := false
		it.Indoor = &f
	}
	return &it, nil
}

func toMinutes(v sql.NullFloat64, unit string) *float64 {
	if !v.Valid {
		return nil
	}
	m := v.Float64
	switch unit {
	case "s", "sec":
		m /= 60
	case "hr", "h":
		m *= 60
	}
	return &m
}

// ListWorkouts returns workouts newest first plus the total matching count.
func (db *DB) ListWorkouts(f WorkoutFilter) ([]WorkoutItem, int64, error) {
	var args []any
	where := ""
	if f.Type != "" {
		where += " AND w.activity_type = ?"
		args = append(args, f.Type)
	}
	where += rangeClause("w.start_date", f.From, f.To, &args)

	var total int64
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM workouts w WHERE 1=1"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := workoutSelect + where + " ORDER BY w.start_date DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing workouts: %w", err)
	}
	defer rows.Close()
	items := []WorkoutItem{}
	for rows.Next() {
		it, err := db.scanWorkoutItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *it)
	}
	return items, total, rows.Err()
}

// WorkoutTypes returns activity types with counts, most frequent first.
func (db *DB) WorkoutTypes() ([]WorkoutTypeCount, error) {
	rows, err := db.conn.Query(`SELECT activity_type, COUNT(*) FROM workouts GROUP BY activity_type ORDER BY 2 DESC, 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkoutTypeCount{}
	for rows.Next() {
		var t WorkoutTypeCount
		if err := rows.Scan(&t.Type, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// WorkoutDetail is everything known about one workout.
type WorkoutDetail struct {
	WorkoutItem
	SourceVersion string            `json:"source_version,omitempty"`
	Device        string            `json:"device,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	Statistics    []map[string]any  `json:"statistics"`
	Events        []map[string]any  `json:"events"`
	Zones         []map[string]any  `json:"zones"`
	Route         *map[string]any   `json:"route,omitempty"`
	HRSamples     []SeriesPoint     `json:"hr_samples"`
}

// GetWorkout loads a workout with all children and the heart-rate samples
// recorded inside its window (Apple Watch preferred when several sources
// overlap).
func (db *DB) GetWorkout(id int64) (*WorkoutDetail, error) {
	row := db.conn.QueryRow(workoutSelect+" AND w.id = ?", id)
	it, err := db.scanWorkoutItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d := &WorkoutDetail{WorkoutItem: *it, Metadata: map[string]string{}}

	var sv, meta sql.NullString
	var devID sql.NullInt64
	if err := db.conn.QueryRow(`SELECT source_version, device_id, metadata FROM workouts WHERE id = ?`, id).Scan(&sv, &devID, &meta); err == nil {
		d.SourceVersion = sv.String
		if devID.Valid {
			db.conn.QueryRow(`SELECT description FROM devices WHERE id = ?`, devID.Int64).Scan(&d.Device)
		}
		d.Metadata = parseMetadata(meta.String)
	}

	d.Statistics, err = db.rowsAsMaps(`SELECT type, start_date, end_date, sum, average, minimum, maximum, unit FROM workout_statistics WHERE workout_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	d.Events, err = db.rowsAsMaps(`SELECT type, date, duration, duration_unit, metadata FROM workout_events WHERE workout_id = ? ORDER BY date, id`, id)
	if err != nil {
		return nil, err
	}
	d.Zones, err = db.rowsAsMaps(`SELECT group_type, group_unit, zone_index, minimum, maximum, duration, duration_unit FROM workout_zones WHERE workout_id = ? ORDER BY group_type, zone_index`, id)
	if err != nil {
		return nil, err
	}
	routes, err := db.rowsAsMaps(`SELECT id, start_date, end_date, point_count, distance_m, elevation_gain_m, file_path FROM workout_routes WHERE workout_id = ? AND point_count > 0 ORDER BY start_date LIMIT 1`, id)
	if err != nil {
		return nil, err
	}
	if len(routes) > 0 {
		d.Route = &routes[0]
	}

	d.HRSamples, err = db.heartRateBetween(it.Start, it.End)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// heartRateBetween returns heart-rate rows inside a window, preferring the
// highest-priority source when several sources overlap on the same minute.
func (db *DB) heartRateBetween(start, end string) ([]SeriesPoint, error) {
	rows, err := db.conn.Query(`SELECT source_name, start_date, value FROM heart_rate WHERE start_date >= ? AND start_date <= ? ORDER BY start_date`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type sample struct {
		src string
		t   string
		v   float64
	}
	var all []sample
	for rows.Next() {
		var s sample
		if err := rows.Scan(&s.src, &s.t, &s.v); err != nil {
			return nil, err
		}
		all = append(all, s)
	}
	best := 0
	for _, s := range all {
		if p := sourcePriority(s.src); p > best {
			best = p
		}
	}
	out := []SeriesPoint{}
	for _, s := range all {
		if sourcePriority(s.src) != best {
			continue
		}
		out = append(out, SeriesPoint{T: s.t, V: s.v, N: 1})
	}
	return out, rows.Err()
}

// RoutePoints returns a route's track points in order.
func (db *DB) RoutePoints(routeID int64) ([]RoutePoint, error) {
	rows, err := db.conn.Query(`SELECT seq, time, lat, lon, ele, speed, course, h_acc, v_acc FROM workout_route_points WHERE route_id = ? ORDER BY seq`, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoutePoint{}
	for rows.Next() {
		var p RoutePoint
		var t sql.NullString
		var ele, speed, course, hacc, vacc sql.NullFloat64
		if err := rows.Scan(&p.Seq, &t, &p.Lat, &p.Lon, &ele, &speed, &course, &hacc, &vacc); err != nil {
			return nil, err
		}
		p.Time = t.String
		p.Ele, p.Speed, p.Course, p.HAcc, p.VAcc = anyF(ele), anyF(speed), anyF(course), anyF(hacc), anyF(vacc)
		out = append(out, p)
	}
	return out, rows.Err()
}

// RouteForWorkout returns the id of the workout's route, or 0.
func (db *DB) RouteForWorkout(workoutID int64) (int64, error) {
	var id int64
	err := db.conn.QueryRow(`SELECT id FROM workout_routes WHERE workout_id = ? AND point_count > 0 ORDER BY start_date LIMIT 1`, workoutID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func anyF(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func (db *DB) rowsAsMaps(q string, args ...any) ([]map[string]any, error) {
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanRows(rows)
	if out == nil {
		out = []map[string]any{}
	}
	return out, err
}

// parseMetadata decodes the flat JSON object written by the parser.
func parseMetadata(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	// The object is flat {"k":"v",...}; use encoding/json for correctness.
	var m map[string]string
	if err := jsonUnmarshal([]byte(s), &m); err == nil {
		return m
	}
	return out
}

// --- ECG ---

// ListECG returns recordings newest first, without samples.
func (db *DB) ListECG(from, to string) ([]ECGRow, error) {
	q := `SELECT id, recorded_date, classification, symptoms, software_version, device, sample_rate_hz, lead, unit, sample_count, average_hr, file_name FROM ecg WHERE 1=1`
	var args []any
	q += rangeClause("recorded_date", from, to, &args)
	q += " ORDER BY recorded_date DESC"
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ECGRow{}
	for rows.Next() {
		e, err := scanECG(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetECG loads one recording including its samples.
func (db *DB) GetECG(id int64) (*ECGRow, error) {
	row := db.conn.QueryRow(`SELECT id, recorded_date, classification, symptoms, software_version, device, sample_rate_hz, lead, unit, sample_count, average_hr, file_name, samples FROM ecg WHERE id = ?`, id)
	e, err := scanECG(row, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func scanECG(r interface{ Scan(...any) error }, withSamples bool) (*ECGRow, error) {
	var e ECGRow
	var cls, sym, sw, dev, lead, unit, file sql.NullString
	var rate, avg sql.NullFloat64
	dest := []any{&e.ID, &e.RecordedDate, &cls, &sym, &sw, &dev, &rate, &lead, &unit, &e.SampleCount, &avg, &file}
	if withSamples {
		dest = append(dest, &e.Samples)
	}
	if err := r.Scan(dest...); err != nil {
		return nil, err
	}
	e.Classification, e.Symptoms, e.SoftwareVersion, e.Device = cls.String, sym.String, sw.String, dev.String
	e.Lead, e.Unit, e.FileName = lead.String, unit.String, file.String
	e.SampleRateHz = rate.Float64
	e.AverageHR = nf(avg)
	return &e, nil
}
