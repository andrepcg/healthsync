package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/BRO3886/healthsync/internal/hk"
)

// schemaVersion is stored in PRAGMA user_version. Bump it when migrate() gains
// a step that older databases need to run.
const schemaVersion = 2

// DB wraps the sql.DB connection and provides health data operations.
type DB struct {
	conn *sql.DB
}

// DefaultDBPath returns ~/.healthsync/healthsync.db
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "healthsync.db"
	}
	return filepath.Join(home, ".healthsync", "healthsync.db")
}

// Open opens (or creates) the SQLite database at the given path.
func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode and foreign keys
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB for direct access.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// fidelityColumnTypes are the columns appended to every record-shaped table.
// They are also what ensureColumns adds to databases created before v2.
var fidelityColumnTypes = [][2]string{
	{"source_version", "TEXT"},
	{"device_id", "INTEGER"},
	{"creation_date", "TEXT"},
	{"metadata", "TEXT"},
}

// recordTableDDL generates the CREATE TABLE for a metric table. Category
// tables store TEXT values and have no unit column. Generic fallback tables
// carry a leading `type` column that is part of the dedup key.
func recordTableDDL(table string, noUnit bool, withType bool) []string {
	var cols []string
	cols = append(cols, "id INTEGER PRIMARY KEY AUTOINCREMENT")
	if withType {
		cols = append(cols, "type TEXT NOT NULL")
	}
	cols = append(cols,
		"source_name TEXT NOT NULL",
		"start_date TEXT NOT NULL",
		"end_date TEXT NOT NULL",
	)
	if noUnit {
		cols = append(cols, "value TEXT NOT NULL")
	} else {
		cols = append(cols, "value REAL NOT NULL", "unit TEXT NOT NULL")
	}
	for _, fc := range fidelityColumnTypes {
		cols = append(cols, fc[0]+" "+fc[1])
	}
	cols = append(cols, "created_at TEXT DEFAULT CURRENT_TIMESTAMP")
	unique := "UNIQUE(source_name, start_date, end_date, value)"
	if withType {
		unique = "UNIQUE(type, source_name, start_date, end_date, value)"
	}
	cols = append(cols, unique)

	stmts := []string{
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\t%s\n)", table, strings.Join(cols, ",\n\t")),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_start_date ON %s(start_date)", table, table),
	}
	if withType {
		stmts = append(stmts, fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_type_start ON %s(type, start_date)", table, table))
	}
	return stmts
}

// recordTables lists every record-shaped table (metric tables plus the two
// generic fallbacks) with its shape, so migration and ensureColumns agree.
func recordTables() []struct {
	name     string
	noUnit   bool
	withType bool
} {
	var out []struct {
		name     string
		noUnit   bool
		withType bool
	}
	for _, t := range hk.Tables() {
		out = append(out, struct {
			name     string
			noUnit   bool
			withType bool
		}{t, hk.IsNoUnitTable(t), false})
	}
	out = append(out,
		struct {
			name     string
			noUnit   bool
			withType bool
		}{hk.OtherQuantityTable, false, true},
		struct {
			name     string
			noUnit   bool
			withType bool
		}{hk.OtherCategoryTable, true, true},
	)
	return out
}

