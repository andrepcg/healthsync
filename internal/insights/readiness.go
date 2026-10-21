package insights

import (
	"fmt"
	"math"
	"time"

	"github.com/BRO3886/healthsync/internal/storage"
)

// SleepNeed is how much sleep a person needs per night: their own sleep goal
// from the Health app when exported, else the recommended minimum for their
// age (AASM/NSF: 7 h for adults), else the adult figure.
func SleepNeed(db *storage.DB, dob, asOf string) (hours float64, source string) {
	if lv, _, err := db.LatestValue("sleep_duration_goal", asOf); err == nil && lv != nil && lv.V > 3 && lv.V < 12 {
		return lv.V, "your sleep goal"
	}
	b, err1 := time.Parse(day, dob)
	a, err2 := time.Parse(day, asOf)
	if err1 != nil || err2 != nil {
		return 7, "adult default (no date of birth)"
	}
	age := a.Year() - b.Year()
	if a.YearDay() < b.YearDay() {
		age--
	}
	switch {
	case age < 3:
		hours = 11
	case age < 6:
		hours = 10
	case age < 13:
		hours = 9
	case age < 18:
		hours = 8
	default:
		hours = 7
	}
	return hours, fmt.Sprintf("recommended minimum at age %d", age)
}

// ReadinessComponent is one input to the readiness score, with everything
// needed to show how it was scored.
type ReadinessComponent struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Usual  float64 `json:"usual"`  // median of previous nights; the need for sleep
	Spread float64 `json:"spread"` // typical night-to-night spread
	Z      float64 `json:"z"`      // deviation in spreads, signed so + is better
	Score  float64 `json:"score"`  // 0–100, 75 = usual
	Weight float64 `json:"weight"` // after renormalising over present components
}

// ReadinessDay is the score for one wake day.
type ReadinessDay struct {
	Date       string               `json:"date"`  // the morning
	Night      string               `json:"night"` // the evening the night began
	Score      *int                 `json:"score"` // nil when too few components
	Band       string               `json:"band,omitempty"`
	Components []ReadinessComponent `json:"components"`
	Missing    []Skipped            `json:"missing"`
}

// Readiness is the response of the readiness endpoint.
type Readiness struct {
	Days       []ReadinessDay `json:"days"`
	Need       float64        `json:"sleep_need"`
	NeedSource string         `json:"sleep_need_source"`
}

const (
	readinessBaseline = 28
	readinessMinBase  = 14
	readinessMinParts = 3
)

// ComputeReadiness scores each morning in [from, to] from last night's
// overnight HRV, sleeping heart rate and respiratory rate (each against the
// previous 28 nights' median and spread) and sleep against need.
//
// A component sits at 75 when it equals the person's usual and moves 12.5
// points per typical spread (MAD), so +2 spreads is 100 and −2 is 50; sleep
// moves 12.5 points per half hour against need. Weights (HRV .35, sleeping HR
// .25, sleep .25, respiratory rate .15) are renormalised over the components
// present: a missing signal lowers coverage, never the score. Fewer than three
// components, or neither HRV nor heart rate, gives no score. Descriptive only.
func ComputeReadiness(db *storage.DB, from, to string, need float64) ([]ReadinessDay, error) {
	f, err1 := time.Parse(day, from)
	if _, err2 := time.Parse(day, to); err1 != nil || err2 != nil {
		return nil, fmt.Errorf("invalid range %s..%s", from, to)
	}
	all, err := db.SleepNights(f.AddDate(0, 0, -(readinessBaseline+2)).Format(day), to)
	if err != nil {
		return nil, err
	}
	var nights []storage.SleepNight
	for _, n := range all {
		if n.Onset != "" {
			nights = append(nights, n)
		}
	}

	type vital struct {
		key, label, unit string
		weight           float64
		dir              int // +1 higher is better, -1 lower is better, 0 any change is worse
		get              func(storage.SleepNight) *float64
	}
	vitals := []vital{
		{"hrv", "Overnight HRV", "ms", 0.35, +1, func(n storage.SleepNight) *float64 { return n.Vitals.HRV }},
		{"sleeping_hr", "Sleeping heart rate", "bpm", 0.25, -1, func(n storage.SleepNight) *float64 { return n.Vitals.HeartRateAvg }},
		{"resp_rate", "Respiratory rate", "/min", 0.15, 0, func(n storage.SleepNight) *float64 { return n.Vitals.RespRate }},
	}

	out := []ReadinessDay{}
	for i, n := range nights {
		wake := n.Wake[:10]
		if wake < from || wake > to {
			continue
		}
		rd := ReadinessDay{Date: wake, Night: n.Night, Components: []ReadinessComponent{}, Missing: []Skipped{}}
		cutoff := mustDate(n.Night).AddDate(0, 0, -readinessBaseline).Format(day)
		var base []storage.SleepNight
		for _, b := range nights[:i] {
			if b.Night >= cutoff {
				base = append(base, b)
			}
		}
		for _, v := range vitals {
			x := v.get(n)
			if x == nil {
				rd.Missing = append(rd.Missing, Skipped{v.key, "no reading last night"})
				continue
			}
			var hist []float64
			for _, b := range base {
				if bv := v.get(b); bv != nil {
					hist = append(hist, *bv)
				}
			}
			if len(hist) < readinessMinBase {
				rd.Missing = append(rd.Missing, Skipped{v.key, fmt.Sprintf("needs %d nights of history, has %d", readinessMinBase, len(hist))})
				continue
			}
			med := median(hist)
			// A floor on the spread keeps a very steady baseline from turning
			// a trivial change into a large deviation.
			sp := math.Max(mad(hist), 0.03*math.Abs(med))
			dev := (*x - med) / sp
			z := dev * float64(v.dir)
			if v.dir == 0 {
				z = -math.Abs(dev)
			}
			rd.Components = append(rd.Components, component(v.key, v.label, v.unit, *x, med, sp, z, v.weight))
		}
		rd.Components = append(rd.Components, component("sleep", "Sleep vs need", "h", n.Hours, need, 0.5, (n.Hours-need)/0.5, 0.25))

		hasHeart := false
		var wsum, ssum float64
		for _, c := range rd.Components {
			wsum += c.Weight
			ssum += c.Weight * c.Score
			hasHeart = hasHeart || c.Key == "hrv" || c.Key == "sleeping_hr"
		}
		for j := range rd.Components {
			rd.Components[j].Weight /= wsum
		}
		if len(rd.Components) >= readinessMinParts && hasHeart {
			s := int(math.Round(ssum / wsum))
			rd.Score = &s
			rd.Band = readinessBand(s)
		}
		out = append(out, rd)
	}
	return out, nil
}

func component(key, label, unit string, value, usual, spread, z, weight float64) ReadinessComponent {
	z = math.Max(-4, math.Min(2, z))
	return ReadinessComponent{Key: key, Label: label, Value: value, Unit: unit, Usual: usual, Spread: spread, Z: z,
		Score: math.Max(0, math.Min(100, 75+12.5*z)), Weight: weight}
}

// readinessBand: 70 and above is good, 50–69 fair, below 50 low.
func readinessBand(s int) string {
	switch {
	case s >= 70:
		return "good"
	case s >= 50:
		return "fair"
	}
	return "low"
}

func mustDate(s string) time.Time {
	t, _ := time.Parse(day, s)
	return t
}
