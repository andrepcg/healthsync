package people

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_CRUD(t *testing.T) {
	s := openStore(t)
	p, err := s.Create(Person{Name: "  Ana  ", DOB: "1990-05-01"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.Name != "Ana" || p.Color == "" || p.Emoji == "" {
		t.Errorf("created: %+v", p)
	}
	if _, err := os.Stat(s.DBPath(p.ID)); err != nil {
		t.Errorf("db file not created: %v", err)
	}
	if _, err := s.Create(Person{Name: "ana"}); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("duplicate name (case-insensitive) should fail, got %v", err)
	}
	if _, err := s.Create(Person{Name: "  "}); err == nil {
		t.Error("blank name must fail")
	}
	byName, err := s.Get("ANA")
	if err != nil || byName.ID != p.ID {
		t.Errorf("get by name: %v %+v", err, byName)
	}
	if _, err := s.Get("nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	up, err := s.Update(Person{ID: p.ID, Emoji: "🏃", Color: "#000000"})
	if err != nil || up.Emoji != "🏃" || up.Name != "Ana" || up.DOB != "1990-05-01" {
		t.Errorf("update: %v %+v", err, up)
	}
	changed, err := s.FillProfile(p.ID, map[string]string{"date_of_birth": "2000-01-01", "biological_sex": "Female"})
	if err != nil || !changed {
		t.Fatalf("fill: %v %v", err, changed)
	}
	got, _ := s.Get(p.ID)
	if got.DOB != "1990-05-01" || got.Sex != "female" {
		t.Errorf("fill must not overwrite dob but must set empty sex: %+v", got)
	}

	db1, _ := s.DB(p.ID)
	db2, _ := s.DB(p.ID)
	if db1 != db2 {
		t.Error("DB() should be cached")
	}
	list, _ := s.List()
	if len(list) != 1 {
		t.Errorf("list: %d", len(list))
	}

	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	for _, suf := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(s.DBPath(p.ID) + suf); !os.IsNotExist(err) {
			t.Errorf("file %s%s still exists", filepath.Base(s.DBPath(p.ID)), suf)
		}
	}
	if _, err := s.Get(p.ID); !errors.Is(err, ErrNotFound) {
		t.Error("deleted person still found")
	}
	if err := s.Delete(p.ID); !errors.Is(err, ErrNotFound) {
		t.Error("double delete should be not found")
	}
}

func TestDefaultDataDir_Env(t *testing.T) {
	t.Setenv("HEALTHSYNC_DATA_DIR", "/data")
	if DefaultDataDir() != "/data" {
		t.Error("env var not honoured")
	}
}
