package storage

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/BRO3886/healthsync/internal/hk"
)

// Highlight is one notable fact about a period.
type Highlight struct {
	Kind   string   `json:"kind"`
	Title  string   `json:"title"`
	Detail string   `json:"detail"`
	Date   string   `json:"date,omitempty"`
	Value  *float64 `json:"value,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Link   string   `json:"link,omitempty"`
	Tone   string   `json:"tone"` // good|bad|neutral|info
}

// Highlights derives the notable facts of a period: best days, streaks,
// trends, records and data-quality notes. Every number carries its
// denominator, and nothing is fabricated for days without data.
func (db *DB) Highlights(p Period) ([]Highlight, error) {
	var out []Highlight
	add := func(h Highlight) { out = append(out, h) }

	// Best days for cumulative activity metrics.
	for _, key := range []string{"steps", "active-energy", "exercise-time", "distance-walking-running", "flights-climbed"} {
		m := hk.ByKey[key]
		s, err := db.QuerySeries(m, p.From, p.To, BucketDay)
		if err != nil {
			return nil, err
		}
		if len(s.Points) < 3 {
			continue
		}
		best := s.Points[0]
		for _, pt := range s.Points[1:] {
			if pt.V > best.V {
				best = pt
			}
		}
		v := best.V
		add(Highlight{Kind: "best-day", Title: "Best " + strings.ToLower(m.Name) + " day", Detail: fmt.Sprintf("%s on %s (%d days with data)", fmtWithUnit(v, s.Unit), best.T, len(s.Points)), Date: best.T, Value: &v, Unit: s.Unit, Tone: "good", Link: "activity"})
	}

	// Ring streaks.
	rings, err := db.ActivityRings(p.From, p.To)
	if err != nil {
		return nil, err
	}
	if rings.Streaks.Days > 0 {
		st := rings.Streaks
		if st.CurrentAll >= 3 {
			v := float64(st.CurrentAll)
			add(Highlight{Kind: "streak", Title: "All rings closed streak", Detail: fmt.Sprintf("%d days in a row at the end of the period (longest in period: %d)", st.CurrentAll, st.LongestAll), Value: &v, Unit: "days", Tone: "good", Link: "activity"})
		} else if st.LongestAll >= 3 {
			v := float64(st.LongestAll)
			add(Highlight{Kind: "streak", Title: "Longest all-rings streak", Detail: fmt.Sprintf("%d consecutive days; rings closed on %d of %d days with data", st.LongestAll, st.ClosedAll, st.Days), Value: &v, Unit: "days", Tone: "neutral", Link: "activity"})
		} else {
			add(Highlight{Kind: "rings", Title: "Rings closed", Detail: fmt.Sprintf("All three rings closed on %d of %d days with data", st.ClosedAll, st.Days), Tone: "info", Link: "activity"})
		}
	}

	// Trends for slow-moving sample metrics via least-squares slope.
	trend := func(key, unit string, threshold float64, decimals int) error {
		m := hk.ByKey[key]
		s, err := db.QuerySeries(m, p.From, p.To, BucketDay)
		if err != nil {
			return err
		}
		if len(s.Points) < 7 {
			return nil
		}
		change := slopeChange(s.Points)
		if math.Abs(change) < threshold {
			return nil
		}
		tone := "neutral"
		if m.Good == "up" {
			tone = map[bool]string{true: "good", false: "bad"}[change > 0]
		} else if m.Good == "down" {
			tone = map[bool]string{true: "good", false: "bad"}[change < 0]
		}
		dir := "up"
		if change < 0 {
			dir = "down"
		}
		v := change
		add(Highlight{Kind: "trend", Title: m.Name + " trending " + dir, Detail: fmt.Sprintf("%+.*f %s over the period (%d days with data)", decimals, change, unit, len(s.Points)), Value: &v, Unit: unit, Tone: tone, Link: "heart"})
		return nil
	}
	if err := trend("resting-heart-rate", "bpm", 1.0, 1); err != nil {
		return nil, err
	}
	if err := trend("hrv", "ms", 3.0, 1); err != nil {
		return nil, err
	}

	// First-to-last changes.
	firstLast := func(key, unit string, threshold float64, link string) error {
		m := hk.ByKey[key]
		first, err := db.FirstValue(m.Table, p.From)
		if err != nil || first == nil || first.T > p.To {
			return err
		}
		last, _, err := db.LatestValue(m.Table, p.To)
		if err != nil || last == nil || last.T == first.T {
			return err
		}
		d := last.V - first.V
		if math.Abs(d) < threshold {
			return nil
		}
		tone := "neutral"
		if m.Good == "up" {
			tone = map[bool]string{true: "good", false: "bad"}[d > 0]
		} else if m.Good == "down" {
			tone = map[bool]string{true: "good", false: "bad"}[d < 0]
		}
		v := d
		add(Highlight{Kind: "change", Title: m.Name + " changed", Detail: fmt.Sprintf("%s → %s %s (%s to %s)", fmtNum(first.V), fmtNum(last.V), unit, first.T, last.T), Value: &v, Unit: unit, Tone: tone, Link: link, Date: last.T})
		return nil
	}
	if err := firstLast("vo2max", "mL/kg·min", 0.5, "heart"); err != nil {
		return nil, err
	}
	if err := firstLast("body-mass", "kg", 0.5, "body"); err != nil {
		return nil, err
	}

	// Sleep average with denominator.
	nights, err := db.sleepNightTotals(QueryParams{From: p.From, To: p.To})
	if err != nil {
		return nil, err
	}
	var sum float64
	var n int
	for _, t := range nights {
		if !t.Onset.IsZero() {
			sum += t.Hours
			n++
		}
	}
	if n > 0 {
		avg := sum / float64(n)
		tone := "neutral"
		if avg >= 7 {
			tone = "good"
		} else if avg < 6 {
			tone = "bad"
		}
		add(Highlight{Kind: "sleep", Title: "Average night sleep", Detail: fmt.Sprintf("%.1f h across the %d nights recorded (of %d in the period)", avg, n, p.Days()), Value: &avg, Unit: "h", Tone: tone, Link: "sleep"})
	}

	// Workouts: longest, most frequent, records.
	items, total, err := db.ListWorkouts(WorkoutFilter{From: p.From, To: p.To})
	if err != nil {
		return nil, err
	}
	if total > 0 {
		var longest *WorkoutItem
		counts := map[string]int{}
		for i := range items {
			it := &items[i]
			counts[it.ActivityType]++
			if it.DurationMin != nil && (longest == nil || *it.DurationMin > *longest.DurationMin) {
				longest = it
			}
		}
		if longest != nil {
			v := *longest.DurationMin
			add(Highlight{Kind: "workout", Title: "Longest workout", Detail: fmt.Sprintf("%s, %s on %s", activityName(longest.ActivityType), fmtDuration(v), longest.Start[:10]), Date: longest.Start[:10], Value: &v, Unit: "min", Tone: "good", Link: fmt.Sprintf("workouts/%d", longest.ID)})
		}
		type kv struct {
			k string
			v int
		}
		var kvs []kv
		for k, v := range counts {
			kvs = append(kvs, kv{k, v})
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v || (kvs[i].v == kvs[j].v && kvs[i].k < kvs[j].k) })
		if len(kvs) > 0 {
			add(Highlight{Kind: "workout", Title: fmt.Sprintf("%d workouts", total), Detail: fmt.Sprintf("Most frequent: %s (%d)", activityName(kvs[0].k), kvs[0].v), Tone: "info", Link: "workouts"})
		}
		records, err := db.workoutRecords(p, items)
		if err != nil {
			return nil, err
		}
		out = append(out, records...)
	}

	// Cardio events.
	var events int64
	for _, t := range []string{"high_heart_rate_events", "low_heart_rate_events", "irregular_rhythm_events"} {
		var args []any
		q := "SELECT COUNT(*) FROM " + t + " WHERE 1=1" + rangeClause("start_date", p.From, p.To, &args)
		var c int64
		if err := db.conn.QueryRow(q, args...).Scan(&c); err == nil {
			events += c
		}
	}
	if events > 0 {
		v := float64(events)
		add(Highlight{Kind: "events", Title: "Heart rhythm notifications", Detail: fmt.Sprintf("%d high/low/irregular heart rate notifications in the period", events), Value: &v, Tone: "bad", Link: "heart"})
	}

	if out == nil {
		out = []Highlight{}
	}
	return out, nil
}

// workoutRecords finds workouts in the period that beat every earlier workout
// of the same type on distance, energy or duration.
func (db *DB) workoutRecords(p Period, items []WorkoutItem) ([]Highlight, error) {
	var out []Highlight
	seen := map[string]bool{}
	for _, it := range items {
		key := it.ActivityType
		if seen[key] {
			continue
		}
		seen[key] = true
		// Best in period for this type.
		var bestDist, bestDur *WorkoutItem
		for i := range items {
			c := &items[i]
			if c.ActivityType != key {
				continue
			}
			if c.DistanceKm != nil && (bestDist == nil || *c.DistanceKm > *bestDist.DistanceKm) {
				bestDist = c
			}
			if c.DurationMin != nil && (bestDur == nil || *c.DurationMin > *bestDur.DurationMin) {
				bestDur = c
			}
		}
		var prevDist, prevDur float64
		var prevCount int64
		err := db.conn.QueryRow(`SELECT COUNT(*), COALESCE(MAX(total_distance),0), COALESCE(MAX(duration),0) FROM workouts WHERE activity_type = ? AND start_date < ?`, key, p.From).Scan(&prevCount, &prevDist, &prevDur)
		if err != nil {
			return nil, err
		}
		if prevCount == 0 {
			continue // no history to beat
		}
		if bestDist != nil && *bestDist.DistanceKm > prevDist*1.0001 && prevDist > 0 {
			v := *bestDist.DistanceKm
			out = append(out, Highlight{Kind: "record", Title: "Longest " + activityName(key) + " distance", Detail: fmt.Sprintf("%.2f km on %s (previous best %.2f km)", v, bestDist.Start[:10], prevDist), Date: bestDist.Start[:10], Value: &v, Unit: "km", Tone: "good", Link: fmt.Sprintf("workouts/%d", bestDist.ID)})
		}
		if bestDur != nil && *bestDur.DurationMin > prevDur*1.0001 {
			v := *bestDur.DurationMin
			out = append(out, Highlight{Kind: "record", Title: "Longest " + activityName(key) + " session", Detail: fmt.Sprintf("%s on %s (previous best %s)", fmtDuration(v), bestDur.Start[:10], fmtDuration(prevDur)), Date: bestDur.Start[:10], Value: &v, Unit: "min", Tone: "good", Link: fmt.Sprintf("workouts/%d", bestDur.ID)})
		}
	}
	return out, nil
}

// slopeChange fits value = a + b*index and returns b * (n-1): the change the
// trend line implies across the whole period.
func slopeChange(pts []SeriesPoint) float64 {
	n := float64(len(pts))
	var sx, sy, sxx, sxy float64
	for i, p := range pts {
		x := float64(i)
		sx += x
		sy += p.V
		sxx += x * x
		sxy += x * p.V
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	b := (n*sxy - sx*sy) / den
	return b * (n - 1)
}

func fmtNum(v float64) string {
	if math.Abs(v) >= 100 || v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// fmtWithUnit renders a value with its unit, dropping the meaningless "count".
func fmtWithUnit(v float64, unit string) string {
	switch unit {
	case "count", "":
		return fmtNum(v)
	case "count/min":
		return fmtNum(v) + " bpm"
	}
	return fmtNum(v) + " " + unit
}

func fmtDuration(minutes float64) string {
	h := int(minutes) / 60
	m := int(minutes) % 60
	if h == 0 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d h %02d min", h, m)
}

// activityName turns HKWorkoutActivityTypeHighIntensityIntervalTraining into
// "High Intensity Interval Training".
func activityName(t string) string {
	s := strings.TrimPrefix(t, "HKWorkoutActivityType")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := s[i-1]
			if prev >= 'a' && prev <= 'z' {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}
