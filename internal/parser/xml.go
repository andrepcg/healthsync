package parser

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/BRO3886/healthsync/internal/hk"
	"github.com/BRO3886/healthsync/internal/storage"
)

// Progress is a snapshot of import counters. It is what the progress callback
// receives and what ParseResult embeds at the end.
type Progress struct {
	Records      int64 `json:"records"`
	Workouts     int64 `json:"workouts"`
	Routes       int64 `json:"routes"`
	RoutePoints  int64 `json:"route_points"`
	ECGs         int64 `json:"ecgs"`
	ActivityDays int64 `json:"activity_days"`
	HRVBeats     int64 `json:"hrv_beats"`
	// Errors counts elements that could not be decoded or stored. They are
	// reported rather than silently skipped so a broken export is visible.
	Errors int64 `json:"errors"`
}

// ParseResult holds the parsing outcome.
type ParseResult struct {
	Progress
	// Total is the number of records stored (alias of Progress.Records kept
	// for existing callers).
	Total      int64
	Stats      []*storage.InsertStats
	Profile    map[string]string
	ExportDate string
	Locale     string
	// Warnings holds the first few non-fatal problems (bad GPX, ECG, …).
	Warnings []string
}

// ProgressFunc is called periodically during parsing.
type ProgressFunc func(Progress)

const (
	batchSize        = 1000
	progressEvery    = 10000
	maxWarnings      = 50
	hrvBeatsTable    = "hrv_beats"
	activityTable    = "activity_summary"
	bpSystolicTable  = "blood_pressure_systolic"
	bpDiastolicTable = "blood_pressure_diastolic"
)

// ParseFile parses an Apple Health export (.zip, .xml or unpacked directory)
// and inserts everything it contains into the DB.
func ParseFile(path string, db *storage.DB, progress ProgressFunc) (*ParseResult, error) {
	exp, err := OpenExport(path)
	if err != nil {
		return nil, err
	}
	defer exp.Close()
	return ParseExport(exp, db, progress)
}

// ParseExport imports an already-opened Export.
func ParseExport(exp Export, db *storage.DB, progress ProgressFunc) (*ParseResult, error) {
	imp := &importer{
		db:          db,
		exp:         exp,
		progress:    progress,
		buffers:     map[string][][]any{},
		stats:       map[string]*storage.InsertStats{},
		deviceCache: map[string]int64{},
		bpStaging:   map[bpKey]*bpEntry{},
		profile:     map[string]string{},
	}

	main, err := exp.OpenMain()
	if err != nil {
		return nil, fmt.Errorf("opening export xml: %w", err)
	}
	xmlErr := imp.parseXML(main)
	main.Close()

	if err := imp.flushAll(); err != nil {
		return nil, err
	}
	if xmlErr != nil {
		return imp.result(), xmlErr
	}

	imp.importECGs()
	if err := db.SetProfile(imp.profile); err != nil {
		return nil, fmt.Errorf("storing profile: %w", err)
	}
	imp.report()
	return imp.result(), nil
}

// importer holds the state of one import.
type importer struct {
	db       *storage.DB
	exp      Export
	progress ProgressFunc

	buffers map[string][][]any
	stats   map[string]*storage.InsertStats
	prog    Progress

	deviceCache map[string]int64
	bpStaging   map[bpKey]*bpEntry

	// Correlation wraps records (blood pressure) and carries metadata that
	// belongs to the children; it is captured while inside the element.
	inCorrelation   bool
	correlationMeta []MetadataEntry

	profile    map[string]string
	exportDate string
	locale     string
	warnings   []string
}

// bpKey identifies a blood pressure reading by source + start time.
type bpKey struct{ source, start string }

type bpEntry struct {
	endDate   string
	systolic  any
	diastolic any
	unit      string
	fidelity  []any
}

