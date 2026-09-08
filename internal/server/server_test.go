package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BRO3886/healthsync/internal/people"
)

func newTestServer(t *testing.T) (*handlers, http.Handler) {
	t.Helper()
	store, err := people.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h := newHandlers(store, "test")
	return h, NewRouter(h)
}

func do(t *testing.T, router http.Handler, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decode(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("decoding %q: %v", rr.Body.String(), err)
	}
}

func createPerson(t *testing.T, router http.Handler, name string) string {
	t.Helper()
	rr := do(t, router, "POST", "/api/people", strings.NewReader(fmt.Sprintf(`{"name":%q}`, name)), "application/json")
	if rr.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rr.Code, rr.Body.String())
	}
	var p struct{ ID string }
	decode(t, rr, &p)
	return p.ID
}

const testXML = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE HealthData [
<!ELEMENT HealthData (Record*)>
]>
<HealthData locale="en_PT">
 <ExportDate value="2024-01-10 08:00:00 +0000"/>
 <Me HKCharacteristicTypeIdentifierDateOfBirth="1990-05-01" HKCharacteristicTypeIdentifierBiologicalSex="HKBiologicalSexFemale"/>
 <Record type="HKQuantityTypeIdentifierStepCount" sourceName="Watch" unit="count" value="1000" startDate="2024-01-01 08:00:00 +0000" endDate="2024-01-01 08:10:00 +0000"/>
 <Record type="HKQuantityTypeIdentifierStepCount" sourceName="Watch" unit="count" value="2000" startDate="2024-01-02 08:00:00 +0000" endDate="2024-01-02 08:10:00 +0000"/>
 <Record type="HKQuantityTypeIdentifierHeartRate" sourceName="Watch" unit="count/min" value="72" startDate="2024-01-01 08:00:00 +0000" endDate="2024-01-01 08:00:00 +0000"/>
 <Record type="HKCategoryTypeIdentifierSleepAnalysis" sourceName="Watch" value="HKCategoryValueSleepAnalysisAsleepCore" startDate="2024-01-01 23:00:00 +0000" endDate="2024-01-02 06:00:00 +0000"/>
 <Workout workoutActivityType="HKWorkoutActivityTypeRunning" duration="30" durationUnit="min" totalDistance="5" totalDistanceUnit="km" sourceName="Watch" startDate="2024-01-02 09:00:00 +0000" endDate="2024-01-02 09:30:00 +0000"/>
 <ActivitySummary dateComponents="2024-01-01" activeEnergyBurned="500" activeEnergyBurnedGoal="400" activeEnergyBurnedUnit="kcal" appleMoveTime="0" appleMoveTimeGoal="0" appleExerciseTime="40" appleExerciseTimeGoal="30" appleStandHours="12" appleStandHoursGoal="12"/>
