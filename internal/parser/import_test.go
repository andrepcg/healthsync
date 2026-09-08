package parser

import (
	"archive/zip"
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BRO3886/healthsync/internal/storage"
)

func count(t *testing.T, db *storage.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Conn().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestParse_CorrelationBloodPressureIsNotSkipped(t *testing.T) {
	// Regression: the parser used to call Skip() on <Correlation>, dropping the
	// blood pressure records inside it.
	xml := makeTestXML(`
 <Correlation type="HKCorrelationTypeIdentifierBloodPressure" sourceName="Health" startDate="2026-07-21 09:36:00 +0100" endDate="2026-07-21 09:36:00 +0100">
  <MetadataEntry key="HKWasUserEntered" value="1"/>
  <Record type="HKQuantityTypeIdentifierBloodPressureDiastolic" sourceName="Health" unit="mmHg" startDate="2026-07-21 09:36:00 +0100" endDate="2026-07-21 09:36:00 +0100" value="70"/>
  <Record type="HKQuantityTypeIdentifierBloodPressureSystolic" sourceName="Health" unit="mmHg" startDate="2026-07-21 09:36:00 +0100" endDate="2026-07-21 09:36:00 +0100" value="110"/>
 </Correlation>
 <Record type="HKQuantityTypeIdentifierHeartRate" sourceName="Watch" unit="count/min" value="72" startDate="2026-07-21 10:00:00 +0100" endDate="2026-07-21 10:00:00 +0100"/>
`)
	db := tempDB(t)
	res, err := ParseReader(strings.NewReader(xml), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Errorf("expected 2 records (1 paired BP + 1 HR), got %d", res.Total)
	}
	var sys, dia float64
	var meta string
	if err := db.Conn().QueryRow(`SELECT systolic, diastolic, metadata FROM blood_pressure`).Scan(&sys, &dia, &meta); err != nil {
		t.Fatalf("blood_pressure row missing: %v", err)
	}
	if sys != 110 || dia != 70 {
		t.Errorf("got %v/%v", sys, dia)
	}
	if !strings.Contains(meta, `"HKWasUserEntered":"1"`) {
		t.Errorf("correlation metadata not attached: %q", meta)
	}
	// The correlation state must not leak into records after it.
	if got := count(t, db, `SELECT COUNT(*) FROM heart_rate WHERE metadata IS NOT NULL`); got != 0 {
		t.Errorf("heart rate row inherited correlation metadata")
	}
}

const richWorkout = `
 <Workout workoutActivityType="HKWorkoutActivityTypeCycling" duration="29.77" durationUnit="min" sourceName="Watch" sourceVersion="11.1" device="&lt;&lt;HKDevice: 0x74703ff240&gt;, name:Apple Watch, manufacturer:Apple Inc., model:Watch, hardware:Watch7,11, software:11.1&gt;" creationDate="2024-12-08 16:35:45 +0100" startDate="2024-12-08 16:05:56 +0100" endDate="2024-12-08 16:35:43 +0100">
  <MetadataEntry key="HKIndoorWorkout" value="0"/>
  <MetadataEntry key="HKAverageMETs" value="5.24514 kcal/hr·kg"/>
  <WorkoutEvent type="HKWorkoutEventTypeSegment" date="2024-12-08 16:05:56 +0100" duration="7.67" durationUnit="min"/>
  <WorkoutEvent type="HKWorkoutEventTypePause" date="2024-12-08 16:13:37 +0100"/>
  <WorkoutStatistics type="HKQuantityTypeIdentifierHeartRate" startDate="2024-12-08 16:05:56 +0100" endDate="2024-12-08 16:35:43 +0100" average="100.696" minimum="73" maximum="135" unit="count/min"/>
  <WorkoutStatistics type="HKQuantityTypeIdentifierDistanceCycling" startDate="2024-12-08 16:05:56 +0100" endDate="2024-12-08 16:35:43 +0100" sum="2.4924" unit="km"/>
  <WorkoutStatistics type="HKQuantityTypeIdentifierActiveEnergyBurned" startDate="2024-12-08 16:05:56 +0100" endDate="2024-12-08 16:35:43 +0100" sum="117.119" unit="kcal"/>
  <WorkoutZoneGroup type="HKQuantityTypeIdentifierHeartRate" source="system" unit="count/min">
   <WorkoutZone maximum="132" duration="22.0863" durationUnit="min"/>
   <WorkoutZone minimum="132" maximum="146" duration="0.0833333" durationUnit="min"/>
   <WorkoutZone minimum="146" duration="0" durationUnit="min"/>
  </WorkoutZoneGroup>
  <WorkoutRoute sourceName="Watch" sourceVersion="27.0" creationDate="2024-12-08 16:35:50 +0100" startDate="2024-12-08 16:05:56 +0100" endDate="2024-12-08 16:35:43 +0100">
   <MetadataEntry key="HKMetadataKeySyncVersion" value="2"/>
   <FileReference path="/workout-routes/route_2024-12-08_4.05pm.gpx"/>
  </WorkoutRoute>
  <Record type="HKQuantityTypeIdentifierEstimatedWorkoutEffortScore" sourceName="Watch" unit="appleEffortScore" creationDate="2024-12-08 16:35:43 +0100" startDate="2024-12-08 16:05:57 +0100" endDate="2024-12-08 16:35:43 +0100" value="4"/>
 </Workout>
`

const testGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Apple Health Export" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><name>Route</name><trkseg>
      <trkpt lon="-8.865413" lat="40.152135"><ele>20</ele><time>2024-12-08T15:05:56Z</time><extensions><speed>0.95</speed><course>193.3</course><hAcc>4.0</hAcc><vAcc>2.0</vAcc></extensions></trkpt>
      <trkpt lon="-8.865413" lat="40.153135"><ele>25</ele><time>2024-12-08T15:06:56Z</time><extensions><speed>1.46</speed><course>194.1</course><hAcc>3.6</hAcc><vAcc>1.6</vAcc></extensions></trkpt>
      <trkpt lon="-8.865413" lat="40.154135"><ele>22</ele><time>2024-12-08T15:07:56Z</time><extensions><speed>1.52</speed><course>188.9</course><hAcc>99</hAcc><vAcc>1.4</vAcc></extensions></trkpt>
  </trkseg></trk>
</gpx>`

const testECG = "Name,Test Person\nDate of Birth,\"4 Jan 1992\"\nRecorded Date,2026-08-26 18:15:35 +0100\nClassification,Sinus Rhythm\nSymptoms,\nSoftware Version,1.90\nDevice,\"Watch7,11\"\nSample Rate,512 hertz\n\n\nLead,Lead I\nUnit,µV\n\n-265,868\n-271,819\n12\n0,5\n"

func TestParse_WorkoutChildrenAndNestedRecords(t *testing.T) {
	db := tempDB(t)
	res, err := ParseReader(strings.NewReader(makeTestXML(richWorkout)), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Workouts != 1 {
		t.Fatalf("workouts: %d", res.Workouts)
	}
	// Nested record (effort score) must not be swallowed by the workout decode.
	if res.Total != 1 || count(t, db, `SELECT COUNT(*) FROM estimated_workout_effort_score`) != 1 {
		t.Errorf("nested EstimatedWorkoutEffortScore record was lost (records=%d)", res.Total)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workout_statistics`); n != 3 {
		t.Errorf("statistics: %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workout_events`); n != 2 {
		t.Errorf("events: %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workout_zones`); n != 3 {
		t.Errorf("zones: %d", n)
	}
	// Totals populated from statistics when attributes are absent.
	var dist, energy float64
	var meta, sv string
	var dev int64
	if err := db.Conn().QueryRow(`SELECT total_distance, total_energy_burned, metadata, source_version, device_id FROM workouts`).Scan(&dist, &energy, &meta, &sv, &dev); err != nil {
		t.Fatal(err)
	}
	if dist != 2.4924 || energy != 117.119 || sv != "11.1" || dev == 0 {
		t.Errorf("workout row: dist=%v energy=%v sv=%q dev=%d", dist, energy, sv, dev)
	}
	if !strings.Contains(meta, `"HKAverageMETs":"5.24514 kcal/hr·kg"`) {
		t.Errorf("metadata: %q", meta)
	}
	var desc string
	db.Conn().QueryRow(`SELECT description FROM devices WHERE id = ?`, dev).Scan(&desc)
	if strings.Contains(desc, "0x74703ff240") || !strings.HasPrefix(desc, "name:Apple Watch") {
		t.Errorf("device pointer prefix not stripped: %q", desc)
	}
	// The route file is missing in a bare stream: header stored, warning counted.
	if n := count(t, db, `SELECT COUNT(*) FROM workout_routes WHERE point_count = 0`); n != 1 {
		t.Errorf("route header rows: %d", n)
	}
	if res.Errors != 1 || len(res.Warnings) != 1 {
		t.Errorf("expected exactly one warning for the missing gpx, got errors=%d warnings=%v", res.Errors, res.Warnings)
	}

	// Re-import is idempotent: children are replaced, not duplicated.
	if _, err := ParseReader(strings.NewReader(makeTestXML(richWorkout)), db, nil); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workouts`); n != 1 {
		t.Errorf("workouts duplicated: %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workout_zones`); n != 3 {
		t.Errorf("zones duplicated: %d", n)
	}
}

