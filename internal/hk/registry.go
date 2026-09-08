// Package hk is the single registry of Apple HealthKit types that healthsync
// understands. It is a leaf package: both the parser (which maps XML records to
// tables) and storage (which generates the schema from it) import it, so the
// two can never disagree about what a table looks like.
package hk

import "strings"

// Kind distinguishes quantity samples (numeric value + unit) from category
// samples (enumerated text value, no unit attribute in the export).
type Kind int

const (
	Quantity Kind = iota
	Category
)

// Agg describes how a metric aggregates over a day/week/month bucket.
type Agg int

const (
	// Cumulative values are summed after overlap deduplication by source
	// priority (steps, energy, distance). Two sources recording the same walk
	// must not be added together.
	Cumulative Agg = iota
	// Sample values are point measurements: avg/min/max per bucket (heart rate,
	// weight, SpO2).
	Sample
	// Duration metrics are intervals whose length matters (sleep, mindful
	// sessions, stand hours).
	Duration
	// Event metrics are occurrences to be counted (high heart rate events).
	Event
)

func (a Agg) String() string {
	switch a {
	case Cumulative:
		return "cumulative"
	case Sample:
		return "sample"
	case Duration:
		return "duration"
	case Event:
		return "event"
	}
	return "unknown"
}

func (k Kind) String() string {
	if k == Category {
		return "category"
	}
	return "quantity"
}

// Metric is one HealthKit type and the table it lands in.
type Metric struct {
	Key        string // CLI/API key, hyphenated: "resting-heart-rate"
	Identifier string // HKQuantityTypeIdentifierRestingHeartRate
	Table      string // resting_heart_rate
	Kind       Kind
	Agg        Agg
	Unit       string // hint for display; the real unit is stored per row
	Name       string // display name
	Group      string // UI grouping
	// Paired marks the blood pressure sentinels which are staged in the parser
	// and written to the blood_pressure table; they have no table of their own.
	Paired bool
	// Good says which direction of change is desirable for KPI deltas:
	// "up", "down" or "" (neutral).
	Good string
}

// NoUnit reports whether the export omits the unit attribute for this type,
// which means the value column is TEXT and there is no unit column.
func (m Metric) NoUnit() bool { return m.Kind == Category }

// UI groups.
const (
	GroupActivity     = "activity"
	GroupHeart        = "heart"
	GroupSleep        = "sleep"
	GroupBody         = "body"
	GroupMobility     = "mobility"
	GroupRunning      = "running"
	GroupCycling      = "cycling"
	GroupRespiratory  = "respiratory"
	GroupHearing      = "hearing"
	GroupEnvironment  = "environment"
	GroupNutrition    = "nutrition"
	GroupReproductive = "reproductive"
	GroupMindfulness  = "mindfulness"
	GroupSymptoms     = "symptoms"
	GroupOther        = "other"
)

// Generic fallback tables. Any Record whose type is not in the registry is
// written here with its raw HK identifier in the `type` column, so an export
// never loses a record just because we have not named its table yet.
const (
	OtherQuantityTable = "other_quantity_records"
	OtherCategoryTable = "other_category_records"
	BloodPressureTable = "blood_pressure"
	WorkoutsTable      = "workouts"
)

// FidelityColumns are appended to every record-shaped table so nothing from the
// export is dropped: who wrote it, from which device, when, and its metadata.
var FidelityColumns = []string{"source_version", "device_id", "creation_date", "metadata"}

func q(key, ident, table, unit, name, group string, agg Agg, good string) Metric {
	return Metric{Key: key, Identifier: "HKQuantityTypeIdentifier" + ident, Table: table, Kind: Quantity, Agg: agg, Unit: unit, Name: name, Group: group, Good: good}
}

func c(key, ident, table, name, group string, agg Agg) Metric {
	return Metric{Key: key, Identifier: "HKCategoryTypeIdentifier" + ident, Table: table, Kind: Category, Agg: agg, Name: name, Group: group}
}