func (imp *importer) result() *ParseResult {
	res := &ParseResult{
		Progress:   imp.prog,
		Total:      imp.prog.Records,
		Profile:    imp.profile,
		ExportDate: imp.exportDate,
		Locale:     imp.locale,
		Warnings:   imp.warnings,
	}
	tables := make([]string, 0, len(imp.stats))
	for t := range imp.stats {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		res.Stats = append(res.Stats, imp.stats[t])
	}
	return res
}

func (imp *importer) warn(format string, args ...any) {
	imp.prog.Errors++
	if len(imp.warnings) < maxWarnings {
		imp.warnings = append(imp.warnings, fmt.Sprintf(format, args...))
	}
}

func (imp *importer) report() {
	if imp.progress != nil {
		imp.progress(imp.prog)
	}
}

// stripDTD pipes the input through a goroutine that removes the DOCTYPE section.
// Uses io.Pipe so no bytes are lost to buffering.
func stripDTD(r io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		inDTD := false

		for scanner.Scan() {
			line := scanner.Text()

			if strings.Contains(line, "<!DOCTYPE") {
				inDTD = true
				continue
			}
			if inDTD {
				if strings.Contains(line, "]>") {
					inDTD = false
				}
				continue
			}

			if _, err := pw.Write([]byte(line + "\n")); err != nil {
				pw.CloseWithError(err)
				return
			}
		}

		if err := scanner.Err(); err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr
}

func (imp *importer) parseXML(r io.Reader) error {
	decoder := xml.NewDecoder(stripDTD(r))
	decoder.Strict = false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A token error is not recoverable: the decoder returns the same
			// error on every subsequent call. Stop and report how far we got.
			imp.warn("xml decode error: %v", err)
			return fmt.Errorf("xml decode error after %d records: %w", imp.prog.Records, err)
		}

		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "Correlation" {
				imp.inCorrelation = false
				imp.correlationMeta = nil
			}

		case xml.StartElement:
			switch t.Name.Local {
			case "HealthData":
				imp.locale = attr(t, "locale")
				// Do not skip: it holds every child we need.

			case "ExportDate":
				imp.exportDate = normalizeTimestamp(attr(t, "value"))

			case "Me":
				var me Me
				if err := decoder.DecodeElement(&me, &t); err != nil {
					imp.warn("decoding Me: %v", err)
					continue
				}
				imp.setProfile(me)

			case "Correlation":
				// Descend: the child <Record>s (blood pressure) flow through the
				// normal record path. Its own metadata is attached to them.
				imp.inCorrelation = true
				imp.correlationMeta = nil

			case "MetadataEntry":
				if imp.inCorrelation {
					imp.correlationMeta = append(imp.correlationMeta, MetadataEntry{Key: attr(t, "key"), Value: attr(t, "value")})
				}

			case "Record":
				var rec Record
				if err := decoder.DecodeElement(&rec, &t); err != nil {
					imp.warn("decoding Record: %v", err)
					continue
				}
				var extra []MetadataEntry
				if imp.inCorrelation {
					extra = imp.correlationMeta
				}
				if err := imp.handleRecord(&rec, extra); err != nil {
					return err
				}

			case "Workout":
				var w Workout
				if err := decoder.DecodeElement(&w, &t); err != nil {
					imp.warn("decoding Workout: %v", err)
					continue
				}
				if err := imp.handleWorkout(&w); err != nil {
					return err
				}

			case "ActivitySummary":
				var a ActivitySummary
				if err := decoder.DecodeElement(&a, &t); err != nil {
					imp.warn("decoding ActivitySummary: %v", err)
					continue
				}
				if err := imp.handleActivitySummary(&a); err != nil {
					return err
				}

			default:
				if err := decoder.Skip(); err != nil && err != io.EOF {
					imp.warn("skipping <%s>: %v", t.Name.Local, err)
				}
			}
		}
	}
	return nil
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (imp *importer) setProfile(me Me) {
	set := func(k, v, prefix string) {
		if v == "" {
			return
		}
		imp.profile[k] = strings.TrimPrefix(v, prefix)
	}
	set("date_of_birth", me.DateOfBirth, "")
	set("biological_sex", me.BiologicalSex, "HKBiologicalSex")
	set("blood_type", me.BloodType, "HKBloodType")
	set("skin_type", me.SkinType, "HKFitzpatrickSkinType")
	set("cardio_fitness_medications", me.CardioFitnessMeds, "")
	set("wheelchair_use", me.WheelchairUse, "HKWheelchairUse")
}

