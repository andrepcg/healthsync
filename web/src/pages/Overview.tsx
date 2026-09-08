import { useMemo } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useHighlights, useObservations, useRings, useSleepNights, useSummary, useWorkouts } from '../api/hooks'
import { ObservationCard } from '../components/ObservationCard'
import { KpiTile } from '../components/KpiTile'
import { HighlightList } from '../components/HighlightCard'
import { RingsStrip } from '../components/Rings'
import { ErrorNote, Loading, Section } from '../components/Section'
import { WorkoutCard } from '../components/WorkoutCard'
import { fmtDate, fmtHours, fmtInt, fmtNum } from '../format'
import { subDays, parseISO, format } from 'date-fns'
import { usePage } from './usePage'
import { NoData, PageHead } from './PageHead'

const TILE_LINKS: Record<string, string> = {
  steps: 'activity',
  'active-energy': 'activity',
  'exercise-time': 'activity',
  'stand-hours': 'activity',
  'distance-walking-running': 'activity',
  'flights-climbed': 'activity',
  sleep: 'sleep',
  'resting-heart-rate': 'heart',
  hrv: 'heart',
  'heart-rate': 'heart',
  'walking-heart-rate': 'heart',
  vo2max: 'heart',
  spo2: 'heart',
  'respiratory-rate': 'sleep',
  'wrist-temperature': 'sleep',
  'body-mass': 'body',
  'body-fat': 'body',
  'time-in-daylight': 'environment',
  'headphone-audio-exposure': 'environment',
  'mindful-sessions': 'mindfulness',
}

export function OverviewPage() {
  const page = usePage()
  const { personId, period, person } = page
  const nav = useNavigate()
  const summary = useSummary(personId, period.range, period.compare)
  const highlights = useHighlights(personId, period.range)
  const observations = useObservations(personId)
  const last14 = useMemo(() => {
    const end = person?.last_date ? parseISO(person.last_date) : new Date()
    return { from: format(subDays(end, 13), 'yyyy-MM-dd'), to: format(end, 'yyyy-MM-dd') }
  }, [person?.last_date])
  const rings = useRings(personId, last14)
  const sleep = useSleepNights(personId, last14)
  const workouts = useWorkouts(personId, { limit: 5 })

  if (page.loading) return <Loading />
  if (person && !person.has_data) {
    return (
      <>
        <PageHead title="Overview" page={page} />
        <NoData page={page} />
      </>
    )
  }

  const lastNight = sleep.data?.nights.length ? sleep.data.nights[sleep.data.nights.length - 1] : undefined
  const ringDays = rings.data?.days ?? []

  return (
    <>
      <PageHead title="Overview" sub={person ? <>{person.name} · {fmtDate(period.from)} – {fmtDate(period.to)}</> : undefined} page={page} />

      {observations.data && observations.data.observations.length > 0 && (
        <Section
          title="Observations"
          hint={`as of ${fmtDate(observations.data.as_of)} · ${observations.data.checked.length} checks`}
          right={<Link to={`/p/${personId}/observations`} className="small">All observations →</Link>}
          className=""
        >
          <div className="grid auto-wide" style={{ gap: 10, marginBottom: 0 }}>
            {observations.data.observations.filter((o) => o.category !== 'data').slice(0, 4).map((o) => (
              <ObservationCard key={o.id} o={o} personId={personId} compact />
            ))}
          </div>
        </Section>
      )}
      <div style={{ height: 16 }} />

      {summary.isLoading && <Loading lines={2} />}
      {summary.error && <ErrorNote error={summary.error} />}
      {summary.data && (
        <div className="grid auto" style={{ marginBottom: 16 }}>
          {summary.data.tiles.map((tile) => (
            <KpiTile key={tile.metric} tile={tile} onClick={TILE_LINKS[tile.metric] ? () => nav(`/p/${personId}/${TILE_LINKS[tile.metric]}${location.search}`) : undefined} />
          ))}
          {summary.data.tiles.length === 0 && <div className="card muted">No metrics have data in this period. Try a wider range.</div>}
        </div>
      )}

      <div className="grid cols-3">
        <Section title="Highlights" hint={`${fmtDate(period.from, 'd MMM')} – ${fmtDate(period.to, 'd MMM')}`} className="span-2">
          {highlights.isLoading ? <Loading /> : highlights.error ? <ErrorNote error={highlights.error} /> : <HighlightList items={highlights.data ?? []} personId={personId} />}
        </Section>

        <div className="stack">
          <Section title="Last night" right={<Link to={`/p/${personId}/sleep`} className="small">Sleep →</Link>}>
            {lastNight ? (
              <>
                <div className="row spread">
                  <div>
                    <div style={{ fontSize: 24, fontWeight: 650 }}>{lastNight.onset ? fmtHours(lastNight.hours) : '–'}</div>
                    <div className="muted small">
                      night of {fmtDate(lastNight.night)}
                      {lastNight.onset && ` · ${lastNight.onset.slice(11)} → ${lastNight.wake.slice(11)}`}
                    </div>
                  </div>
                </div>
                <StageBar n={lastNight} />
                <div className="row small muted" style={{ marginTop: 8, gap: 12 }}>
                  {lastNight.vitals.hr_min !== undefined && <span>❤️ min {fmtInt(lastNight.vitals.hr_min)} bpm</span>}
                  {lastNight.vitals.resp_rate !== undefined && <span>🫁 {fmtNum(lastNight.vitals.resp_rate, 1)}/min</span>}
                  {lastNight.vitals.spo2_avg !== undefined && <span>O₂ {fmtNum(lastNight.vitals.spo2_avg, 0)} %</span>}
                  {lastNight.naps > 0 && <span>😴 nap {fmtHours(lastNight.naps)}</span>}
                </div>
              </>
            ) : (
              <div className="muted small">No sleep recorded in the last 14 nights.</div>
            )}
          </Section>

          {ringDays.length > 0 && (
            <Section title="Rings" hint="last 14 days" right={<Link to={`/p/${personId}/activity`} className="small">Activity →</Link>}>
              <RingsStrip days={ringDays} />
              {rings.data && (
                <div className="muted small" style={{ marginTop: 10 }}>
                  All rings closed {rings.data.streaks.days_all_closed} of {rings.data.streaks.days} days
                  {rings.data.streaks.current_all > 1 && <> · current streak <b>{rings.data.streaks.current_all}</b></>}
                </div>
              )}
            </Section>
          )}
        </div>
      </div>

      {workouts.data && workouts.data.total > 0 && (
        <Section title="Recent workouts" hint={`${workouts.data.total} total`} right={<Link to={`/p/${personId}/workouts`} className="small">All workouts →</Link>} className="" >
          <div style={{ marginTop: -6 }}>
            {workouts.data.items.map((w) => (
              <WorkoutCard key={w.id} w={w} personId={personId} />
            ))}
          </div>
        </Section>
      )}

      {page.availability && (
        <div className="muted small" style={{ marginTop: 16 }}>
          Data from {fmtDate(page.availability.first_date)} to {fmtDate(page.availability.last_date)} · {fmtInt(page.availability.total_rows)} rows across {page.availability.tables.length} metrics
          {person?.last_import_at && <> · last import {fmtDate(person.last_import_at, 'd MMM yyyy, HH:mm')}</>}
        </div>
      )}
    </>
  )
}