// Metrics is the ordered registry. Order drives CLI help and UI grouping.
var Metrics = []Metric{
	// --- Activity ---
	q("steps", "StepCount", "steps", "count", "Steps", GroupActivity, Cumulative, "up"),
	q("active-energy", "ActiveEnergyBurned", "active_energy", "kcal", "Active Energy", GroupActivity, Cumulative, "up"),
	q("basal-energy", "BasalEnergyBurned", "basal_energy", "kcal", "Resting Energy", GroupActivity, Cumulative, ""),
	q("exercise-time", "AppleExerciseTime", "exercise_time", "min", "Exercise Minutes", GroupActivity, Cumulative, "up"),
	q("stand-time", "AppleStandTime", "stand_time", "min", "Stand Minutes", GroupActivity, Cumulative, "up"),
	q("move-time", "AppleMoveTime", "apple_move_time", "min", "Move Minutes", GroupActivity, Cumulative, "up"),
	q("flights-climbed", "FlightsClimbed", "flights_climbed", "count", "Flights Climbed", GroupActivity, Cumulative, "up"),
	q("distance-walking-running", "DistanceWalkingRunning", "distance_walking_running", "km", "Walking + Running Distance", GroupActivity, Cumulative, "up"),
	q("distance-cycling", "DistanceCycling", "distance_cycling", "km", "Cycling Distance", GroupActivity, Cumulative, "up"),
	q("distance-swimming", "DistanceSwimming", "distance_swimming", "m", "Swimming Distance", GroupActivity, Cumulative, "up"),
	q("swimming-strokes", "SwimmingStrokeCount", "swimming_stroke_count", "count", "Swimming Strokes", GroupActivity, Cumulative, ""),
	q("distance-downhill-snow-sports", "DistanceDownhillSnowSports", "distance_downhill_snow_sports", "km", "Downhill Snow Sports Distance", GroupActivity, Cumulative, ""),
	q("distance-cross-country-skiing", "DistanceCrossCountrySkiing", "distance_cross_country_skiing", "km", "Cross Country Skiing Distance", GroupActivity, Cumulative, ""),
	q("distance-wheelchair", "DistanceWheelchair", "distance_wheelchair", "km", "Wheelchair Distance", GroupActivity, Cumulative, ""),
	q("push-count", "PushCount", "push_count", "count", "Wheelchair Pushes", GroupActivity, Cumulative, ""),
	q("distance-paddle-sports", "DistancePaddleSports", "distance_paddle_sports", "km", "Paddle Sports Distance", GroupActivity, Cumulative, ""),
	q("distance-rowing", "DistanceRowing", "distance_rowing", "km", "Rowing Distance", GroupActivity, Cumulative, ""),
	q("physical-effort", "PhysicalEffort", "physical_effort", "kcal/hr·kg", "Physical Effort", GroupActivity, Sample, ""),
	q("workout-effort-score", "WorkoutEffortScore", "workout_effort_score", "appleEffortScore", "Workout Effort", GroupActivity, Sample, ""),
	q("estimated-workout-effort-score", "EstimatedWorkoutEffortScore", "estimated_workout_effort_score", "appleEffortScore", "Estimated Workout Effort", GroupActivity, Sample, ""),
	q("underwater-depth", "UnderwaterDepth", "underwater_depth", "m", "Underwater Depth", GroupActivity, Sample, ""),
	q("times-fallen", "NumberOfTimesFallen", "number_of_times_fallen", "count", "Times Fallen", GroupActivity, Cumulative, "down"),
	c("stand-hours", "AppleStandHour", "stand_hours", "Stand Hours", GroupActivity, Event),

	// --- Heart ---
	q("heart-rate", "HeartRate", "heart_rate", "count/min", "Heart Rate", GroupHeart, Sample, ""),
	q("resting-heart-rate", "RestingHeartRate", "resting_heart_rate", "count/min", "Resting Heart Rate", GroupHeart, Sample, "down"),
	q("walking-heart-rate", "WalkingHeartRateAverage", "walking_heart_rate", "count/min", "Walking Heart Rate", GroupHeart, Sample, "down"),
	q("hrv", "HeartRateVariabilitySDNN", "hrv", "ms", "Heart Rate Variability", GroupHeart, Sample, "up"),
	q("heart-rate-recovery", "HeartRateRecoveryOneMinute", "heart_rate_recovery", "count/min", "Heart Rate Recovery", GroupHeart, Sample, "up"),
	q("vo2max", "VO2Max", "vo2_max", "mL/min·kg", "VO₂ Max", GroupHeart, Sample, "up"),
	q("afib-burden", "AtrialFibrillationBurden", "atrial_fibrillation_burden", "%", "AFib Burden", GroupHeart, Sample, "down"),
	q("peripheral-perfusion-index", "PeripheralPerfusionIndex", "peripheral_perfusion_index", "%", "Peripheral Perfusion Index", GroupHeart, Sample, ""),
	{Key: "blood-pressure-systolic", Identifier: "HKQuantityTypeIdentifierBloodPressureSystolic", Table: BloodPressureTable, Kind: Quantity, Agg: Sample, Unit: "mmHg", Name: "Blood Pressure (Systolic)", Group: GroupHeart, Paired: true, Good: "down"},
	{Key: "blood-pressure-diastolic", Identifier: "HKQuantityTypeIdentifierBloodPressureDiastolic", Table: BloodPressureTable, Kind: Quantity, Agg: Sample, Unit: "mmHg", Name: "Blood Pressure (Diastolic)", Group: GroupHeart, Paired: true, Good: "down"},
	c("high-heart-rate-events", "HighHeartRateEvent", "high_heart_rate_events", "High Heart Rate Events", GroupHeart, Event),
	c("low-heart-rate-events", "LowHeartRateEvent", "low_heart_rate_events", "Low Heart Rate Events", GroupHeart, Event),
	c("irregular-rhythm-events", "IrregularHeartRhythmEvent", "irregular_rhythm_events", "Irregular Rhythm Events", GroupHeart, Event),

	// --- Respiratory ---
	q("respiratory-rate", "RespiratoryRate", "respiratory_rate", "count/min", "Respiratory Rate", GroupRespiratory, Sample, ""),
	q("spo2", "OxygenSaturation", "spo2", "%", "Blood Oxygen", GroupRespiratory, Sample, "up"),
	q("forced-vital-capacity", "ForcedVitalCapacity", "forced_vital_capacity", "L", "Forced Vital Capacity", GroupRespiratory, Sample, "up"),
	q("fev1", "ForcedExpiratoryVolume1", "fev1", "L", "FEV1", GroupRespiratory, Sample, "up"),
	q("peak-expiratory-flow", "PeakExpiratoryFlowRate", "peak_expiratory_flow_rate", "L/min", "Peak Expiratory Flow", GroupRespiratory, Sample, "up"),
	q("inhaler-usage", "InhalerUsage", "inhaler_usage", "count", "Inhaler Usage", GroupRespiratory, Cumulative, "down"),

	// --- Sleep ---
	c("sleep", "SleepAnalysis", "sleep", "Sleep", GroupSleep, Duration),
	q("wrist-temperature", "AppleSleepingWristTemperature", "wrist_temperature", "degC", "Wrist Temperature", GroupSleep, Sample, ""),
	q("breathing-disturbances", "AppleSleepingBreathingDisturbances", "sleeping_breathing_disturbances", "count", "Breathing Disturbances", GroupSleep, Sample, "down"),
	{Key: "sleep-duration-goal", Identifier: "HKDataTypeSleepDurationGoal", Table: "sleep_duration_goal", Kind: Quantity, Agg: Sample, Unit: "hr", Name: "Sleep Duration Goal", Group: GroupSleep},

	// --- Body ---
	q("body-mass", "BodyMass", "body_mass", "kg", "Weight", GroupBody, Sample, ""),
	q("bmi", "BodyMassIndex", "body_mass_index", "count", "Body Mass Index", GroupBody, Sample, ""),
	q("height", "Height", "height", "cm", "Height", GroupBody, Sample, ""),
	q("body-fat", "BodyFatPercentage", "body_fat_percentage", "%", "Body Fat", GroupBody, Sample, "down"),
	q("lean-body-mass", "LeanBodyMass", "lean_body_mass", "kg", "Lean Body Mass", GroupBody, Sample, "up"),
	q("waist-circumference", "WaistCircumference", "waist_circumference", "cm", "Waist Circumference", GroupBody, Sample, "down"),
	q("body-temperature", "BodyTemperature", "body_temperature", "degC", "Body Temperature", GroupBody, Sample, ""),
	q("basal-body-temperature", "BasalBodyTemperature", "basal_body_temperature", "degC", "Basal Body Temperature", GroupBody, Sample, ""),
	q("blood-glucose", "BloodGlucose", "blood_glucose", "mg/dL", "Blood Glucose", GroupBody, Sample, ""),
	q("insulin-delivery", "InsulinDelivery", "insulin_delivery", "IU", "Insulin Delivery", GroupBody, Cumulative, ""),
	q("blood-alcohol", "BloodAlcoholContent", "blood_alcohol_content", "%", "Blood Alcohol Content", GroupBody, Sample, "down"),
	q("electrodermal-activity", "ElectrodermalActivity", "electrodermal_activity", "mcS", "Electrodermal Activity", GroupBody, Sample, ""),

	// --- Mobility / Walking ---
	q("walking-speed", "WalkingSpeed", "walking_speed", "km/hr", "Walking Speed", GroupMobility, Sample, "up"),
	q("walking-step-length", "WalkingStepLength", "walking_step_length", "cm", "Walking Step Length", GroupMobility, Sample, "up"),
	q("walking-asymmetry", "WalkingAsymmetryPercentage", "walking_asymmetry", "%", "Walking Asymmetry", GroupMobility, Sample, "down"),
	q("walking-double-support", "WalkingDoubleSupportPercentage", "walking_double_support", "%", "Double Support Time", GroupMobility, Sample, "down"),
	q("walking-steadiness", "AppleWalkingSteadiness", "walking_steadiness", "%", "Walking Steadiness", GroupMobility, Sample, "up"),
	q("stair-ascent-speed", "StairAscentSpeed", "stair_ascent_speed", "m/s", "Stair Ascent Speed", GroupMobility, Sample, "up"),
	q("stair-descent-speed", "StairDescentSpeed", "stair_descent_speed", "m/s", "Stair Descent Speed", GroupMobility, Sample, "up"),
	q("six-minute-walk", "SixMinuteWalkTestDistance", "six_minute_walk", "m", "Six-Minute Walk Distance", GroupMobility, Sample, "up"),

	// --- Running ---
	q("running-speed", "RunningSpeed", "running_speed", "km/hr", "Running Speed", GroupRunning, Sample, "up"),
	q("running-power", "RunningPower", "running_power", "W", "Running Power", GroupRunning, Sample, "up"),
	q("running-stride-length", "RunningStrideLength", "running_stride_length", "m", "Running Stride Length", GroupRunning, Sample, ""),
	q("running-ground-contact-time", "RunningGroundContactTime", "running_ground_contact_time", "ms", "Ground Contact Time", GroupRunning, Sample, "down"),
	q("running-vertical-oscillation", "RunningVerticalOscillation", "running_vertical_oscillation", "cm", "Vertical Oscillation", GroupRunning, Sample, "down"),

	// --- Cycling ---
	q("cycling-power", "CyclingPower", "cycling_power", "W", "Cycling Power", GroupCycling, Sample, "up"),
	q("cycling-cadence", "CyclingCadence", "cycling_cadence", "count/min", "Cycling Cadence", GroupCycling, Sample, ""),
	q("cycling-speed", "CyclingSpeed", "cycling_speed", "km/hr", "Cycling Speed", GroupCycling, Sample, "up"),
	q("cycling-ftp", "CyclingFunctionalThresholdPower", "cycling_functional_threshold_power", "W", "Cycling FTP", GroupCycling, Sample, "up"),

	// --- Hearing ---
	q("headphone-audio-exposure", "HeadphoneAudioExposure", "headphone_audio_exposure", "dBASPL", "Headphone Audio Exposure", GroupHearing, Sample, "down"),
	q("environmental-audio-exposure", "EnvironmentalAudioExposure", "environmental_audio_exposure", "dBASPL", "Environmental Sound Levels", GroupHearing, Sample, "down"),
	c("audio-exposure-events", "AudioExposureEvent", "audio_exposure_events", "Audio Exposure Events", GroupHearing, Event),
	c("environmental-audio-exposure-events", "EnvironmentalAudioExposureEvent", "environmental_audio_exposure_events", "Environmental Noise Events", GroupHearing, Event),
	c("headphone-audio-exposure-events", "HeadphoneAudioExposureEvent", "headphone_audio_exposure_events", "Headphone Noise Events", GroupHearing, Event),

	// --- Environment ---
	q("time-in-daylight", "TimeInDaylight", "time_in_daylight", "min", "Time in Daylight", GroupEnvironment, Cumulative, "up"),
	q("uv-exposure", "UVExposure", "uv_exposure", "count", "UV Exposure", GroupEnvironment, Sample, ""),
	q("water-temperature", "WaterTemperature", "water_temperature", "degC", "Water Temperature", GroupEnvironment, Sample, ""),

	// --- Nutrition ---
	q("dietary-water", "DietaryWater", "dietary_water", "mL", "Water", GroupNutrition, Cumulative, "up"),
	q("dietary-energy", "DietaryEnergyConsumed", "dietary_energy", "kcal", "Dietary Energy", GroupNutrition, Cumulative, ""),
	q("dietary-protein", "DietaryProtein", "dietary_protein", "g", "Protein", GroupNutrition, Cumulative, ""),
	q("dietary-carbohydrates", "DietaryCarbohydrates", "dietary_carbohydrates", "g", "Carbohydrates", GroupNutrition, Cumulative, ""),
	q("dietary-fat", "DietaryFatTotal", "dietary_fat_total", "g", "Total Fat", GroupNutrition, Cumulative, ""),
	q("dietary-fiber", "DietaryFiber", "dietary_fiber", "g", "Fiber", GroupNutrition, Cumulative, ""),
	q("dietary-sugar", "DietarySugar", "dietary_sugar", "g", "Sugar", GroupNutrition, Cumulative, ""),
	q("dietary-sodium", "DietarySodium", "dietary_sodium", "mg", "Sodium", GroupNutrition, Cumulative, ""),
	q("dietary-caffeine", "DietaryCaffeine", "dietary_caffeine", "mg", "Caffeine", GroupNutrition, Cumulative, ""),

	// --- Mindfulness ---
	c("mindful-sessions", "MindfulSession", "mindful_sessions", "Mindful Sessions", GroupMindfulness, Duration),

	// --- Reproductive health ---
	c("menstrual-flow", "MenstrualFlow", "menstrual_flow", "Menstrual Flow", GroupReproductive, Event),
	c("intermenstrual-bleeding", "IntermenstrualBleeding", "intermenstrual_bleeding", "Spotting", GroupReproductive, Event),
	c("ovulation-test", "OvulationTestResult", "ovulation_test_result", "Ovulation Test Result", GroupReproductive, Event),
	c("cervical-mucus", "CervicalMucusQuality", "cervical_mucus_quality", "Cervical Mucus Quality", GroupReproductive, Event),
	c("sexual-activity", "SexualActivity", "sexual_activity", "Sexual Activity", GroupReproductive, Event),
	c("pregnancy-test", "PregnancyTestResult", "pregnancy_test_result", "Pregnancy Test Result", GroupReproductive, Event),

	// --- Symptoms / hygiene ---
	c("handwashing", "HandwashingEvent", "handwashing_events", "Handwashing", GroupSymptoms, Duration),
	c("toothbrushing", "ToothbrushingEvent", "toothbrushing_events", "Toothbrushing", GroupSymptoms, Duration),
	c("headache", "Headache", "headache", "Headache", GroupSymptoms, Event),
	c("nausea", "Nausea", "nausea", "Nausea", GroupSymptoms, Event),
	c("fatigue", "Fatigue", "fatigue", "Fatigue", GroupSymptoms, Event),
	c("sleep-changes", "SleepChanges", "sleep_changes", "Sleep Changes", GroupSymptoms, Event),
}

