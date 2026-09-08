package storage

import (
	"math"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
)

// Period is an inclusive date range.
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Days returns the number of calendar days in the period.
func (p Period) Days() int {
	a, err1 := time.Parse(dateLayout, p.From)
	b, err2 := time.Parse(dateLayout, p.To)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours()/24) + 1
}

// Previous returns the same-length period immediately before p.
func (p Period) Previous() Period {
	a, err1 := time.Parse(dateLayout, p.From)
	b, err2 := time.Parse(dateLayout, p.To)
	if err1 != nil || err2 != nil {
		return Period{}
	}
	n := int(b.Sub(a).Hours()/24) + 1
	return Period{
		From: a.AddDate(0, 0, -n).Format(dateLayout),
		To:   a.AddDate(0, 0, -1).Format(dateLayout),
	}
}

// Tile is one KPI on the overview.
type Tile struct {
	Metric string `json:"metric"`
	Name   string `json:"name"`
	Unit   string `json:"unit"`
	Kind   string `json:"kind"` // cumulative|sample|duration|event|latest
	// Value is the headline: daily mean for cumulative/event metrics, mean of
	// samples, mean hours per night for sleep, latest reading for "latest".
	Value float64 `json:"value"`
	// Total is the period sum for cumulative/event metrics.
	Total *float64 `json:"total,omitempty"`
	// Days is how many days (or nights) contributed. It is the denominator the
	// UI must show next to any average.
	Days      int64         `json:"days"`
	Previous  *float64      `json:"previous,omitempty"`
	DeltaPct  *float64      `json:"delta_pct,omitempty"`
	Direction string        `json:"direction"` // up|down|flat|""
	Good      string        `json:"good"`      // up|down|""
	Sparkline []SeriesPoint `json:"sparkline"`
}

// Summary is the overview payload.
type Summary struct {
	Period   Period  `json:"period"`
	Previous *Period `json:"previous,omitempty"`
	Tiles    []Tile  `json:"tiles"`
}

// tileMetrics lists the overview KPIs in display order. "latest" metrics show
// the most recent reading rather than a mean.
var tileMetrics = []struct {
	key    string
	latest bool
}{
	{"steps", false},
	{"active-energy", false},
	{"exercise-time", false},
	{"stand-hours", false},
	{"distance-walking-running", false},
	{"flights-climbed", false},
	{"sleep", false},
	{"resting-heart-rate", false},
	{"hrv", false},
	{"heart-rate", false},
	{"walking-heart-rate", false},
	{"vo2max", true},
	{"body-mass", true},
	{"body-fat", true},
	{"spo2", false},
	{"respiratory-rate", false},
	{"wrist-temperature", false},
	{"time-in-daylight", false},
	{"headphone-audio-exposure", false},
	{"mindful-sessions", false},
	{"dietary-water", false},
	{"blood-glucose", false},
}

// Summary computes the KPI tiles for a period, comparing against the same
// length period before it when compare is true. Metrics with no rows in the
// period produce no tile.
func (db *DB) Summary(p Period, compare bool) (*Summary, error) {
	out := &Summary{Period: p}
	var prev Period
	if compare {
		prev = p.Previous()
		out.Previous = &prev
	}

	for _, tm := range tileMetrics {
		m, ok := hk.ByKey[tm.key]
		if !ok {
			continue
		}
		tile, err := db.tile(m, tm.latest, p, prev, compare)
		if err != nil {
			return nil, err
		}
		if tile != nil {
			out.Tiles = append(out.Tiles, *tile)
		}
	}
	return out, nil
}

func (db *DB) tile(m *hk.Metric, latest bool, p, prev Period, compare bool) (*Tile, error) {
	cur, err := db.QuerySeries(m, p.From, p.To, BucketDay)
	if err != nil {
		return nil, err
	}
	if len(cur.Points) == 0 {
		return nil, nil
	}
	t := &Tile{Metric: m.Key, Name: m.Name, Unit: cur.Unit, Kind: m.Agg.String(), Good: m.Good, Sparkline: cur.Points}

	if latest {
		t.Kind = "latest"
		last := cur.Points[len(cur.Points)-1]
		t.Value = last.V
		t.Days = int64(len(cur.Points))
		if compare {
			pv, unit, err := db.LatestValue(m.Table, prev.To)
			if err != nil {
				return nil, err
			}
			if pv != nil {
				setDelta(t, pv.V*percentScale(unit, pv.V))
			}
		}
		return t, nil
	}

	t.Value, t.Total, t.Days = headline(m, cur.Points)
	if compare {
		ps, err := db.QuerySeries(m, prev.From, prev.To, BucketDay)
		if err != nil {
			return nil, err
		}
		if len(ps.Points) > 0 {
			pv, _, _ := headline(m, ps.Points)
			setDelta(t, pv)
		}
	}
	return t, nil
}

// headline reduces daily points to the tile value: a per-day mean over days
// with data for cumulative/event metrics (with the total alongside), a
// sample-weighted mean for sample metrics, the mean per night for sleep.
func headline(m *hk.Metric, pts []SeriesPoint) (float64, *float64, int64) {
	switch m.Agg {
	case hk.Cumulative, hk.Event, hk.Duration:
		if m.Table == "sleep" {
			var sum float64
			for _, p := range pts {
				sum += p.V
			}
			return sum / float64(len(pts)), nil, int64(len(pts))
		}
		var sum float64
		for _, p := range pts {
			sum += p.V
		}
		total := sum
		return sum / float64(len(pts)), &total, int64(len(pts))
	default:
		var sum float64
		var n int64
		for _, p := range pts {
			sum += p.V * float64(p.N)
			n += p.N
		}
		if n == 0 {
			return 0, nil, 0
		}
		return sum / float64(n), nil, int64(len(pts))
	}
}

func setDelta(t *Tile, prev float64) {
	pv := prev
	t.Previous = &pv
	if prev != 0 {
		d := (t.Value - prev) / math.Abs(prev) * 100
		t.DeltaPct = &d
	}
	switch {
	case math.Abs(t.Value-prev) < 1e-9:
		t.Direction = "flat"
	case t.Value > prev:
		t.Direction = "up"
	default:
		t.Direction = "down"
	}
}
