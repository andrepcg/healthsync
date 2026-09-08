package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/BRO3886/healthsync/internal/hk"
)

func TestMigrate_CreatesEveryRegistryTable(t *testing.T) {
	db := tempDB(t)
	tables := map[string]bool{}
	rows, err := db.Conn().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		tables[n] = true
	}
	rows.Close()
	for _, tbl := range hk.Tables() {
		if !tables[tbl] {
			t.Errorf("missing table %s", tbl)
		}
	}
	for _, tbl := range []string{hk.OtherQuantityTable, hk.OtherCategoryTable, "blood_pressure", "workouts", "workout_statistics", "workout_events", "workout_zones", "workout_routes", "workout_route_points", "activity_summary", "hrv_beats", "ecg", "devices", "profile", "imports"} {
		if !tables[tbl] {
			t.Errorf("missing table %s", tbl)
		}
	}
	// Category tables have TEXT value and no unit; quantity tables have both.
	for _, tbl := range []string{"sleep", "high_heart_rate_events", "menstrual_flow"} {
		if hasColumn(t, db, tbl, "unit") {
			t.Errorf("%s should not have a unit column", tbl)
		}
	}
	for _, tbl := range []string{"heart_rate", "dietary_caffeine", hk.OtherQuantityTable} {
		if !hasColumn(t, db, tbl, "unit") || !hasColumn(t, db, tbl, "metadata") {
			t.Errorf("%s should have unit and metadata columns", tbl)
		}
	}
	var ver int
	db.Conn().QueryRow(`PRAGMA user_version`).Scan(&ver)
	if ver != schemaVersion {
		t.Errorf("user_version = %d", ver)
	}
}

func hasColumn(t *testing.T, db *DB, table, col string) bool {
	t.Helper()
	rows, err := db.Conn().Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk)
		if name == col {
			return true
		}
	}
	return false
}

