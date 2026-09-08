package parser

import "github.com/BRO3886/healthsync/internal/hk"

// MetadataEntry is a <MetadataEntry key="" value=""/> child of most elements.
type MetadataEntry struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

// InstantaneousBPM is one beat inside <HeartRateVariabilityMetadataList>.
type InstantaneousBPM struct {
	BPM  string `xml:"bpm,attr"`
	Time string `xml:"time,attr"`
}

// HRVList holds the beat-to-beat samples behind an HRV reading.
type HRVList struct {
	Beats []InstantaneousBPM `xml:"InstantaneousBeatsPerMinute"`
}

// Record represents a single Apple Health Record element.
type Record struct {
	Type          string          `xml:"type,attr"`
	Unit          string          `xml:"unit,attr"`
	Value         string          `xml:"value,attr"`
	SourceName    string          `xml:"sourceName,attr"`
	SourceVersion string          `xml:"sourceVersion,attr"`
	Device        string          `xml:"device,attr"`
	CreationDate  string          `xml:"creationDate,attr"`
	StartDate     string          `xml:"startDate,attr"`
	EndDate       string          `xml:"endDate,attr"`
	Metadata      []MetadataEntry `xml:"MetadataEntry"`
	HRV           *HRVList        `xml:"HeartRateVariabilityMetadataList"`
}

// WorkoutStatistics is a child element of <Workout> that provides aggregated metrics.
type WorkoutStatistics struct {
	Type      string `xml:"type,attr"`
	StartDate string `xml:"startDate,attr"`
	EndDate   string `xml:"endDate,attr"`
	Sum       string `xml:"sum,attr"`
	Average   string `xml:"average,attr"`
	Minimum   string `xml:"minimum,attr"`
	Maximum   string `xml:"maximum,attr"`
	Unit      string `xml:"unit,attr"`
}

// WorkoutEvent is a pause/resume/segment/marker inside a workout.
type WorkoutEvent struct {
	Type         string          `xml:"type,attr"`
	Date         string          `xml:"date,attr"`
	Duration     string          `xml:"duration,attr"`
	DurationUnit string          `xml:"durationUnit,attr"`
	Metadata     []MetadataEntry `xml:"MetadataEntry"`
}

// WorkoutZone is one heart-rate zone bucket.
type WorkoutZone struct {
	Minimum      string `xml:"minimum,attr"`
	Maximum      string `xml:"maximum,attr"`
	Duration     string `xml:"duration,attr"`
	DurationUnit string `xml:"durationUnit,attr"`
}

// WorkoutZoneGroup groups zones of one type (heart rate).
type WorkoutZoneGroup struct {
	Type   string        `xml:"type,attr"`
	Source string        `xml:"source,attr"`
	Unit   string        `xml:"unit,attr"`
	Zones  []WorkoutZone `xml:"WorkoutZone"`
}

// FileReference points at a GPX file inside the export.
type FileReference struct {
	Path string `xml:"path,attr"`
}

// WorkoutRoute is a GPS route recorded during a workout.
type WorkoutRoute struct {
	SourceName    string          `xml:"sourceName,attr"`
	SourceVersion string          `xml:"sourceVersion,attr"`
	Device        string          `xml:"device,attr"`
	CreationDate  string          `xml:"creationDate,attr"`
	StartDate     string          `xml:"startDate,attr"`
	EndDate       string          `xml:"endDate,attr"`
	Metadata      []MetadataEntry `xml:"MetadataEntry"`
	File          *FileReference  `xml:"FileReference"`
}