func (db *DB) migrate() error {
	var statements []string

	for _, t := range recordTables() {
		statements = append(statements, recordTableDDL(t.name, t.noUnit, t.withType)...)
	}

	statements = append(statements,
		`CREATE TABLE IF NOT EXISTS blood_pressure (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_name TEXT NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			systolic REAL NOT NULL,
			diastolic REAL NOT NULL,
			unit TEXT NOT NULL,
			source_version TEXT,
			device_id INTEGER,
			creation_date TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(source_name, start_date, end_date, systolic, diastolic)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_blood_pressure_start_date ON blood_pressure(start_date)`,

		`CREATE TABLE IF NOT EXISTS workouts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			activity_type TEXT NOT NULL,
			source_name TEXT NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			duration REAL,
			duration_unit TEXT,
			total_distance REAL,
			total_distance_unit TEXT,
			total_energy_burned REAL,
			total_energy_burned_unit TEXT,
			source_version TEXT,
			device_id INTEGER,
			creation_date TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(activity_type, start_date, end_date, source_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workouts_start_date ON workouts(start_date)`,

		`CREATE TABLE IF NOT EXISTS workout_statistics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workout_id INTEGER NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			start_date TEXT,
			end_date TEXT,
			sum REAL,
			average REAL,
			minimum REAL,
			maximum REAL,
			unit TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workout_statistics_workout ON workout_statistics(workout_id)`,

		`CREATE TABLE IF NOT EXISTS workout_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workout_id INTEGER NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			date TEXT NOT NULL,
			duration REAL,
			duration_unit TEXT,
			metadata TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workout_events_workout ON workout_events(workout_id)`,

		`CREATE TABLE IF NOT EXISTS workout_zones (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workout_id INTEGER NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
			group_type TEXT NOT NULL,
			group_unit TEXT,
			zone_index INTEGER NOT NULL,
			minimum REAL,
			maximum REAL,
			duration REAL,
			duration_unit TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workout_zones_workout ON workout_zones(workout_id)`,

		`CREATE TABLE IF NOT EXISTS workout_routes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workout_id INTEGER REFERENCES workouts(id) ON DELETE SET NULL,
			source_name TEXT NOT NULL,
			source_version TEXT,
			device_id INTEGER,
			creation_date TEXT,
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			file_path TEXT,
			metadata TEXT,
			point_count INTEGER NOT NULL DEFAULT 0,
			distance_m REAL,
			elevation_gain_m REAL,
			UNIQUE(source_name, start_date, end_date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workout_routes_workout ON workout_routes(workout_id)`,

		`CREATE TABLE IF NOT EXISTS workout_route_points (
			route_id INTEGER NOT NULL REFERENCES workout_routes(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			time TEXT,
			lat REAL NOT NULL,
			lon REAL NOT NULL,
			ele REAL,
			speed REAL,
			course REAL,
			h_acc REAL,
			v_acc REAL,
			PRIMARY KEY(route_id, seq)
		) WITHOUT ROWID`,

		`CREATE TABLE IF NOT EXISTS activity_summary (
			date TEXT PRIMARY KEY,
			active_energy REAL,
			active_energy_goal REAL,
			active_energy_unit TEXT,
			move_time REAL,
			move_time_goal REAL,
			exercise_time REAL,
			exercise_time_goal REAL,
			stand_hours REAL,
			stand_hours_goal REAL
		)`,

		`CREATE TABLE IF NOT EXISTS hrv_beats (
			source_name TEXT NOT NULL,
			start_date TEXT NOT NULL,
			seq INTEGER NOT NULL,
			time TEXT,
			bpm REAL NOT NULL,
			PRIMARY KEY(source_name, start_date, seq)
		) WITHOUT ROWID`,

		`CREATE TABLE IF NOT EXISTS ecg (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_date TEXT NOT NULL UNIQUE,
			classification TEXT,
			symptoms TEXT,
			software_version TEXT,
			device TEXT,
			sample_rate_hz REAL,
			lead TEXT,
			unit TEXT,
			sample_count INTEGER NOT NULL,
			average_hr REAL,
			file_name TEXT,
			samples BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ecg_recorded ON ecg(recorded_date)`,

		`CREATE TABLE IF NOT EXISTS devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			description TEXT NOT NULL UNIQUE
		)`,

		`CREATE TABLE IF NOT EXISTS profile (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS imports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT,
			size_bytes INTEGER,
			export_date TEXT,
			locale TEXT,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			status TEXT NOT NULL,
			error TEXT,
			records INTEGER DEFAULT 0,
			workouts INTEGER DEFAULT 0,
			routes INTEGER DEFAULT 0,
			ecgs INTEGER DEFAULT 0,
			activity_days INTEGER DEFAULT 0,
			hrv_beats INTEGER DEFAULT 0,
			errors INTEGER DEFAULT 0,
			table_stats TEXT
		)`,
	)

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			head := stmt
			if len(head) > 60 {
				head = head[:60]
			}
			return fmt.Errorf("executing %q: %w", head, err)
		}
	}

	// Databases created before schema v2 lack the fidelity columns. Adding
	// them is additive and idempotent, so run it unconditionally.
	for _, t := range recordTables() {
		if err := ensureColumns(tx, t.name, fidelityColumnTypes); err != nil {
			return err
		}
	}
	for _, t := range []string{"blood_pressure", "workouts"} {
		if err := ensureColumns(tx, t, fidelityColumnTypes); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return err
	}

	return tx.Commit()
}

