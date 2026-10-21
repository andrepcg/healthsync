import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { addDays, format } from 'date-fns'
import { useDay } from '../api/hooks'
import type { Band, DayMetric } from '../api/types'
import { Chart } from '../charts/Chart'
import { intradayLine } from '../charts/options'
import { tokens } from '../charts/theme'
import { fmtDate, fmtHours, fmtInt, fmtNum, fmtValue } from '../format'
import { useDayParam } from '../state/day'
import { parseDay } from '../state/period'
import { useThemeMode } from '../state/ThemeContext'
import { ErrorNote, Loading } from './Section'
import { WorkoutCard } from './WorkoutCard'

const FIRST_ROWS = 8

function position(m: DayMetric): 'above' | 'below' | null {
  if (!m.normal) return null
  if (m.value > m.normal.hi) return 'above'
  if (m.value < m.normal.lo) return 'below'
  return null
}

/** One day's data beside the page: opened from any daily chart, carried in ?day=. */
export function DayPanel({ personId }: { personId: string }) {
  const [day, setDay] = useDayParam()
  const q = useDay(personId, day)
  const mode = useThemeMode()
  const t = useMemo(() => tokens(), [mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const [all, setAll] = useState(false)

  const first = q.data?.first_date
  const last = q.data?.last_date
  const step = (n: number) => {
    const d = day && parseDay(day)
    if (!d) return
    const next = format(addDays(d, n), 'yyyy-MM-dd')
    if ((first && next < first) || (last && next > last)) return
    setDay(next, true)
  }

  useEffect(() => {
    if (!day) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setDay(null)
      else if (e.altKey && e.key === 'ArrowLeft') step(-1)
      else if (e.altKey && e.key === 'ArrowRight') step(1)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  })

  const metrics = useMemo(() => {
    // Anything outside its usual range or with a note first, registry order otherwise.
    const ms = q.data?.metrics ?? []
    const flagged = (m: DayMetric) => (position(m) || m.note ? 0 : 1)
    return [...ms].sort((a, b) => flagged(a) - flagged(b))
  }, [q.data])

  if (!day) return null
  const d = q.data
  const hrReadings = d?.heart_rate.reduce((a, p) => a + p.n, 0) ?? 0
  const empty = d && !d.metrics.length && !d.night && !d.workouts.length && !d.events.length && !d.heart_rate.length

  return (
    <aside className="day-panel" role="dialog" aria-label={`Day ${day}`}>
      <div className="day-head">
        <button className="btn sm" onClick={() => step(-1)} disabled={!!first && day <= first} aria-label="Previous day" title="Previous day (Alt+←)">‹</button>
        <h2>{fmtDate(day, 'EEE d MMM yyyy')}</h2>
        <button className="btn sm" onClick={() => step(1)} disabled={!!last && day >= last} aria-label="Next day" title="Next day (Alt+→)">›</button>
        <button className="btn sm" onClick={() => setDay(null)} aria-label="Close" title="Close (Esc)">✕</button>
      </div>
      {q.isLoading && <Loading />}
      {q.error && <ErrorNote error={q.error} />}
      {empty && <div className="muted">Nothing recorded on this day.</div>}
      {d && (
        <div className="stack" style={{ gap: 18 }}>
          {d.night && (
            <div>
              <h3>Night before</h3>
              <div className="row" style={{ gap: 12, alignItems: 'baseline' }}>
                <span style={{ fontSize: 22, fontWeight: 650 }}>{fmtHours(d.night.hours)}</span>
                <span className="muted small">{d.night.onset.slice(11)} → {d.night.wake.slice(11)}</span>
              </div>
              <div className="row small muted" style={{ gap: 12, marginTop: 4 }}>
                {d.night.vitals.hr_avg !== undefined && <span>❤️ avg {fmtInt(d.night.vitals.hr_avg)} bpm</span>}
                {d.night.vitals.hrv !== undefined && <span>HRV {fmtInt(d.night.vitals.hrv)} ms</span>}
                {d.night.vitals.resp_rate !== undefined && <span>🫁 {fmtNum(d.night.vitals.resp_rate, 1)}/min</span>}
              </div>
            </div>
          )}

          {d.heart_rate.length > 0 && (
            <div>
              <h3>
                Heart rate <span className="hint">10-minute averages · {fmtInt(hrReadings)} readings · gaps are not filled</span>
              </h3>
              <Chart option={intradayLine(t, d.heart_rate, 'count/min', d.date)} height={160} />
            </div>
          )}

          {metrics.length > 0 && (
            <div>
              <h3>
                Readings <span className="hint">against the usual range of the 30 days before</span>
              </h3>
              <div className="day-metrics">
                {(all ? metrics : metrics.slice(0, FIRST_ROWS)).map((m) => {
                  const pos = position(m)
                  return (
                    <div key={m.key} className="day-metric">
                      <div className="nm">{m.name}</div>
                      <div className="val">
                        {fmtValue(m.value, m.unit)}
                        {m.agg === 'sample' && m.n > 1 && <span className="faint small"> · avg of {fmtInt(m.n)}</span>}
                      </div>
                      {m.partial ? <div className="faint small">day still in progress when exported</div> : <RangeBar band={m.normal} value={m.value} />}
                      <div className="tags">
                        {pos && <span className="badge warn">{pos} usual</span>}
                        {m.note && <span className="badge info">{m.note}</span>}
                      </div>
                    </div>
                  )
                })}
              </div>
              {metrics.length > FIRST_ROWS && (
                <button className="btn sm" onClick={() => setAll(!all)} style={{ marginTop: 6 }}>
                  {all ? 'Fewer values' : `All values (${metrics.length})`}
                </button>
              )}
            </div>
          )}

          {d.workouts.length > 0 && (
            <div>
              <h3>Workouts</h3>
              {d.workouts.map((w) => (
                <WorkoutCard key={w.id} w={w} personId={personId} />
              ))}
            </div>
          )}

          {d.events.length > 0 && (
            <div>
              <h3>Events</h3>
              <table className="data">
                <tbody>
                  {d.events.map((e, i) => (
                    <tr key={i}>
                      <td className="nowrap faint">{e.time.slice(11, 16)}</td>
                      <td>{e.link ? <Link to={`/p/${personId}/${e.link}`}>{e.name}</Link> : e.name}</td>
                      <td className="muted">{e.value}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </aside>
  )
}

/** Usual range as a bar with the day's value as a dot. */
function RangeBar({ band, value }: { band?: Band; value: number }) {
  if (!band) return <div className="faint small">no usual range yet</div>
  const lo = Math.min(band.lo, value)
  const hi = Math.max(band.hi, value)
  const pad = (hi - lo) * 0.15 || Math.abs(value) * 0.1 || 1
  const x = (v: number) => `${((v - (lo - pad)) / (hi - lo + 2 * pad)) * 100}%`
  return (
    <div className="range-bar" title={`usual ${fmtNum(band.lo)} – ${fmtNum(band.hi)} (${band.days} days)`}>
      <div className="band" style={{ left: x(band.lo), width: `calc(${x(band.hi)} - ${x(band.lo)})` }} />
      <div className="dot" style={{ left: x(value) }} />
    </div>
  )
}
