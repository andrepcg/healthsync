# healthsync HTTP API reference

Base URL: `{{BASE_URL}}`. All responses are JSON unless noted. Dates are
`YYYY-MM-DD`, inclusive, local time of the wearer. `{id}` is a person id
(short hex string) or a case-insensitive name.

## Global

| Method | Path | Returns |
|---|---|---|
| GET | `/api/healthz` | `{status, version, people}` |
| GET | `/api/metrics` | `[{key, name, unit, group, kind, table, good}]` — every metric; `kind` ∈ cumulative, sample, duration, event |
| GET | `/skill/SKILL.md`, `/skill/api.md` | this skill, rendered for this server |

## People

| Method | Path | Body / params | Returns |
|---|---|---|---|
| GET | `/api/people` | | `[Person]` |
| POST | `/api/people` | `{name, emoji?, color?, dob?, sex?}` | `Person` (201) |
| GET | `/api/people/{id}` | | `Person` |
| PATCH | `/api/people/{id}` | partial fields | `Person` |
| DELETE | `/api/people/{id}` | | 204 (409 while importing) |

`Person`: `{id, name, color, emoji, dob, sex, has_data, first_date, last_date, last_import_at, importing, total_rows}`

## Import (write; only on explicit request)

| Method | Path | Notes |
|---|---|---|
| POST | `/api/people/{id}/upload` | multipart field `file` (export.zip or export.xml) → 202; 409 if that person is importing |
| GET | `/api/people/{id}/upload/status` | `{status: idle\|running\|completed\|failed, progress:{records, workouts, routes, route_points, ecgs, activity_days, hrv_beats, errors}, elapsed, error?, result?}` |
| GET | `/api/people/{id}/imports` | import history rows |

## Per-person read endpoints (prefix `/api/people/{id}`)

