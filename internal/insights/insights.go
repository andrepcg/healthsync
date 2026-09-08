// Package insights turns a person's data into observations: deviations from
// their own baseline, training-load swings, sleep patterns, habit changes,
// fitness trends and red flags. Everything is deterministic and computed on
// demand, so it is reproducible, testable and never leaves the machine.
//
// Rules of the house:
//   - Compare a person against themselves (rolling baseline), not against
//     population norms.
//   - Never claim anything on thin data: every detector has minimum sample
//     requirements and says how many days it used.
//   - Missing days are missing, not zero.
//   - Red flags say "discuss with a doctor"; nothing here is a diagnosis.
package insights

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

// Severity orders observations. Alerts are red flags, warnings need attention
// this week, notices are worth knowing, info is context or good news.
const (
	SevAlert   = "alert"
	SevWarning = "warning"
	SevNotice  = "notice"
	SevInfo    = "info"
)

var sevRank = map[string]int{SevAlert: 0, SevWarning: 1, SevNotice: 2, SevInfo: 3}

// Categories group observations in the UI.
const (
	CatRecovery = "recovery"
	CatTraining = "training"
	CatSleep    = "sleep"
	CatActivity = "activity"
	CatFitness  = "fitness"
	CatHeart    = "heart"
	CatHearing  = "hearing"
	CatBody     = "body"
	CatData     = "data"
)

// Evidence is one number behind an observation.
type Evidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Observation is one finding.
type Observation struct {
	ID       string     `json:"id"`
	Category string     `json:"category"`
	Severity string     `json:"severity"`
	Tone     string     `json:"tone"` // good|bad|neutral
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Advice   string     `json:"advice,omitempty"`
	Window   string     `json:"window"`
	Evidence []Evidence `json:"evidence"`
	Link     string     `json:"link,omitempty"`
	// Days is the number of days (or nights) of data the finding rests on.
	Days int `json:"days"`
}

// Report is the full set of observations as of a date.
type Report struct {
	AsOf         string        `json:"as_of"`
	Observations []Observation `json:"observations"`
	// Checked lists every detector that ran, so the UI can say "12 checks,
	// 3 findings" and the absence of an observation is meaningful.
	Checked []string `json:"checked"`
	// Skipped lists detectors that had too little data, with the reason.
	Skipped []Skipped `json:"skipped"`
}

// Skipped records a detector that could not run.
type Skipped struct {
	Check  string `json:"check"`
	Reason string `json:"reason"`
}

const day = "2006-01-02"

type ctx struct {
	db    *storage.DB
	asOf  time.Time
	rep   *Report
	cache map[string]*storage.Series
}

func (c *ctx) date(daysBack int) string { return c.asOf.AddDate(0, 0, -daysBack).Format(day) }

func (c *ctx) add(o Observation) {
	if o.Tone == "" {
		o.Tone = "neutral"
	}
	c.rep.Observations = append(c.rep.Observations, o)
}

func (c *ctx) checked(name string) { c.rep.Checked = append(c.rep.Checked, name) }

func (c *ctx) skip(name, reason string) {
	c.rep.Skipped = append(c.rep.Skipped, Skipped{Check: name, Reason: reason})
}

// series returns the daily series for a metric over the last n days (cached).
func (c *ctx) series(key string, daysBack int) ([]storage.SeriesPoint, string, error) {
	m, ok := hk.ByKey[key]
	if !ok {
		return nil, "", fmt.Errorf("unknown metric %s", key)
	}
	ck := fmt.Sprintf("%s:%d", key, daysBack)
	if s, ok := c.cache[ck]; ok {
		return s.Points, s.Unit, nil
	}
	s, err := c.db.QuerySeries(m, c.date(daysBack), c.date(0), storage.BucketDay)
	if err != nil {
		return nil, "", err
	}
	c.cache[ck] = s
	return s.Points, s.Unit, nil
}

// Compute runs every detector as of the given day (YYYY-MM-DD). An empty asOf
// means the last day with data.
func Compute(db *storage.DB, asOf string) (*Report, error) {
	if asOf == "" {
		av, err := db.Availability()
		if err != nil {
			return nil, err
		}
		asOf = av.LastDate
		if asOf == "" {
			return &Report{Observations: []Observation{}, Checked: []string{}, Skipped: []Skipped{}}, nil
		}
	}
	t, err := time.Parse(day, asOf)
	if err != nil {
		return nil, fmt.Errorf("invalid as_of %q", asOf)
	}
	c := &ctx{db: db, asOf: t, rep: &Report{AsOf: asOf, Observations: []Observation{}, Checked: []string{}, Skipped: []Skipped{}}, cache: map[string]*storage.Series{}}

	steps := []func(*ctx) error{
		checkRecoveryBaselines,
		checkStrainComposite,
		checkTrainingLoad,
		checkSleep,
		checkActivityHabits,
		checkFitnessTrends,
		checkRedFlags,
		checkHearing,
		checkBody,
		checkDataQuality,
	}
	for _, f := range steps {
		if err := f(c); err != nil {
			return nil, err
		}
	}

	sort.SliceStable(c.rep.Observations, func(i, j int) bool {
		a, b := c.rep.Observations[i], c.rep.Observations[j]
		if sevRank[a.Severity] != sevRank[b.Severity] {
			return sevRank[a.Severity] < sevRank[b.Severity]
		}
		return a.Category < b.Category
	})
	return c.rep, nil
}