var (
	// ByIdentifier maps an HK identifier to its metric.
	ByIdentifier = map[string]*Metric{}
	// ByTable maps a table name to its metric. Blood pressure sentinels are not
	// included (two metrics share one table); use BloodPressureTable directly.
	ByTable = map[string]*Metric{}
	// ByKey maps CLI/API keys in both hyphen and underscore forms, plus the
	// raw table name, to the metric.
	ByKey = map[string]*Metric{}
)

func init() {
	for i := range Metrics {
		m := &Metrics[i]
		if _, dup := ByIdentifier[m.Identifier]; dup {
			panic("hk: duplicate identifier " + m.Identifier)
		}
		ByIdentifier[m.Identifier] = m
		if !m.Paired {
			if _, dup := ByTable[m.Table]; dup {
				panic("hk: duplicate table " + m.Table)
			}
			ByTable[m.Table] = m
		}
		ByKey[m.Key] = m
		ByKey[strings.ReplaceAll(m.Key, "-", "_")] = m
		if !m.Paired {
			ByKey[m.Table] = m
		}
	}
	// Legacy aliases kept for CLI compatibility.
	ByKey["vo2_max"] = ByIdentifier["HKQuantityTypeIdentifierVO2Max"]
	ByKey["body-mass-index"] = ByIdentifier["HKQuantityTypeIdentifierBodyMassIndex"]
	ByKey["body_mass_index"] = ByKey["body-mass-index"]
}