// ensureColumns adds any of the wanted columns that are missing from table.
func ensureColumns(tx *sql.Tx, table string, wanted [][2]string) error {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("table_info %s: %w", table, err)
	}
	existing := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notnull   int
			dflt      sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, w := range wanted {
		if existing[w[0]] {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, w[0], w[1])); err != nil {
			return fmt.Errorf("adding %s.%s: %w", table, w[0], err)
		}
	}
	return nil
}

// InsertStats tracks how many rows were inserted vs skipped (duplicates).
type InsertStats struct {
	Table    string
	Inserted int64
	Skipped  int64
}

// BatchInsertRecords inserts health records in batches using INSERT OR IGNORE.
// records should be a slice of []interface{} where each element is a row's values.
func (db *DB) BatchInsertRecords(table string, columns []string, records [][]interface{}) (*InsertStats, error) {
	return db.batchWrite("INSERT OR IGNORE", table, columns, records)
}

// BatchReplaceRecords is BatchInsertRecords with INSERT OR REPLACE semantics,
// for tables where the latest export should win (activity_summary goals).
func (db *DB) BatchReplaceRecords(table string, columns []string, records [][]interface{}) (*InsertStats, error) {
	return db.batchWrite("INSERT OR REPLACE", table, columns, records)
}

func (db *DB) batchWrite(verb, table string, columns []string, records [][]interface{}) (*InsertStats, error) {
	if len(records) == 0 {
		return &InsertStats{Table: table}, nil
	}

	stats := &InsertStats{Table: table}
	// SQLite caps bound parameters per statement (32766 by default), so size
	// the batch from the column count instead of a fixed 1000 rows.
	batchSize := 1000
	if maxRows := 32000 / len(columns); maxRows < batchSize {
		batchSize = maxRows
	}

	placeholders := "(" + strings.Repeat("?,", len(columns)-1) + "?)"
	baseQuery := fmt.Sprintf("%s INTO %s (%s) VALUES ", verb, table, strings.Join(columns, ", "))

	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		tx, err := db.conn.Begin()
		if err != nil {
			return stats, fmt.Errorf("beginning transaction: %w", err)
		}

		valuePlaceholders := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)*len(columns))
		for j, row := range batch {
			valuePlaceholders[j] = placeholders
			args = append(args, row...)
		}

		query := baseQuery + strings.Join(valuePlaceholders, ", ")
		result, err := tx.Exec(query, args...)
		if err != nil {
			tx.Rollback()
			return stats, fmt.Errorf("inserting batch into %s: %w", table, err)
		}

		rowsAffected, _ := result.RowsAffected()
		stats.Inserted += rowsAffected
		stats.Skipped += int64(len(batch)) - rowsAffected

		if err := tx.Commit(); err != nil {
			return stats, fmt.Errorf("committing transaction: %w", err)
		}
	}

	return stats, nil
}

// UpsertDevice returns the id for a device description, inserting it if new.
func (db *DB) UpsertDevice(description string) (int64, error) {
	if _, err := db.conn.Exec(`INSERT OR IGNORE INTO devices (description) VALUES (?)`, description); err != nil {
		return 0, fmt.Errorf("inserting device: %w", err)
	}
	var id int64
	if err := db.conn.QueryRow(`SELECT id FROM devices WHERE description = ?`, description).Scan(&id); err != nil {
		return 0, fmt.Errorf("looking up device: %w", err)
	}
	return id, nil
}

// SetProfile writes key/value pairs from the export's <Me> element.
func (db *DB) SetProfile(kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range kv {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO profile (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Profile returns the stored <Me> characteristics.
func (db *DB) Profile() (map[string]string, error) {
	rows, err := db.conn.Query(`SELECT key, value FROM profile`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v.String
	}
	return out, rows.Err()
}
