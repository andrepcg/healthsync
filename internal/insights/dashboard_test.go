package insights

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

// vitalsAt adds one reading of table at hh:mm after the start of night dayN(b).
func vitalsAt(t *testing.T, db *storage.DB, table string, nights int, offset time.Duration, f func(back int) float64) {
	t.Helper()
	cols := hk.RecordColumns(table)
	unit := hk.ByTable[table].Unit
	var rows [][]any
	for b := 1; b <= nights; b++ {
		night, _ := time.Parse(day, dayN(b))
		ts := night.Add(offset).Format("2006-01-02 15:04:05")
		rows = append(rows, []any{"Watch", ts, ts, f(b), unit, nil, nil, nil, nil})
	}
	if _, err := db.BatchInsertRecords(table, cols, rows); err != nil {
		t.Fatal(err)
	}
}

func TestNormalBand(t *testing.T) {
	db := tempDB(t)
	insert(t, db, "resting_heart_rate", 20, func(b int) (float64, bool) { return float64(50 + b%3), true })
	band, err := Normal(db, hk.ByKey["resting-heart-rate"], asOf)
	if err != nil || band == nil {
		t.Fatalf("band: %v %v", band, err)
	}
	if band.Median != 51 || band.Days != 20 || !(band.Lo < 51 && band.Hi > 51) {
		t.Errorf("band: %+v", band)
	}
	if b, _ := Normal(db, hk.ByKey["steps"], asOf); b != nil {
		t.Error("totals have no normal band")
	}
	db2 := tempDB(t)
	insert(t, db2, "resting_heart_rate", 10, func(int) (float64, bool) { return 50, true })
	if b, _ := Normal(db2, hk.ByKey["resting-heart-rate"], asOf); b != nil {
		t.Error("10 days must not produce a band")
	}
}

func readinessFixture(t *testing.T, lastHRV, lastHR float64) *storage.DB {
	db := tempDB(t)
	sleepRows(t, db, 30, func(int) float64 { return 7 }, func(int) int { return 23 })
	vitalsAt(t, db, "hrv", 30, 25*time.Hour, func(b int) float64 {
		if b == 1 {
			return lastHRV
		}
		return float64(60 + b%5)
	})
	vitalsAt(t, db, "heart_rate", 30, 25*time.Hour, func(b int) float64 {
		if b == 1 {
			return lastHR
		}
		return float64(50 + b%3)
	})
	vitalsAt(t, db, "respiratory_rate", 30, 25*time.Hour, func(b int) float64 { return 14 + float64(b%2)*0.5 })
	return db
}

