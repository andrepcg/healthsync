package insights

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

const asOf = "2026-09-08"

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func dayN(back int) string {
	d, _ := time.Parse(day, asOf)
	return d.AddDate(0, 0, -back).Format(day)
}

// insert adds one row per day for the last n days: value = f(daysBack).
func insert(t *testing.T, db *storage.DB, table string, n int, f func(back int) (float64, bool)) {
	t.Helper()
	cols := hk.RecordColumns(table)
	unit := hk.ByTable[table].Unit
	var rows [][]any
	for b := 0; b < n; b++ {
		v, ok := f(b)
		if !ok {
			continue
		}
		rows = append(rows, []any{"Watch", dayN(b) + " 08:00:00", dayN(b) + " 08:01:00", v, unit, nil, nil, nil, nil})
	}
	if _, err := db.BatchInsertRecords(table, cols, rows); err != nil {
		t.Fatal(err)
	}
}

func find(rep *Report, id string) *Observation {
	for i := range rep.Observations {
		if rep.Observations[i].ID == id {
			return &rep.Observations[i]
		}
	}
	return nil
}

func TestEmptyDatabase(t *testing.T) {
	rep, err := Compute(tempDB(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Observations) != 0 || rep.AsOf != "" {
		t.Errorf("empty db should yield nothing: %+v", rep)
	}
	if _, err := Compute(tempDB(t), "not-a-date"); err == nil {
		t.Error("bad as_of accepted")
	}
}

func TestRestingHeartRateElevated(t *testing.T) {
	db := tempDB(t)
	// 28 baseline days at 50±1, last 3 days at 58.
	insert(t, db, "resting_heart_rate", 31, func(b int) (float64, bool) {
		if b < 3 {
			return 58, true
		}
		return 50 + float64(b%3) - 1, true
	})
	rep, err := Compute(db, asOf)
	if err != nil {
		t.Fatal(err)
	}
	o := find(rep, "resting-heart-rate-above")
	if o == nil {
		t.Fatalf("expected elevated RHR observation, got %+v", rep.Observations)
	}
	if o.Tone != "bad" || o.Severity != SevWarning || o.Days != 31 {
		t.Errorf("observation: %+v", *o)
	}
	if find(rep, "strain") != nil {
		t.Error("one marker must not trigger the strain composite")
	}
}

func TestBaselineNoisyDoesNotFire(t *testing.T) {
	db := tempDB(t)
	// Very noisy baseline (±8) and a recent mean only 4 above: within spread.
	insert(t, db, "resting_heart_rate", 31, func(b int) (float64, bool) {
		if b < 3 {
			return 54, true
		}
		return 50 + float64((b%5)-2)*4, true
	})
	rep, _ := Compute(db, asOf)
	if find(rep, "resting-heart-rate-above") != nil {
		t.Error("deviation inside the day-to-day spread must not fire")
	}
}

func TestStrainComposite(t *testing.T) {
	db := tempDB(t)
	insert(t, db, "resting_heart_rate", 31, func(b int) (float64, bool) {
		if b < 3 {
			return 60, true
		}
		return 50, true
	})
	insert(t, db, "wrist_temperature", 31, func(b int) (float64, bool) {
		if b < 3 {
			return 36.6, true
		}
		return 36.0, true
	})
	insert(t, db, "hrv", 31, func(b int) (float64, bool) {
		if b < 3 {
			return 40, true
		}
		return 70, true
	})
	rep, _ := Compute(db, asOf)
	o := find(rep, "strain")
	if o == nil || o.Severity != SevAlert {
		t.Fatalf("expected strain alert with 3 markers, got %+v", rep.Observations)
	}
	if rep.Observations[0].ID != "strain" {
		t.Errorf("alerts must sort first, got %s", rep.Observations[0].ID)
	}
}

func TestInsufficientDataIsSkipped(t *testing.T) {
	db := tempDB(t)
	insert(t, db, "resting_heart_rate", 5, func(int) (float64, bool) { return 50, true })
	rep, _ := Compute(db, asOf)
	if len(rep.Observations) != 0 {
		t.Errorf("5 days must not produce observations: %+v", rep.Observations)
	}
	found := false
	for _, s := range rep.Skipped {
		if s.Check == "baseline:resting-heart-rate" {
			found = true
		}
	}
	if !found {
		t.Errorf("skip reason missing: %+v", rep.Skipped)
	}
}

func TestTrainingLoadSpike(t *testing.T) {
	db := tempDB(t)
	insert(t, db, "exercise_time", 28, func(b int) (float64, bool) {
		if b < 7 {
			return 90, true
		}
		return 30, true
	})
	rep, _ := Compute(db, asOf)
	o := find(rep, "load-spike")
	if o == nil {
		t.Fatalf("expected load spike, got %+v", rep.Observations)
	}
	// acute 90, chronic (7*90+21*30)/28 = 45 → ratio 2.0 → warning
	if o.Severity != SevWarning {
		t.Errorf("ratio 2.0 should be a warning: %+v", *o)
	}
}

func sleepRows(t *testing.T, db *storage.DB, nights int, hours func(back int) float64, onsetHour func(back int) int) {
	t.Helper()
	cols := hk.RecordColumns("sleep")
	var rows [][]any
	for b := 1; b <= nights; b++ {
		night, _ := time.Parse(day, dayN(b))
		start := night.Add(time.Duration(onsetHour(b)) * time.Hour)
		end := start.Add(time.Duration(hours(b) * float64(time.Hour)))
		rows = append(rows, []any{"Watch", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"), "HKCategoryValueSleepAnalysisAsleepCore", nil, nil, nil, nil})
	}
	if _, err := db.BatchInsertRecords("sleep", cols, rows); err != nil {
		t.Fatal(err)
	}
}

func TestSleepShortNightsAndIrregularBedtime(t *testing.T) {
	db := tempDB(t)
	// 28 nights: last 7 nights 5 h, earlier 7.5 h; bedtime alternates 22:00 / 01:00.
	sleepRows(t, db, 28, func(b int) float64 {
		if b <= 7 {
			return 5
		}
		return 7.5
	}, func(b int) int {
		if b%2 == 0 {
			return 22
		}
		return 25
	})
	rep, _ := Compute(db, asOf)
	if find(rep, "sleep-short-nights") == nil {
		t.Errorf("expected short nights, got %+v", rep.Observations)
	}
	if find(rep, "sleep-irregular") == nil {
		t.Errorf("expected irregular bedtime, got %+v", rep.Observations)
	}
	if find(rep, "sleep-drop") == nil {
		t.Errorf("expected sleep drop vs previous weeks")
	}
}

func TestRedFlagsAndRings(t *testing.T) {
	db := tempDB(t)
	if _, err := db.BatchInsertRecords("irregular_rhythm_events", hk.RecordColumns("irregular_rhythm_events"), [][]any{{"Watch", dayN(2) + " 23:00:00", dayN(2) + " 23:00:00", "HKCategoryValueNotApplicable", nil, nil, nil, nil}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertECG(storage.ECGRow{RecordedDate: dayN(3) + " 21:00:00", Classification: "Atrial Fibrillation", SampleRateHz: 512, SampleCount: 1, Samples: []byte{0, 0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	cols := []string{"date", "active_energy", "active_energy_goal", "active_energy_unit", "move_time", "move_time_goal", "exercise_time", "exercise_time_goal", "stand_hours", "stand_hours_goal"}
	var rows [][]any
	for b := 0; b < 56; b++ {
		closed := b < 28 && b%4 != 0 // this month 75 %, last month 0 %
		e := 100.0
		if closed {
			e = 500
		}
		rows = append(rows, []any{dayN(b), e, 400.0, "kcal", 0.0, 0.0, 40.0, 30.0, 12.0, 12.0})
	}
	if _, err := db.BatchReplaceRecords("activity_summary", cols, rows); err != nil {
		t.Fatal(err)
	}
	rep, _ := Compute(db, asOf)
	if o := find(rep, "heart-notifications"); o == nil || o.Severity != SevAlert {
		t.Errorf("irregular rhythm must be an alert: %+v", o)
	}
	if o := find(rep, "ecg-abnormal"); o == nil || o.Severity != SevAlert {
		t.Errorf("AFib ECG must be an alert: %+v", o)
	}
	if find(rep, "rings-improving") == nil {
		t.Errorf("expected rings improving, got %+v", rep.Observations)
	}
}

func TestStaleExportAndNotWorn(t *testing.T) {
	db := tempDB(t)
	// Only 3 of the last 14 nights recorded, as of 30 days ago.
	old := time.Now().AddDate(0, 0, -30).Format(day)
	cols := hk.RecordColumns("sleep")
	var rows [][]any
	for b := 1; b <= 3; b++ {
		n, _ := time.Parse(day, old)
		s := n.AddDate(0, 0, -b).Add(23 * time.Hour)
		rows = append(rows, []any{"Watch", s.Format("2006-01-02 15:04:05"), s.Add(7 * time.Hour).Format("2006-01-02 15:04:05"), "HKCategoryValueSleepAnalysisAsleepCore", nil, nil, nil, nil})
	}
	db.BatchInsertRecords("sleep", cols, rows)
	rep, err := Compute(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.AsOf == "" {
		t.Fatal("as_of should default to last data day")
	}
	if find(rep, "stale-export") == nil || find(rep, "watch-not-worn") == nil {
		t.Errorf("expected data-quality notes, got %+v", rep.Observations)
	}
}

func TestHelpers(t *testing.T) {
	if median([]float64{3, 1, 2}) != 2 || median([]float64{1, 2, 3, 4}) != 2.5 {
		t.Error("median")
	}
	if m := mad([]float64{1, 1, 1, 1}); m != 0 {
		t.Error("mad of constant")
	}
	if clock(minutesAfter18("2026-01-01 23:30")) != "23:30" || clock(minutesAfter18("2026-01-02 01:15")) != "01:15" {
		t.Error("clock round trip")
	}
	if fmtHours(7.25) != "7 h 15 min" || fmtHours(0.5) != "30 min" {
		t.Error(fmtHours(7.25))
	}
	if s := slopeChange([]float64{1, 2, 3, 4}); s != 3 {
		t.Errorf("slope %v", s)
	}
	_ = fmt.Sprint
}
