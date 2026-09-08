package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/BRO3886/healthsync/internal/parser"
	"github.com/BRO3886/healthsync/internal/people"
	"github.com/BRO3886/healthsync/internal/storage"
)

type handlers struct {
	store   *people.Store
	version string

	skillFS   fs.FS
	publicURL string

	jobsMu sync.Mutex
	jobs   map[string]*parseJob
}

func newHandlers(store *people.Store, version string) *handlers {
	return &handlers{store: store, version: version, jobs: map[string]*parseJob{}}
}

// parseJob tracks the state of one person's async import.
type parseJob struct {
	mu        sync.RWMutex
	running   bool
	progress  parser.Progress
	filename  string
	startedAt time.Time
	result    *parser.ParseResult
	err       error
	profileUp bool
}

func (j *parseJob) status() map[string]any {
	j.mu.RLock()
	defer j.mu.RUnlock()

	s := map[string]any{
		"running":  j.running,
		"progress": j.progress,
		"filename": j.filename,
	}
	if !j.startedAt.IsZero() {
		s["started_at"] = j.startedAt.Format(time.RFC3339)
		s["elapsed"] = time.Since(j.startedAt).Round(time.Millisecond).String()
	}
	switch {
	case j.running:
		s["status"] = "running"
	case j.err != nil:
		s["status"] = "failed"
		s["error"] = j.err.Error()
		if j.result != nil {
			s["result"] = j.result
		}
	case j.result != nil:
		s["status"] = "completed"
		s["result"] = j.result
		s["profile_updated"] = j.profileUp
	default:
		s["status"] = "idle"
	}
	return s
}

func (h *handlers) job(personID string) *parseJob {
	h.jobsMu.Lock()
	defer h.jobsMu.Unlock()
	j, ok := h.jobs[personID]
	if !ok {
		j = &parseJob{}
		h.jobs[personID] = j
	}
	return j
}

func (h *handlers) isImporting(personID string) bool {
	j := h.job(personID)
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.running
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	writeJSON(w, status, map[string]any{"error": msg})
}

type ctxKey int

const (
	ctxPerson ctxKey = iota
	ctxDB
)