// fidelity returns the four trailing columns shared by every record row.
func (imp *importer) fidelity(sourceVersion, device, creationDate string, meta ...[]MetadataEntry) ([]any, error) {
	deviceID, err := imp.deviceID(device)
	if err != nil {
		return nil, err
	}
	return []any{
		nilIfEmpty(sourceVersion),
		deviceID,
		nilIfEmpty(normalizeTimestamp(creationDate)),
		metadataJSON(meta...),
	}, nil
}

func (imp *importer) handleRecord(rec *Record, extraMeta []MetadataEntry) error {
	startDate := normalizeTimestamp(rec.StartDate)
	endDate := normalizeTimestamp(rec.EndDate)

	fid, err := imp.fidelity(rec.SourceVersion, rec.Device, rec.CreationDate, rec.Metadata, extraMeta)
	if err != nil {
		return err
	}

	metric, known := hk.ByIdentifier[rec.Type]

	// Blood pressure: stage both values; only emit row when pair is complete.
	if known && metric.Paired {
		key := bpKey{rec.SourceName, startDate}
		e, exists := imp.bpStaging[key]
		if !exists {
			e = &bpEntry{endDate: endDate, unit: rec.Unit, fidelity: fid}
			imp.bpStaging[key] = e
		}
		v := parseFloat(rec.Value)
		if rec.Type == "HKQuantityTypeIdentifierBloodPressureSystolic" {
			e.systolic = v
		} else {
			e.diastolic = v
		}
		if e.systolic != nil && e.diastolic != nil {
			row := append([]any{key.source, key.start, e.endDate, e.systolic, e.diastolic, e.unit}, e.fidelity...)
			delete(imp.bpStaging, key)
			return imp.add(hk.BloodPressureTable, row)
		}
		return nil
	}

	var table string
	var row []any
	switch {
	case known && metric.NoUnit():
		table = metric.Table
		row = []any{rec.SourceName, startDate, endDate, rec.Value}
	case known:
		table = metric.Table
		v := parseFloat(rec.Value)
		if v == nil {
			// A quantity without a numeric value cannot satisfy REAL NOT NULL;
			// keep it in the generic category table rather than drop it.
			table = hk.OtherCategoryTable
			row = []any{rec.Type, rec.SourceName, startDate, endDate, rec.Value}
		} else {
			row = []any{rec.SourceName, startDate, endDate, v, rec.Unit}
		}
	default:
		// Unknown type: keep it, with its identifier, in the generic tables.
		v := parseFloat(rec.Value)
		if hk.IsQuantityIdentifier(rec.Type, rec.Unit != "") && v != nil {
			table = hk.OtherQuantityTable
			row = []any{rec.Type, rec.SourceName, startDate, endDate, v, rec.Unit}
		} else {
			table = hk.OtherCategoryTable
			row = []any{rec.Type, rec.SourceName, startDate, endDate, rec.Value}
		}
	}
	row = append(row, fid...)
	if err := imp.add(table, row); err != nil {
		return err
	}

	if rec.HRV != nil {
		for i, b := range rec.HRV.Beats {
			bpm := parseFloat(b.BPM)
			if bpm == nil {
				continue
			}
			if err := imp.add(hrvBeatsTable, []any{rec.SourceName, startDate, i, nilIfEmpty(b.Time), bpm}); err != nil {
				return err
			}
			imp.prog.HRVBeats++
		}
	}
	return nil
}

