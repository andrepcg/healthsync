package insights

import (
	"fmt"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

// DayMetric is one metric's value on a day against the person's usual range
// over the 30 days before it.
type DayMetric struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Group  string  `json:"group"`
	Unit   string  `json:"unit"`
	Agg    string  `json:"agg"`
	Value  float64 `json:"value"`
	N      int64   `json:"n"`
	Normal *Band   `json:"normal,omitempty"`
	// Note is a rule-based remark: "highest in 90 days", "first reading".
	Note string `json:"note,omitempty"`
	// Partial marks a total on the last day with data: the export was taken
	// mid-day, so it is neither compared with the usual range nor noted.
	Partial bool `json:"partial,omitempty"`
}

// DayView is everything recorded on one calendar day.
type DayView struct {
	Date    string      `json:"date"`
	Metrics []DayMetric `json:"metrics"`
	// Night is the sleep that ended on the morning of Date.
	Night     *storage.SleepNight     `json:"night,omitempty"`
	Workouts  []storage.WorkoutItem   `json:"workouts"`
	Events    []storage.DayEvent      `json:"events"`
	HeartRate []storage.IntradayPoint `json:"heart_rate"`
	First     string                  `json:"first_date,omitempty"`
	Last      string                  `json:"last_date,omitempty"`
}

const (
	dayUsualDays   = 30
	dayMinUsual    = 7
	dayRecordDays  = 90
	dayMinForNotes = 30
)

// Day assembles the day view. Sleep is reported as the night, not as a
// metric, because a night belongs to two calendar days.
func Day(db *storage.DB, date string) (*DayView, error) {
	d, err := time.Parse(day, date)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q", date)
	}
	av, err := db.Availability()
	if err != nil {
		return nil, err
	}
	out := &DayView{Date: date, Metrics: []DayMetric{}, First: av.FirstDate, Last: av.LastDate}
	tables := map[string]storage.TableAvailability{}
	for _, t := range av.Tables {
		tables[t.Table] = t
	}

	usualFrom := d.AddDate(0, 0, -dayUsualDays).Format(day)
	for i := range hk.Metrics {
		m := &hk.Metrics[i]
		if m.Paired || m.Agg == hk.Event || m.Table == "sleep" {
			continue
		}
		t, ok := tables[m.Table]
		if !ok || t.Rows == 0 || (t.First != "" && t.First[:10] > date) || (t.Last != "" && t.Last[:10] < date) {
			continue
		}
		s, err := db.QuerySeries(m, d.AddDate(0, 0, -dayRecordDays).Format(day), date, storage.BucketDay)
		if err != nil {
			return nil, err
		}
		if len(s.Points) == 0 || s.Points[len(s.Points)-1].T != date {
			continue
		}
		cur := s.Points[len(s.Points)-1]
		prior := s.Points[:len(s.Points)-1]
		dm := DayMetric{Key: m.Key, Name: m.Name, Group: m.Group, Unit: s.Unit, Agg: s.Agg, Value: cur.V, N: cur.N}
		var recent []float64
		for _, p := range prior {
			if p.T >= usualFrom {
				recent = append(recent, p.V)
			}
		}
		if m.Agg != hk.Sample && av.LastDate != "" && av.LastDate[:10] == date {
			dm.Partial = true
			out.Metrics = append(out.Metrics, dm)
			continue
		}
		dm.Normal = bandOf(recent, usualFrom, d.AddDate(0, 0, -1).Format(day), dayMinUsual)
		switch {
		case t.First != "" && t.First[:10] == date:
			dm.Note = "first reading"
		case len(prior) >= dayMinForNotes:
			lo, hi := prior[0].V, prior[0].V
			for _, p := range prior {
				lo, hi = min(lo, p.V), max(hi, p.V)
			}
			if cur.V > hi {
				dm.Note = "highest in 90 days"
			} else if cur.V < lo {
				dm.Note = "lowest in 90 days"
			}
		}
		out.Metrics = append(out.Metrics, dm)
	}

	prev := d.AddDate(0, 0, -1).Format(day)
	nights, err := db.SleepNights(prev, date)
	if err != nil {
		return nil, err
	}
	for i := range nights {
		if nights[i].Onset != "" && nights[i].Wake[:10] == date {
			out.Night = &nights[i]
		}
	}
	if out.Workouts, _, err = db.ListWorkouts(storage.WorkoutFilter{From: date, To: date, Limit: 50}); err != nil {
		return nil, err
	}
	if out.Workouts == nil {
		out.Workouts = []storage.WorkoutItem{}
	}
	if out.Events, err = db.DayEvents(date); err != nil {
		return nil, err
	}
	if out.HeartRate, err = db.Intraday("heart_rate", date); err != nil {
		return nil, err
	}
	return out, nil
}