// withPerson resolves {id} to a person and their database.
func (h *handlers) withPerson(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, err := h.store.Get(id)
		if err != nil {
			if errors.Is(err, people.ErrNotFound) {
				writeError(w, http.StatusNotFound, "person %q not found", id)
				return
			}
			writeError(w, http.StatusInternalServerError, "loading person: %v", err)
			return
		}
		db, err := h.store.DB(p.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "opening database: %v", err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxPerson, p)
		ctx = context.WithValue(ctx, ctxDB, db)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func personFrom(r *http.Request) people.Person { return r.Context().Value(ctxPerson).(people.Person) }
func dbFrom(r *http.Request) *storage.DB       { return r.Context().Value(ctxDB).(*storage.DB) }

// --- health / registry ---

func (h *handlers) handleHealthz(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": h.version, "people": len(list)})
}

// --- people ---

type personView struct {
	people.Person
	HasData      bool   `json:"has_data"`
	FirstDate    string `json:"first_date,omitempty"`
	LastDate     string `json:"last_date,omitempty"`
	LastImportAt string `json:"last_import_at,omitempty"`
	Importing    bool   `json:"importing"`
	TotalRows    int64  `json:"total_rows"`
}

func (h *handlers) view(p people.Person) (personView, error) {
	v := personView{Person: p, Importing: h.isImporting(p.ID)}
	db, err := h.store.DB(p.ID)
	if err != nil {
		return v, err
	}
	av, err := db.Availability()
	if err != nil {
		return v, err
	}
	v.HasData = av.TotalRows > 0 || av.Workouts > 0
	v.FirstDate, v.LastDate, v.TotalRows = av.FirstDate, av.LastDate, av.TotalRows
	if imports, err := db.Imports(); err == nil {
		for _, im := range imports {
			if im.Status == "completed" {
				v.LastImportAt = im.FinishedAt
				break
			}
		}
	}
	return v, nil
}

func (h *handlers) handleListPeople(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	out := make([]personView, 0, len(list))
	for _, p := range list {
		v, err := h.view(p)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (h *handlers) handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	var p people.Person
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	created, err := h.store.Create(p)
	if err != nil {
		if errors.Is(err, people.ErrDuplicateName) {
			writeError(w, http.StatusConflict, "%v", err)
			return
		}
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	v, err := h.view(created)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) handleGetPerson(w http.ResponseWriter, r *http.Request) {
	v, err := h.view(personFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	var p people.Person
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	p.ID = personFrom(r).ID
	updated, err := h.store.Update(p)
	if err != nil {
		if errors.Is(err, people.ErrDuplicateName) {
			writeError(w, http.StatusConflict, "%v", err)
			return
		}
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	v, err := h.view(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) handleDeletePerson(w http.ResponseWriter, r *http.Request) {
	p := personFrom(r)
	if h.isImporting(p.ID) {
		writeError(w, http.StatusConflict, "an import is running for this person")
		return
	}
	if err := h.store.Delete(p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	h.jobsMu.Lock()
	delete(h.jobs, p.ID)
	h.jobsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- upload / import ---

// maxUpload caps the request body. Exports compress well (a 250 MB XML is an
// ~11 MB zip) but a bare export.xml can be large, so allow 4 GB.
const maxUpload = 4 << 30

func (h *handlers) handleUpload(w http.ResponseWriter, r *http.Request) {
	p := personFrom(r)
	db := dbFrom(r)
	job := h.job(p.ID)

	job.mu.Lock()
	if job.running {
		job.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "an import is already running for this person", "status": job.status()})
		return
	}
	// Claim the job before reading the body so two concurrent uploads for the
	// same person cannot both start.
	job.running = true
	job.progress = parser.Progress{}
	job.result, job.err, job.profileUp = nil, nil, false
	job.startedAt = time.Now()
	job.filename = ""
	job.mu.Unlock()

	release := func(err error) {
		job.mu.Lock()
		job.running = false
		job.err = err
		job.mu.Unlock()
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		release(nil)
		writeError(w, http.StatusBadRequest, "expected multipart form: %v", err)
		return
	}

	var tmpPath, filename string
	var size int64
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			release(nil)
			writeError(w, http.StatusBadRequest, "reading upload: %v", err)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		filename = filepath.Base(part.FileName())
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != ".zip" && ext != ".xml" {
			part.Close()
			release(nil)
			writeError(w, http.StatusBadRequest, "unsupported file type %q (expected .zip or .xml)", ext)
			return
		}
		tmp, err := os.CreateTemp(h.store.TmpDir(), "upload-*"+ext)
		if err != nil {
			part.Close()
			release(nil)
			writeError(w, http.StatusInternalServerError, "creating temp file: %v", err)
			return
		}
		tmpPath = tmp.Name()
		size, err = io.Copy(tmp, part)
		tmp.Close()
		part.Close()
		if err != nil {
			os.Remove(tmpPath)
			release(nil)
			writeError(w, http.StatusBadRequest, "saving upload: %v", err)
			return
		}
		break
	}
	if tmpPath == "" {
		release(nil)
		writeError(w, http.StatusBadRequest, `missing "file" field`)
		return
	}

	job.mu.Lock()
	job.filename = filename
	job.mu.Unlock()

	log.Printf("[%s] upload received: %s (%d bytes), starting import", p.Name, filename, size)
	go h.runImport(p, db, job, tmpPath, filename, size)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":  "accepted",
		"message": "file uploaded, importing in background",
		"poll":    fmt.Sprintf("/api/people/%s/upload/status", p.ID),
	})
}

func (h *handlers) runImport(p people.Person, db *storage.DB, job *parseJob, tmpPath, filename string, size int64) {
	defer os.Remove(tmpPath)
	startedAt := time.Now()
	importID, ierr := db.BeginImport(filename, size, startedAt.UTC().Format(time.RFC3339))
	if ierr != nil {
		log.Printf("[%s] recording import: %v", p.Name, ierr)
	}

	progress := func(pr parser.Progress) {
		job.mu.Lock()
		job.progress = pr
		job.mu.Unlock()
	}

	result, err := parser.ParseFile(tmpPath, db, progress)

	row := storage.ImportRow{FinishedAt: time.Now().UTC().Format(time.RFC3339), Status: "completed"}
	if err != nil {
		row.Status = "failed"
		row.Error = err.Error()
	}
	profileUp := false
	if result != nil {
		row.ExportDate, row.Locale = result.ExportDate, result.Locale
		row.Records, row.Workouts, row.Routes = result.Records, result.Workouts, result.Routes
		row.ECGs, row.ActivityDays, row.HRVBeats, row.Errors = result.ECGs, result.ActivityDays, result.HRVBeats, result.Errors
		if b, jerr := json.Marshal(result.Stats); jerr == nil {
			row.TableStats = string(b)
		}
		if err == nil {
			if up, perr := h.store.FillProfile(p.ID, result.Profile); perr == nil {
				profileUp = up
			}
		}
	}
	if ierr == nil {
		if ferr := db.FinishImport(importID, row); ferr != nil {
			log.Printf("[%s] finishing import row: %v", p.Name, ferr)
		}
	}

	job.mu.Lock()
	job.result = result
	job.err = err
	job.profileUp = profileUp
	job.running = false
	if result != nil {
		job.progress = result.Progress
	}
	job.mu.Unlock()

	if err != nil {
		log.Printf("[%s] import failed: %v", p.Name, err)
	} else {
		log.Printf("[%s] import completed: %d records, %d workouts, %d routes, %d ECGs in %s", p.Name, result.Records, result.Workouts, result.Routes, result.ECGs, time.Since(startedAt).Round(time.Millisecond))
	}
}

func (h *handlers) handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.job(personFrom(r).ID).status())
}

func (h *handlers) handleImports(w http.ResponseWriter, r *http.Request) {
	rows, err := dbFrom(r).Imports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if rows == nil {
		rows = []storage.ImportRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// handleExportDB streams a consistent snapshot of the person's database.
func (h *handlers) handleExportDB(w http.ResponseWriter, r *http.Request) {
	p := personFrom(r)
	db := dbFrom(r)
	tmp := filepath.Join(h.store.TmpDir(), fmt.Sprintf("export-%s-%d.db", p.ID, time.Now().UnixNano()))
	if _, err := db.Conn().Exec(`VACUUM INTO ?`, tmp); err != nil {
		writeError(w, http.StatusInternalServerError, "snapshot: %v", err)
		return
	}
	defer os.Remove(tmp)
	f, err := os.Open(tmp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	defer f.Close()
	name := fmt.Sprintf("healthsync-%s.db", safeName(p.Name))
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	io.Copy(w, f)
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "person"
	}
	return b.String()
}
