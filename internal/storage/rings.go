package storage

import (
	"database/sql"
	"sort"
)

// ActivityDay is one row of activity_summary with ring closure flags.
type ActivityDay struct {
	Date         string   `json:"date"`
	Energy       *float64 `json:"energy"`
	EnergyGoal   *float64 `json:"energy_goal"`
	EnergyUnit   string   `json:"energy_unit"`
	Exercise     *float64 `json:"exercise"`
	ExerciseGoal *float64 `json:"exercise_goal"`
	Stand        *float64 `json:"stand"`
	StandGoal    *float64 `json:"stand_goal"`
	Move         *float64 `json:"move"`
	MoveGoal     *float64 `json:"move_goal"`
	Closed       struct {
		Energy   bool `json:"energy"`
		Exercise bool `json:"exercise"`
		Stand    bool `json:"stand"`
		All      bool `json:"all"`
	} `json:"closed"`
}

// Streaks summarises consecutive ring closures.
type Streaks struct {
	CurrentAll  int `json:"current_all"`
	LongestAll  int `json:"longest_all"`
	CurrentMove int `json:"current_energy"`
	LongestMove int `json:"longest_energy"`
	ClosedAll   int `json:"days_all_closed"`
	Days        int `json:"days"`
}

// Rings is the activity rings payload.
type Rings struct {
	Days    []ActivityDay `json:"days"`
	Streaks Streaks       `json:"streaks"`
}

// ActivityRings returns activity_summary rows in range plus streak stats.
// Streaks are computed over consecutive calendar days present in the range;
// a day with no row breaks the streak (the watch was not worn, and we do not
// invent a closed ring).
func (db *DB) ActivityRings(from, to string) (*Rings, error) {
	q := `SELECT date, active_energy, active_energy_goal, active_energy_unit, move_time, move_time_goal, exercise_time, exercise_time_goal, stand_hours, stand_hours_goal FROM activity_summary WHERE 1=1`
	var args []any
	if from != "" {
		q += " AND date >= ?"
		args = append(args, from[:min(len(from), 10)])
	}
	if to != "" {
		q += " AND date <= ?"
		args = append(args, to[:min(len(to), 10)])
	}
	q += " ORDER BY date"
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &Rings{}
	for rows.Next() {
		var d ActivityDay
		var unit sql.NullString
		var e, eg, mv, mvg, ex, exg, st, stg sql.NullFloat64
		if err := rows.Scan(&d.Date, &e, &eg, &unit, &mv, &mvg, &ex, &exg, &st, &stg); err != nil {
			return nil, err
		}
		d.Energy, d.EnergyGoal = nf(e), nf(eg)
		d.Move, d.MoveGoal = nf(mv), nf(mvg)
		d.Exercise, d.ExerciseGoal = nf(ex), nf(exg)
		d.Stand, d.StandGoal = nf(st), nf(stg)
		d.EnergyUnit = unit.String
		d.Closed.Energy = closed(d.Energy, d.EnergyGoal)
		d.Closed.Exercise = closed(d.Exercise, d.ExerciseGoal)
		d.Closed.Stand = closed(d.Stand, d.StandGoal)
		d.Closed.All = d.Closed.Energy && d.Closed.Exercise && d.Closed.Stand
		out.Days = append(out.Days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out.Days, func(i, j int) bool { return out.Days[i].Date < out.Days[j].Date })
	out.Streaks = computeStreaks(out.Days)
	return out, nil
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

// closed reports whether a ring is closed. A zero goal means the ring was not
// configured that day (early Apple Watch setups), which is not a closure.
func closed(v, goal *float64) bool {
	return v != nil && goal != nil && *goal > 0 && *v >= *goal
}

func computeStreaks(days []ActivityDay) Streaks {
	var s Streaks
	s.Days = len(days)
	var curAll, curMove int
	prevDate := ""
	for _, d := range days {
		consecutive := prevDate != "" && nextDay(prevDate) == d.Date
		if !consecutive {
			curAll, curMove = 0, 0
		}
		if d.Closed.All {
			curAll++
			s.ClosedAll++
		} else {
			curAll = 0
		}
		if d.Closed.Energy {
			curMove++
		} else {
			curMove = 0
		}
		if curAll > s.LongestAll {
			s.LongestAll = curAll
		}
		if curMove > s.LongestMove {
			s.LongestMove = curMove
		}
		prevDate = d.Date
	}
	s.CurrentAll, s.CurrentMove = curAll, curMove
	return s
}

func nextDay(d string) string {
	t, err := parseDay(d)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format(dateLayout)
}
