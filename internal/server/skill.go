package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/BRO3886/healthsync/internal/storage"
)

// baseURL is the address agents should call. An explicit --public-url wins;
// otherwise it is rebuilt from the request so a curl against
// http://10.0.0.14:1000/skill/SKILL.md yields a skill pointing at that host.
func (h *handlers) baseURL(r *http.Request) string {
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func (h *handlers) handleSkillIndex(w http.ResponseWriter, r *http.Request) {
	if h.skillFS == nil {
		writeError(w, http.StatusNotFound, "skill not embedded in this build")
		return
	}
	base := h.baseURL(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "healthsync agent skill\n\n")
	fmt.Fprintf(w, "  %s/skill/SKILL.md   skill entry point (Agent Skills format)\n", base)
	fmt.Fprintf(w, "  %s/skill/api.md     full endpoint reference\n\n", base)
	fmt.Fprintf(w, "Install into Hermes Agent:\n  hermes skills install %s/skill/SKILL.md --name healthsync\n\n", base)
	fmt.Fprintf(w, "Claude Code / Codex: save both files into <skills dir>/healthsync-api/\n")
}

func (h *handlers) handleSkillFile(w http.ResponseWriter, r *http.Request) {
	if h.skillFS == nil {
		writeError(w, http.StatusNotFound, "skill not embedded in this build")
		return
	}
	name := chi.URLParam(r, "file")
	if strings.Contains(name, "/") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".md") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	b, err := fs.ReadFile(h.skillFS, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	body := strings.ReplaceAll(string(b), "{{BASE_URL}}", h.baseURL(r))
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(body))
}

// handleDigest bundles what an agent needs for a report in one call: the
// person, availability, KPI tiles vs the previous period, observations,
// highlights, sleep nights and workouts for the last N days ending on the
// person's last day with data.
func (h *handlers) handleDigest(w http.ResponseWriter, r *http.Request) {
	p := personFrom(r)
	db := dbFrom(r)
	days := queryInt(r, "days", 7, 366)
	if days < 1 {
		days = 7
	}

	av, err := db.Availability()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	view, _ := h.view(p)
	out := map[string]any{"person": view, "availability": av}
	if av.LastDate == "" {
		out["period"] = nil
		out["note"] = "no data imported yet"
		writeJSON(w, http.StatusOK, out)
		return
	}
	end, _ := time.Parse("2006-01-02", av.LastDate)
	period := storage.Period{From: end.AddDate(0, 0, -(days - 1)).Format("2006-01-02"), To: av.LastDate}
	out["period"] = period

	summary, err := db.Summary(period, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary: %v", err)
		return
	}
	if summary.Tiles == nil {
		summary.Tiles = []storage.Tile{}
	}
	out["summary"] = summary

	rep, err := insightsCompute(db, av.LastDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "observations: %v", err)
		return
	}
	out["observations"] = rep

	hl, err := db.Highlights(period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "highlights: %v", err)
		return
	}
	out["highlights"] = hl

	nights, err := db.SleepNights(period.From, period.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sleep: %v", err)
		return
	}
	out["sleep"] = map[string]any{"nights": nights, "days_in_period": period.Days()}

	items, total, err := db.ListWorkouts(storage.WorkoutFilter{From: period.From, To: period.To, Limit: 200})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workouts: %v", err)
		return
	}
	out["workouts"] = map[string]any{"total": total, "items": items}

	if prof, err := db.Profile(); err == nil {
		// Keep the digest useful without leaking more than the dashboard shows.
		keep := map[string]string{}
		for _, k := range []string{"biological_sex", "date_of_birth", "blood_type"} {
			if v, ok := prof[k]; ok && v != "" && v != "NotSet" {
				keep[k] = v
			}
		}
		out["profile"] = keep
	}
	sort.Strings(rep.Checked)
	writeJSON(w, http.StatusOK, out)
}