func TestMigrate_UpgradesV1Database(t *testing.T) {
	// A database created by the old hand-written schema lacks the fidelity
	// columns; opening it must add them without losing rows.
	path := filepath.Join(t.TempDir(), "old.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`CREATE TABLE heart_rate (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_name TEXT NOT NULL, start_date TEXT NOT NULL, end_date TEXT NOT NULL,
		value REAL NOT NULL, unit TEXT NOT NULL, created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(source_name, start_date, end_date, value));
		INSERT INTO heart_rate (source_name, start_date, end_date, value, unit) VALUES ('W', '2024-01-01 00:00:00', '2024-01-01 00:00:00', 70, 'count/min');`)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	defer db.Close()
	for _, c := range []string{"source_version", "device_id", "creation_date", "metadata"} {
		if !hasColumn(t, db, "heart_rate", c) {
			t.Errorf("column %s not added", c)
		}
	}
	var n int
	db.Conn().QueryRow(`SELECT COUNT(*) FROM heart_rate`).Scan(&n)
	if n != 1 {
		t.Errorf("rows lost: %d", n)
	}
	// New-shape insert works on the upgraded table.
	if _, err := db.BatchInsertRecords("heart_rate", hk.RecordColumns("heart_rate"), [][]any{{"W", "2024-01-01 00:01:00", "2024-01-01 00:01:00", 71.0, "count/min", "1.0", nil, nil, nil}}); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertWorkout_IdempotentAndChildrenReplaced(t *testing.T) {
	db := tempDB(t)
	row := []any{"HKWorkoutActivityTypeRunning", "W", "2024-01-01 08:00:00", "2024-01-01 08:30:00", 30.0, "min", 5.0, "km", 300.0, "kcal", nil, nil, nil, nil}
	id1, err := db.UpsertWorkout(hk.WorkoutColumns(), row)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := db.UpsertWorkout(hk.WorkoutColumns(), row)
	if err != nil || id1 != id2 {
		t.Fatalf("ids differ: %d %d (%v)", id1, id2, err)
	}
	if err := db.ReplaceWorkoutChildren(id1, []WorkoutStatRow{{Type: "HKQuantityTypeIdentifierHeartRate", Average: 120.0}}, nil, []WorkoutZoneRow{{GroupType: "hr", ZoneIndex: 0}, {GroupType: "hr", ZoneIndex: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceWorkoutChildren(id1, nil, []WorkoutEventRow{{Type: "Pause", Date: "2024-01-01 08:10:00"}}, []WorkoutZoneRow{{GroupType: "hr", ZoneIndex: 0}}); err != nil {
		t.Fatal(err)
	}
	var stats, events, zones int
	db.Conn().QueryRow(`SELECT COUNT(*) FROM workout_statistics`).Scan(&stats)
	db.Conn().QueryRow(`SELECT COUNT(*) FROM workout_events`).Scan(&events)
	db.Conn().QueryRow(`SELECT COUNT(*) FROM workout_zones`).Scan(&zones)
	if stats != 0 || events != 1 || zones != 1 {
		t.Errorf("children not replaced: stats=%d events=%d zones=%d", stats, events, zones)
	}

	items, total, err := db.ListWorkouts(WorkoutFilter{})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list: %v %d", err, total)
	}
	if items[0].DistanceKm == nil || *items[0].DistanceKm != 5 || *items[0].DurationMin != 30 {
		t.Errorf("item: %+v", items[0])
	}
	d, err := db.GetWorkout(id1)
	if err != nil || d == nil || len(d.Events) != 1 {
		t.Fatalf("detail: %v %+v", err, d)
	}
	if missing, _ := db.GetWorkout(999); missing != nil {
		t.Error("expected nil for unknown workout")
	}
}

func TestUpsertDevice(t *testing.T) {
	db := tempDB(t)
	a, _ := db.UpsertDevice("name:Apple Watch")
	b, _ := db.UpsertDevice("name:Apple Watch")
	c, _ := db.UpsertDevice("name:iPhone")
	if a != b || a == c {
		t.Errorf("ids: %d %d %d", a, b, c)
	}
}

func insertRows(t *testing.T, db *DB, table string, rows [][]any) {
	t.Helper()
	cols := hk.RecordColumns(table)
	full := make([][]any, len(rows))
	for i, r := range rows {
		full[i] = append(append([]any{}, r...), nil, nil, nil, nil)
	}
	if _, err := db.BatchInsertRecords(table, cols, full); err != nil {
		t.Fatal(err)
	}
}

func TestQuerySeries_Cumulative_DedupsAndBuckets(t *testing.T) {
	db := tempDB(t)
	insertRows(t, db, "steps", [][]any{
		// Watch and iPhone record the same walk: only the watch counts.
		{"André’s Apple Watch", "2024-01-01 08:00:00", "2024-01-01 08:10:00", 1000.0, "count"},
		{"iPhone", "2024-01-01 08:02:00", "2024-01-01 08:08:00", 900.0, "count"},
		{"André’s Apple Watch", "2024-01-02 08:00:00", "2024-01-02 08:10:00", 500.0, "count"},
		{"André’s Apple Watch", "2024-01-08 08:00:00", "2024-01-08 08:10:00", 200.0, "count"}, // next ISO week (Monday)
	})
	m := hk.ByKey["steps"]
	s, err := db.QuerySeries(m, "2024-01-01", "2024-01-31", BucketDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Points) != 3 || s.Points[0].V != 1000 || s.Points[1].V != 500 || s.Unit != "count" {
		t.Errorf("day points: %+v", s.Points)
	}
	w, err := db.QuerySeries(m, "2024-01-01", "2024-01-31", BucketWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Points) != 2 || w.Points[0].T != "2024-01-01" || w.Points[0].V != 1500 || w.Points[1].T != "2024-01-08" {
		t.Errorf("week points: %+v", w.Points)
	}
	mo, _ := db.QuerySeries(m, "", "", BucketMonth)
	if len(mo.Points) != 1 || mo.Points[0].T != "2024-01-01" || mo.Points[0].V != 1700 {
		t.Errorf("month points: %+v", mo.Points)
	}
	// Range filter is inclusive of "to".
	r, _ := db.QuerySeries(m, "2024-01-02", "2024-01-02", BucketDay)
	if len(r.Points) != 1 || r.Points[0].V != 500 {
		t.Errorf("inclusive range: %+v", r.Points)
	}
}

func TestQuerySeries_SampleMinMaxAndEmptyDaysOmitted(t *testing.T) {
	db := tempDB(t)
	insertRows(t, db, "heart_rate", [][]any{
		{"W", "2024-01-01 08:00:00", "2024-01-01 08:00:00", 60.0, "count/min"},
		{"W", "2024-01-01 09:00:00", "2024-01-01 09:00:00", 80.0, "count/min"},
		{"W", "2024-01-03 09:00:00", "2024-01-03 09:00:00", 100.0, "count/min"},
	})
	s, err := db.QuerySeries(hk.ByKey["heart-rate"], "2024-01-01", "2024-01-05", BucketDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Points) != 2 {
		t.Fatalf("days with no data must be omitted, got %+v", s.Points)
	}
	p := s.Points[0]
	if p.V != 70 || *p.Min != 60 || *p.Max != 80 || p.N != 2 {
		t.Errorf("day 1: %+v", p)
	}
	w, _ := db.QuerySeries(hk.ByKey["heart-rate"], "2024-01-01", "2024-01-05", BucketWeek)
	if len(w.Points) != 1 || w.Points[0].V != 80 || *w.Points[0].Max != 100 || w.Points[0].N != 3 {
		t.Errorf("weekly weighted mean: %+v", w.Points)
	}
}

func TestQuerySeries_EventAndDurationAndSleep(t *testing.T) {
	db := tempDB(t)
	insertRows(t, db, "high_heart_rate_events", [][]any{
		{"W", "2024-01-01 08:00:00", "2024-01-01 08:10:00", "HKCategoryValueNotApplicable"},
		{"W", "2024-01-01 09:00:00", "2024-01-01 09:10:00", "HKCategoryValueNotApplicable"},
	})
	insertRows(t, db, "mindful_sessions", [][]any{
		{"W", "2024-01-01 08:00:00", "2024-01-01 08:10:00", "HKCategoryValueNotApplicable"},
	})
	insertRows(t, db, "sleep", [][]any{
		{"W", "2024-01-01 23:00:00", "2024-01-02 03:00:00", "HKCategoryValueSleepAnalysisAsleepCore"},
		{"W", "2024-01-02 03:00:00", "2024-01-02 07:00:00", "HKCategoryValueSleepAnalysisAsleepDeep"},
		{"W", "2024-01-02 07:00:00", "2024-01-02 07:30:00", "HKCategoryValueSleepAnalysisInBed"},
	})
	e, _ := db.QuerySeries(hk.ByKey["high-heart-rate-events"], "", "", BucketDay)
	if len(e.Points) != 1 || e.Points[0].V != 2 {
		t.Errorf("events: %+v", e.Points)
	}
	d, _ := db.QuerySeries(hk.ByKey["mindful-sessions"], "", "", BucketDay)
	if len(d.Points) != 1 || d.Points[0].V < 9.99 || d.Points[0].V > 10.01 || d.Unit != "min" {
		t.Errorf("duration: %+v %s", d.Points, d.Unit)
	}
	s, _ := db.QuerySeries(hk.ByKey["sleep"], "2024-01-01", "2024-01-02", BucketDay)
	if len(s.Points) != 1 || s.Points[0].T != "2024-01-01" || s.Points[0].V != 8 {
		t.Errorf("sleep: %+v", s.Points)
	}
	nights, err := db.SleepNights("2024-01-01", "2024-01-02")
	if err != nil || len(nights) != 1 {
		t.Fatalf("nights: %v %+v", err, nights)
	}
	n := nights[0]
	if n.Stages.Core != 4 || n.Stages.Deep != 4 || n.Stages.InBed != 0.5 || n.Onset != "2024-01-01 23:00" || n.Wake != "2024-01-02 07:00" {
		t.Errorf("stages: %+v", n)
	}
}

func TestSummary_TilesAndDeltas(t *testing.T) {
	db := tempDB(t)
	insertRows(t, db, "steps", [][]any{
		{"W", "2024-01-01 08:00:00", "2024-01-01 08:10:00", 1000.0, "count"},
		{"W", "2024-01-02 08:00:00", "2024-01-02 08:10:00", 3000.0, "count"},
		{"W", "2024-01-08 08:00:00", "2024-01-08 08:10:00", 4000.0, "count"},
	})
	insertRows(t, db, "resting_heart_rate", [][]any{
		{"W", "2024-01-02 08:00:00", "2024-01-02 08:10:00", 60.0, "count/min"},
		{"W", "2024-01-08 08:00:00", "2024-01-08 08:10:00", 54.0, "count/min"},
	})
	insertRows(t, db, "body_mass", [][]any{
		{"S", "2024-01-08 07:00:00", "2024-01-08 07:00:00", 80.0, "kg"},
	})
	p := Period{From: "2024-01-08", To: "2024-01-14"}
	if p.Days() != 7 || p.Previous().From != "2024-01-01" || p.Previous().To != "2024-01-07" {
		t.Fatalf("period math: %d %+v", p.Days(), p.Previous())
	}
	s, err := db.Summary(p, true)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Tile{}
	for _, tile := range s.Tiles {
		byKey[tile.Metric] = tile
	}
	steps := byKey["steps"]
	if steps.Value != 4000 || *steps.Total != 4000 || steps.Days != 1 || steps.Previous == nil || *steps.Previous != 2000 || steps.Direction != "up" || steps.Good != "up" {
		t.Errorf("steps tile: %+v prev=%v", steps, steps.Previous)
	}
	if *steps.DeltaPct != 100 {
		t.Errorf("delta: %v", *steps.DeltaPct)
	}
	rhr := byKey["resting-heart-rate"]
	if rhr.Value != 54 || *rhr.Previous != 60 || rhr.Direction != "down" || rhr.Good != "down" {
		t.Errorf("rhr tile: %+v", rhr)
	}
	bm := byKey["body-mass"]
	if bm.Kind != "latest" || bm.Value != 80 || bm.Previous != nil {
		t.Errorf("body mass tile: %+v", bm)
	}
	if _, ok := byKey["sleep"]; ok {
		t.Error("metrics with no data must not produce tiles")
	}
}

func TestActivityRings_Streaks(t *testing.T) {
	db := tempDB(t)
	cols := []string{"date", "active_energy", "active_energy_goal", "active_energy_unit", "move_time", "move_time_goal", "exercise_time", "exercise_time_goal", "stand_hours", "stand_hours_goal"}
	rows := [][]any{
		{"2024-01-01", 500.0, 400.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0},
		{"2024-01-02", 500.0, 400.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0},
		{"2024-01-03", 100.0, 400.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0},
		// gap on the 4th
		{"2024-01-05", 500.0, 400.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0},
		{"2024-01-06", 500.0, 0.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0}, // zero goal is not a closure
	}
	if _, err := db.BatchReplaceRecords("activity_summary", cols, rows); err != nil {
		t.Fatal(err)
	}
	r, err := db.ActivityRings("2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Days) != 5 || !r.Days[0].Closed.All || r.Days[2].Closed.All || r.Days[4].Closed.Energy {
		t.Errorf("closure flags wrong: %+v", r.Days)
	}
	if r.Streaks.LongestAll != 2 || r.Streaks.CurrentAll != 0 || r.Streaks.ClosedAll != 3 {
		t.Errorf("streaks: %+v", r.Streaks)
	}
}

func TestTableRows_AndAvailability(t *testing.T) {
	db := tempDB(t)
	insertRows(t, db, "heart_rate", [][]any{
		{"W", "2024-01-01 08:00:00", "2024-01-01 08:00:00", 60.0, "count/min"},
		{"W", "2024-01-02 08:00:00", "2024-01-02 08:00:00", 61.0, "count/min"},
	})
	if _, err := db.BatchInsertRecords(hk.OtherQuantityTable, hk.OtherColumns(hk.OtherQuantityTable), [][]any{{"HKQuantityTypeIdentifierFuture", "W", "2024-01-03 08:00:00", "2024-01-03 08:00:00", 1.0, "x", nil, nil, nil, nil}}); err != nil {
		t.Fatal(err)
	}
	cols, rows, total, err := db.TableRows(TableQuery{Table: "heart_rate", Limit: 1, Desc: true})
	if err != nil || total != 2 || len(rows) != 1 || rows[0]["value"].(float64) != 61 {
		t.Errorf("table rows: %v total=%d rows=%v", err, total, rows)
	}
	if len(cols) == 0 || cols[0] != "id" {
		t.Errorf("columns: %v", cols)
	}
	if _, _, _, err := db.TableRows(TableQuery{Table: "nope"}); err == nil {
		t.Error("unknown table must error")
	}
	if _, ok := ResolveTable("heart-rate"); !ok {
		t.Error("hyphen name must resolve")
	}
	if tbl, ok := ResolveTable("activity_summary"); !ok || tbl != "activity_summary" {
		t.Error("extra tables must resolve")
	}
	av, err := db.Availability()
	if err != nil {
		t.Fatal(err)
	}
	if av.FirstDate != "2024-01-01" || av.LastDate != "2024-01-03" || av.TotalRows != 3 || len(av.Tables) != 1 || len(av.OtherTypes) != 1 {
		t.Errorf("availability: %+v", av)
	}
	if av.OtherTypes[0].Type != "HKQuantityTypeIdentifierFuture" {
		t.Errorf("other types: %+v", av.OtherTypes)
	}
	n := 0
	err = db.StreamTableCSV(TableQuery{Table: "heart_rate"}, func(cols []string, row []any) error { n++; return nil })
	if err != nil || n != 2 {
		t.Errorf("csv stream: %v %d", err, n)
	}
}

func TestImportsAndECG(t *testing.T) {
	db := tempDB(t)
	id, err := db.BeginImport("export.zip", 123, "2024-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishImport(id, ImportRow{FinishedAt: "2024-01-01T00:01:00Z", Status: "completed", Records: 5, ExportDate: "2024-01-01 00:00:00"}); err != nil {
		t.Fatal(err)
	}
	imports, _ := db.Imports()
	if len(imports) != 1 || imports[0].Status != "completed" || imports[0].Records != 5 || imports[0].Filename != "export.zip" {
		t.Errorf("imports: %+v", imports)
	}
	ins, err := db.InsertECG(ECGRow{RecordedDate: "2024-01-01 10:00:00", Classification: "Sinus Rhythm", SampleRateHz: 512, SampleCount: 2, Samples: []byte{0, 0, 0, 0, 0, 0, 0, 0}})
	if err != nil || !ins {
		t.Fatalf("insert ecg: %v %v", err, ins)
	}
	ins, _ = db.InsertECG(ECGRow{RecordedDate: "2024-01-01 10:00:00", SampleCount: 2, Samples: []byte{0, 0, 0, 0, 0, 0, 0, 0}})
	if ins {
		t.Error("duplicate recorded_date must be skipped")
	}
	list, _ := db.ListECG("", "")
	if len(list) != 1 || list[0].Samples != nil {
		t.Errorf("list: %+v", list)
	}
	e, _ := db.GetECG(list[0].ID)
	if e == nil || len(e.Samples) != 8 {
		t.Errorf("get: %+v", e)
	}
}

func TestHighlights_Basic(t *testing.T) {
	db := tempDB(t)
	var rows [][]any
	for d := 1; d <= 10; d++ {
		rows = append(rows, []any{"W", "2024-01-" + pad(d) + " 08:00:00", "2024-01-" + pad(d) + " 08:10:00", float64(d * 1000), "count"})
	}
	insertRows(t, db, "steps", rows)
	var rhr [][]any
	for d := 1; d <= 10; d++ {
		rhr = append(rhr, []any{"W", "2024-01-" + pad(d) + " 08:00:00", "2024-01-" + pad(d) + " 08:00:00", float64(70 - d), "count/min"})
	}
	insertRows(t, db, "resting_heart_rate", rhr)
	hl, err := db.Highlights(Period{From: "2024-01-01", To: "2024-01-10"})
	if err != nil {
		t.Fatal(err)
	}
	var best, trend bool
	for _, h := range hl {
		if h.Kind == "best-day" && h.Date == "2024-01-10" && *h.Value == 10000 {
			best = true
		}
		if h.Kind == "trend" && h.Tone == "good" && *h.Value < -8 {
			trend = true
		}
	}
	if !best || !trend {
		t.Errorf("highlights missing: %+v", hl)
	}
}

func pad(d int) string {
	if d < 10 {
		return "0" + string(rune('0'+d))
	}
	return string(rune('0'+d/10)) + string(rune('0'+d%10))
}