// ---------------------------------------------------------------- statistics

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

// mad is the median absolute deviation scaled to be comparable to a standard
// deviation for normal data (×1.4826), robust to the odd wild day.
func mad(xs []float64) float64 {
	m := median(xs)
	d := make([]float64, len(xs))
	for i, x := range xs {
		d[i] = math.Abs(x - m)
	}
	return 1.4826 * median(d)
}

func stddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := mean(xs)
	s := 0.0
	for _, x := range xs {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(xs)-1))
}

// split divides daily points into baseline (older than recentDays) and recent.
func split(pts []storage.SeriesPoint, cutoff string) (base, recent []storage.SeriesPoint) {
	for _, p := range pts {
		if p.T > cutoff {
			recent = append(recent, p)
		} else {
			base = append(base, p)
		}
	}
	return
}

func values(pts []storage.SeriesPoint) []float64 {
	out := make([]float64, len(pts))
	for i, p := range pts {
		out[i] = p.V
	}
	return out
}

func fmtN(v float64, dec int) string { return fmt.Sprintf("%.*f", dec, v) }

func fmtSigned(v float64, dec int) string {
	if v > 0 {
		return "+" + fmtN(v, dec)
	}
	return fmtN(v, dec)
}

func fmtHours(h float64) string {
	total := int(math.Round(h * 60))
	if total < 60 {
		return fmt.Sprintf("%d min", total)
	}
	return fmt.Sprintf("%d h %02d min", total/60, total%60)
}

func pluralDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

// ------------------------------------------------------------- 1. baselines

type baselineSpec struct {
	key      string
	name     string
	unit     string
	dec      int
	minAbs   float64 // minimum absolute deviation worth mentioning
	minRel   float64 // or relative (fraction of baseline), whichever is larger
	goodUp   bool    // is an increase good?
	neutral  bool    // no good/bad reading (temperature)
	link     string
	category string
	upText   string
	downText string
}

var baselines = []baselineSpec{
	{key: "resting-heart-rate", name: "Resting heart rate", unit: "bpm", dec: 0, minAbs: 3, minRel: 0.05, goodUp: false, link: "heart", category: CatRecovery,
		upText:   "An elevated resting heart rate for several days often accompanies incomplete recovery, the start of an illness, poor sleep, alcohol or heat.",
		downText: "A lower resting heart rate than your usual is generally a sign of good recovery and improving fitness."},
	{key: "hrv", name: "Heart rate variability", unit: "ms", dec: 0, minAbs: 5, minRel: 0.12, goodUp: true, link: "heart", category: CatRecovery,
		upText:   "Higher variability than your usual points to good recovery.",
		downText: "Suppressed variability for several days is a recovery signal: stress, hard training, poor sleep or illness all lower it."},
	{key: "wrist-temperature", name: "Wrist temperature", unit: "°C", dec: 2, minAbs: 0.3, minRel: 0, neutral: true, link: "sleep", category: CatRecovery,
		upText:   "A sustained rise in overnight wrist temperature can precede a fever or follow hard training, alcohol or a late meal. In menstruating people it also tracks the cycle.",
		downText: "Overnight wrist temperature is below your usual."},
	{key: "respiratory-rate", name: "Sleeping respiratory rate", unit: "/min", dec: 1, minAbs: 1.0, minRel: 0.07, goodUp: false, link: "sleep", category: CatRecovery,
		upText:   "Breathing faster than usual during sleep is one of the earlier signs of a respiratory infection or of high strain.",
		downText: "Sleeping respiratory rate is below your usual, which is not a concern on its own."},
	{key: "spo2", name: "Blood oxygen", unit: "%", dec: 1, minAbs: 1.5, minRel: 0, goodUp: true, link: "heart", category: CatRecovery,
		upText:   "Blood oxygen is above your usual.",
		downText: "Overnight blood oxygen is below your usual. Altitude, congestion, alcohol and sleep position all lower it; if it persists, mention it to a doctor."},
}

const (
	baselineDays   = 28
	recentDays     = 3
	minBaselineN   = 14
	minRecentN     = 2
	deviationSigma = 2.0
)