| Path | Params | Returns |
|---|---|---|
| `/digest` | `days` (default 7, max 366) | `{person, period:{from,to}, availability, summary, observations, highlights, sleep:{nights, days_in_period, timing}, readiness, workouts, profile}` — one call for a report |
| `/availability` | | `{first_date, last_date, total_rows, tables:[{table, metric_key, name, group, kind, unit, rows, first, last}], other_types:[...], workouts, routes, ecgs, activity_days}` |
| `/profile` | | `{date_of_birth, biological_sex, blood_type, ...}` from the export's `<Me>` |
| `/summary` | `from`, `to` (required), `compare=previous\|year\|none` | `{period, previous, tiles:[Tile]}`; `year` compares with the same dates one year earlier |
| `/series/{metric}` | `from`, `to`, `bucket=day\|week\|month` | `{metric, name, unit, bucket, agg, points:[{t, v, n, days?, min?, max?}], normal?:Band}`; `metric` is a key from `/api/metrics` (hyphen or underscore) or `other:<HKIdentifier>`. Cumulative week/month `v` is the average day over `days`. `normal` (sample metrics and sleep, when `to` is set) is the person's usual range over the 90 days ending at `to` |
| `/day` | `date` (required) | `{date, metrics:[{key, name, group, unit, agg, value, n, normal?:Band (30 days before), note?: "highest in 90 days"\|"lowest in 90 days"\|"first reading", partial?}], night?:SleepNight (ended that morning), workouts, events:[{time, kind, name, value?, link?}], heart_rate:[{t: "YYYY-MM-DD HH:M0", v, min, max, n}] (10-minute buckets, gaps absent), first_date, last_date}` — `partial` marks totals on the export's last, unfinished day |
| `/readiness` | `from`, `to` (required) | `{days:[{date (morning), night, score: 0–100\|null, band: good\|fair\|low, components:[{key: hrv\|sleeping_hr\|resp_rate\|sleep, label, value, unit, usual, spread, z, score, weight}], missing:[{check, reason}]}], sleep_need, sleep_need_source}` — 75 = the person's usual; null score with fewer than 3 signals |
| `/observations` | `as_of` (default last_date) | `{as_of, lead?, observations:[Observation], checked:[...], skipped:[{check, reason}]}`; `lead` is one rule-based sentence for the top of a report |
| `/highlights` | `from`, `to` (required) | `[{kind, title, detail, date?, value?, unit?, link?, tone}]` |
| `/activity/rings` | `from`, `to` | `{days:[{date, energy, energy_goal, exercise, exercise_goal, stand, stand_goal, closed:{energy, exercise, stand, all}}], streaks:{current_all, longest_all, days_all_closed, days}}` |
| `/sleep/nights` | `from`, `to` | `{nights:[{night, hours, naps, onset, wake, stages:{core, deep, rem, awake, unspecified, inbed}, vitals:{hr_min, hr_avg, hrv, hrv_rmssd, resp_rate, spo2_min, spo2_avg, wrist_temp, breathing_disturbances}}], days_in_period, timing?:{debt:{minutes, nights, learning, need_hours, from, to}, chronotype?:{msf, msfsc, msw, label, social_jetlag_min, free_nights, work_nights}, chronotype_reason?, chronotype_window, need_source}}` — nights without data are omitted; `timing` (when `to` is set): sleep debt over the 5 mornings ending at `to`, MCTQ chronotype over the 8 weeks ending at `to` |
| `/heart/overview` | `from`, `to`, `bucket` | `{heart_rate, resting_heart_rate, hrv, hrv_rmssd, walking_heart_rate, heart_rate_recovery, vo2max, spo2, respiratory_rate, afib_burden (each a Series with `normal`, present only with data), events:[{start_date, kind: high\|low\|irregular, value, metadata}], blood_pressure:[{start_date, systolic, diastolic}]}` |
| `/heart/hrv` | `from`, `to` | HRV readings `[{id, start_date, value, unit, beats}]` |
| `/heart/hrv/{hrvID}/beats` | | beat-to-beat `[{seq, time, bpm}]` |
| `/environment` | `from`, `to`, `bucket` | `{headphone_audio_exposure, environmental_audio_exposure, time_in_daylight, uv_exposure, water_temperature, events}` |
| `/workouts` | `from`, `to`, `type`, `limit` (≤1000), `offset` | `{total, items:[{id, activity_type, source, start, end, duration_min, distance_km, energy_kcal, avg_hr, max_hr, effort_score, has_route, indoor}]}` |
| `/workouts/types` | | `[{type, count}]` |
| `/workouts/{wid}` | | item + `{statistics, events, zones, metadata, route?, hr_samples:[{t, v}], device, source_version}` |
| `/workouts/{wid}/route` | `format=json\|geojson\|gpx` | GPS points |
| `/ecg` | `from`, `to` | `[{id, recorded_at, classification, symptoms, sample_rate, sample_count, device, avg_hr?}]` |
| `/ecg/{eid}` | `points=N` (envelope) or none (full samples), `format=csv` | waveform in µV |
| `/ecg/{eid}/analysis` | | `{id, recorded_at, classification, analysis:{beats, hr_mean, hr_min, hr_max, rr_ms:[...], sdnn_ms, rmssd_ms, pnn50_pct, rr_cv_pct, irregularity: regular\|mildly irregular\|irregular\|very irregular, premature_beats, quality: good\|fair\|poor, snr, r_amplitude_mv, notes:[...], template_mv:[...], template_t0_ms, template_dt_ms}}` — derived rhythm metrics; no PR/QRS/QT (not reliable from a wrist lead) |
| `/tables/{table}` | `from`, `to`, `limit` (≤1000, default 100), `offset`, `order=asc\|desc`, `type` (for `other_quantity`/`other_category`), `format=csv` | `{table, columns, total, rows}`; table = metric key, table name, `workouts`, `activity_summary`, `ecg`, `hrv_beats`, `devices`, `profile`, `imports`, `workout_*` |
| `/export.db` | | SQLite snapshot of the person's database (binary) |

### Tile

`{metric, name, unit, kind: cumulative|sample|duration|event|latest, value, total?, days, previous?, delta_pct?, direction: up|down|flat, good: up|down|"", sparkline:[Point]}`

- cumulative/event: `value` = mean per day with data, `total` = period sum
- sample: `value` = mean of daily values (each day is the mean of its hourly means, so dense workout sampling does not dominate); sleep: mean hours per night, `days` = nights
- latest (VO2max, weight, body fat): last reading; `previous` = last reading before the period
- `good` says which direction is desirable; a tile is absent when the metric has no data

### Band

`{median, lo, hi, days, from, to}` — median ± one MAD-based spread of daily values, clipped to the values seen.

### Observation

`{id, category: recovery|training|sleep|activity|fitness|heart|hearing|body|data, severity: alert|warning|notice|info, tone: good|bad|neutral, title, detail, advice?, window, evidence:[{label, value}], link?, days}`

## Semantics

- Cumulative metrics are de-duplicated across overlapping Watch/iPhone rows before summing.
- Sleep nights are reconstructed from sessions (gap > 2 h splits), assigned to the night of onset; naps (daytime onset, < 4 h) are separate.
- Percentages are 0–100 in all responses even though raw tables hold Apple's fractions.
- Timestamps in raw tables are local wall-clock without offset (`YYYY-MM-DD HH:MM:SS`).
- Every record row has `source_name, start_date, end_date, value[, unit], source_version, device_id, creation_date, metadata` (JSON string).
