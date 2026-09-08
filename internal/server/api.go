package server

import (
	"encoding/csv"
	"fmt"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/insights"
	"github.com/BRO3886/healthsync/internal/parser"
	"github.com/BRO3886/healthsync/internal/storage"
)

// --- shared query parsing ---

func queryInt(r *http.Request, key string, def, max int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

// period reads from/to (YYYY-MM-DD, inclusive). Both must be present and
// valid for endpoints that compute previous-period comparisons.
func period(r *http.Request) (storage.Period, error) {
	q := r.URL.Query()
	p := storage.Period{From: q.Get("from"), To: q.Get("to")}
	for _, d := range []string{p.From, p.To} {
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return p, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", d)
		}
	}
	if p.From != "" && p.To != "" && p.From > p.To {
		return p, fmt.Errorf("from is after to")
	}
	return p, nil
}

func requirePeriod(w http.ResponseWriter, r *http.Request) (storage.Period, bool) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return p, false
	}
	if p.From == "" || p.To == "" {
		writeError(w, http.StatusBadRequest, "from and to are required (YYYY-MM-DD)")
		return p, false
	}
	return p, true
}

// --- registry ---

type metricView struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Unit  string `json:"unit"`
	Group string `json:"group"`
	Kind  string `json:"kind"`
	Table string `json:"table"`
	Good  string `json:"good,omitempty"`
}