// add buffers a row for table and flushes when the buffer is full. Every
// record-shaped row also bumps the record counter and progress callback.
func (imp *importer) add(table string, row []any) error {
	imp.buffers[table] = append(imp.buffers[table], row)
	if table != hrvBeatsTable && table != activityTable {
		imp.prog.Records++
		if imp.prog.Records%progressEvery == 0 {
			imp.report()
		}
	}
	if len(imp.buffers[table]) >= batchSize {
		return imp.flushTable(table)
	}
	return nil
}

func (imp *importer) columnsFor(table string) []string {
	switch table {
	case hk.BloodPressureTable:
		return hk.BloodPressureColumns()
	case hk.OtherQuantityTable, hk.OtherCategoryTable:
		return hk.OtherColumns(table)
	case hrvBeatsTable:
		return []string{"source_name", "start_date", "seq", "time", "bpm"}
	case activityTable:
		return []string{"date", "active_energy", "active_energy_goal", "active_energy_unit", "move_time", "move_time_goal", "exercise_time", "exercise_time_goal", "stand_hours", "stand_hours_goal"}
	}
	return hk.RecordColumns(table)
}

func (imp *importer) flushTable(table string) error {
	rows := imp.buffers[table]
	if len(rows) == 0 {
		return nil
	}
	var (
		st  *storage.InsertStats
		err error
	)
	if table == activityTable {
		st, err = imp.db.BatchReplaceRecords(table, imp.columnsFor(table), rows)
	} else {
		st, err = imp.db.BatchInsertRecords(table, imp.columnsFor(table), rows)
	}
	if err != nil {
		return fmt.Errorf("flushing %s: %w", table, err)
	}
	imp.buffers[table] = nil
	acc, ok := imp.stats[table]
	if !ok {
		acc = &storage.InsertStats{Table: table}
		imp.stats[table] = acc
	}
	acc.Inserted += st.Inserted
	acc.Skipped += st.Skipped
	return nil
}

func (imp *importer) flushAll() error {
	tables := make([]string, 0, len(imp.buffers))
	for t := range imp.buffers {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		if err := imp.flushTable(t); err != nil {
			return err
		}
	}
	return nil
}

func (imp *importer) handleActivitySummary(a *ActivitySummary) error {
	if a.Date == "" {
		return nil
	}
	row := []any{
		a.Date,
		parseFloat(a.ActiveEnergyBurned), parseFloat(a.ActiveEnergyBurnedGoal), nilIfEmpty(a.ActiveEnergyBurnedUnit),
		parseFloat(a.AppleMoveTime), parseFloat(a.AppleMoveTimeGoal),
		parseFloat(a.AppleExerciseTime), parseFloat(a.AppleExerciseTimeGoal),
		parseFloat(a.AppleStandHours), parseFloat(a.AppleStandHoursGoal),
	}
	imp.prog.ActivityDays++
	return imp.add(activityTable, row)
}

