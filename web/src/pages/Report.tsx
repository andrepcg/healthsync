import { useHeart, useHighlights, useObservations, useReadiness, useSleepNights, useSummary, useWorkouts } from '../api/hooks'
import type { Tile } from '../api/types'
import { Loading, Section } from '../components/Section'
import { activityName, fmtDate, fmtDuration, fmtHours, fmtInt, fmtNum, fmtValue } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

function tileValue(t: Tile, v: number) {
  return t.metric === 'sleep' ? fmtHours(v) : fmtValue(v, t.unit)
}

/** One printable page for a doctor's visit: the period's numbers, findings and events. Save as PDF from the print dialog. */
export function ReportPage() {
  const page = usePage()
  const { personId, period, person } = page
  const summary = useSummary(personId, period.range, 'previous')
  const observations = useObservations(personId, period.to)
  const sleep = useSleepNights(personId, period.range)
  const readiness = useReadiness(personId, period.range)
  const workouts = useWorkouts(personId, { from: period.from, to: period.to, limit: 200 })
  const highlights = useHighlights(personId, period.range)
  const heart = useHeart(personId, period.range)

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Doctor report" page={page} compare={false} /><NoData page={page} /></>

  const nights = (sleep.data?.nights ?? []).filter((n) => n.onset)
  const avgSleep = nights.length ? nights.reduce((a, n) => a + n.hours, 0) / nights.length : null
  const scored = (readiness.data?.days ?? []).filter((d) => d.score !== null)
  const avgReady = scored.length ? scored.reduce((a, d) => a + (d.score ?? 0), 0) / scored.length : null
  const bp = (heart.data?.blood_pressure ?? []) as { start_date: string; systolic: number; diastolic: number }[]
  const flags = (observations.data?.observations ?? []).filter((o) => o.category !== 'data')
  const timing = sleep.data?.timing

  return (
    <>
      <div className="no-print">
        <PageHead title="Doctor report" sub="A printable summary of the selected period. Use your browser's print dialog to save it as PDF." page={page} compare={false} right={<button className="btn primary" onClick={() => window.print()}>Print / save PDF</button>} />
      </div>
      <div className="stack">
        <section className="card">
          <h1>{person?.name ?? 'Health'} · health summary</h1>
          <div className="muted" style={{ marginTop: 4 }}>
            {fmtDate(period.from)} – {fmtDate(period.to)}
            {person?.dob && <> · born {fmtDate(person.dob)}</>}
            {person?.sex && <> · {person.sex}</>} · from Apple Health exports · generated {fmtDate(new Date().toISOString())}
          </div>
          {observations.data?.lead && <p className="lead" style={{ marginTop: 10 }}>{observations.data.lead}</p>}
          <p className="faint small" style={{ marginTop: 8 }}>
            Consumer wearable data. Averages are over days with data; the number of days is shown next to each. Comparisons are with the person's own history, not diagnoses.
          </p>
        </section>

        {summary.data && summary.data.tiles.length > 0 && (
          <Section title="Key measurements" hint={summary.data.previous ? `previous: ${fmtDate(summary.data.previous.from, 'd MMM')} – ${fmtDate(summary.data.previous.to, 'd MMM yyyy')}` : undefined}>
            <table className="data">
              <thead>
                <tr><th>Measure</th><th className="num">This period</th><th className="num">Days</th><th className="num">Previous</th></tr>
              </thead>
              <tbody>
                {summary.data.tiles.map((t) => (
                  <tr key={t.metric}>
                    <td>{t.name}{t.kind === 'latest' ? ' (latest)' : t.kind === 'cumulative' || t.kind === 'event' ? ' (per day)' : ''}</td>
                    <td className="num">{tileValue(t, t.value)}</td>
                    <td className="num">{t.days}</td>
                    <td className="num">{t.previous !== undefined ? tileValue(t, t.previous) : '–'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>
        )}

        {flags.length > 0 && (
          <Section title="Observations" hint={`as of ${fmtDate(observations.data?.as_of)}`}>
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {flags.map((o) => (
                <li key={o.id} style={{ marginBottom: 6 }}>
                  <b>{o.title}.</b> {o.detail}
                </li>
              ))}
            </ul>
          </Section>
        )}

        <Section title="Sleep and recovery">
          <ul style={{ margin: 0, paddingLeft: 18 }}>
            <li>Average night {avgSleep !== null ? fmtHours(avgSleep) : '–'} over {nights.length} nights with data of {sleep.data?.days_in_period ?? 0}.</li>
            {timing && !timing.debt.learning && <li>Sleep debt at the end of the period: {fmtHours(timing.debt.minutes / 60)} against a need of {fmtHours(timing.debt.need_hours)} ({timing.need_source}).</li>}
            {timing?.chronotype && <li>Chronotype {timing.chronotype.label} (free-day mid-sleep {timing.chronotype.msfsc}); social jetlag {fmtInt(timing.chronotype.social_jetlag_min)} min.</li>}
            {avgReady !== null && <li>Readiness averaged {Math.round(avgReady)} (75 = their usual) over {scored.length} scored mornings.</li>}
          </ul>
        </Section>

        {bp.length > 0 && (
          <Section title="Blood pressure" hint={`${bp.length} readings`}>
            <div className="muted small" style={{ marginBottom: 6 }}>
              Average {fmtInt(bp.reduce((a, r) => a + r.systolic, 0) / bp.length)}/{fmtInt(bp.reduce((a, r) => a + r.diastolic, 0) / bp.length)} mmHg
            </div>
            <table className="data">
              <tbody>
                {bp.slice(-20).map((r) => (
                  <tr key={r.start_date}><td>{fmtDate(r.start_date, 'd MMM yyyy HH:mm')}</td><td className="num">{fmtInt(r.systolic)}/{fmtInt(r.diastolic)} mmHg</td></tr>
                ))}
              </tbody>
            </table>
          </Section>
        )}

        {workouts.data && workouts.data.total > 0 && (
          <Section title="Workouts" hint={`${workouts.data.total} in period`}>
            <table className="data">
              <tbody>
                {workouts.data.items.slice(0, 30).map((w) => (
                  <tr key={w.id}>
                    <td>{fmtDate(w.start, 'd MMM')}</td>
                    <td>{activityName(w.activity_type)}</td>
                    <td className="num">{fmtDuration(w.duration_min)}</td>
                    <td className="num">{w.distance_km ? `${fmtNum(w.distance_km, 1)} km` : ''}</td>
                    <td className="num">{w.avg_hr ? `${fmtInt(w.avg_hr)} bpm` : ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>
        )}

        {highlights.data && highlights.data.length > 0 && (
          <Section title="Highlights">
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {highlights.data.map((h, i) => (
                <li key={i}><b>{h.title}</b> {h.detail}</li>
              ))}
            </ul>
          </Section>
        )}
      </div>
    </>
  )
}
