package insights

import (
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

// Band is a person's usual range for a metric: the median of their daily
// values ± one robust spread (MAD scaled to a standard deviation). It shades
// "your normal" behind charts and is the yardstick in the day view.
type Band struct {
	Median float64 `json:"median"`
	Lo     float64 `json:"lo"`
	Hi     float64 `json:"hi"`
	Days   int     `json:"days"`
	From   string  `json:"from"`
	To     string  `json:"to"`
}

const (
	normalDays    = 90
	minNormalDays = 14
)

// HasNormal reports whether a "your normal" band makes sense for a metric:
// point measurements and nightly sleep, not totals or counts.
func HasNormal(m *hk.Metric) bool {
	return m != nil && !m.Paired && (m.Agg == hk.Sample || m.Table == "sleep")
}

// Normal returns the band over the 90 days ending at `to` (inclusive), or nil
// with fewer than 14 days of data.
func Normal(db *storage.DB, m *hk.Metric, to string) (*Band, error) {
	end, err := time.Parse(day, to)
	if err != nil || !HasNormal(m) {
		return nil, err
	}
	from := end.AddDate(0, 0, -(normalDays - 1)).Format(day)
	s, err := db.QuerySeries(m, from, to, storage.BucketDay)
	if err != nil {
		return nil, err
	}
	return bandOf(values(s.Points), from, to, minNormalDays), nil
}

func bandOf(vals []float64, from, to string, minN int) *Band {
	if len(vals) < minN {
		return nil
	}
	med, sp := median(vals), mad(vals)
	// Never extend the band past values actually seen (no negative
	// asymmetry, no SpO2 above the highest reading).
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = min(lo, v), max(hi, v)
	}
	return &Band{Median: med, Lo: max(med-sp, lo), Hi: min(med+sp, hi), Days: len(vals), From: from, To: to}
}