function StageBar({ n }: { n: { stages: { deep: number; core: number; rem: number; awake: number; unspecified: number }; hours: number } }) {
  const total = n.stages.deep + n.stages.core + n.stages.rem + n.stages.awake + n.stages.unspecified
  if (total <= 0) return null
  const seg = (v: number, c: string, label: string) =>
    v > 0 ? <div key={label} title={`${label}: ${fmtHours(v)}`} style={{ width: `${(v / total) * 100}%`, background: c }} /> : null
  return (
    <>
      <div style={{ display: 'flex', height: 10, borderRadius: 5, overflow: 'hidden', marginTop: 10, background: 'var(--surface-2)' }}>
        {seg(n.stages.deep, 'var(--sleep-deep)', 'Deep')}
        {seg(n.stages.core, 'var(--sleep-core)', 'Core')}
        {seg(n.stages.rem, 'var(--sleep-rem)', 'REM')}
        {seg(n.stages.unspecified, 'var(--faint)', 'Asleep')}
        {seg(n.stages.awake, 'var(--sleep-awake)', 'Awake')}
      </div>
      <div className="legend" style={{ marginTop: 6 }}>
        {n.stages.deep > 0 && <span style={{ ['--sw' as string]: 'var(--sleep-deep)' }}>Deep {fmtHours(n.stages.deep)}</span>}
        {n.stages.core > 0 && <span style={{ ['--sw' as string]: 'var(--sleep-core)' }}>Core {fmtHours(n.stages.core)}</span>}
        {n.stages.rem > 0 && <span style={{ ['--sw' as string]: 'var(--sleep-rem)' }}>REM {fmtHours(n.stages.rem)}</span>}
        {n.stages.awake > 0 && <span style={{ ['--sw' as string]: 'var(--sleep-awake)' }}>Awake {fmtHours(n.stages.awake)}</span>}
      </div>
    </>
  )
}
