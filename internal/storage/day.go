package storage

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/BRO3886/healthsync/internal/hk"
)

// IntradayPoint is a 10-minute bucket of a sample metric within one day.
type IntradayPoint struct {
	T   string  `json:"t"` // "YYYY-MM-DD HH:M0"
	V   float64 `json:"v"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	N   int64   `json:"n"`
}

// Intraday averages a sample table into 10-minute buckets for one day. Empty
// buckets are absent, never bridged.
func (db *DB) Intraday(table, date string) ([]IntradayPoint, error) {
	m, ok := hk.ByTable[table]
	if !ok || m.Agg != hk.Sample {
		return nil, fmt.Errorf("%s is not a sample metric", table)
	}
	// substr(…, 1, 15) keeps "YYYY-MM-DD HH:M": the tens digit of the minute.
	q := fmt.Sprintf(`SELECT substr(start_date, 1, 15) AS b, AVG(value), MIN(value), MAX(value), COUNT(*)
		FROM %s WHERE start_date >= ? AND start_date < date(?, '+1 day') GROUP BY b ORDER BY b`, table)
	rows, err := db.conn.Query(q, date, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IntradayPoint{}
	for rows.Next() {
		var p IntradayPoint
		if err := rows.Scan(&p.T, &p.V, &p.Min, &p.Max, &p.N); err != nil {
			return nil, err
		}
		p.T += "0"
		out = append(out, p)
	}
	return out, rows.Err()
}

// DayEvent is one timestamped occurrence on a day: a category event (rhythm
// notification, symptom, noise alert…) or an ECG recording.
type DayEvent struct {
	Time  string `json:"time"` // "YYYY-MM-DD HH:MM:SS"
	Kind  string `json:"kind"` // metric key, "ecg", or the raw HK identifier
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Link  string `json:"link,omitempty"` // page path relative to the person
}

// DayEvents lists every event-type record and ECG on a day, in clock order.
// Stand hours are left out: they are a daily tally, not an event.
func (db *DB) DayEvents(date string) ([]DayEvent, error) {
	out := []DayEvent{}
	scan := func(q string, args []any, f func(start, typ, value string)) error {
		rows, err := db.conn.Query(q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var start, typ, value string
			if err := rows.Scan(&start, &typ, &value); err != nil {
				return err
			}
			f(start, typ, value)
		}
		return rows.Err()
	}
	const where = ` WHERE start_date >= ? AND start_date < date(?, '+1 day')`
	for _, m := range hk.Metrics {
		if m.Agg != hk.Event || m.Key == "stand-hours" {
			continue
		}
		m := m
		err := scan(`SELECT start_date, '', COALESCE(value, '') FROM `+m.Table+where, []any{date, date}, func(start, _, value string) {
			out = append(out, DayEvent{Time: start, Kind: m.Key, Name: m.Name, Value: categoryLabel(value), Link: eventLink(m.Group)})
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Table, err)
		}
	}
	err := scan(`SELECT start_date, type, COALESCE(value, '') FROM `+hk.OtherCategoryTable+where, []any{date, date}, func(start, typ, value string) {
		out = append(out, DayEvent{Time: start, Kind: typ, Name: identifierLabel(typ), Value: categoryLabel(value), Link: "explore"})
	})
	if err != nil {
		return nil, err
	}
	ecgs, err := db.ListECG(date, date)
	if err != nil {
		return nil, err
	}
	for _, e := range ecgs {
		out = append(out, DayEvent{Time: e.RecordedDate, Kind: "ecg", Name: "ECG", Value: identifierLabel(e.Classification), Link: fmt.Sprintf("ecg/%d", e.ID)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, nil
}

func eventLink(group string) string {
	switch group {
	case hk.GroupHeart:
		return "heart"
	case hk.GroupHearing:
		return "environment"
	}
	return "explore"
}

// identifierLabel turns "HKCategoryTypeIdentifierRunnyNose" or
// "SinusRhythm" into "Runny nose" / "Sinus rhythm".
func identifierLabel(s string) string {
	for _, p := range []string{"HKCategoryTypeIdentifier", "HKQuantityTypeIdentifier", "HKElectrocardiogramClassification", "HKCategoryValue"} {
		s = strings.TrimPrefix(s, p)
	}
	var b strings.Builder
	prev := ' '
	for _, r := range s {
		if unicode.IsUpper(r) && b.Len() > 0 {
			if prev != ' ' {
				b.WriteByte(' ')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// categoryLabel renders a category value, dropping the meaningless
// "not applicable" most event types carry.
func categoryLabel(v string) string {
	if v == "" || strings.HasSuffix(v, "NotApplicable") {
		return ""
	}
	if !strings.HasPrefix(v, "HKCategoryValue") {
		return v
	}
	return identifierLabel(v)
}