func (imp *importer) handleWorkout(w *Workout) error {
	// Populate totals from WorkoutStatistics children if attributes not present (watchOS 10+)
	for _, s := range w.Statistics {
		switch {
		case s.Type == "HKQuantityTypeIdentifierActiveEnergyBurned":
			if w.TotalEnergyBurned == "" {
				w.TotalEnergyBurned = s.Sum
				w.TotalEnergyBurnedUnit = s.Unit
			}
		case strings.HasPrefix(s.Type, "HKQuantityTypeIdentifierDistance"):
			if w.TotalDistance == "" {
				w.TotalDistance = s.Sum
				w.TotalDistanceUnit = s.Unit
			}
		}
	}

	fid, err := imp.fidelity(w.SourceVersion, w.Device, w.CreationDate, w.Metadata)
	if err != nil {
		return err
	}
	row := append([]any{
		w.ActivityType, w.SourceName, normalizeTimestamp(w.StartDate), normalizeTimestamp(w.EndDate),
		parseFloat(w.Duration), nilIfEmpty(w.DurationUnit),
		parseFloat(w.TotalDistance), nilIfEmpty(w.TotalDistanceUnit),
		parseFloat(w.TotalEnergyBurned), nilIfEmpty(w.TotalEnergyBurnedUnit),
	}, fid...)

	id, err := imp.db.UpsertWorkout(hk.WorkoutColumns(), row)
	if err != nil {
		return fmt.Errorf("storing workout: %w", err)
	}
	imp.prog.Workouts++

	stats := make([]storage.WorkoutStatRow, 0, len(w.Statistics))
	for _, s := range w.Statistics {
		stats = append(stats, storage.WorkoutStatRow{
			Type: s.Type, StartDate: normalizeTimestamp(s.StartDate), EndDate: normalizeTimestamp(s.EndDate),
			Sum: parseFloat(s.Sum), Average: parseFloat(s.Average), Minimum: parseFloat(s.Minimum), Maximum: parseFloat(s.Maximum),
			Unit: nilIfEmpty(s.Unit),
		})
	}
	events := make([]storage.WorkoutEventRow, 0, len(w.Events))
	for _, e := range w.Events {
		events = append(events, storage.WorkoutEventRow{
			Type: e.Type, Date: normalizeTimestamp(e.Date),
			Duration: parseFloat(e.Duration), DurationUnit: nilIfEmpty(e.DurationUnit),
			Metadata: metadataJSON(e.Metadata),
		})
	}
	var zones []storage.WorkoutZoneRow
	for _, g := range w.ZoneGroups {
		for i, z := range g.Zones {
			zones = append(zones, storage.WorkoutZoneRow{
				GroupType: g.Type, GroupUnit: nilIfEmpty(g.Unit), ZoneIndex: i,
				Minimum: parseFloat(z.Minimum), Maximum: parseFloat(z.Maximum),
				Duration: parseFloat(z.Duration), DurationUnit: nilIfEmpty(z.DurationUnit),
			})
		}
	}
	if err := imp.db.ReplaceWorkoutChildren(id, stats, events, zones); err != nil {
		return fmt.Errorf("storing workout children: %w", err)
	}

	for i := range w.Routes {
		imp.handleRoute(id, &w.Routes[i])
	}

	for i := range w.Records {
		if err := imp.handleRecord(&w.Records[i], nil); err != nil {
			return err
		}
	}

	imp.report()
	return nil
}

// handleRoute stores a route header and its GPX points. A missing or broken
// GPX file is a warning, not a fatal error: the workout itself is still valid.
func (imp *importer) handleRoute(workoutID int64, r *WorkoutRoute) {
	deviceID, err := imp.deviceID(r.Device)
	if err != nil {
		imp.warn("route device: %v", err)
		return
	}
	row := storage.RouteRow{
		WorkoutID:     workoutID,
		SourceName:    r.SourceName,
		SourceVersion: nilIfEmpty(r.SourceVersion),
		DeviceID:      deviceID,
		CreationDate:  nilIfEmpty(normalizeTimestamp(r.CreationDate)),
		StartDate:     normalizeTimestamp(r.StartDate),
		EndDate:       normalizeTimestamp(r.EndDate),
		Metadata:      metadataJSON(r.Metadata),
	}

	var points []storage.RoutePoint
	if r.File != nil && r.File.Path != "" {
		row.FilePath = r.File.Path
		rc, err := imp.exp.Open(r.File.Path)
		if err != nil {
			imp.warn("route file %s: %v", r.File.Path, err)
		} else {
			summary, perr := parseGPX(rc)
			rc.Close()
			if perr != nil {
				imp.warn("route file %s: %v", r.File.Path, perr)
			} else {
				points = summary.Points
				row.DistanceM = summary.DistanceM
				row.ElevationGainM = summary.ElevationGainM
			}
		}
	}

	if _, err := imp.db.ReplaceRoute(row, points); err != nil {
		imp.warn("storing route %s: %v", r.StartDate, err)
		return
	}
	imp.prog.Routes++
	imp.prog.RoutePoints += int64(len(points))
}

