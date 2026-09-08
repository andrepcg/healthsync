package storage

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// SleepNight is one night with per-stage hours and overnight vitals.
type SleepNight struct {
	Night  string  `json:"night"`
	Hours  float64 `json:"hours"`
	Naps   float64 `json:"naps"`
	Onset  string  `json:"onset"` // "YYYY-MM-DD HH:MM"
	Wake   string  `json:"wake"`
	Stages struct {
		Core        float64 `json:"core"`
		Deep        float64 `json:"deep"`
		REM         float64 `json:"rem"`
		Awake       float64 `json:"awake"`
		Unspecified float64 `json:"unspecified"`
		InBed       float64 `json:"inbed"`
	} `json:"stages"`
	Vitals struct {
		WristTemp             *float64 `json:"wrist_temp,omitempty"`
		RespRate              *float64 `json:"resp_rate,omitempty"`
		SpO2Min               *float64 `json:"spo2_min,omitempty"`
		SpO2Avg               *float64 `json:"spo2_avg,omitempty"`
		BreathingDisturbances *float64 `json:"breathing_disturbances,omitempty"`
		HeartRateMin          *float64 `json:"hr_min,omitempty"`
		HeartRateAvg          *float64 `json:"hr_avg,omitempty"`
		HRV                   *float64 `json:"hrv,omitempty"`
	} `json:"vitals"`
}

type stageSegment struct {
	stage      string
	start, end time.Time
}

func stageOf(value string) string {
	switch {
	case strings.HasSuffix(value, "AsleepCore"):
		return "core"
	case strings.HasSuffix(value, "AsleepDeep"):
		return "deep"
	case strings.HasSuffix(value, "AsleepREM"):
		return "rem"
	case strings.HasSuffix(value, "Awake"):
		return "awake"
	case strings.HasSuffix(value, "InBed"):
		return "inbed"
	case strings.Contains(value, "Asleep"):
		return "unspecified"
	}
	return ""
}

// SleepNights returns one entry per night with data in [from, to], with
// stage hours and the vitals recorded while asleep. It reuses the session
// clustering of QuerySleepDailyTotal so a night is never split by the clock.
func (db *DB) SleepNights(from, to string) ([]SleepNight, error) {
	nights, err := db.sleepNightTotals(QueryParams{From: from, To: to})
	if err != nil {
		return nil, err
	}
	if len(nights) == 0 {
		return []SleepNight{}, nil
	}

	out := make([]SleepNight, 0, len(nights))
	for _, t := range nights {
		n := SleepNight{Night: t.Night, Hours: t.Hours, Naps: t.Naps}
		if !t.Onset.IsZero() {
			n.Onset = t.Onset.Format(sleepTimeLayout)
			n.Wake = t.Wake.Format(sleepTimeLayout)
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Night < out[j].Night })

	// Load every stage segment in a window wide enough to cover the nights.
	first, last := out[0].Night, out[len(out)-1].Night
	q := `SELECT value, start_date, end_date FROM sleep WHERE start_date >= ? AND start_date < date(?, '+2 days') ORDER BY start_date`
	rows, err := db.conn.Query(q, first, last)
	if err != nil {
		return nil, err
	}
	var segs []stageSegment
	for rows.Next() {
		var v, s, e string
		if err := rows.Scan(&v, &s, &e); err != nil {
			rows.Close()
			return nil, err
		}
		st, err1 := time.Parse(sleepTimeLayout, s)
		en, err2 := time.Parse(sleepTimeLayout, e)
		if err1 != nil || err2 != nil || !en.After(st) {
			continue
		}
		if stage := stageOf(v); stage != "" {
			segs = append(segs, stageSegment{stage, st, en})
		}
	}
	rows.Close()

	for i := range out {
		n := &out[i]
		if n.Onset == "" {
			continue
		}
		onset, err1 := time.Parse(sleepTimeLayout, n.Onset)
		wake, err2 := time.Parse(sleepTimeLayout, n.Wake)
		if err1 != nil || err2 != nil {
			continue
		}
		// InBed can begin before the first asleep segment; widen slightly.
		lo, hi := onset.Add(-90*time.Minute), wake.Add(30*time.Minute)
		perStage := map[string][]stageSegment{}
		for _, s := range segs {
			if s.end.Before(lo) || s.start.After(hi) {
				continue
			}
			perStage[s.stage] = append(perStage[s.stage], s)
		}
		for stage, list := range perStage {
			h := mergedHours(list)
			switch stage {
			case "core":
				n.Stages.Core = h
			case "deep":
				n.Stages.Deep = h
			case "rem":
				n.Stages.REM = h
			case "awake":
				n.Stages.Awake = h
			case "unspecified":
				n.Stages.Unspecified = h
			case "inbed":
				n.Stages.InBed = h
			}
		}
		n.Onset = onset.Format("2006-01-02 15:04")
		n.Wake = wake.Format("2006-01-02 15:04")

		vs := onset.Format(sleepTimeLayout)
		ve := wake.Format(sleepTimeLayout)
		n.Vitals.WristTemp = db.avgBetween("wrist_temperature", onset.Add(-2*time.Hour).Format(sleepTimeLayout), ve)
		n.Vitals.RespRate = db.avgBetween("respiratory_rate", vs, ve)
		n.Vitals.SpO2Avg = scalePct(db.avgBetween("spo2", vs, ve))
		n.Vitals.SpO2Min = scalePct(db.minBetween("spo2", vs, ve))
		n.Vitals.BreathingDisturbances = db.avgBetween("sleeping_breathing_disturbances", onset.Add(-2*time.Hour).Format(sleepTimeLayout), ve)
		n.Vitals.HeartRateMin = db.minBetween("heart_rate", vs, ve)
		n.Vitals.HeartRateAvg = db.avgBetween("heart_rate", vs, ve)
		n.Vitals.HRV = db.avgBetween("hrv", vs, ve)
	}
	return out, nil
}

// mergedHours sums a stage's segments after merging overlaps (two sources
// recording the same stage count once).
func mergedHours(list []stageSegment) float64 {
	sort.Slice(list, func(i, j int) bool { return list[i].start.Before(list[j].start) })
	var total time.Duration
	cur := list[0]
	for _, s := range list[1:] {
		if !s.start.After(cur.end) {
			if s.end.After(cur.end) {
				cur.end = s.end
			}
			continue
		}
		total += cur.end.Sub(cur.start)
		cur = s
	}
	total += cur.end.Sub(cur.start)
	return total.Hours()
}

func (db *DB) avgBetween(table, from, to string) *float64 {
	var v *float64
	db.conn.QueryRow(fmt.Sprintf(`SELECT AVG(value) FROM %s WHERE start_date >= ? AND start_date <= ?`, table), from, to).Scan(&v)
	return v
}

func (db *DB) minBetween(table, from, to string) *float64 {
	var v *float64
	db.conn.QueryRow(fmt.Sprintf(`SELECT MIN(value) FROM %s WHERE start_date >= ? AND start_date <= ?`, table), from, to).Scan(&v)
	return v
}

// scalePct turns a fractional SpO2 (0.97) into a percentage (97).
func scalePct(v *float64) *float64 {
	if v == nil || *v > 1 {
		return v
	}
	x := *v * 100
	return &x
}
