// Package people manages the family registry: one row per person in
// DATA_DIR/people.db and one full healthsync database per person in
// DATA_DIR/people/<id>.db. Keeping each person in their own file means the
// existing single-user schema, CLI and query helpers apply unchanged, imports
// never contend across people, and deleting a person is deleting a file.
package people

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/BRO3886/healthsync/internal/storage"
)

// ErrNotFound is returned when no person matches.
var ErrNotFound = errors.New("person not found")

// ErrDuplicateName is returned when a name is already taken.
var ErrDuplicateName = errors.New("a person with that name already exists")

// Person is one family member.
type Person struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Emoji     string `json:"emoji"`
	DOB       string `json:"dob"`
	Sex       string `json:"sex"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Store is the registry plus a cache of open per-person databases.
type Store struct {
	dataDir string
	reg     *sql.DB

	mu   sync.Mutex
	open map[string]*storage.DB
}

// DefaultDataDir returns ~/.healthsync, or HEALTHSYNC_DATA_DIR when set.
func DefaultDataDir() string {
	if v := os.Getenv("HEALTHSYNC_DATA_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".healthsync"
	}
	return filepath.Join(home, ".healthsync")
}

// Open creates the data directory layout and opens the registry.
func Open(dataDir string) (*Store, error) {
	for _, d := range []string{dataDir, filepath.Join(dataDir, "people"), filepath.Join(dataDir, "tmp")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", d, err)
		}
	}
	reg, err := sql.Open("sqlite", filepath.Join(dataDir, "people.db"))
	if err != nil {
		return nil, fmt.Errorf("opening people.db: %w", err)
	}
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL"} {
		if _, err := reg.Exec(p); err != nil {
			reg.Close()
			return nil, err
		}
	}
	if _, err := reg.Exec(`CREATE TABLE IF NOT EXISTS people (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		color TEXT,
		emoji TEXT,
		dob TEXT,
		sex TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		reg.Close()
		return nil, fmt.Errorf("migrating people.db: %w", err)
	}
	return &Store{dataDir: dataDir, reg: reg, open: map[string]*storage.DB{}}, nil
}

// DataDir returns the root directory.
func (s *Store) DataDir() string { return s.dataDir }

// TmpDir is where uploads are staged before parsing (same filesystem as the
// databases, so it works inside a container with a single volume).
func (s *Store) TmpDir() string { return filepath.Join(s.dataDir, "tmp") }

// DBPath returns the SQLite path for a person id.
func (s *Store) DBPath(id string) string {
	return filepath.Join(s.dataDir, "people", id+".db")
}

// Close closes every cached person database and the registry.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, db := range s.open {
		db.Close()
		delete(s.open, id)
	}
	return s.reg.Close()
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const cols = `id, name, color, emoji, dob, sex, created_at, updated_at`

func scan(r interface{ Scan(...any) error }) (Person, error) {
	var p Person
	var color, emoji, dob, sex sql.NullString
	if err := r.Scan(&p.ID, &p.Name, &color, &emoji, &dob, &sex, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return p, err
	}
	p.Color, p.Emoji, p.DOB, p.Sex = color.String, emoji.String, dob.String, sex.String
	return p, nil
}

// List returns everyone, oldest first.
func (s *Store) List() ([]Person, error) {
	rows, err := s.reg.Query(`SELECT ` + cols + ` FROM people ORDER BY created_at, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get finds a person by id, falling back to a case-insensitive name match.
func (s *Store) Get(idOrName string) (Person, error) {
	p, err := scan(s.reg.QueryRow(`SELECT `+cols+` FROM people WHERE id = ?`, idOrName))
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	p, err = scan(s.reg.QueryRow(`SELECT `+cols+` FROM people WHERE name = ? COLLATE NOCASE`, idOrName))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// Create adds a person and creates their empty database.
func (s *Store) Create(p Person) (Person, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, errors.New("name is required")
	}
	id, err := newID()
	if err != nil {
		return p, err
	}
	p.ID = id
	p.CreatedAt = now()
	p.UpdatedAt = p.CreatedAt
	if p.Color == "" {
		p.Color = defaultColor(p.Name)
	}
	if p.Emoji == "" {
		p.Emoji = "🙂"
	}
	_, err = s.reg.Exec(`INSERT INTO people (`+cols+`) VALUES (?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Color, p.Emoji, p.DOB, p.Sex, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return p, ErrDuplicateName
		}
		return p, fmt.Errorf("inserting person: %w", err)
	}
	// Create the database now so the file exists and the schema is ready.
	if _, err := s.DB(p.ID); err != nil {
		s.reg.Exec(`DELETE FROM people WHERE id = ?`, p.ID)
		return p, err
	}
	return p, nil
}

// Update changes name/color/emoji/dob/sex. Empty fields are left as they are.
func (s *Store) Update(p Person) (Person, error) {
	cur, err := s.Get(p.ID)
	if err != nil {
		return cur, err
	}
	if n := strings.TrimSpace(p.Name); n != "" {
		cur.Name = n
	}
	if p.Color != "" {
		cur.Color = p.Color
	}
	if p.Emoji != "" {
		cur.Emoji = p.Emoji
	}
	if p.DOB != "" {
		cur.DOB = p.DOB
	}
	if p.Sex != "" {
		cur.Sex = p.Sex
	}
	cur.UpdatedAt = now()
	_, err = s.reg.Exec(`UPDATE people SET name=?, color=?, emoji=?, dob=?, sex=?, updated_at=? WHERE id=?`,
		cur.Name, cur.Color, cur.Emoji, cur.DOB, cur.Sex, cur.UpdatedAt, cur.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return cur, ErrDuplicateName
		}
		return cur, err
	}
	return cur, nil
}

// FillProfile sets dob/sex from an import's <Me> data when they are empty.
func (s *Store) FillProfile(id string, profile map[string]string) (bool, error) {
	cur, err := s.Get(id)
	if err != nil {
		return false, err
	}
	changed := false
	if cur.DOB == "" && profile["date_of_birth"] != "" {
		cur.DOB = profile["date_of_birth"]
		changed = true
	}
	if cur.Sex == "" && profile["biological_sex"] != "" && profile["biological_sex"] != "NotSet" {
		cur.Sex = strings.ToLower(profile["biological_sex"])
		changed = true
	}
	if !changed {
		return false, nil
	}
	_, err = s.reg.Exec(`UPDATE people SET dob=?, sex=?, updated_at=? WHERE id=?`, cur.DOB, cur.Sex, now(), cur.ID)
	return err == nil, err
}

// Delete removes the person and their database files.
func (s *Store) Delete(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	s.mu.Lock()
	if db, ok := s.open[id]; ok {
		db.Close()
		delete(s.open, id)
	}
	s.mu.Unlock()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(s.DBPath(id) + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", s.DBPath(id)+suffix, err)
		}
	}
	_, err := s.reg.Exec(`DELETE FROM people WHERE id = ?`, id)
	return err
}

// DB returns the (cached) open database for a person.
func (s *Store) DB(id string) (*storage.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.open[id]; ok {
		return db, nil
	}
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	db, err := storage.Open(s.DBPath(id))
	if err != nil {
		return nil, err
	}
	s.open[id] = db
	return db, nil
}

var palette = []string{"#e4572e", "#17bebb", "#ffc914", "#2e282a", "#76b041", "#8a4fff", "#ff7f51", "#3c91e6"}

func defaultColor(name string) string {
	var h uint32
	for _, r := range name {
		h = h*31 + uint32(r)
	}
	return palette[int(h%uint32(len(palette)))]
}