// Workout represents a single Apple Health Workout element.
type Workout struct {
	ActivityType          string              `xml:"workoutActivityType,attr"`
	Duration              string              `xml:"duration,attr"`
	DurationUnit          string              `xml:"durationUnit,attr"`
	TotalDistance         string              `xml:"totalDistance,attr"`
	TotalDistanceUnit     string              `xml:"totalDistanceUnit,attr"`
	TotalEnergyBurned     string              `xml:"totalEnergyBurned,attr"`
	TotalEnergyBurnedUnit string              `xml:"totalEnergyBurnedUnit,attr"`
	SourceName            string              `xml:"sourceName,attr"`
	SourceVersion         string              `xml:"sourceVersion,attr"`
	Device                string              `xml:"device,attr"`
	CreationDate          string              `xml:"creationDate,attr"`
	StartDate             string              `xml:"startDate,attr"`
	EndDate               string              `xml:"endDate,attr"`
	Metadata              []MetadataEntry     `xml:"MetadataEntry"`
	Statistics            []WorkoutStatistics `xml:"WorkoutStatistics"`
	Events                []WorkoutEvent      `xml:"WorkoutEvent"`
	ZoneGroups            []WorkoutZoneGroup  `xml:"WorkoutZoneGroup"`
	Routes                []WorkoutRoute      `xml:"WorkoutRoute"`
	// Records nested inside a workout (e.g. EstimatedWorkoutEffortScore) are
	// consumed by DecodeElement and would otherwise be lost.
	Records []Record `xml:"Record"`
}

// ActivitySummary is one day of Activity rings.
type ActivitySummary struct {
	Date                   string `xml:"dateComponents,attr"`
	ActiveEnergyBurned     string `xml:"activeEnergyBurned,attr"`
	ActiveEnergyBurnedGoal string `xml:"activeEnergyBurnedGoal,attr"`
	ActiveEnergyBurnedUnit string `xml:"activeEnergyBurnedUnit,attr"`
	AppleMoveTime          string `xml:"appleMoveTime,attr"`
	AppleMoveTimeGoal      string `xml:"appleMoveTimeGoal,attr"`
	AppleExerciseTime      string `xml:"appleExerciseTime,attr"`
	AppleExerciseTimeGoal  string `xml:"appleExerciseTimeGoal,attr"`
	AppleStandHours        string `xml:"appleStandHours,attr"`
	AppleStandHoursGoal    string `xml:"appleStandHoursGoal,attr"`
}

// Me carries the user characteristics exported once per file.
type Me struct {
	DateOfBirth       string `xml:"HKCharacteristicTypeIdentifierDateOfBirth,attr"`
	BiologicalSex     string `xml:"HKCharacteristicTypeIdentifierBiologicalSex,attr"`
	BloodType         string `xml:"HKCharacteristicTypeIdentifierBloodType,attr"`
	SkinType          string `xml:"HKCharacteristicTypeIdentifierFitzpatrickSkinType,attr"`
	CardioFitnessMeds string `xml:"HKCharacteristicTypeIdentifierCardioFitnessMedicationsUse,attr"`
	WheelchairUse     string `xml:"HKCharacteristicTypeIdentifierWheelchairUse,attr"`
}

// TargetRecordTypes maps HK identifiers to table names. It is derived from the
// hk registry and kept for callers that predate the registry. Blood pressure
// uses sentinel names "blood_pressure_systolic"/"blood_pressure_diastolic" and
// is paired in the parser before writing to the "blood_pressure" table.
var TargetRecordTypes = func() map[string]string {
	m := make(map[string]string, len(hk.Metrics))
	for _, metric := range hk.Metrics {
		if metric.Paired {
			if metric.Identifier == "HKQuantityTypeIdentifierBloodPressureSystolic" {
				m[metric.Identifier] = "blood_pressure_systolic"
			} else {
				m[metric.Identifier] = "blood_pressure_diastolic"
			}
			continue
		}
		m[metric.Identifier] = metric.Table
	}
	return m
}()

// RecordColumns returns the column names for a given metric table.
func RecordColumns(table string) []string { return hk.RecordColumns(table) }

// BloodPressureColumns returns the column names for the blood_pressure table.
func BloodPressureColumns() []string { return hk.BloodPressureColumns() }

// WorkoutColumns returns the column names for the workouts table.
func WorkoutColumns() []string { return hk.WorkoutColumns() }