func (h *handlers) handleMetrics(w http.ResponseWriter, r *http.Request) {
	out := make([]metricView, 0, len(hk.Metrics))
	for _, m := range hk.Metrics {
		if m.Paired {
			continue
		}
		out = append(out, metricView{Key: m.Key, Name: m.Name, Unit: m.Unit, Group: m.Group, Kind: m.Agg.String(), Table: m.Table, Good: m.Good})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- per-person data ---

func (h *handlers) handleAvailability(w http.ResponseWriter, r *http.Request) {
	av, err := dbFrom(r).Availability()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, av)
}

func (h *handlers) handleProfile(w http.ResponseWriter, r *http.Request) {
	prof, err := dbFrom(r).Profile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (h *handlers) handleSeries(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	bucket, err := storage.ParseBucket(r.URL.Query().Get("bucket"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := chi.URLParam(r, "metric")
	db := dbFrom(r)
	var s *storage.Series
	if strings.HasPrefix(key, "other:") {
		s, err = db.QueryOtherSeries(strings.TrimPrefix(key, "other:"), p.From, p.To, bucket)
	} else {
		m, ok := hk.Lookup(key)
		if !ok || m.Paired {
			writeError(w, http.StatusNotFound, "unknown metric %q", key)
			return
		}
		s, err = db.QuerySeries(m, p.From, p.To, bucket)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) handleSummary(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePeriod(w, r)
	if !ok {
		return
	}
	compare := r.URL.Query().Get("compare") != "none"
	s, err := dbFrom(r).Summary(p, compare)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if s.Tiles == nil {
		s.Tiles = []storage.Tile{}
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) handleHighlights(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePeriod(w, r)
	if !ok {
		return
	}
	hl, err := dbFrom(r).Highlights(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, hl)
}

// insightsCompute is indirected so the digest and observations handlers share it.
var insightsCompute = insights.Compute

// handleObservations runs the rule-based insight engine as of a date
// (default: the person's last day with data).
func (h *handlers) handleObservations(w http.ResponseWriter, r *http.Request) {
	asOf := r.URL.Query().Get("as_of")
	if asOf != "" {
		if _, err := time.Parse("2006-01-02", asOf); err != nil {
			writeError(w, http.StatusBadRequest, "invalid as_of %q (want YYYY-MM-DD)", asOf)
			return
		}
	}
	rep, err := insights.Compute(dbFrom(r), asOf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *handlers) handleRings(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	rings, err := dbFrom(r).ActivityRings(p.From, p.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if rings.Days == nil {
		rings.Days = []storage.ActivityDay{}
	}
	writeJSON(w, http.StatusOK, rings)
}

func (h *handlers) handleSleepNights(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	nights, err := dbFrom(r).SleepNights(p.From, p.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nights":         nights,
		"days_in_period": p.Days(),
	})
}

// handleHeartOverview bundles everything the Heart page needs in one call.
func (h *handlers) handleHeartOverview(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	bucket, err := storage.ParseBucket(r.URL.Query().Get("bucket"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	db := dbFrom(r)
	out := map[string]any{}
	for _, key := range []string{"heart-rate", "resting-heart-rate", "hrv", "walking-heart-rate", "heart-rate-recovery", "vo2max", "spo2", "respiratory-rate", "afib-burden"} {
		m := hk.ByKey[key]
		s, err := db.QuerySeries(m, p.From, p.To, bucket)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		if len(s.Points) > 0 {
			out[strings.ReplaceAll(key, "-", "_")] = s
		}
	}
	events := []map[string]any{}
	for _, t := range []struct{ table, kind string }{{"high_heart_rate_events", "high"}, {"low_heart_rate_events", "low"}, {"irregular_rhythm_events", "irregular"}} {
		rows, err := db.EventRows(t.table, p.From, p.To)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		for _, row := range rows {
			row["kind"] = t.kind
			events = append(events, row)
		}
	}
	out["events"] = events
	bp, err := db.BloodPressureRows(p.From, p.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	out["blood_pressure"] = bp
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) handleHRVReadings(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	rows, err := dbFrom(r).HRVReadings(p.From, p.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handlers) handleHRVBeats(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "hrvID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	rows, err := dbFrom(r).HRVBeats(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if rows == nil {
		writeError(w, http.StatusNotFound, "hrv reading not found")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handlers) handleEnvironment(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	bucket, err := storage.ParseBucket(r.URL.Query().Get("bucket"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	db := dbFrom(r)
	out := map[string]any{}
	for _, key := range []string{"headphone-audio-exposure", "environmental-audio-exposure", "time-in-daylight", "uv-exposure", "water-temperature"} {
		s, err := db.QuerySeries(hk.ByKey[key], p.From, p.To, bucket)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		if len(s.Points) > 0 {
			out[strings.ReplaceAll(key, "-", "_")] = s
		}
	}
	events := []map[string]any{}
	for _, t := range []string{"audio_exposure_events", "environmental_audio_exposure_events", "headphone_audio_exposure_events"} {
		rows, err := db.EventRows(t, p.From, p.To)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		events = append(events, rows...)
	}
	out["events"] = events
	writeJSON(w, http.StatusOK, out)
}

// --- workouts ---

func (h *handlers) handleWorkouts(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	f := storage.WorkoutFilter{
		From: p.From, To: p.To,
		Type:   r.URL.Query().Get("type"),
		Limit:  queryInt(r, "limit", 50, 1000),
		Offset: queryInt(r, "offset", 0, 0),
	}
	items, total, err := dbFrom(r).ListWorkouts(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "items": items})
}

func (h *handlers) handleWorkoutTypes(w http.ResponseWriter, r *http.Request) {
	types, err := dbFrom(r).WorkoutTypes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, types)
}

func (h *handlers) handleWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "wid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workout id")
		return
	}
	d, err := dbFrom(r).GetWorkout(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "workout not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) handleWorkoutRoute(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "wid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workout id")
		return
	}
	db := dbFrom(r)
	routeID, err := db.RouteForWorkout(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if routeID == 0 {
		writeError(w, http.StatusNotFound, "no route for this workout")
		return
	}
	pts, err := db.RoutePoints(routeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	switch r.URL.Query().Get("format") {
	case "geojson":
		coords := make([][]float64, 0, len(pts))
		for _, p := range pts {
			c := []float64{p.Lon, p.Lat}
			if e, ok := p.Ele.(float64); ok {
				c = append(c, e)
			}
			coords = append(coords, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type": "Feature", "properties": map[string]any{"workout_id": id, "route_id": routeID},
			"geometry": map[string]any{"type": "LineString", "coordinates": coords},
		})
	case "gpx":
		w.Header().Set("Content-Type", "application/gpx+xml")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fmt.Sprintf("route-%d.gpx", id)}))
		fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<gpx version=\"1.1\" creator=\"healthsync\" xmlns=\"http://www.topografix.com/GPX/1/1\">\n<trk><trkseg>\n")
		for _, p := range pts {
			fmt.Fprintf(w, "<trkpt lat=\"%.6f\" lon=\"%.6f\">", p.Lat, p.Lon)
			if e, ok := p.Ele.(float64); ok {
				fmt.Fprintf(w, "<ele>%.2f</ele>", e)
			}
			if p.Time != "" {
				fmt.Fprintf(w, "<time>%s</time>", p.Time)
			}
			fmt.Fprint(w, "</trkpt>\n")
		}
		fmt.Fprint(w, "</trkseg></trk>\n</gpx>\n")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"route_id": routeID, "points": pts})
	}
}

// --- ECG ---

func (h *handlers) handleECGList(w http.ResponseWriter, r *http.Request) {
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	rows, err := dbFrom(r).ListECG(p.From, p.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handlers) handleECG(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "eid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ecg id")
		return
	}
	e, err := dbFrom(r).GetECG(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if e == nil {
		writeError(w, http.StatusNotFound, "ecg not found")
		return
	}
	samples := parser.DecodeECGSamples(e.Samples)

	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fmt.Sprintf("ecg-%s.csv", strings.ReplaceAll(e.RecordedDate[:10], "-", ""))}))
		cw := csv.NewWriter(w)
		cw.Write([]string{"t_seconds", e.Unit})
		for i, v := range samples {
			t := float64(i) / e.SampleRateHz
			cw.Write([]string{strconv.FormatFloat(t, 'f', 5, 64), strconv.FormatFloat(float64(v), 'f', 3, 32)})
		}
		cw.Flush()
		return
	}

	out := map[string]any{
		"id": e.ID, "recorded_at": e.RecordedDate, "classification": e.Classification, "symptoms": e.Symptoms,
		"software_version": e.SoftwareVersion, "device": e.Device, "sample_rate": e.SampleRateHz, "lead": e.Lead,
		"unit": e.Unit, "sample_count": e.SampleCount, "avg_hr": e.AverageHR, "duration_s": float64(e.SampleCount) / e.SampleRateHz, "file_name": e.FileName,
	}
	if n := queryInt(r, "points", 0, 20000); n > 0 && n < len(samples) {
		out["envelope"] = envelope(samples, n)
	} else {
		out["samples"] = samples
	}
	writeJSON(w, http.StatusOK, out)
}