// importECGs finds the electrocardiogram CSVs, which the XML never references.
func (imp *importer) importECGs() {
	for _, name := range imp.exp.List(isECGPath) {
		rc, err := imp.exp.Open(name)
		if err != nil {
			imp.warn("ecg %s: %v", name, err)
			continue
		}
		br := bufio.NewReader(rc)
		head, _ := br.Peek(2048)
		if !looksLikeECG(head) {
			rc.Close()
			continue
		}
		row, err := parseECG(br, name)
		rc.Close()
		if err != nil {
			imp.warn("%v", err)
			continue
		}
		inserted, err := imp.db.InsertECG(*row)
		if err != nil {
			imp.warn("ecg %s: %v", name, err)
			continue
		}
		imp.prog.ECGs++
		acc, ok := imp.stats["ecg"]
		if !ok {
			acc = &storage.InsertStats{Table: "ecg"}
			imp.stats["ecg"] = acc
		}
		if inserted {
			acc.Inserted++
		} else {
			acc.Skipped++
		}
	}
}

// deviceID resolves a device description to its devices.id, caching per import.
// The export prefixes each description with a per-run memory address
// ("<<HKDevice: 0x74703ff240>, name:Apple Watch, …>") which would make every
// export create new device rows; it is stripped before storing.
func (imp *importer) deviceID(device string) (any, error) {
	desc := normalizeDevice(device)
	if desc == "" {
		return nil, nil
	}
	if id, ok := imp.deviceCache[desc]; ok {
		return id, nil
	}
	id, err := imp.db.UpsertDevice(desc)
	if err != nil {
		return nil, err
	}
	imp.deviceCache[desc] = id
	return id, nil
}

func normalizeDevice(device string) string {
	s := strings.TrimSpace(device)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "<<HKDevice:") {
		if i := strings.Index(s, ">, "); i >= 0 {
			s = s[i+3:]
		}
	}
	s = strings.TrimSuffix(s, ">")
	return strings.TrimSpace(s)
}

// metadataJSON serialises metadata lists into one JSON object, or nil when
// there is nothing to store. It is hand-rolled because it runs half a million
// times per import and encoding/json on a map is several times slower.
func metadataJSON(lists ...[]MetadataEntry) any {
	n := 0
	for _, l := range lists {
		n += len(l)
	}
	if n == 0 {
		return nil
	}
	var b strings.Builder
	b.Grow(n * 48)
	b.WriteByte('{')
	first := true
	for _, l := range lists {
		for _, e := range l {
			if !first {
				b.WriteByte(',')
			}
			first = false
			writeJSONString(&b, e.Key)
			b.WriteByte(':')
			writeJSONString(&b, e.Value)
		}
	}
	b.WriteByte('}')
	return b.String()
}

func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

func parseFloat(s string) any {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// normalizeTimestamp strips the trailing timezone offset (e.g. " +0530", " -0800")
// from Apple Health timestamps, leaving plain "YYYY-MM-DD HH:MM:SS" local time.
// This is required because SQLite's julianday()/date() cannot parse the
// space-separated ±HHMM offset format that Apple Health uses.
func normalizeTimestamp(s string) string {
	// Apple Health format: "2024-01-01 22:00:00 +0530"
	// We want: "2024-01-01 22:00:00"
	// The timestamp portion is always 19 chars ("YYYY-MM-DD HH:MM:SS").
	if len(s) > 19 && s[19] == ' ' {
		return s[:19]
	}
	return s
}

// ParseReader imports a HealthKit XML stream that has no sibling files
// (routes and ECGs are unavailable).
func ParseReader(r io.Reader, db *storage.DB, progress ProgressFunc) (*ParseResult, error) {
	return ParseExport(&readerExport{r: r}, db, progress)
}