func checkRecoveryBaselines(c *ctx) error {
	for _, spec := range baselines {
		name := "baseline:" + spec.key
		pts, _, err := c.series(spec.key, baselineDays+recentDays)
		if err != nil {
			return err
		}
		base, recent := split(pts, c.date(recentDays))
		if len(base) < minBaselineN || len(recent) < minRecentN {
			if len(pts) > 0 {
				c.skip(name, fmt.Sprintf("needs %d baseline days and %d recent days, have %d and %d", minBaselineN, minRecentN, len(base), len(recent)))
			}
			continue
		}
		c.checked(name)
		bv := values(base)
		bm := median(bv)
		spread := mad(bv)
		rm := mean(values(recent))
		delta := rm - bm
		threshold := math.Max(spec.minAbs, spec.minRel*math.Abs(bm))
		if math.Abs(delta) < threshold || (spread > 0 && math.Abs(delta) < deviationSigma*spread) {
			continue
		}
		up := delta > 0
		tone := "neutral"
		if !spec.neutral {
			if up == spec.goodUp {
				tone = "good"
			} else {
				tone = "bad"
			}
		}
		sev := SevNotice
		if tone == "bad" && (spread == 0 || math.Abs(delta) > 3*spread) {
			sev = SevWarning
		}
		if tone == "good" {
			sev = SevInfo
		}
		if spec.neutral && up {
			sev = SevNotice
		}
		dir := "above"
		text := spec.upText
		if !up {
			dir = "below"
			text = spec.downText
		}
		c.add(Observation{
			ID: spec.key + "-" + dir, Category: spec.category, Severity: sev, Tone: tone,
			Title:  fmt.Sprintf("%s %s your baseline", spec.name, dir),
			Detail: fmt.Sprintf("Last %d days averaged %s %s against a 4-week baseline of %s %s (%s %s).", len(recent), fmtN(rm, spec.dec), spec.unit, fmtN(bm, spec.dec), spec.unit, fmtSigned(delta, spec.dec), spec.unit),
			Advice: text,
			Window: fmt.Sprintf("last %d days vs previous %d", recentDays, baselineDays),
			Evidence: []Evidence{
				{"recent", fmtN(rm, spec.dec) + " " + spec.unit},
				{"baseline", fmtN(bm, spec.dec) + " " + spec.unit},
				{"typical day-to-day spread", "±" + fmtN(spread, spec.dec) + " " + spec.unit},
			},
			Link: spec.link, Days: len(base) + len(recent),
		})
	}
	return nil
}

// ----------------------------------------------------- 2. strain composite

// checkStrainComposite looks for the classic cluster: resting HR up, wrist
// temperature up, respiratory rate up, HRV down, all at once. Two or more
// together is a stronger signal than any one alone.
func checkStrainComposite(c *ctx) error {
	c.checked("strain-composite")
	hits := 0
	var parts []string
	for _, o := range c.rep.Observations {
		if o.Category != CatRecovery {
			continue
		}
		switch o.ID {
		case "resting-heart-rate-above", "wrist-temperature-above", "respiratory-rate-above", "hrv-below", "spo2-below":
			hits++
			parts = append(parts, strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(o.Title, " above your baseline"), " below your baseline")))
		}
	}
	if hits < 2 {
		return nil
	}
	sev := SevWarning
	if hits >= 3 {
		sev = SevAlert
	}
	c.add(Observation{
		ID: "strain", Category: CatRecovery, Severity: sev, Tone: "bad",
		Title:  "Several recovery signals are off at once",
		Detail: fmt.Sprintf("%d of the overnight markers moved in the strain direction over the last %d days: %s.", hits, recentDays, strings.Join(parts, ", ")),
		Advice: "This pattern usually means the body is fighting something or has not recovered from recent load. Favour an easy day or two, sleep, and fluids. If you feel unwell or it lasts beyond a few days, see a doctor.",
		Window: fmt.Sprintf("last %d days", recentDays),
		Link:   "heart",
	})
	return nil
}

// ---------------------------------------------------------- 3. training load

// checkTrainingLoad computes the acute:chronic ratio of daily exercise minutes
// (7-day mean over 28-day mean, both over days with data).
func checkTrainingLoad(c *ctx) error {
	const name = "training-load"
	key := "exercise-time"
	pts, unit, err := c.series(key, 27)
	if err != nil {
		return err
	}
	if len(pts) < 10 {
		key = "active-energy"
		pts, unit, err = c.series(key, 27)
		if err != nil {
			return err
		}
	}
	base, recent := split(pts, c.date(7))
	if len(recent) < 4 || len(base) < 10 {
		if len(pts) > 0 {
			c.skip(name, fmt.Sprintf("needs 4 recent and 10 earlier days with data, have %d and %d", len(recent), len(base)))
		}
		return nil
	}
	c.checked(name)
	acute := mean(values(recent))
	chronic := mean(values(pts))
	if chronic <= 0 {
		return nil
	}
	ratio := acute / chronic
	label := "exercise minutes"
	if key == "active-energy" {
		label = "active energy"
	}
	ev := []Evidence{
		{"last 7 days", fmtN(acute, 0) + " " + unitLabel(unit) + " / day"},
		{"4-week average", fmtN(chronic, 0) + " " + unitLabel(unit) + " / day"},
		{"acute:chronic ratio", fmtN(ratio, 2)},
	}
	switch {
	case ratio >= 1.5:
		sev := SevNotice
		if ratio >= 2.0 {
			sev = SevWarning
		}
		c.add(Observation{
			ID: "load-spike", Category: CatTraining, Severity: sev, Tone: "bad",
			Title:  "Training load jumped this week",
			Detail: fmt.Sprintf("Daily %s over the last 7 days is %.0f%% of the 4-week average (ratio %.2f). Ratios above 1.5 are where overuse niggles and illness tend to show up.", label, ratio*100, ratio),
			Advice: "Big weeks are fine if they are planned; make the next one easier and watch resting heart rate and HRV for the recovery response.",
			Window: "last 7 days vs last 28", Evidence: ev, Link: "activity", Days: len(pts),
		})
	case ratio <= 0.6 && acute >= 0:
		c.add(Observation{
			ID: "load-drop", Category: CatTraining, Severity: SevInfo, Tone: "neutral",
			Title:  "A much quieter week than usual",
			Detail: fmt.Sprintf("Daily %s over the last 7 days is %.0f%% of the 4-week average.", label, ratio*100),
			Advice: "Nothing wrong with a down week. If it was not planned, an easy session will keep the habit alive.",
			Window: "last 7 days vs last 28", Evidence: ev, Link: "activity", Days: len(pts),
		})
	}
	return nil
}

