package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/BRO3886/healthsync/internal/people"
	"github.com/BRO3886/healthsync/internal/web"
)

// Config holds server configuration.
type Config struct {
	Host    string
	Port    int
	Store   *people.Store
	Version string
	// SkillFS holds the HTTP-API agent skill (SKILL.md, api.md) served under
	// /skill/ with {{BASE_URL}} rendered. May be nil.
	SkillFS fs.FS
	// PublicURL is the address agents should use to reach this server, e.g.
	// http://10.0.0.14:1000. When empty it is derived from each request.
	PublicURL string
}

// NewRouter builds the full router: JSON API under /api and the embedded SPA
// for everything else. The API is mounted first so an unknown /api path is a
// JSON 404, never index.html.
func NewRouter(h *handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "application/json", "text/csv", "text/html", "text/javascript", "text/css"))

	r.Route("/api", func(api chi.Router) {
		api.Use(middleware.Logger)
		api.Use(noStore)
		api.Get("/healthz", h.handleHealthz)
		api.Get("/metrics", h.handleMetrics)

		api.Get("/people", h.handleListPeople)
		api.Post("/people", h.handleCreatePerson)
		api.Route("/people/{id}", func(pr chi.Router) {
			pr.Use(h.withPerson)
			pr.Get("/", h.handleGetPerson)
			pr.Patch("/", h.handleUpdatePerson)
			pr.Delete("/", h.handleDeletePerson)

			pr.Post("/upload", h.handleUpload)
			pr.Get("/upload/status", h.handleUploadStatus)
			pr.Get("/imports", h.handleImports)

			pr.Get("/availability", h.handleAvailability)
			pr.Get("/profile", h.handleProfile)
			pr.Get("/series/{metric}", h.handleSeries)
			pr.Get("/summary", h.handleSummary)
			pr.Get("/highlights", h.handleHighlights)
			pr.Get("/observations", h.handleObservations)
			pr.Get("/digest", h.handleDigest)
			pr.Get("/activity/rings", h.handleRings)
			pr.Get("/sleep/nights", h.handleSleepNights)
			pr.Get("/heart/overview", h.handleHeartOverview)
			pr.Get("/heart/hrv", h.handleHRVReadings)
			pr.Get("/heart/hrv/{hrvID}/beats", h.handleHRVBeats)
			pr.Get("/environment", h.handleEnvironment)
			pr.Get("/workouts", h.handleWorkouts)
			pr.Get("/workouts/types", h.handleWorkoutTypes)
			pr.Get("/workouts/{wid}", h.handleWorkout)
			pr.Get("/workouts/{wid}/route", h.handleWorkoutRoute)
			pr.Get("/ecg", h.handleECGList)
			pr.Get("/ecg/{eid}", h.handleECG)
			pr.Get("/ecg/{eid}/analysis", h.handleECGAnalysis)
			pr.Get("/tables/{table}", h.handleTable)
			pr.Get("/export.db", h.handleExportDB)
		})
		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
	})

	// Agent skill, rendered for this server's address. Mounted before the SPA
	// fallback so /skill/* never returns index.html.
	r.Get("/skill", h.handleSkillIndex)
	r.Get("/skill/{file}", h.handleSkillFile)

	r.NotFound(web.Handler().ServeHTTP)
	return r
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Start creates and starts the HTTP server with graceful shutdown.
func Start(cfg Config) error {
	h := newHandlers(cfg.Store, cfg.Version)
	h.skillFS = cfg.SkillFS
	h.publicURL = cfg.PublicURL
	r := NewRouter(h)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		// No ReadTimeout: a multi-hundred-MB export over home Wi-Fi can take
		// many minutes to upload. WriteTimeout stays generous for CSV streams.
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-done
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("Server listening on %s (data dir %s, ui built: %v)", addr, cfg.Store.DataDir(), web.Built())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