// Lookup resolves a CLI/API key, table name or HK identifier to a metric.
func Lookup(name string) (*Metric, bool) {
	if m, ok := ByKey[name]; ok {
		return m, true
	}
	if m, ok := ByIdentifier[name]; ok {
		return m, true
	}
	return nil, false
}

// Tables returns every metric table name (excluding blood pressure sentinels),
// in registry order.
func Tables() []string {
	out := make([]string, 0, len(Metrics))
	for _, m := range Metrics {
		if m.Paired {
			continue
		}
		out = append(out, m.Table)
	}
	return out
}

// Keys returns the canonical hyphenated CLI keys of all metrics that have their
// own table, plus "blood-pressure" and "workouts".
func Keys() []string {
	out := make([]string, 0, len(Metrics)+2)
	for _, m := range Metrics {
		if m.Paired {
			continue
		}
		out = append(out, m.Key)
	}
	out = append(out, "blood-pressure", "workouts")
	return out
}

// RecordColumns returns the insert columns for a metric table. Category tables
// have no unit column. Fidelity columns are always appended.
func RecordColumns(table string) []string {
	base := []string{"source_name", "start_date", "end_date", "value"}
	if !IsNoUnitTable(table) {
		base = append(base, "unit")
	}
	return append(base, FidelityColumns...)
}