</HealthData>`

func makeTestZip(t *testing.T, xmlContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	zf, err := w.Create("apple_health_export/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	zf.Write([]byte(xmlContent))
	w.Close()
	return buf.Bytes()
}

func upload(t *testing.T, router http.Handler, personID, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	mw.Close()
	return do(t, router, "POST", "/api/people/"+personID+"/upload", &body, mw.FormDataContentType())
}

func waitImport(t *testing.T, router http.Handler, personID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rr := do(t, router, "GET", "/api/people/"+personID+"/upload/status", nil, "")
		var st map[string]any
		decode(t, rr, &st)
		if st["status"] == "completed" || st["status"] == "failed" {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("import did not finish")
	return nil
}

func TestPeople_CRUD(t *testing.T) {
	_, router := newTestServer(t)

	rr := do(t, router, "GET", "/api/people", nil, "")
	if rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %s", rr.Code, rr.Body.String())
	}

	id := createPerson(t, router, "Ana")
	if rr := do(t, router, "POST", "/api/people", strings.NewReader(`{"name":"ana"}`), "application/json"); rr.Code != http.StatusConflict {
		t.Errorf("duplicate name: %d", rr.Code)
	}
	if rr := do(t, router, "POST", "/api/people", strings.NewReader(`{"name":"x","bogus":1}`), "application/json"); rr.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id, nil, "")
	var v map[string]any
	decode(t, rr, &v)
	if v["name"] != "Ana" || v["has_data"] != false {
		t.Errorf("get: %v", v)
	}

	rr = do(t, router, "PATCH", "/api/people/"+id, strings.NewReader(`{"emoji":"🏃","dob":"1990-05-01"}`), "application/json")
	decode(t, rr, &v)
	if rr.Code != 200 || v["emoji"] != "🏃" || v["dob"] != "1990-05-01" {
		t.Errorf("patch: %d %v", rr.Code, v)
	}

	if rr := do(t, router, "GET", "/api/people/nope", nil, ""); rr.Code != http.StatusNotFound {
		t.Errorf("unknown person: %d", rr.Code)
	}
	if rr := do(t, router, "DELETE", "/api/people/"+id, nil, ""); rr.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rr.Code)
	}
	if rr := do(t, router, "GET", "/api/people/"+id, nil, ""); rr.Code != http.StatusNotFound {
		t.Errorf("deleted person still resolves: %d", rr.Code)
	}
}

func TestUpload_ImportsAndServesDashboardData(t *testing.T) {
	_, router := newTestServer(t)
	id := createPerson(t, router, "Ana")

	rr := upload(t, router, id, "export.zip", makeTestZip(t, testXML))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rr.Code, rr.Body.String())
	}
	st := waitImport(t, router, id)
	if st["status"] != "completed" {
		t.Fatalf("import: %v", st)
	}
	if st["profile_updated"] != true {
		t.Error("dob/sex should be filled from <Me>")
	}
	prog := st["progress"].(map[string]any)
	if prog["records"].(float64) != 4 || prog["workouts"].(float64) != 1 || prog["activity_days"].(float64) != 1 {
		t.Errorf("progress: %v", prog)
	}

	// Person view reflects data.
	rr = do(t, router, "GET", "/api/people/"+id, nil, "")
	var pv map[string]any
	decode(t, rr, &pv)
	if pv["has_data"] != true || pv["first_date"] != "2024-01-01" || pv["sex"] != "female" || pv["last_import_at"] == "" {
		t.Errorf("person view: %v", pv)
	}

	// Imports history.
	rr = do(t, router, "GET", "/api/people/"+id+"/imports", nil, "")
	var imports []map[string]any
	decode(t, rr, &imports)
	if len(imports) != 1 || imports[0]["status"] != "completed" || imports[0]["export_date"] != "2024-01-10 08:00:00" {
		t.Errorf("imports: %v", imports)
	}

	// Series, summary, highlights, rings, sleep, workouts, availability, tables.
	rr = do(t, router, "GET", "/api/people/"+id+"/series/steps?from=2024-01-01&to=2024-01-31", nil, "")
	var series struct {
		Points []struct {
			T string
			V float64
		}
	}
	decode(t, rr, &series)
	if len(series.Points) != 2 || series.Points[1].V != 2000 {
		t.Errorf("series: %+v", series)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/series/nope", nil, ""); rr.Code != 404 {
		t.Errorf("unknown metric: %d", rr.Code)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/series/steps?from=bad", nil, ""); rr.Code != 400 {
		t.Errorf("bad date: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/summary?from=2024-01-01&to=2024-01-07", nil, "")
	var summary struct {
		Tiles    []map[string]any
		Previous map[string]string
	}
	decode(t, rr, &summary)
	if rr.Code != 200 || len(summary.Tiles) < 3 || summary.Previous["from"] != "2023-12-25" {
		t.Errorf("summary: %d %+v", rr.Code, summary)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/summary", nil, ""); rr.Code != 400 {
		t.Errorf("summary without period: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/highlights?from=2024-01-01&to=2024-01-07", nil, "")
	if rr.Code != 200 || !strings.HasPrefix(strings.TrimSpace(rr.Body.String()), "[") {
		t.Errorf("highlights: %d %s", rr.Code, rr.Body.String())
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/observations", nil, "")
	var obs struct {
		AsOf    string `json:"as_of"`
		Checked []string
	}
	decode(t, rr, &obs)
	if rr.Code != 200 || obs.AsOf != "2024-01-02" || len(obs.Checked) == 0 {
		t.Errorf("observations: %d %+v", rr.Code, obs)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/observations?as_of=nope", nil, ""); rr.Code != 400 {
		t.Errorf("bad as_of: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/activity/rings?from=2024-01-01&to=2024-01-07", nil, "")
	var rings struct{ Days []map[string]any }
	decode(t, rr, &rings)
	if len(rings.Days) != 1 || rings.Days[0]["closed"].(map[string]any)["all"] != true {
		t.Errorf("rings: %v", rings)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/sleep/nights?from=2024-01-01&to=2024-01-07", nil, "")
	var nights struct{ Nights []map[string]any }
	decode(t, rr, &nights)
	if len(nights.Nights) != 1 || nights.Nights[0]["hours"].(float64) != 7 {
		t.Errorf("nights: %v", nights)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/workouts", nil, "")
	var workouts struct {
		Total int
		Items []map[string]any
	}
	decode(t, rr, &workouts)
	if workouts.Total != 1 || workouts.Items[0]["distance_km"].(float64) != 5 {
		t.Errorf("workouts: %+v", workouts)
	}
	wid := int(workouts.Items[0]["id"].(float64))
	if rr := do(t, router, "GET", fmt.Sprintf("/api/people/%s/workouts/%d", id, wid), nil, ""); rr.Code != 200 {
		t.Errorf("workout detail: %d", rr.Code)
	}
	if rr := do(t, router, "GET", fmt.Sprintf("/api/people/%s/workouts/%d/route", id, wid), nil, ""); rr.Code != 404 {
		t.Errorf("route for routeless workout: %d", rr.Code)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/workouts/999", nil, ""); rr.Code != 404 {
		t.Errorf("missing workout: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/availability", nil, "")
	var av struct {
		Tables    []map[string]any
		FirstDate string `json:"first_date"`
	}
	decode(t, rr, &av)
	if av.FirstDate != "2024-01-01" || len(av.Tables) < 4 {
		t.Errorf("availability: %+v", av)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/tables/heart-rate?limit=10", nil, "")
	var tbl struct {
		Total int
		Rows  []map[string]any
	}
	decode(t, rr, &tbl)
	if tbl.Total != 1 || len(tbl.Rows) != 1 {
		t.Errorf("table: %+v", tbl)
	}
	rr = do(t, router, "GET", "/api/people/"+id+"/tables/steps?format=csv", nil, "")
	if rr.Code != 200 || !strings.Contains(rr.Header().Get("Content-Type"), "text/csv") || strings.Count(rr.Body.String(), "\n") != 3 {
		t.Errorf("csv: %d %q %q", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/tables/nope", nil, ""); rr.Code != 404 {
		t.Errorf("unknown table: %d", rr.Code)
	}

	rr = do(t, router, "GET", "/api/people/"+id+"/heart/overview?from=2024-01-01&to=2024-01-07", nil, "")
	var heart map[string]any
	decode(t, rr, &heart)
	if _, ok := heart["heart_rate"]; !ok {
		t.Errorf("heart overview: %v", heart)
	}
	if rr := do(t, router, "GET", "/api/people/"+id+"/ecg", nil, ""); rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Errorf("ecg list: %d %s", rr.Code, rr.Body.String())
	}
	rr = do(t, router, "GET", "/api/people/"+id+"/export.db", nil, "")
	if rr.Code != 200 || !bytes.HasPrefix(rr.Body.Bytes(), []byte("SQLite format 3")) {
		t.Errorf("export.db: %d", rr.Code)
	}
}

func TestUpload_Validation(t *testing.T) {
	_, router := newTestServer(t)
	id := createPerson(t, router, "Ana")

	if rr := upload(t, router, id, "notes.txt", []byte("x")); rr.Code != http.StatusBadRequest {
		t.Errorf("bad extension: %d", rr.Code)
	}
	if rr := upload(t, router, "nope", "export.zip", makeTestZip(t, testXML)); rr.Code != http.StatusNotFound {
		t.Errorf("unknown person: %d", rr.Code)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("other", "x")
	mw.Close()
	if rr := do(t, router, "POST", "/api/people/"+id+"/upload", &body, mw.FormDataContentType()); rr.Code != http.StatusBadRequest {
		t.Errorf("missing file field: %d", rr.Code)
	}
	// A bad archive fails the import but leaves the job idle again.
	rr := upload(t, router, id, "export.zip", []byte("not a zip"))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("accepted expected, got %d", rr.Code)
	}
	st := waitImport(t, router, id)
	if st["status"] != "failed" || st["error"] == "" {
		t.Errorf("failed status expected: %v", st)
	}
	imports := do(t, router, "GET", "/api/people/"+id+"/imports", nil, "")
	if !strings.Contains(imports.Body.String(), `"status":"failed"`) {
		t.Errorf("failed import not recorded: %s", imports.Body.String())
	}
	// Recovers: next upload works.
	if rr := upload(t, router, id, "export.zip", makeTestZip(t, testXML)); rr.Code != http.StatusAccepted {
		t.Errorf("upload after failure: %d", rr.Code)
	}
	waitImport(t, router, id)
}

func TestUpload_ConflictIsPerPerson(t *testing.T) {
	h, router := newTestServer(t)
	a := createPerson(t, router, "A")
	b := createPerson(t, router, "B")

	job := h.job(a)
	job.mu.Lock()
	job.running = true
	job.mu.Unlock()

	if rr := upload(t, router, a, "export.zip", makeTestZip(t, testXML)); rr.Code != http.StatusConflict {
		t.Errorf("person A busy: %d", rr.Code)
	}
	if rr := do(t, router, "DELETE", "/api/people/"+a, nil, ""); rr.Code != http.StatusConflict {
		t.Errorf("delete while importing: %d", rr.Code)
	}
	if rr := upload(t, router, b, "export.zip", makeTestZip(t, testXML)); rr.Code != http.StatusAccepted {
		t.Errorf("person B should not be blocked: %d", rr.Code)
	}
	waitImport(t, router, b)
}

func TestHealthzMetricsAndSPAFallback(t *testing.T) {
	_, router := newTestServer(t)
	rr := do(t, router, "GET", "/api/healthz", nil, "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Errorf("healthz: %d %s", rr.Code, rr.Body.String())
	}
	rr = do(t, router, "GET", "/api/metrics", nil, "")
	var metrics []map[string]any
	decode(t, rr, &metrics)
	if len(metrics) < 90 {
		t.Errorf("metrics: %d", len(metrics))
	}
	// Unknown API path is JSON 404, never the SPA.
	rr = do(t, router, "GET", "/api/nope", nil, "")
	if rr.Code != 404 || !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Errorf("api 404: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	// Any non-API path serves the UI (or its placeholder) as HTML.
	rr = do(t, router, "GET", "/p/abc/overview", nil, "")
	if rr.Code != 200 || !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
		t.Errorf("spa fallback: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
}