func TestReadiness(t *testing.T) {
	days, err := ComputeReadiness(readinessFixture(t, 62, 51), asOf, asOf, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Score == nil {
		t.Fatalf("expected one scored morning: %+v", days)
	}
	usual := *days[0].Score
	if usual < 70 || usual > 80 || len(days[0].Components) != 4 {
		t.Errorf("a usual night should score about 75 with 4 components: %+v", days[0])
	}
	var wsum float64
	for _, c := range days[0].Components {
		wsum += c.Weight
	}
	if math.Abs(wsum-1) > 1e-9 {
		t.Errorf("weights must renormalise to 1, got %v", wsum)
	}

	low, _ := ComputeReadiness(readinessFixture(t, 35, 62), asOf, asOf, 7)
	if low[0].Score == nil || *low[0].Score >= 50 || low[0].Band != "low" {
		t.Errorf("suppressed HRV and raised sleeping HR must score low: %+v", low[0])
	}

	// Sleep only: not enough components for a score, and the reasons say why.
	db := tempDB(t)
	sleepRows(t, db, 3, func(int) float64 { return 7 }, func(int) int { return 23 })
	only, _ := ComputeReadiness(db, asOf, asOf, 7)
	if len(only) != 1 || only[0].Score != nil || len(only[0].Missing) != 3 {
		t.Errorf("one of four components must not be scored: %+v", only)
	}
}

func TestSleepNeed(t *testing.T) {
	db := tempDB(t)
	if h, _ := SleepNeed(db, "", asOf); h != 7 {
		t.Errorf("no dob: %v", h)
	}
	if h, src := SleepNeed(db, "2016-01-01", asOf); h != 9 || !strings.Contains(src, "age 10") {
		t.Errorf("child: %v %s", h, src)
	}
}

func TestSleepDebt(t *testing.T) {
	db := tempDB(t)
	// Oldest → newest: 6, 6, 6, 9 h against a 7 h need: 60+60+60, then pay
	// back half of 120 → 120 min owed.
	hours := map[int]float64{4: 6, 3: 6, 2: 6, 1: 9}
	sleepRows(t, db, 4, func(b int) float64 { return hours[b] }, func(int) int { return 23 })
	tm, err := ComputeSleepTiming(db, asOf, 7)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Debt.Minutes != 120 || tm.Debt.Nights != 4 || tm.Debt.Learning {
		t.Errorf("debt: %+v", tm.Debt)
	}

	db2 := tempDB(t)
	sleepRows(t, db2, 3, func(int) float64 { return 2 }, func(int) int { return 23 })
	tm2, _ := ComputeSleepTiming(db2, asOf, 7)
	if tm2.Debt.Minutes != 540 || !tm2.Debt.Learning {
		t.Errorf("each night caps at 3 h and 3 nights is still learning: %+v", tm2.Debt)
	}
}

func TestChronotype(t *testing.T) {
	db := tempDB(t)
	free := func(b int) bool {
		wd := mustDate(dayN(b)).Weekday()
		return wd == time.Friday || wd == time.Saturday
	}
	// Weekdays 23:00 + 7 h (mid 02:30); weekends 01:00 + 9 h (mid 05:30).
	sleepRows(t, db, 28, func(b int) float64 {
		if free(b) {
			return 9
		}
		return 7
	}, func(b int) int {
		if free(b) {
			return 25
		}
		return 23
	})
	tm, err := ComputeSleepTiming(db, asOf, 7)
	if err != nil {
		t.Fatal(err)
	}
	c := tm.Chronotype
	if c == nil {
		t.Fatalf("no chronotype: %s", tm.ChronotypeReason)
	}
	// SDweek = (5·7 + 2·9)/7 = 7.571 h → MSFsc = 05:30 − 0.714 h ≈ 04:47.
	if c.MSF != "05:30" || c.MSW != "02:30" || c.MSFsc != "04:47" || c.Label != "late" || c.SocialJetlag != 180 {
		t.Errorf("chronotype: %+v", c)
	}
}

func TestCircMeanWrapsMidnight(t *testing.T) {
	if m := circMean([]float64{23*60 + 30, 30}); math.Abs(m) > 1e-6 && math.Abs(m-minutesPerDay) > 1e-6 {
		t.Errorf("23:30 and 00:30 must average to 00:00, got %v", m)
	}
	if d := circDiff(30, 23*60+30); d != 60 {
		t.Errorf("00:30 − 23:30 = 60 min, got %v", d)
	}
}

func TestLeadLine(t *testing.T) {
	db := tempDB(t)
	sleepRows(t, db, 20, func(b int) float64 {
		if b == 1 {
			return 5
		}
		return 7.5
	}, func(int) int { return 23 })
	rep, err := Compute(db, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rep.Lead, "Last night: 5 h 00 min of sleep, 2 h 30 min less than your usual 7 h 30 min") {
		t.Errorf("lead: %q", rep.Lead)
	}

	db2 := tempDB(t)
	sleepRows(t, db2, 20, func(int) float64 { return 7.5 }, func(int) int { return 23 })
	insert(t, db2, "resting_heart_rate", 31, func(b int) (float64, bool) { return float64(50 + b%3), true })
	rep2, _ := Compute(db2, asOf)
	if rep2.Lead != "Resting heart rate is within your usual range, and last night was an ordinary night (7 h 30 min)." {
		t.Errorf("lead: %q", rep2.Lead)
	}
}

func TestDayView(t *testing.T) {
	db := tempDB(t)
	insert(t, db, "resting_heart_rate", 40, func(b int) (float64, bool) {
		if b == 0 {
			return 70, true // today: well above every earlier day
		}
		return float64(50 + b%3), true
	})
	insert(t, db, "steps", 40, func(b int) (float64, bool) { return 8000, true })
	sleepRows(t, db, 2, func(int) float64 { return 7 }, func(int) int { return 23 })
	cols := hk.RecordColumns("heart_rate")
	db.BatchInsertRecords("heart_rate", cols, [][]any{
		{"Watch", asOf + " 10:01:00", asOf + " 10:01:00", 60.0, "count/min", nil, nil, nil, nil},
		{"Watch", asOf + " 10:07:00", asOf + " 10:07:00", 80.0, "count/min", nil, nil, nil, nil},
		{"Watch", asOf + " 10:12:00", asOf + " 10:12:00", 90.0, "count/min", nil, nil, nil, nil},
	})
	ec := hk.RecordColumns("high_heart_rate_events")
	db.BatchInsertRecords("high_heart_rate_events", ec, [][]any{
		{"Watch", asOf + " 11:00:00", asOf + " 11:10:00", "HKCategoryValueNotApplicable", nil, nil, nil, nil},
	})

	v, err := Day(db, asOf)
	if err != nil {
		t.Fatal(err)
	}
	var rhr *DayMetric
	for i := range v.Metrics {
		if v.Metrics[i].Key == "resting-heart-rate" {
			rhr = &v.Metrics[i]
		}
	}
	if rhr == nil || rhr.Value != 70 || rhr.Normal == nil || rhr.Normal.Days != 30 || rhr.Note != "highest in 90 days" {
		t.Errorf("resting HR on the day: %+v", rhr)
	}
	for _, m := range v.Metrics {
		if m.Key == "steps" && (!m.Partial || m.Normal != nil) {
			t.Errorf("totals on the last day with data are partial, not compared: %+v", m)
		}
	}
	if rhr != nil && rhr.Partial {
		t.Error("sample metrics are not partial")
	}
	if len(v.HeartRate) != 2 || v.HeartRate[0].T != asOf+" 10:00" || v.HeartRate[0].V != 70 || v.HeartRate[1].N != 1 {
		t.Errorf("10-minute heart rate: %+v", v.HeartRate)
	}
	if len(v.Events) != 1 || v.Events[0].Kind != "high-heart-rate-events" || v.Events[0].Value != "" {
		t.Errorf("events: %+v", v.Events)
	}
	if v.Night == nil || v.Night.Wake[:10] != asOf {
		t.Errorf("the night ending this morning: %+v", v.Night)
	}
}
