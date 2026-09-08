# healthsync

## Overview
Self-hosted **family health dashboard** for Apple Health exports. A Go server
(CLI + HTTP API + embedded React UI) imports `export.zip` files into one SQLite
database per person and serves charts, highlights, period comparisons and raw
data. No auth: it is meant for a home LAN / Tailscale. Ships as a Docker image
for Portainer.

Upstream (`BRO3886/healthsync`) is the single-user CLI; this fork adds the
multi-person server, the web UI, 100% import coverage and Docker.

## Architecture
```
cmd/           — Cobra CLI: parse, query, db, server, skills, version
                 global flags: --db (single-user), --data-dir, --person <id|name>
internal/
  hk/          — THE metric registry (leaf pkg): identifier → table, kind, agg, unit, group.
                 Both parser and storage import it; schema DDL is generated from it.
  parser/      — xml.go (streaming importer), export.go (zip/dir/reader abstraction),
                 gpx.go (routes), ecg.go (CSV waveforms)
  storage/     — sqlite.go (schema generated from hk + migrations), queries.go (CLI totals,
                 sleep sessions), series.go (bucketed aggregation), summary.go (KPI tiles),
                 insights.go (highlights), sleepnights.go, rings.go, workouts*.go, tables.go
  insights/    — deterministic observation engine (baselines, load, sleep, habits, red flags)
  people/      — people.db registry + per-person *storage.DB cache
  server/      — chi router: /api/... JSON + SPA fallback; per-person async import jobs
  web/         — //go:embed all:dist (Vite build output; placeholder page if not built)
web/           — Vite + React + TypeScript SPA (ECharts, Leaflet, TanStack Query)
Dockerfile, docker-compose.yml, .github/workflows/docker.yml, docs/deploy-portainer.md
```

Data directory (`--data-dir` → `$HEALTHSYNC_DATA_DIR` → `~/.healthsync`):
```
people.db          people(id, name UNIQUE NOCASE, color, emoji, dob, sex, ...)
people/<id>.db     full schema per person
tmp/               upload staging (same filesystem as the volume)
```

## Build & Test
```bash
make build          # web UI (npm) + Go binary with UI embedded → bin/healthsync
make build-go       # Go only (serves a placeholder page if internal/web/dist is empty)
make web            # cd web && npm run build  (→ internal/web/dist, git-ignored)
make test           # go test ./... + tsc --noEmit + vitest
make dev-api        # go run . server --data-dir ./.data
make dev-web        # vite dev server on :5173 proxying /api → :8080
make docker         # docker build -t healthsync:local .
```
`internal/web/dist/*` is git-ignored except `.gitkeep`; `go build`/`go test`
never need Node.

## Key Technical Details

### Registry (`internal/hk`)
- Adding a metric = one line in `hk.Metrics`. The table, DDL, CLI name, API key, availability
  and Explore listing all follow. Keys are hyphenated; `ByKey` also accepts underscores and
  table names; `Lookup` accepts HK identifiers too.
- `Agg` decides aggregation: `Cumulative` (sum after overlap dedup), `Sample` (avg/min/max),
  `Duration` (interval length; sleep delegates to the session logic), `Event` (count).
- Blood pressure sentinels are `Paired` and write to `blood_pressure`; they have no table.
- Anything not in the registry lands in `other_quantity_records` / `other_category_records`
  with its raw identifier in `type`. **Nothing from an export is dropped.**

### XML Parsing
- DTD must be stripped via `io.Pipe` goroutine (not `bufio.Scanner` + `MultiReader`)
- Must NOT call `decoder.Skip()` on `<HealthData>` **or `<Correlation>`**: Correlation wraps the
  blood-pressure Records from the Health app; skipping it silently dropped them (fixed here).
  Correlation `MetadataEntry` (e.g. `HKWasUserEntered`) is merged into the child records.
- Records nested inside `<Workout>` (`EstimatedWorkoutEffortScore`) are captured via
  `Workout.Records` and fed through the same `handleRecord` path.
- Every record row carries fidelity columns: `source_version, device_id, creation_date,
  metadata` (flat JSON object, NULL when empty). Devices are normalised into `devices` after
  stripping the per-export `<<HKDevice: 0x…>, ` pointer prefix.
- HRV beat-to-beat (`InstantaneousBeatsPerMinute`) → `hrv_beats(source_name, start_date, seq,
  time, bpm)`; `time` is UTC wall-clock as exported, joined to `hrv` on (source_name, start_date).
