package hk

import (
	"strings"
	"testing"
)

func TestRegistry_UniqueAndConsistent(t *testing.T) {
	ids := map[string]bool{}
	tables := map[string]bool{}
	keys := map[string]bool{}
	for _, m := range Metrics {
		if ids[m.Identifier] {
			t.Errorf("duplicate identifier %s", m.Identifier)
		}
		ids[m.Identifier] = true
		if keys[m.Key] {
			t.Errorf("duplicate key %s", m.Key)
		}
		keys[m.Key] = true
		if !m.Paired {
			if tables[m.Table] {
				t.Errorf("duplicate table %s", m.Table)
			}
			tables[m.Table] = true
		}
		if strings.Contains(m.Key, "_") {
			t.Errorf("key %s must be hyphenated", m.Key)
		}
		if m.Kind == Category && m.Unit != "" {
			t.Errorf("category %s must not have a unit", m.Key)
		}
		if m.Name == "" || m.Group == "" {
			t.Errorf("%s needs a name and group", m.Key)
		}
	}
	if len(Metrics) < 100 {
		t.Errorf("expected 100+ metrics, got %d", len(Metrics))
	}
}

func TestLookup_AcceptsAllForms(t *testing.T) {
	for _, name := range []string{"resting-heart-rate", "resting_heart_rate", "HKQuantityTypeIdentifierRestingHeartRate", "vo2max", "vo2_max", "bmi", "body_mass_index"} {
		if _, ok := Lookup(name); !ok {
			t.Errorf("Lookup(%q) failed", name)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("unknown name resolved")
	}
	if m := ByKey["blood-pressure-systolic"]; m == nil || !m.Paired || m.Table != BloodPressureTable {
		t.Error("blood pressure sentinel misconfigured")
	}
	if _, ok := ByTable[BloodPressureTable]; ok {
		t.Error("paired metrics must not claim the table")
	}
}

func TestColumns(t *testing.T) {
	if got := RecordColumns("heart_rate"); len(got) != 9 || got[4] != "unit" || got[8] != "metadata" {
		t.Errorf("quantity columns: %v", got)
	}
	if got := RecordColumns("sleep"); len(got) != 8 || got[3] != "value" || got[4] != "source_version" {
		t.Errorf("category columns: %v", got)
	}
	if got := OtherColumns(OtherQuantityTable); got[0] != "type" || len(got) != 10 {
		t.Errorf("other quantity columns: %v", got)
	}
	if got := OtherColumns(OtherCategoryTable); len(got) != 9 {
		t.Errorf("other category columns: %v", got)
	}
	if !IsNoUnitTable("high_heart_rate_events") || IsNoUnitTable("steps") || !IsNoUnitTable(OtherCategoryTable) {
		t.Error("IsNoUnitTable wrong")
	}
	if len(WorkoutColumns()) != 14 || len(BloodPressureColumns()) != 10 {
		t.Error("workout/bp column counts")
	}
	if !IsQuantityIdentifier("HKQuantityTypeIdentifierFuture", false) || IsQuantityIdentifier("HKCategoryTypeIdentifierFuture", false) || !IsQuantityIdentifier("HKDataTypeX", true) {
		t.Error("IsQuantityIdentifier wrong")
	}
	keys := Keys()
	if keys[len(keys)-1] != "workouts" || keys[len(keys)-2] != "blood-pressure" {
		t.Errorf("Keys tail: %v", keys[len(keys)-2:])
	}
}
