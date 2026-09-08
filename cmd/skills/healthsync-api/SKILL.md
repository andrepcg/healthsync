---
name: healthsync-api
description: Query a family's Apple Health data (activity, heart, sleep, workouts, ECG, body, environment) and the rule-based health observations from a local healthsync server over HTTP. Use when asked about anyone's health metrics, trends, sleep, training, recovery, or for a health report/summary. Read-only.
platforms: [linux, macos, windows]
---

# healthsync API

healthsync is a self-hosted family health dashboard on the local network. It
holds each family member's full Apple Health export in its own database and
exposes a JSON API. This skill is **read-only**: never POST/PATCH/DELETE
anything except when the user explicitly asks you to upload an export.

**Base URL:** `{{BASE_URL}}`
Full endpoint reference (always matches the running version): `{{BASE_URL}}/skill/api.md`

Everything is on the LAN. Do not forward this data to third-party services
unless the user asks; it is medical data about real people.

## Workflow

1. **Who?** `GET {{BASE_URL}}/api/people` → list of people with `id`, `name`,
   `first_date`, `last_date` (last day with data), `has_data`. Always confirm
   whose data you are reading; names are case-insensitive but use the `id` in URLs.
2. **One call for a report:** `GET {{BASE_URL}}/api/people/{id}/digest?days=7`
   returns the person, KPI tiles with previous-period deltas, observations,
   highlights, sleep nights, workouts and data availability for the last N
   days ending on their `last_date`. Start here for any "how is X doing"
   question. `days` defaults to 7; use 30 or 90 for monthly/quarterly views.
3. **Drill down** only when the digest does not answer the question:
   - `GET /api/people/{id}/observations?as_of=YYYY-MM-DD` — deterministic
     findings (alerts, warnings, notices, info) with evidence and windows.
   - `GET /api/people/{id}/series/{metric}?from=&to=&bucket=day|week|month`
     — aggregated time series for any metric key from `GET /api/metrics`.
   - `GET /api/people/{id}/sleep/nights?from=&to=` — per-night stages and vitals.
   - `GET /api/people/{id}/workouts?from=&to=&type=&limit=` and `/workouts/{wid}`.
   - `GET /api/people/{id}/heart/overview?from=&to=&bucket=` — HR, resting, HRV,
     VO2max, SpO2, rhythm notifications, blood pressure in one call.
   - `GET /api/people/{id}/ecg` and `/ecg/{eid}?points=300` — recordings.
   - `GET /api/people/{id}/tables/{table}?from=&to=&limit=&offset=` — raw rows
     of any table (paginated, max 1000 per page; `format=csv` streams all).

```bash
curl -s {{BASE_URL}}/api/people
curl -s "{{BASE_URL}}/api/people/$ID/digest?days=7"
curl -s "{{BASE_URL}}/api/people/$ID/series/steps?from=2026-08-01&to=2026-08-31&bucket=week"
```

## Data semantics you must respect

- **Dates** are `YYYY-MM-DD`, inclusive, in the wearer's local time. Data is
  only as recent as the last export: use `last_date`, not today, as "now".
- **Days without data are absent**, never zero. A missing night means the
  watch was not worn, not that the person did not sleep. Every average must
  state its denominator ("7.1 h across 20 of 30 nights").
- **Cumulative metrics** (steps, energy, distance, exercise/stand minutes) are
  already de-duplicated across Watch and iPhone in `/series` and the tiles.
  Never `SUM` raw rows from `/tables/steps`: they double count.
- **Sleep** is grouped by session, never by a clock hour. Use `/sleep/nights`
  or the `sleep` series; do not rebuild nights from raw sleep rows. Naps are
  reported separately and are not part of `hours`.
- **Percentages** (SpO2, body fat) are 0–100 in every API response.
- **Series points** are `{t, v, n, min?, max?}`: `v` is the sum for cumulative
  metrics, the mean for samples, hours per night for sleep, count for events;
  `n` is the number of underlying rows. Bucket `week` = ISO week starting
  Monday, `month` = first of month.
- **Observations** compare the person only with their own recent history
  (28-day baselines, 7 vs 28 day load, last 7 nights, 90-day trends). `checked`
  lists what ran; `skipped` lists checks with too little data and why. Absence
  of an observation is meaningful only for checks in `checked`.
- **Red flags** (rhythm notifications, non-sinus ECG, low SpO2 nights) repeat
  what the watch reported. Present them calmly, suggest a doctor, never diagnose.

## Efficiency rules

- Prefer `digest`, then `heart/overview` and `summary`, over many `series` calls.
- For ranges over 90 days use `bucket=week`; over a year use `bucket=month`.
- Never page through raw `heart_rate` or `physical_effort` for long ranges
  (tens of thousands of rows per month); ask `series` for the aggregate.
- Cache `GET /api/metrics` (static) and `GET /api/people` per session.
- Responses are JSON with `Cache-Control: no-store`; errors are
  `{"error": "..."}` with 400/404/409/500.

## Reporting rules

- Say whose data it is and the date range, and quote `last_date` if the export
  is more than a few days old.
- Compare people only with themselves over time; avoid ranking family members.
- Use the person's own goals (rings, sleep goal) when present.
- This is not medical advice; keep tone practical and specific.