- `<ActivitySummary>` → `activity_summary` with `INSERT OR REPLACE` (goals change; latest wins).
- `<Me>` → `profile` table; the server auto-fills a person's empty dob/sex from it.
- Workouts insert individually (`UpsertWorkout` returns the id); statistics/events/zones are
  delete-and-reinsert per workout, routes + points replaced per route, so re-import is idempotent.
  The export can legitimately contain identical duplicate workouts (same app, same timestamps);
  they collapse into one row by design.
- GPX routes are resolved through the `Export` abstraction (`/workout-routes/x.gpx` relative to
  the XML's directory inside the zip, with a suffix-match fallback). Distance skips points with
  `hAcc > 50 m`; elevation gain uses a 3-point smoothed altitude.
- ECGs are **CSV files not referenced by the XML** (`electrocardiograms/ecg_*.csv`). Discovery is
  by `.csv` extension + a content sniff for `Sample Rate`. Voltages may use a **comma decimal
  separator** (`-265,868` on pt locales); a purely numeric line is a sample, everything before is
  the key/value header. Samples are stored as a little-endian float32 BLOB. Dedup on
  `recorded_date`.
- Category types (sleep, events, …) are `HKCategoryType` (no unit attribute) — TEXT value, no unit
  column. Quantity tables have `value REAL NOT NULL`; a quantity record with a non-numeric value
  is routed to `other_category_records` rather than dropped.
- Timestamps are normalized at parse time: `normalizeTimestamp` strips the trailing ` ±HHMM` so
  stored values are plain local `YYYY-MM-DD HH:MM:SS`. Day bucketing is therefore right at home
  and off while travelling; documented, not fixed.
- Progress callback receives a `Progress` struct (records, workouts, routes, points, ecgs,
  activity days, hrv beats, errors). Decode errors are counted and surfaced as warnings in the
  import row instead of silently `continue`d.
- Real-world numbers: a 245 MB export.xml (528k records) imports in ~25 s into a ~160 MB DB.

### SQLite
- WAL mode; `INSERT OR IGNORE` with UNIQUE constraints for dedup; batches sized from the column
  count so the bound-parameter cap is respected
- Schema is generated in `migrate()` from `hk.Metrics`; `ensureColumns` adds the fidelity columns
  to databases created before schema v2 (`PRAGMA user_version`)
- Extra tables: devices, profile, imports, activity_summary, hrv_beats, ecg, workout_statistics,
  workout_events, workout_zones, workout_routes, workout_route_points

### Query / aggregation
- `TableNameMap`/`ValidTableNames` are derived from the registry; `ResolveTable` also accepts the
  extra tables for Explore.
- `--total` and `QuerySeries` for cumulative metrics dedup overlapping rows by source priority
  (Watch=2 > iPhone=1 > other=0) before summing. A raw `SUM(value)` double counts.
- `sleep --total` / `SleepNights` group by **session**, not by a clock boundary (see #17 upstream).
  Nights with no data are omitted, never zero. Naps are separate. `SleepNights` adds per-stage
  hours (overlaps merged per stage) and overnight vitals joined on the [onset, wake] window.
- Percent metrics Apple stores as fractions (SpO2 0.97 with unit `%`) are rescaled to 0–100 in
  `QuerySeries`/tiles/sleep vitals (`normalizePercent`) so every consumer sees one scale.
- `Summary` tiles: cumulative/event → per-day mean over days with data + total; sample → weighted
  mean; sleep → mean over nights present; "latest" metrics (VO2max, weight, body fat) → last
  reading. Previous period = same length ending the day before `from`. Tiles with no rows are
  absent, never zero.
- `Highlights`: best days, ring streaks (zero goal ≠ closed), least-squares trends for resting HR
  and HRV, first→last changes, sleep average with denominator, longest/most frequent workout,
  per-type records vs all history, cardio event counts.

### Observations (`internal/insights`)
- `insights.Compute(db, asOf)` runs ~16 detectors and returns a `Report` with observations sorted
  alert > warning > notice > info, plus `Checked` and `Skipped` (with reasons) so absence is meaningful.
- Everything compares the person with **their own** history: 28-day median + MAD baseline for
  resting HR / HRV / wrist temp / respiratory rate / SpO2 (recent = last 3 days; fires only beyond
  both an absolute threshold and 2×spread), a strain composite when ≥2 markers move together,
  7d:28d acute:chronic exercise ratio, last-7-nights sleep debt/short nights/drop plus 4-week
  bedtime spread and weekend shift, steps and ring-closure trends, 90-day VO2max and resting-HR
  trends, weight change, headphone dB energy-average vs WHO 80 dB, and red flags (rhythm
  notifications, non-sinus ECG, SpO2 < 90 % nights).
- Minimum-data rules are explicit per detector; missing days are unknown, never zero. Red flags say
  "discuss with a doctor", never diagnose. Adding a detector = one function appended to `steps`.
- API: `GET /api/people/{id}/observations?as_of=YYYY-MM-DD` (default: last day with data). UI:
  Observations page + top-4 panel on Overview.

### Server / API
- `/api` is mounted first; unknown `/api/*` → JSON 404; everything else → embedded SPA
  (`index.html` fallback, immutable cache on `/assets/*`).
- Upload streams multipart to `DATA_DIR/tmp` (`MultipartReader`, 4 GB cap, no ReadTimeout).
  Import jobs are per person: 409 only if *that* person is importing; DELETE is refused during
  an import. Status carries the `Progress` struct; the People page polls it.
- Routes: `/api/healthz`, `/api/metrics`, `/api/people[...]`, and per person: `upload`,
  `upload/status`, `imports`, `availability`, `profile`, `series/{metric}`, `summary`,
  `highlights`, `activity/rings`, `sleep/nights`, `heart/overview`, `heart/hrv[/{id}/beats]`,
  `environment`, `workouts[/types|/{id}|/{id}/route?format=json|geojson|gpx]`,
  `ecg[/{id}?points=N&format=csv]`, `tables/{table}?format=csv`, `export.db` (VACUUM INTO).
- Compare has no endpoint: the UI composes it from `/series`.
- `GET …/digest?days=N` bundles person, availability, summary (vs previous), observations, highlights,
  sleep nights, workouts and a trimmed profile for agents: one call per report.
- **Agent skill over HTTP**: `cmd/skills/healthsync-api/{SKILL.md,api.md}` are embedded and served at
  `/skill/SKILL.md` and `/skill/api.md` with `{{BASE_URL}}` rendered from `--public-url` /
  `$HEALTHSYNC_PUBLIC_URL`, else `X-Forwarded-Proto/Host`, else the request Host. Hermes Agent installs
  it via `hermes skills install <url>/skill/SKILL.md --name healthsync`. Keep `api.md` in sync when
  endpoints change; `go:embed` in `cmd/skills_embed.go` covers both skill dirs.

### Web UI (`web/`)
- Period lives in the URL (`?preset=30d` or `?from&to`, `&compare=none`) and is anchored on the
  person's **last day with data**, not today. `autoBucket`: day ≤ 92 d, week ≤ 460 d, else month.
- `MetricChart` fetches a series and renders bars (cumulative/event) or a band line (sample);
  cards hide themselves when there is no data. X axes are pinned to the selected period so single
  points and sparse series render correctly.
- Theme: CSS tokens in `styles/tokens.css`; ECharts theme is rebuilt from the tokens on switch.
- Leaflet: call `fitBounds` **before** adding vector layers, or Leaflet throws in `_clipPoints`.
- `ErrorBoundary` wraps the page outlet so one widget cannot blank the app.

### Docker
- Multi-stage: node:22-alpine (UI) → golang:1.26-alpine (`CGO_ENABLED=0`) → alpine:3.20 with
  tzdata + wget, non-root uid 1000, `/data` volume, healthcheck on `/api/healthz`.
- `.github/workflows/docker.yml` pushes `ghcr.io/<repo>` (amd64+arm64) on `main` and `v*` tags.
  Make the GHCR package public once. Portainer instructions: `docs/deploy-portainer.md`.

## Tests (2026-09-08)
- `internal/hk` — registry uniqueness, lookups, column shapes
- `internal/parser` — ~45 tests incl. regressions: BP inside `<Correlation>`, records nested in
  `<Workout>`, unknown types → generic tables, workout children idempotent on re-import, zip with
  GPX + ECG end to end, ECG comma/dot decimals, `<Me>`/`ExportDate`/rings/HRV beats, metadata JSON
- `internal/storage` — ~70 tests: generated DDL for every table, v1→v2 column upgrade, series per
  Agg × bucket, summary deltas, ring streaks, table rows/CSV/availability, imports, ECG, highlights,
  plus the original dedup/sleep-session suites
- `internal/insights` — each detector on synthetic data (elevated RHR, noisy baseline must not fire,
  strain composite, load spike, short/irregular sleep, red flags, rings, stale/unworn), empty DB
- `internal/people` — CRUD, name lookup, file removal, DB cache, profile fill
- `internal/server` — people CRUD, upload → poll → every dashboard endpoint, per-person 409,
  validation, SPA fallback vs JSON 404
- `web/` — vitest: period math, formatting, LTTB; `tsc --noEmit` on build
- No mocks: real temp SQLite databases and real zip fixtures throughout

## Dependencies
Go:
- `github.com/spf13/cobra` — CLI
- `github.com/go-chi/chi/v5` — HTTP router
- `modernc.org/sqlite` — pure Go SQLite (no CGO)
- `github.com/jedib0t/go-pretty/v6` — table output (query command)
- `github.com/charmbracelet/huh` — interactive prompts (skills install agent picker)
- `github.com/fatih/color` — terminal color output

Web (`web/package.json`): react, react-router-dom, @tanstack/react-query, echarts (tree-shaken via
`echarts/core`), leaflet + react-leaflet, date-fns; dev: vite, typescript, vitest. No Tailwind, no
component library — plain CSS with tokens.

## Install Script
- `scripts/install.sh` — curl installer, supports macOS and Linux (arm64 + amd64)
- Copied verbatim to `website/static/install` (Hugo serves it at `/install`, no extension)
- Install: `curl -fsSL https://healthsync.sidv.dev/install | bash`
- Pattern: pre-flight OS/arch check → resolve latest GitHub release → download tar.gz → extract → install to `/usr/local/bin` (sudo if needed)
- No agent skill prompt in install script itself — skills are installed via `healthsync skills install`

## Skills Command (v0.3.0+)
- `healthsync skills install/uninstall/status` — installs agent skill prompt into `~/.claude/skills/healthsync/` (claude) or `~/.agents/skills/healthsync/` (codex)
- `--agent claude|codex|all` flag; interactive `huh.MultiSelect` when TTY; auto-detect when non-TTY
- Skills files embedded via `//go:embed skills/healthsync` in `cmd/skills_embed.go`
- **go:embed path constraint**: paths are relative to source file, no `..` — skills must live under `cmd/` (not root) because root is `package main`
- Version tracking via `.healthsync-version` file inside each installed skill dir
- Tests use `testing/fstest.MapFS` as fake embedded FS (no real files needed)

## Latest Release
- v0.5.3 — `sleep --total` grouped by session instead of a fixed hour boundary (#17): a night running past 6 AM was split across two dates, fabricating hours for unworn nights and undercounting real ones. Adds `naps`, `onset`, `wake` columns; omits nights with no data
- v0.5.2 — date parsing fixes
- v0.5.1 — strip tz offsets from stored timestamps, `sleep --total` with 6h night shift, localized zip support (Chinese etc. via content sniffing)
- v0.5.0 — `db info` subcommand, background update checker, OpenClaw skills target
- v0.4.0 — 40+ Apple Health metrics, multi-format query output (table/json/csv), --total flag
- v0.3.0 — skills install/uninstall/status command; 6 platform archives

## Release Process
Steps in order — do not skip or reorder:
1. `git push` — push all commits to main **first**. Never tag unpushed commits.
2. `git tag vX.Y.Z` — tag after push so the tag points to a commit already on remote main
3. `git push origin vX.Y.Z` — push the tag explicitly
4. `make release` — builds all 6 archives (`bin/*.tar.gz bin/*.zip`)
5. `gh release create vX.Y.Z bin/*.tar.gz bin/*.zip`

- **CRITICAL: Push before tag.** Tagging an unpushed commit then running `gh release create` pushes the tag + that commit but leaves `main` behind on remote — release binary is built from code not reachable from main.

## IndexNow

`make indexnow` submits the live sitemap URLs to IndexNow (Bing, Yandex, Naver, Seznam, Yep). Run it after any deploy that changes page content — not needed for binary-only releases.

The key lives in two places that must stay in sync:
- `website/static/09d76431580e356eafd8d91aeecc0906.txt` — the key file served at `https://healthsync.sidv.dev/<key>.txt` (filename stem == file contents)
- `KEY` variable in `scripts/indexnow.sh`

Live submission only works once the key file is deployed to Cloudflare Pages. Running the script before the key file is live will return HTTP 403 from the IndexNow endpoint — expected, not a bug.

## Conventions
- Conventional commits
- Never render 0 for missing data; always show the denominator next to an average
- No mocks — tests use real temp SQLite databases
- Be proactive, not reactive — when given a task, just do it; don't ask for approval before starting
