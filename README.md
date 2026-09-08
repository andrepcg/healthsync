# healthsync

Self-hosted **family health dashboard** for Apple Health. Upload each person's
`export.zip`, get a fast local web app with charts, highlights, period-over-period
comparisons and access to every row of data. Everything stays on your machine in
SQLite; no account, no cloud.

- **Complete import.** Every `Record` type (100+ known metrics plus a generic
  fallback for anything new), workouts with heart-rate zones, events and GPS
  routes, ECG waveforms, Activity rings, HRV beat-to-beat, metadata and devices.
- **Multiple people.** One database per family member; switch with one click;
  compare two people or two periods.
- **Honest numbers.** Overlapping Watch/iPhone records are de-duplicated, sleep is
  grouped by session (never by a clock hour), missing days are gaps, not zeros,
  and every average shows its denominator.
- **Docker-ready.** Single ~30 MB image with the UI embedded; one volume.
  See [docs/deploy-portainer.md](docs/deploy-portainer.md).

> The dashboard has **no authentication**. Run it on your home network or a
> Tailscale network only.

## Run with Docker

```bash
docker compose up -d          # pulls ghcr.io/andrepcg/healthsync:latest
open http://localhost:8080    # add a person, drop export.zip on their card
```

Port 8080 taken? Set `HEALTHSYNC_PORT` (in `.env`, see `.env.example`, or as a
Portainer stack variable): `HEALTHSYNC_PORT=8090 docker compose up -d`.

To build the image yourself: `make docker` (or swap `image:` for `build: .` in
`docker-compose.yml`).

## Run from source

```bash
make build                    # needs Go 1.24+ and Node 22 (builds the UI, embeds it)
./bin/healthsync server --data-dir ./.data
```

For UI development: `make dev-api` in one terminal, `make dev-web` in another
(Vite on :5173 proxies `/api` to :8080).

## Install

### Homebrew

```bash
brew tap BRO3886/tap
brew install healthsync
```

Or install via the install script:

```bash
curl -fsSL https://healthsync.sidv.dev/install | bash
```

Or install with Go:

```bash
go install github.com/BRO3886/healthsync@latest
```

Or download a pre-built binary from [GitHub Releases](https://github.com/BRO3886/healthsync/releases) (macOS and Linux, arm64 and amd64).

Or build from source (requires Go 1.24+):

```bash
git clone git@github.com:BRO3886/healthsync.git
cd healthsync
go build -o healthsync .
```

## Usage

### Parse an export

Export your Apple Health data from the Health app on iPhone (Settings > Health > Export All Health Data). Then:

```bash
healthsync parse export.zip
healthsync parse export.zip -v  # verbose logging
```

Accepts `.zip` or raw `.xml` files. Inside a zip, the HealthKit XML is identified by content, so exports from non-English devices (e.g. `导出.xml` on Chinese) work without renaming.

### Query data

```bash
healthsync query heart-rate --limit 10
healthsync query steps --from 2024-01-01 --to 2024-06-30
healthsync query workouts --format json
healthsync query spo2 --format csv
healthsync query resting-heart-rate --limit 30
healthsync query blood-pressure --limit 20
healthsync query body-mass --limit 30
```

Output formats: `table` (default), `json`, `csv`

Use `--total` with `steps`, `active-energy`, `basal-energy`, or `sleep` for deduplicated daily (or nightly) totals:

```bash
healthsync query steps --total --from 2024-01-01
healthsync query active-energy --total --from 2024-01-01
healthsync query sleep --total --from 2024-01-01   # nightly sleep duration, hours
```

### AI agent skills

Install the healthsync skill to teach your AI coding agent how to query your health data:

```bash
healthsync skills install
```

This writes the database schema, CLI reference, and SQL query examples to `~/.claude/skills/healthsync/` (Claude Code), `~/.agents/skills/healthsync/` (Codex CLI), or `~/.openclaw/skills/healthsync/` (OpenClaw). The agent picks it up automatically on the next session start and can then answer questions like "What was my average heart rate last week?" by running queries against your local database.

```bash
# Check installation status
healthsync skills status

# Uninstall
healthsync skills uninstall --agent claude
```

### CLI against a person's database

```bash
healthsync query steps --total --from 2026-08-01 --person Ana --data-dir ./.data
healthsync parse ~/Downloads/export.zip --person Ana
healthsync db info --person Ana
```

Without `--person` the CLI uses the single-user database at `--db`
(default `~/.healthsync/healthsync.db`), exactly as before.

### HTTP API

All endpoints are under `/api` and return JSON. Per person:

```
POST /api/people                          {"name": "Ana", "emoji": "🏃"}
POST /api/people/{id}/upload              multipart "file" (export.zip) → 202, poll …/upload/status
GET  /api/people/{id}/summary?from&to     KPI tiles vs previous period
GET  /api/people/{id}/series/steps?from&to&bucket=day|week|month
GET  /api/people/{id}/sleep/nights?from&to
GET  /api/people/{id}/workouts/{wid}/route?format=gpx
GET  /api/people/{id}/tables/heart-rate?from&to&format=csv
GET  /api/people/{id}/export.db           snapshot of the SQLite file
```

## What gets imported

Everything. The registry in `internal/hk/registry.go` names 100+ HealthKit
types (activity, heart, respiratory, sleep, body, mobility, running, cycling,
hearing, environment, nutrition, mindfulness, reproductive health, symptoms);
each gets its own table with `source_name, start_date, end_date, value[, unit],
source_version, device_id, creation_date, metadata`. Any type the registry does
not know lands in `other_quantity_records` / `other_category_records` with its
raw identifier, so nothing in an export is ever dropped.

Beyond `Record`s:

| Data | Tables |
|---|---|
| Workouts with statistics, pause/segment events, heart-rate zones, metadata (METs, weather, indoor) | `workouts`, `workout_statistics`, `workout_events`, `workout_zones` |
| GPS routes from the `workout-routes/*.gpx` files, with distance and elevation gain | `workout_routes`, `workout_route_points` |
| ECG recordings from `electrocardiograms/*.csv` (512 Hz waveform, classification) | `ecg` |
| Activity rings per day with goals | `activity_summary` |
| HRV beat-to-beat samples | `hrv_beats` |
| Blood pressure (paired systolic/diastolic, including readings inside `<Correlation>`) | `blood_pressure` |
| Devices, profile (`<Me>`), import history | `devices`, `profile`, `imports` |

Run `healthsync query --help` for the CLI names, or open **Explore data** in the
dashboard to browse every table with CSV export.

## Design

- **Streaming XML parser** — constant memory (~10MB) for 950MB+ files using `xml.Decoder` token-based parsing
- **Dedup** — `INSERT OR IGNORE` with UNIQUE constraints for idempotent re-imports
- **Batch inserts** — 1000 rows per transaction for performance
- **Async uploads** — HTTP server parses in background, poll `/api/upload/status` for progress
- **Pure Go SQLite** — uses `modernc.org/sqlite`, no CGO required
- **Agent skills** — embedded skill files (go:embed) installed via `healthsync skills install`

Database is stored at `~/.healthsync/healthsync.db` by default (override with `--db`).

## License

MIT
