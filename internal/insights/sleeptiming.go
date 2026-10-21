package insights

import (
	"fmt"
	"math"
	"time"

	"github.com/BRO3886/healthsync/internal/storage"
)

// SleepDebt is the running balance of sleep owed over the last five nights.
type SleepDebt struct {
	Minutes  float64 `json:"minutes"`
	Nights   int     `json:"nights"` // tracked nights in the window
	Learning bool    `json:"learning"`
	Need     float64 `json:"need_hours"`
	From     string  `json:"from"`
	To       string  `json:"to"`
}

// Chronotype is the MCTQ mid-sleep on free days, corrected for catch-up sleep.
type Chronotype struct {
	MSF          string  `json:"msf"`   // mid-sleep on free days, HH:MM
	MSFsc        string  `json:"msfsc"` // sleep-corrected
	MSW          string  `json:"msw"`   // mid-sleep on work days
	Label        string  `json:"label"`
	SocialJetlag float64 `json:"social_jetlag_min"`
	FreeNights   int     `json:"free_nights"`
	WorkNights   int     `json:"work_nights"`
}

// SleepTiming bundles debt and chronotype as of a day.
type SleepTiming struct {
	Debt             SleepDebt   `json:"debt"`
	Chronotype       *Chronotype `json:"chronotype,omitempty"`
	ChronotypeReason string      `json:"chronotype_reason,omitempty"`
	Window           string      `json:"chronotype_window"`
	NeedSource       string      `json:"need_source"`
}

const (
	debtNights       = 5
	debtMinNights    = 4
	debtNightCapMin  = 180
	debtCapMin       = 600
	chronoWeeks      = 8
	chronoMinFree    = 4
	chronoMinWork    = 8
	minutesPerDay    = 24 * 60
	freeNightWeekday = time.Friday // Friday and Saturday nights are free nights
)

// ComputeSleepTiming returns the sleep debt over the five mornings ending at
// `to` and the chronotype over the eight weeks ending at `to`.
//
// Debt: a night below need adds its shortfall (at most 3 h), a night above
// need pays the balance down at half its surplus; the balance never goes
// below zero and is capped at 10 h. Fewer than four tracked nights is
// "still learning".
//
// Chronotype (Munich ChronoType Questionnaire): MSF is the circular mean of
// free-night mid-sleep; if free nights are longer than the weekly average
// (5 work + 2 free), MSFsc = MSF − ½·(SDf − SDweek). Social jetlag is
// |MSF − MSW|. Without alarm data, Friday and Saturday nights are the free
// nights.
func ComputeSleepTiming(db *storage.DB, to string, need float64) (*SleepTiming, error) {
	end, err := time.Parse(day, to)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q", to)
	}
	from := end.AddDate(0, 0, -(chronoWeeks*7 - 1))
	all, err := db.SleepNights(from.AddDate(0, 0, -1).Format(day), to)
	if err != nil {
		return nil, err
	}
	var nights []storage.SleepNight
	for _, n := range all {
		if n.Onset != "" && n.Wake[:10] >= from.Format(day) && n.Wake[:10] <= to {
			nights = append(nights, n)
		}
	}
	out := &SleepTiming{Window: fmt.Sprintf("%s – %s", from.Format(day), to)}

	// Debt over the last five mornings, oldest first.
	debtFrom := end.AddDate(0, 0, -(debtNights - 1)).Format(day)
	out.Debt = SleepDebt{Need: need, From: debtFrom, To: to}
	bal := 0.0
	for _, n := range nights {
		if n.Wake[:10] < debtFrom {
			continue
		}
		out.Debt.Nights++
		diff := (n.Hours - need) * 60
		if diff < 0 {
			bal += math.Min(-diff, debtNightCapMin)
		} else {
			bal = math.Max(0, bal-diff/2)
		}
		bal = math.Min(bal, debtCapMin)
	}
	out.Debt.Minutes = math.Round(bal)
	out.Debt.Learning = out.Debt.Nights < debtMinNights

	var freeMid, workMid, freeDur, workDur []float64
	for _, n := range nights {
		on, err1 := time.Parse("2006-01-02 15:04", n.Onset)
		wk, err2 := time.Parse("2006-01-02 15:04", n.Wake)
		if err1 != nil || err2 != nil || !wk.After(on) {
			continue
		}
		dur := wk.Sub(on)
		mid := on.Add(dur / 2)
		m := float64(mid.Hour()*60 + mid.Minute())
		wd := mustDate(n.Night).Weekday()
		if wd == freeNightWeekday || wd == freeNightWeekday+1 {
			freeMid, freeDur = append(freeMid, m), append(freeDur, dur.Hours())
		} else {
			workMid, workDur = append(workMid, m), append(workDur, dur.Hours())
		}
	}
	if len(freeMid) < chronoMinFree || len(workMid) < chronoMinWork {
		out.ChronotypeReason = fmt.Sprintf("needs %d weekend and %d weekday nights in the last %d weeks, has %d and %d",
			chronoMinFree, chronoMinWork, chronoWeeks, len(freeMid), len(workMid))
		return out, nil
	}
	msf, msw := circMean(freeMid), circMean(workMid)
	sdf, sdw := mean(freeDur), mean(workDur)
	sdWeek := (5*sdw + 2*sdf) / 7
	msfsc := msf
	if sdf > sdWeek {
		msfsc = math.Mod(msf-(sdf-sdWeek)*60/2+minutesPerDay, minutesPerDay)
	}
	out.Chronotype = &Chronotype{
		MSF: clockOf(msf), MSFsc: clockOf(msfsc), MSW: clockOf(msw), Label: chronotypeLabel(msfsc),
		SocialJetlag: math.Round(math.Abs(circDiff(msf, msw))),
		FreeNights:   len(freeMid), WorkNights: len(workMid),
	}
	return out, nil
}

// circMean averages minutes-of-day on a circle, so 23:30 and 00:30 give 00:00.
func circMean(mins []float64) float64 {
	var s, c float64
	for _, m := range mins {
		a := m / minutesPerDay * 2 * math.Pi
		s += math.Sin(a)
		c += math.Cos(a)
	}
	a := math.Atan2(s, c)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a / (2 * math.Pi) * minutesPerDay
}

// circDiff is the signed shortest distance a−b in minutes.
func circDiff(a, b float64) float64 {
	d := math.Mod(a-b+minutesPerDay*1.5, minutesPerDay) - minutesPerDay/2
	return d
}

func clockOf(mins float64) string {
	t := int(math.Round(mins)) % minutesPerDay
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

// chronotypeLabel buckets MSFsc around the adult population mode (~04:00) in
// one-hour steps.
func chronotypeLabel(msfsc float64) string {
	h := msfsc / 60
	if h >= 12 {
		h -= 24 // evening mid-sleep is earlier than any morning one
	}
	switch {
	case h < 2.5:
		return "very early"
	case h < 3.5:
		return "early"
	case h < 4.5:
		return "intermediate"
	case h < 5.5:
		return "late"
	}
	return "very late"
}