func TestParse_ZipWithRoutesAndECG(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	add("apple_health_export/export.xml", makeTestXML(richWorkout))
	add("apple_health_export/export_cda.xml", `<?xml version="1.0"?><ClinicalDocument xmlns="urn:hl7-org:v3"></ClinicalDocument>`)
	add("apple_health_export/workout-routes/route_2024-12-08_4.05pm.gpx", testGPX)
	add("apple_health_export/electrocardiograms/ecg_2026-08-26.csv", testECG)
	add("apple_health_export/electrocardiograms/notes.csv", "a,b\n1,2\n")
	add("__MACOSX/apple_health_export/._export.xml", "junk")
	zw.Close()

	path := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	db := tempDB(t)
	res, err := ParseFile(path, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Routes != 1 || res.RoutePoints != 3 {
		t.Errorf("routes=%d points=%d", res.Routes, res.RoutePoints)
	}
	if res.ECGs != 1 {
		t.Errorf("ecgs=%d (notes.csv must be ignored)", res.ECGs)
	}
	if res.Errors != 0 {
		t.Errorf("unexpected warnings: %v", res.Warnings)
	}

	var pc int
	var dist, gain float64
	var wid int64
	if err := db.Conn().QueryRow(`SELECT point_count, distance_m, elevation_gain_m, workout_id FROM workout_routes`).Scan(&pc, &dist, &gain, &wid); err != nil {
		t.Fatal(err)
	}
	// Points 1→2 are 0.001° of latitude apart (~111 m). Point 3 has hAcc 99 m
	// and is excluded from the distance, so the total is one leg.
	if pc != 3 || math.Abs(dist-111.2) > 1 {
		t.Errorf("route: points=%d distance=%.1f (want ~111.2)", pc, dist)
	}
	if gain <= 0 {
		t.Errorf("elevation gain should be positive, got %v", gain)
	}
	if wid == 0 {
		t.Error("route not linked to its workout")
	}

	e, err := db.GetECG(1)
	if err != nil || e == nil {
		t.Fatalf("ecg: %v", err)
	}
	if e.RecordedDate != "2026-08-26 18:15:35" || e.Classification != "Sinus Rhythm" || e.Device != "Watch7,11" || e.SampleRateHz != 512 || e.Unit != "µV" {
		t.Errorf("ecg header: %+v", e)
	}
	samples := DecodeECGSamples(e.Samples)
	want := []float32{-265.868, -271.819, 12, 0.5}
	if len(samples) != len(want) {
		t.Fatalf("samples: %v", samples)
	}
	for i := range want {
		if math.Abs(float64(samples[i]-want[i])) > 1e-3 {
			t.Errorf("sample %d: %v want %v", i, samples[i], want[i])
		}
	}
	// Re-import: same recorded_date → skipped, not duplicated.
	if _, err := ParseFile(path, db, nil); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM ecg`); n != 1 {
		t.Errorf("ecg duplicated: %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM workout_route_points`); n != 3 {
		t.Errorf("route points duplicated: %d", n)
	}
}

