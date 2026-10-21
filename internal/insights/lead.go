package insights

import (
	"fmt"
	"math"
	"strings"

	"github.com/BRO3886/healthsync/internal/storage"
)

// lead is the one-line read at the top of the Overview, built from the
// report and last night's sleep with fixed rules (no model):
//
//  1. a red flag, else a recovery marker outside the person's range;
//  2. last night at least an hour off their usual;
//  3. markers within range and/or an ordinary night (within 20 minutes).
//
// Empty when nothing qualifies. Must run after every detector.
func lead(c *ctx) (string, error) {
	for _, o := range c.rep.Observations {
		if o.Severity == SevAlert || (o.Category == CatRecovery && o.Tone == "bad") {
			return o.Title + ". " + o.Detail, nil
		}
	}

	nights, err := c.db.SleepNights(c.date(29), c.date(0))
	if err != nil {
		return "", err
	}
	var last *storage.SleepNight
	var earlier []float64
	for i := range nights {
		n := nights[i]
		if n.Onset == "" {
			continue
		}
		if n.Wake[:10] == c.date(0) {
			last = &nights[i]
		} else {
			earlier = append(earlier, n.Hours)
		}
	}
	night := ""
	if last != nil && len(earlier) >= 7 {
		usual := median(earlier)
		d := last.Hours - usual
		if math.Abs(d) >= 1 {
			more := "less"
			if d > 0 {
				more = "more"
			}
			return fmt.Sprintf("Last night: %s of sleep, %s %s than your usual %s.", fmtHours(last.Hours), fmtHours(math.Abs(d)), more, fmtHours(usual)), nil
		}
		if math.Abs(d) <= 1.0/3 {
			night = fmt.Sprintf("last night was an ordinary night (%s)", fmtHours(last.Hours))
		}
	}

	var inRange []string
	for _, ck := range c.rep.Checked {
		key, ok := strings.CutPrefix(ck, "baseline:")
		if !ok {
			continue
		}
		for _, b := range baselines {
			if b.key == key {
				inRange = append(inRange, strings.ToLower(b.name))
			}
		}
	}
	switch {
	case len(inRange) > 0 && night != "":
		return fmt.Sprintf("%s within your usual range, and %s.", capitalize(joinAnd(inRange)), night), nil
	case len(inRange) > 0:
		return capitalize(joinAnd(inRange)) + " within your usual range.", nil
	case night != "":
		return capitalize(night) + ".", nil
	}
	return "", nil
}

func joinAnd(xs []string) string {
	if len(xs) == 1 {
		return xs[0] + " is"
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1] + " are"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