func unitLabel(u string) string {
	switch u {
	case "count/min":
		return "bpm"
	case "count":
		return ""
	}
	return u
}

// ------------------------------------------------------------------ 4. sleep

func checkSleep(c *ctx) error {
	nights, err := c.db.SleepNights(c.date(27), c.date(0))
	if err != nil {
		return err
	}
	var real []storage.SleepNight
	for _, n := range nights {
		if n.Onset != "" {
			real = append(real, n)
		}
	}
	if len(real) < 5 {
		if len(nights) > 0 {
			c.skip("sleep", fmt.Sprintf("needs 5 nights in the last 4 weeks, have %d", len(real)))
		}
		return nil
	}
	c.checked("sleep")

	cut := c.date(7)
	var recent, earlier []storage.SleepNight
	for _, n := range real {
		if n.Night > cut {
			recent = append(recent, n)
		} else {
			earlier = append(earlier, n)
		}
	}

	// Target: the person's own sleep goal if exported, else 7 h.
	target := 7.0
	if lv, _, err := c.db.LatestValue("sleep_duration_goal", c.date(0)); err == nil && lv != nil && lv.V > 3 && lv.V < 12 {
		target = lv.V
	}

	hoursOf := func(ns []storage.SleepNight) []float64 {
		out := make([]float64, len(ns))
		for i, n := range ns {
			out[i] = n.Hours
		}
		return out
	}

	if len(recent) >= 4 {
		short := 0
		debt := 0.0
		for _, n := range recent {
			if n.Hours < 6 {
				short++
			}
			if n.Hours < target {
				debt += target - n.Hours
			}
		}
		avg := mean(hoursOf(recent))
		if short >= 3 {
			c.add(Observation{
				ID: "sleep-short-nights", Category: CatSleep, Severity: SevWarning, Tone: "bad",
				Title:  fmt.Sprintf("%d short nights this week", short),
				Detail: fmt.Sprintf("%d of the last %d recorded nights were under 6 hours; the week averaged %s per night.", short, len(recent), fmtHours(avg)),
				Advice: "Sleep debt accumulates quietly. Protect the next few bedtimes rather than trying to catch up in one night.",
				Window: "last 7 nights", Evidence: []Evidence{{"nights under 6 h", fmt.Sprint(short)}, {"average", fmtHours(avg)}, {"nights recorded", fmt.Sprint(len(recent))}},
				Link: "sleep", Days: len(recent),
			})
		} else if debt >= 3 {
			c.add(Observation{
				ID: "sleep-debt", Category: CatSleep, Severity: SevNotice, Tone: "bad",
				Title:  fmt.Sprintf("About %s of sleep debt this week", fmtHours(debt)),
				Detail: fmt.Sprintf("Across the last %d recorded nights you slept %s less than a %s target in total (average %s).", len(recent), fmtHours(debt), fmtHours(target), fmtHours(avg)),
				Advice: "An earlier bedtime on two or three nights closes most of it.",
				Window: "last 7 nights", Evidence: []Evidence{{"debt vs target", fmtHours(debt)}, {"target", fmtHours(target)}, {"average", fmtHours(avg)}},
				Link: "sleep", Days: len(recent),
			})
		}
		if len(earlier) >= 7 {
			prev := mean(hoursOf(earlier))
			if d := avg - prev; d <= -0.75 {
				c.add(Observation{
					ID: "sleep-drop", Category: CatSleep, Severity: SevNotice, Tone: "bad",
					Title:  "Sleeping less than in previous weeks",
					Detail: fmt.Sprintf("This week averaged %s per night, %s less than the %s of the three weeks before.", fmtHours(avg), fmtHours(-d), fmtHours(prev)),
					Window: "last 7 nights vs previous 21", Evidence: []Evidence{{"this week", fmtHours(avg)}, {"previous 3 weeks", fmtHours(prev)}},
					Link: "sleep", Days: len(real),
				})
			} else if d >= 0.75 {
				c.add(Observation{
					ID: "sleep-gain", Category: CatSleep, Severity: SevInfo, Tone: "good",
					Title:  "Sleeping more than in previous weeks",
					Detail: fmt.Sprintf("This week averaged %s per night, %s more than the %s of the three weeks before.", fmtHours(avg), fmtHours(d), fmtHours(prev)),
					Window: "last 7 nights vs previous 21", Evidence: []Evidence{{"this week", fmtHours(avg)}, {"previous 3 weeks", fmtHours(prev)}},
					Link: "sleep", Days: len(real),
				})
			}
		}
	}

	// Consistency and social jetlag over the whole 4 weeks.
	if len(real) >= 10 {
		var onsets, wd, we []float64
		for _, n := range real {
			m := minutesAfter18(n.Onset)
			onsets = append(onsets, m)
			t, err := time.Parse(day, n.Night)
			if err != nil {
				continue
			}
			// Friday and Saturday nights are the weekend nights.
			if t.Weekday() == time.Friday || t.Weekday() == time.Saturday {
				we = append(we, m)
			} else {
				wd = append(wd, m)
			}
		}
		sd := stddev(onsets)
		if sd >= 60 {
			c.add(Observation{
				ID: "sleep-irregular", Category: CatSleep, Severity: SevNotice, Tone: "bad",
				Title:  "Bedtime varies a lot",
				Detail: fmt.Sprintf("Over the last %d nights your bedtime varied by ±%d minutes (typical bedtime %s).", len(real), int(sd), clock(median(onsets))),
				Advice: "A consistent bedtime does more for sleep quality than an extra hour on some nights. Aim to keep it within about 30 minutes.",
				Window: "last 4 weeks", Evidence: []Evidence{{"bedtime spread", fmt.Sprintf("±%d min", int(sd))}, {"typical bedtime", clock(median(onsets))}},
				Link: "sleep", Days: len(real),
			})
		}
		if len(wd) >= 6 && len(we) >= 3 {
			if shift := median(we) - median(wd); shift >= 75 {
				c.add(Observation{
					ID: "social-jetlag", Category: CatSleep, Severity: SevNotice, Tone: "bad",
					Title:  "Weekend bedtimes are much later",
					Detail: fmt.Sprintf("Friday and Saturday bedtimes are about %d minutes later than weekdays (%s vs %s).", int(shift), clock(median(we)), clock(median(wd))),
					Advice: "A shift of over an hour acts like a small weekly jet lag and makes Monday harder. Keeping the wake time closer helps most.",
					Window: "last 4 weeks", Evidence: []Evidence{{"weekend bedtime", clock(median(we))}, {"weekday bedtime", clock(median(wd))}},
					Link: "sleep", Days: len(real),
				})
			}
		}
	}
	return nil
}