func TestParse_ECGDotDecimal(t *testing.T) {
	csv := strings.ReplaceAll(testECG, "-265,868", "-265.868")
	row, err := parseECG(strings.NewReader(csv), "x.csv")
	if err != nil {
		t.Fatal(err)
	}
	s := DecodeECGSamples(row.Samples)
	if math.Abs(float64(s[0])+265.868) > 1e-3 {
		t.Errorf("dot decimal parsed as %v", s[0])
	}
	if !looksLikeECG([]byte(csv[:200])) {
		t.Error("sniff failed")
	}
	if looksLikeECG([]byte("a,b\n1,2\n")) {
		t.Error("sniff false positive")
	}
}

func TestParse_MeExportDateActivitySummaryHRVBeats(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<HealthData locale="en_PT">
 <ExportDate value="2026-09-08 08:55:02 +0100"/>
 <Me HKCharacteristicTypeIdentifierDateOfBirth="1992-01-04" HKCharacteristicTypeIdentifierBiologicalSex="HKBiologicalSexMale" HKCharacteristicTypeIdentifierBloodType="HKBloodTypeNotSet" HKCharacteristicTypeIdentifierFitzpatrickSkinType="HKFitzpatrickSkinTypeNotSet" HKCharacteristicTypeIdentifierCardioFitnessMedicationsUse="None"/>
 <Record type="HKQuantityTypeIdentifierHeartRateVariabilitySDNN" sourceName="Watch" unit="ms" startDate="2024-10-28 13:20:58 +0100" endDate="2024-10-28 13:21:54 +0100" value="30.0482">
  <MetadataEntry key="HKAlgorithmVersion" value="2"/>
  <HeartRateVariabilityMetadataList>
   <InstantaneousBeatsPerMinute bpm="62" time="12:21:00,30"/>
   <InstantaneousBeatsPerMinute bpm="60" time="12:21:01,29"/>
   <InstantaneousBeatsPerMinute bpm="59" time="12:21:02,31"/>
  </HeartRateVariabilityMetadataList>
 </Record>
 <Record type="HKQuantityTypeIdentifierStepCount" sourceName="Watch" unit="count" startDate="2024-10-28 13:20:58 +0100" endDate="2024-10-28 13:21:54 +0100" value="12"/>
 <ActivitySummary dateComponents="2023-09-22" activeEnergyBurned="68.078" activeEnergyBurnedGoal="130" activeEnergyBurnedUnit="kcal" appleMoveTime="0" appleMoveTimeGoal="0" appleExerciseTime="10" appleExerciseTimeGoal="30" appleStandHours="3" appleStandHoursGoal="12"/>
 <ActivitySummary dateComponents="2023-09-22" activeEnergyBurned="68.078" activeEnergyBurnedGoal="150" activeEnergyBurnedUnit="kcal" appleMoveTime="0" appleMoveTimeGoal="0" appleExerciseTime="10" appleExerciseTimeGoal="30" appleStandHours="3" appleStandHoursGoal="12"/>
</HealthData>`
	db := tempDB(t)
	res, err := ParseReader(strings.NewReader(xml), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExportDate != "2026-09-08 08:55:02" || res.Locale != "en_PT" {
		t.Errorf("export date/locale: %q %q", res.ExportDate, res.Locale)
	}
	if res.Profile["date_of_birth"] != "1992-01-04" || res.Profile["biological_sex"] != "Male" || res.Profile["blood_type"] != "NotSet" {
		t.Errorf("profile: %v", res.Profile)
	}
	prof, _ := db.Profile()
	if prof["date_of_birth"] != "1992-01-04" {
		t.Errorf("profile not persisted: %v", prof)
	}
	if res.HRVBeats != 3 || count(t, db, `SELECT COUNT(*) FROM hrv_beats`) != 3 {
		t.Errorf("hrv beats: %d", res.HRVBeats)
	}
	var tm string
	var bpm float64
	db.Conn().QueryRow(`SELECT time, bpm FROM hrv_beats WHERE seq = 2`).Scan(&tm, &bpm)
	if tm != "12:21:02,31" || bpm != 59 {
		t.Errorf("beat 2: %q %v", tm, bpm)
	}
	// Steps row without metadata stores NULL, not "{}".
	if n := count(t, db, `SELECT COUNT(*) FROM steps WHERE metadata IS NULL`); n != 1 {
		t.Error("empty metadata should be NULL")
	}
	// Activity summary: last write wins (goal 150), one row.
	if res.ActivityDays != 2 || count(t, db, `SELECT COUNT(*) FROM activity_summary`) != 1 {
		t.Errorf("activity summary rows")
	}
	var goal float64
	db.Conn().QueryRow(`SELECT active_energy_goal FROM activity_summary`).Scan(&goal)
	if goal != 150 {
		t.Errorf("goal not replaced: %v", goal)
	}
	// Records counter excludes hrv beats and activity days.
	if res.Total != 2 {
		t.Errorf("records: %d", res.Total)
	}
}

func TestMetadataJSON(t *testing.T) {
	if metadataJSON(nil, []MetadataEntry{}) != nil {
		t.Error("empty metadata must be nil")
	}
	got := metadataJSON([]MetadataEntry{{Key: `a"b`, Value: "x\ny"}}, []MetadataEntry{{Key: "c", Value: `\`}}).(string)
	want := `{"a\"b":"x\ny","c":"\\"}`
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestNormalizeDevice(t *testing.T) {
	cases := map[string]string{
		"<<HKDevice: 0x74703ff240>, name:Apple Watch, manufacturer:Apple Inc., model:Watch>": "name:Apple Watch, manufacturer:Apple Inc., model:Watch",
		"":                  "",
		"  Custom Device  ": "Custom Device",
	}
	for in, want := range cases {
		if got := normalizeDevice(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestGPX_DistanceAndGain(t *testing.T) {
	sum, err := parseGPX(strings.NewReader(testGPX))
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Points) != 3 {
		t.Fatalf("points: %d", len(sum.Points))
	}
	if math.Abs(sum.DistanceM-111.2) > 1 {
		t.Errorf("distance %.1f", sum.DistanceM)
	}
	if sum.Points[0].Speed.(float64) != 0.95 || sum.Points[0].Time != "2024-12-08T15:05:56Z" {
		t.Errorf("point 0: %+v", sum.Points[0])
	}
}