// OtherColumns returns the insert columns for the generic fallback tables,
// which carry the raw HK type in front of the standard shape.
func OtherColumns(table string) []string {
	return append([]string{"type"}, RecordColumns(table)...)
}

// IsNoUnitTable reports whether a table stores TEXT category values without a
// unit column.
func IsNoUnitTable(table string) bool {
	if table == OtherCategoryTable {
		return true
	}
	if table == OtherQuantityTable {
		return false
	}
	if m, ok := ByTable[table]; ok {
		return m.NoUnit()
	}
	return false
}

// BloodPressureColumns returns the insert columns for the blood_pressure table.
func BloodPressureColumns() []string {
	return append([]string{"source_name", "start_date", "end_date", "systolic", "diastolic", "unit"}, FidelityColumns...)
}

// WorkoutColumns returns the insert columns for the workouts table.
func WorkoutColumns() []string {
	return append([]string{
		"activity_type", "source_name", "start_date", "end_date",
		"duration", "duration_unit",
		"total_distance", "total_distance_unit",
		"total_energy_burned", "total_energy_burned_unit",
	}, FidelityColumns...)
}

// IsQuantityIdentifier guesses the kind of an unknown identifier from its
// name so it can be routed to the right generic table. Anything with a unit
// attribute is quantity; the parser passes that in as hasUnit.
func IsQuantityIdentifier(identifier string, hasUnit bool) bool {
	if hasUnit {
		return true
	}
	return strings.HasPrefix(identifier, "HKQuantityTypeIdentifier")
}