// minutesAfter18 maps "YYYY-MM-DD HH:MM" to minutes after 18:00 so bedtimes
// around midnight sort sensibly.
func minutesAfter18(ts string) float64 {
	if len(ts) < 16 {
		return 0
	}
	var h, m int
	fmt.Sscanf(ts[11:16], "%d:%d", &h, &m)
	mins := h*60 + m - 18*60
	if mins < 0 {
		mins += 24 * 60
	}
	return float64(mins)
}

func clock(minsAfter18 float64) string {
	total := (int(math.Round(minsAfter18)) + 18*60) % (24 * 60)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// -------------------------------------------------------- 5. activity habits

func checkActivityHabits(c *ctx) error {
	// Steps: this week vs the 4 weeks before.
	pts, _, err := c.series("steps", 34)
	if err != nil {
		return err
	}
	base, recent := split(pts, c.date(7))
	if len(recent) >= 4 && len(base) >= 14 {
		c.checked("steps-trend")
		a, b := mean(values(recent)), mean(values(base))
		if b > 0 {
			ch := (a - b) / b
			switch {
			case ch <= -0.25:
				c.add(Observation{
					ID: "steps-drop", Category: CatActivity, Severity: SevNotice, Tone: "bad",
					Title:  "Walking a lot less this week",
					Detail: fmt.Sprintf("%s steps a day over the last %d days, %.0f%% below the %s of the previous four weeks.", fmtN(a, 0), len(recent), -ch*100, fmtN(b, 0)),
					Advice: "If this was not a rest week, a daily walk is the easiest way back.",
					Window: "last 7 days vs previous 28", Evidence: []Evidence{{"this week", fmtN(a, 0) + " / day"}, {"previous 4 weeks", fmtN(b, 0) + " / day"}},
					Link: "activity", Days: len(pts),
				})
			case ch >= 0.25:
				c.add(Observation{
					ID: "steps-rise", Category: CatActivity, Severity: SevInfo, Tone: "good",
					Title:  "Walking a lot more this week",
					Detail: fmt.Sprintf("%s steps a day over the last %d days, %.0f%% above the %s of the previous four weeks.", fmtN(a, 0), len(recent), ch*100, fmtN(b, 0)),
					Window: "last 7 days vs previous 28", Evidence: []Evidence{{"this week", fmtN(a, 0) + " / day"}, {"previous 4 weeks", fmtN(b, 0) + " / day"}},
					Link: "activity", Days: len(pts),
				})
			}
		}
		// Longest recent stretch of very low days (with data).
		run, best := 0, 0
		for _, p := range recent {
			if p.V < 3000 {
				run++
				if run > best {
					best = run
				}
			} else {
				run = 0
			}
		}
		if best >= 3 {
			c.add(Observation{
				ID: "inactive-stretch", Category: CatActivity, Severity: SevNotice, Tone: "bad",
				Title:  fmt.Sprintf("%d consecutive days under 3,000 steps", best),
				Detail: "Days with fewer than 3,000 steps count as sedentary in most guidelines; several in a row is worth noticing.",
				Advice: "Even a 20-minute walk changes the picture.",
				Window: "last 7 days", Evidence: []Evidence{{"consecutive low days", fmt.Sprint(best)}}, Link: "activity", Days: len(recent),
			})
		}
	} else if len(pts) > 0 {
		c.skip("steps-trend", fmt.Sprintf("needs 4 recent and 14 earlier days, have %d and %d", len(recent), len(base)))
	}

	// Rings: closure rate this 4 weeks vs previous 4, and current streak.
	rings, err := c.db.ActivityRings(c.date(55), c.date(0))
	if err != nil {
		return err
	}
	if len(rings.Days) >= 14 {
		c.checked("rings")
		cut := c.date(28)
		var cur, prev []storage.ActivityDay
		for _, d := range rings.Days {
			if d.Date > cut {
				cur = append(cur, d)
			} else {
				prev = append(prev, d)
			}
		}
		rate := func(ds []storage.ActivityDay) float64 {
			if len(ds) == 0 {
				return 0
			}
			n := 0
			for _, d := range ds {
				if d.Closed.All {
					n++
				}
			}
			return float64(n) / float64(len(ds))
		}
		if len(cur) >= 10 && len(prev) >= 10 {
			rc, rp := rate(cur), rate(prev)
			if rp-rc >= 0.25 {
				c.add(Observation{
					ID: "rings-slipping", Category: CatActivity, Severity: SevNotice, Tone: "bad",
					Title:  "Closing the rings less often",
					Detail: fmt.Sprintf("All three rings closed on %.0f%% of days this month against %.0f%% the month before (%d and %d days with data).", rc*100, rp*100, len(cur), len(prev)),
					Advice: "If the Move goal no longer fits your routine, lowering it beats ignoring it.",
					Window: "last 28 days vs previous 28", Evidence: []Evidence{{"this month", fmt.Sprintf("%.0f%%", rc*100)}, {"last month", fmt.Sprintf("%.0f%%", rp*100)}},
					Link: "activity", Days: len(rings.Days),
				})
			} else if rc-rp >= 0.25 {
				c.add(Observation{
					ID: "rings-improving", Category: CatActivity, Severity: SevInfo, Tone: "good",
					Title:  "Closing the rings more often",
					Detail: fmt.Sprintf("All three rings closed on %.0f%% of days this month against %.0f%% the month before.", rc*100, rp*100),
					Window: "last 28 days vs previous 28", Evidence: []Evidence{{"this month", fmt.Sprintf("%.0f%%", rc*100)}, {"last month", fmt.Sprintf("%.0f%%", rp*100)}},
					Link: "activity", Days: len(rings.Days),
				})
			}
		}
		if last := rings.Days[len(rings.Days)-1]; last.Date == c.date(0) || last.Date == c.date(1) {
			if s := rings.Streaks.CurrentAll; s >= 5 {
				c.add(Observation{
					ID: "rings-streak", Category: CatActivity, Severity: SevInfo, Tone: "good",
					Title:  fmt.Sprintf("%d-day streak of closing all rings", s),
					Detail: "Consistency is the whole game; this is what it looks like.",
					Window: "current", Evidence: []Evidence{{"streak", fmt.Sprintf("%d days", s)}, {"longest in 8 weeks", fmt.Sprintf("%d days", rings.Streaks.LongestAll)}},
					Link: "activity", Days: s,
				})
			}
		}
	}
	return nil
}

// ---------------------------------------------------------- 6. fitness trends

func checkFitnessTrends(c *ctx) error {
	// VO2max over 90 days.
	first, err := c.db.FirstValue("vo2_max", c.date(90))
	if err != nil {
		return err
	}
	last, _, err := c.db.LatestValue("vo2_max", c.date(0))
	if err != nil {
		return err
	}
	if first != nil && last != nil && last.T > first.T && last.T >= c.date(90) {
		c.checked("vo2max-trend")
		d := last.V - first.V
		if math.Abs(d) >= 1.0 {
			tone, sev, dir := "good", SevInfo, "improved"
			if d < 0 {
				tone, sev, dir = "bad", SevNotice, "declined"
			}
			c.add(Observation{
				ID: "vo2max-" + dir, Category: CatFitness, Severity: sev, Tone: tone,
				Title:  "Cardio fitness has " + dir,
				Detail: fmt.Sprintf("Estimated VO₂ max went from %.1f to %.1f mL/kg·min between %s and %s.", first.V, last.V, first.T, last.T),
				Advice: map[string]string{"improved": "Regular sessions in zone 2 plus the occasional hard effort are what move this number.", "declined": "VO₂ max estimates need outdoor walks or runs with GPS to update; a decline can also just mean fewer qualifying workouts."}[dir],
				Window: "last 90 days", Evidence: []Evidence{{"first", fmtN(first.V, 1)}, {"latest", fmtN(last.V, 1)}, {"change", fmtSigned(d, 1)}},
				Link: "heart", Days: 2,
			})
		}
	}

	// Resting HR 90-day trend line.
	pts, _, err := c.series("resting-heart-rate", 89)
	if err != nil {
		return err
	}
	if len(pts) >= 45 {
		c.checked("rhr-trend")
		change := slopeChange(values(pts))
		if math.Abs(change) >= 3 {
			tone, sev, dir := "good", SevInfo, "falling"
			if change > 0 {
				tone, sev, dir = "bad", SevNotice, "rising"
			}
			c.add(Observation{
				ID: "rhr-trend-" + dir, Category: CatFitness, Severity: sev, Tone: tone,
				Title:  "Resting heart rate is " + dir + " over the quarter",
				Detail: fmt.Sprintf("The trend across the last %d days with data amounts to %s bpm.", len(pts), fmtSigned(change, 1)),
				Advice: map[string]string{"falling": "A falling resting heart rate over months is one of the most reliable signs of improving aerobic fitness.", "rising": "A slow rise over months can come from less training, more stress, weight gain or poorer sleep. Worth watching alongside HRV."}[dir],
				Window: "last 90 days", Evidence: []Evidence{{"trend", fmtSigned(change, 1) + " bpm"}, {"days with data", fmt.Sprint(len(pts))}},
				Link: "heart", Days: len(pts),
			})
		}
	}
	return nil
}

func slopeChange(ys []float64) float64 {
	n := float64(len(ys))
	var sx, sy, sxx, sxy float64
	for i, y := range ys {
		x := float64(i)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den * (n - 1)
}

// -------------------------------------------------------------- 7. red flags

func checkRedFlags(c *ctx) error {
	c.checked("heart-rhythm")
	type ev struct {
		table, kind string
	}
	counts := map[string]int{}
	total := 0
	for _, e := range []ev{{"irregular_rhythm_events", "irregular rhythm"}, {"high_heart_rate_events", "high heart rate"}, {"low_heart_rate_events", "low heart rate"}} {
		rows, err := c.db.EventRows(e.table, c.date(30), c.date(0))
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			counts[e.kind] = len(rows)
			total += len(rows)
		}
	}
	if total > 0 {
		var parts []string
		for k, n := range counts {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
		sort.Strings(parts)
		sev := SevWarning
		if counts["irregular rhythm"] > 0 {
			sev = SevAlert
		}
		c.add(Observation{
			ID: "heart-notifications", Category: CatHeart, Severity: sev, Tone: "bad",
			Title:  "Heart rhythm notifications in the last 30 days",
			Detail: "The watch raised " + strings.Join(parts, ", ") + " notification(s). Low heart rate alerts during sleep are common in fit people; irregular rhythm alerts are not.",
			Advice: "Notifications are screening, not diagnosis. If irregular rhythm alerts repeat, or any came with symptoms, take the ECG recordings to a doctor.",
			Window: "last 30 days", Evidence: []Evidence{{"notifications", strings.Join(parts, ", ")}}, Link: "heart",
		})
	}

	c.checked("ecg-classification")
	ecgs, err := c.db.ListECG(c.date(90), c.date(0))
	if err != nil {
		return err
	}
	abnormal := map[string]int{}
	for _, e := range ecgs {
		l := strings.ToLower(e.Classification)
		if strings.Contains(l, "fibrillation") || strings.Contains(l, "high heart rate") || strings.Contains(l, "low heart rate") {
			abnormal[e.Classification]++
		}
	}
	if len(abnormal) > 0 {
		var parts []string
		for k, n := range abnormal {
			parts = append(parts, fmt.Sprintf("%d × %s", n, k))
		}
		sort.Strings(parts)
		sev := SevWarning
		for k := range abnormal {
			if strings.Contains(strings.ToLower(k), "fibrillation") {
				sev = SevAlert
			}
		}
		c.add(Observation{
			ID: "ecg-abnormal", Category: CatHeart, Severity: sev, Tone: "bad",
			Title:  "ECG recordings with a non-sinus classification",
			Detail: fmt.Sprintf("Of %d recordings in the last 90 days: %s.", len(ecgs), strings.Join(parts, ", ")),
			Advice: "An Atrial Fibrillation classification should be reviewed by a doctor; the recordings can be exported as CSV from the ECG page.",
			Window: "last 90 days", Evidence: []Evidence{{"recordings", fmt.Sprint(len(ecgs))}, {"flagged", strings.Join(parts, ", ")}}, Link: "ecg",
		})
	}

	// Overnight SpO2 dips.
	nights, err := c.db.SleepNights(c.date(13), c.date(0))
	if err != nil {
		return err
	}
	low := 0
	withSpO2 := 0
	for _, n := range nights {
		if n.Vitals.SpO2Min == nil {
			continue
		}
		withSpO2++
		if *n.Vitals.SpO2Min < 90 {
			low++
		}
	}
	if withSpO2 >= 5 {
		c.checked("spo2-dips")
		if low >= 3 {
			c.add(Observation{
				ID: "spo2-dips", Category: CatHeart, Severity: SevWarning, Tone: "bad",
				Title:  fmt.Sprintf("Blood oxygen dipped below 90%% on %d nights", low),
				Detail: fmt.Sprintf("%d of the last %d nights with readings had a minimum below 90%%. Single low readings are usually sleep position or a loose strap; a pattern is worth a conversation with a doctor, especially with snoring or daytime tiredness.", low, withSpO2),
				Window: "last 14 nights", Evidence: []Evidence{{"nights below 90%", fmt.Sprint(low)}, {"nights with readings", fmt.Sprint(withSpO2)}}, Link: "sleep", Days: withSpO2,
			})
		}
	}
	return nil
}

// ---------------------------------------------------------------- 8. hearing

func checkHearing(c *ctx) error {
	pts, _, err := c.series("headphone-audio-exposure", 6)
	if err != nil {
		return err
	}
	if len(pts) < 3 {
		return nil
	}
	c.checked("headphone-exposure")
	// Energy-average the daily means (dB is logarithmic).
	sum := 0.0
	for _, p := range pts {
		sum += math.Pow(10, p.V/10)
	}
	leq := 10 * math.Log10(sum/float64(len(pts)))
	if leq >= 75 {
		sev := SevNotice
		if leq >= 80 {
			sev = SevWarning
		}
		c.add(Observation{
			ID: "headphones-loud", Category: CatHearing, Severity: sev, Tone: "bad",
			Title:  "Headphone volume is on the loud side",
			Detail: fmt.Sprintf("The last %d days with headphone use averaged %.0f dB. The WHO safe-listening guide is 80 dB for up to 40 hours a week; every 3 dB above halves the safe time.", len(pts), leq),
			Advice: "Turning the volume down a couple of notches, or using noise-cancelling headphones in loud places, keeps you well inside the limit.",
			Window: "last 7 days", Evidence: []Evidence{{"average level", fmt.Sprintf("%.0f dB", leq)}, {"days with headphone use", fmt.Sprint(len(pts))}}, Link: "environment", Days: len(pts),
		})
	}
	return nil
}

// ------------------------------------------------------------------- 9. body

func checkBody(c *ctx) error {
	first, err := c.db.FirstValue("body_mass", c.date(28))
	if err != nil {
		return err
	}
	last, unit, err := c.db.LatestValue("body_mass", c.date(0))
	if err != nil {
		return err
	}
	if first == nil || last == nil || last.T <= first.T || last.T < c.date(28) {
		return nil
	}
	c.checked("weight-change")
	d := last.V - first.V
	if math.Abs(d) >= 2 {
		dir := "up"
		if d < 0 {
			dir = "down"
		}
		c.add(Observation{
			ID: "weight-" + dir, Category: CatBody, Severity: SevNotice, Tone: "neutral",
			Title:  fmt.Sprintf("Weight is %s %.1f %s this month", dir, math.Abs(d), unit),
			Detail: fmt.Sprintf("%.1f %s on %s to %.1f %s on %s.", first.V, unit, first.T, last.V, unit, last.T),
			Advice: "Whether that is good news depends on your goals; day-to-day readings swing by a kilo with hydration, so judge by the weekly trend.",
			Window: "last 28 days", Evidence: []Evidence{{"first", fmtN(first.V, 1) + " " + unit}, {"latest", fmtN(last.V, 1) + " " + unit}}, Link: "body", Days: 2,
		})
	}
	return nil
}

// ----------------------------------------------------------- 10. data quality

func checkDataQuality(c *ctx) error {
	c.checked("data-quality")
	nights, err := c.db.SleepNights(c.date(13), c.date(0))
	if err != nil {
		return err
	}
	recorded := 0
	for _, n := range nights {
		if n.Onset != "" {
			recorded++
		}
	}
	if missing := 14 - recorded; missing >= 4 && recorded > 0 {
		c.add(Observation{
			ID: "watch-not-worn", Category: CatData, Severity: SevInfo, Tone: "neutral",
			Title:  fmt.Sprintf("No sleep data on %d of the last 14 nights", missing),
			Detail: "Recovery and sleep observations are only as good as the nights recorded. Missing nights are treated as unknown, never as zero.",
			Advice: "Wearing the watch to bed (and charging it in the morning) fills the gaps.",
			Window: "last 14 nights", Evidence: []Evidence{{"nights recorded", fmt.Sprint(recorded)}}, Link: "sleep", Days: recorded,
		})
	}
	if age := int(time.Since(c.asOf).Hours() / 24); age >= 10 {
		c.add(Observation{
			ID: "stale-export", Category: CatData, Severity: SevInfo, Tone: "neutral",
			Title:  fmt.Sprintf("Data is %d days old", age),
			Detail: fmt.Sprintf("The latest record is from %s. Observations describe that point in time.", c.asOf.Format(day)),
			Advice: "Export from the Health app and upload again to bring everything up to date.",
			Window: "as of " + c.asOf.Format(day), Link: "people",
		})
	}
	return nil
}