// envelope reduces a waveform to n [min,max] pairs for thumbnails.
func envelope(s []float32, n int) [][2]float32 {
	out := make([][2]float32, 0, n)
	per := float64(len(s)) / float64(n)
	for i := 0; i < n; i++ {
		lo := int(math.Floor(float64(i) * per))
		hi := int(math.Floor(float64(i+1) * per))
		if hi > len(s) {
			hi = len(s)
		}
		if lo >= hi {
			continue
		}
		mn, mx := s[lo], s[lo]
		for _, v := range s[lo:hi] {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		out = append(out, [2]float32{mn, mx})
	}
	return out
}

// --- generic tables (Explore) ---

func (h *handlers) handleTable(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "table")
	table, ok := storage.ResolveTable(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown table %q", name)
		return
	}
	p, err := period(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	q := storage.TableQuery{
		Table:  table,
		Type:   r.URL.Query().Get("type"),
		From:   p.From,
		To:     p.To,
		Limit:  queryInt(r, "limit", 100, 1000),
		Offset: queryInt(r, "offset", 0, 0),
		Desc:   r.URL.Query().Get("order") != "asc",
	}
	db := dbFrom(r)

	if r.URL.Query().Get("format") == "csv" {
		person := personFrom(r)
		fname := fmt.Sprintf("%s-%s", safeName(person.Name), table)
		if p.From != "" || p.To != "" {
			fname += fmt.Sprintf("-%s-%s", p.From, p.To)
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fname + ".csv"}))
		cw := csv.NewWriter(w)
		wroteHeader := false
		err := db.StreamTableCSV(q, func(cols []string, row []any) error {
			if !wroteHeader {
				if err := cw.Write(cols); err != nil {
					return err
				}
				wroteHeader = true
			}
			rec := make([]string, len(row))
			for i, v := range row {
				if v == nil {
					continue
				}
				rec[i] = fmt.Sprintf("%v", v)
			}
			return cw.Write(rec)
		})
		cw.Flush()
		if err != nil && !wroteHeader {
			writeError(w, http.StatusInternalServerError, "%v", err)
		}
		return
	}

	cols, rows, total, err := db.TableRows(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"table": table, "columns": cols, "total": total, "rows": rows, "limit": q.Limit, "offset": q.Offset})
}
