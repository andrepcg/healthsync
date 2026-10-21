import { useMemo } from 'react'
import { useReadiness } from '../api/hooks'
import type { Range } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { multiLine } from '../charts/options'
import type { tokens } from '../charts/theme'
import { fmtDate, fmtHours, fmtNum } from '../format'
import { useDayParam } from '../state/day'
import { Section } from './Section'

type Tok = ReturnType<typeof tokens>

function trend(t: Tok, pts: { t: string; v: number; n: number }[], range: Range) {
  const o = multiLine(t, [{ name: 'Readiness', pts, color: t.accent }], '', 'day', range)
  return { ...o, legend: { show: false }, grid: { left: 4, right: 8, top: 8, bottom: 4, containLabel: true }, yAxis: { ...(o.yAxis as object), splitNumber: 2 } }
}

/** Morning readiness from last night's HRV, sleeping HR, breathing and sleep vs need. */
export function ReadinessCard({ t, personId, range }: { t: Tok; personId: string; range: Range }) {
  const q = useReadiness(personId, range)
  const [, openDay] = useDayParam()
  const scored = useMemo(() => (q.data?.days ?? []).filter((d) => d.score !== null), [q.data])
  if (!q.data || q.data.days.length === 0) return null
  const latest = q.data.days[q.data.days.length - 1]
  const avg = scored.length ? scored.reduce((a, d) => a + (d.score ?? 0), 0) / scored.length : null

  return (
    <Section
      title="Readiness"
      hint={`morning of ${fmtDate(latest.date, 'd MMM')}`}
      right={<button className="btn sm" onClick={() => openDay(latest.date)}>Day →</button>}
    >
      <div className="row" style={{ gap: 14, alignItems: 'baseline' }}>
        {latest.score !== null ? <span className={`score-ring ${latest.band}`}>{latest.score}</span> : <span className="score-ring faint">–</span>}
        <span className="muted small">
          {latest.score !== null ? <>75 = your usual · based on {latest.components.length} of 4 signals</> : <>not enough signals for a score ({latest.components.length} of 4)</>}
        </span>
      </div>
      <div className="stack" style={{ gap: 8, marginTop: 12 }}>
        {latest.components.map((c) => (
          <div key={c.key}>
            <div className="row spread small">
              <span>{c.label}</span>
              <span className="muted">
                {c.key === 'sleep' ? fmtHours(c.value) : `${fmtNum(c.value, c.unit === 'ms' || c.unit === 'bpm' ? 0 : 1)} ${c.unit}`}
                {' · '}
                {c.key === 'sleep' ? `need ${fmtHours(c.usual)}` : `usual ${fmtNum(c.usual, c.unit === 'ms' || c.unit === 'bpm' ? 0 : 1)}`}
              </span>
            </div>
            <div className="meter" title={`${Math.round(c.score)} / 100 · weight ${Math.round(c.weight * 100)} %`}>
              <div style={{ width: `${c.score}%`, background: c.score >= 70 ? 'var(--good)' : c.score >= 50 ? 'var(--warn)' : 'var(--bad)' }} />
            </div>
          </div>
        ))}
        {latest.missing.map((m) => (
          <div key={m.check} className="faint small">
            {m.check.replace('_', ' ')}: {m.reason}
          </div>
        ))}
      </div>
      {scored.length > 1 && (
        <>
          <Chart option={trend(t, scored.map((d) => ({ t: d.date, v: d.score ?? 0, n: 1 })), range)} height={110} />
          <div className="muted small">
            Average {Math.round(avg ?? 0)} over {scored.length} scored mornings of {q.data.days.length} with sleep. Sleep need {fmtHours(q.data.sleep_need)} ({q.data.sleep_need_source}).
          </div>
        </>
      )}
      <div className="faint small" style={{ marginTop: 6 }}>
        Each signal is compared with the previous 28 nights. A description of how this night compares with the usual, not medical advice.
      </div>
    </Section>
  )
}
