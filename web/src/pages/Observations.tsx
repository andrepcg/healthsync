import { useMemo, useState } from 'react'
import { useObservations } from '../api/hooks'
import type { Observation } from '../api/types'
import { CATEGORY_LABEL, ObservationCard } from '../components/ObservationCard'
import { ErrorNote, Loading, Section } from '../components/Section'
import { fmtDate } from '../format'
import { NoData } from './PageHead'
import { usePage } from './usePage'

const CATEGORY_ORDER = ['heart', 'recovery', 'training', 'sleep', 'activity', 'fitness', 'body', 'hearing', 'data']

export function ObservationsPage() {
  const page = usePage()
  const { personId, person } = page
  const [asOf, setAsOf] = useState<string>('')
  const q = useObservations(personId, asOf || undefined)

  const groups = useMemo(() => {
    const m = new Map<string, Observation[]>()
    for (const o of q.data?.observations ?? []) {
      const list = m.get(o.category) ?? []
      list.push(o)
      m.set(o.category, list)
    }
    return CATEGORY_ORDER.filter((c) => m.has(c)).map((c) => ({ category: c, items: m.get(c)! }))
  }, [q.data])

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><div className="page-head"><h1>Observations</h1></div><NoData page={page} /></>

  const rep = q.data
  const counts = { alert: 0, warning: 0, notice: 0, info: 0 }
  for (const o of rep?.observations ?? []) counts[o.severity]++

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Observations</h1>
          <div className="sub">What the data says right now, compared with {person?.name ?? 'this person'}'s own recent history. Not medical advice.</div>
        </div>
        <label className="row">
          <span className="muted small">as of</span>
          <input type="date" value={asOf || rep?.as_of || ''} min={person?.first_date} max={person?.last_date} onChange={(e) => setAsOf(e.target.value)} />
          {asOf && <button className="btn sm" onClick={() => setAsOf('')}>latest</button>}
        </label>
      </div>

      {q.isLoading && <Loading />}
      {q.error && <ErrorNote error={q.error} />}
      {rep && (
        <div className="stack">
          <div className="stat-strip card">
            <div className="stat"><div className="l">Checks run</div><div className="v">{rep.checked.length}</div><div className="muted small">as of {fmtDate(rep.as_of)}</div></div>
            <div className="stat"><div className="l">Alerts</div><div className="v" style={{ color: counts.alert ? 'var(--bad)' : undefined }}>{counts.alert}</div></div>
            <div className="stat"><div className="l">Warnings</div><div className="v" style={{ color: counts.warning ? 'var(--warn)' : undefined }}>{counts.warning}</div></div>
            <div className="stat"><div className="l">Notices</div><div className="v">{counts.notice}</div></div>
            <div className="stat"><div className="l">Good news & context</div><div className="v">{counts.info}</div></div>
          </div>

          {rep.observations.length === 0 && (
            <div className="card empty">
              <div className="big">✅</div>
              <div style={{ fontWeight: 600, color: 'var(--text)' }}>Nothing stands out</div>
              <div style={{ marginTop: 6 }}>{rep.checked.length} checks ran and found no deviation from your usual patterns.</div>
            </div>
          )}

          {groups.map((g) => (
            <Section key={g.category} title={CATEGORY_LABEL[g.category] ?? g.category} hint={`${g.items.length}`}>
              <div className="stack" style={{ gap: 10 }}>
                {g.items.map((o) => <ObservationCard key={o.id} o={o} personId={personId} />)}
              </div>
            </Section>
          ))}

          <Section title="How this works" hint="deterministic, on-device">
            <p className="muted small" style={{ marginBottom: 8 }}>
              Every observation compares {person?.name ?? 'the person'} with their own recent history: a rolling 4-week baseline for recovery markers (median and typical spread, so one odd day does not count),
              a 7-day versus 28-day ratio for training load, the last 7 nights for sleep, this month against last for habits, and 90 days for fitness trends. Checks that do not have enough data are skipped and listed below rather than guessed.
              Missing days are unknown, never zero. Red flags repeat what the watch itself reported; nothing here is a diagnosis.
            </p>
            {rep.skipped.length > 0 && (
              <details>
                <summary className="small" style={{ cursor: 'pointer' }}>{rep.skipped.length} checks skipped for lack of data</summary>
                <ul className="small muted" style={{ margin: '6px 0 0', paddingLeft: 18 }}>
                  {rep.skipped.map((s) => <li key={s.check}><span className="mono">{s.check}</span>: {s.reason}</li>)}
                </ul>
              </details>
            )}
            <div className="faint small" style={{ marginTop: 8 }}>Ran: {rep.checked.join(', ')}</div>
          </Section>
        </div>
      )}
    </>
  )
}
