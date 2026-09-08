import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useAvailability, useMetrics, usePeople, useSeries } from '../api/hooks'
import type { Person, Series } from '../api/types'
import { Chart } from '../charts/Chart'
import { multiLine } from '../charts/options'
import { tokens } from '../charts/theme'
import { Avatar } from '../components/PersonSwitcher'
import { PeriodPicker } from '../components/PeriodPicker'
import { Loading, Section } from '../components/Section'
import { fmtHours, fmtValue } from '../format'
import { useThemeMode } from '../state/ThemeContext'
import { autoBucket, PRESETS, resolvePreset, type Preset } from '../state/period'
import { usePeriod } from '../state/usePeriod'

const DEFAULT_METRICS = ['steps', 'active-energy', 'exercise-time', 'sleep', 'resting-heart-rate', 'hrv', 'vo2max', 'body-mass']

export function ComparePage() {
  const people = usePeople()
  const metrics = useMetrics()
  const mode = useThemeMode()
  const t = useMemo(() => tokens(), [mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const [params, setParams] = useSearchParams()
  const modeParam = (params.get('mode') as 'people' | 'periods') ?? 'people'
  const a = params.get('a') ?? people.data?.[0]?.id ?? ''
  const b = params.get('b') ?? people.data?.[1]?.id ?? people.data?.[0]?.id ?? ''
  const pa = people.data?.find((p) => p.id === a)
  const pb = people.data?.find((p) => p.id === b)
  const anchor = [pa?.last_date, pb?.last_date].filter(Boolean).sort().pop()
  const period = usePeriod(anchor)
  const [selected, setSelected] = useState<string[]>(DEFAULT_METRICS)
  const presetB = (params.get('presetB') as Preset) ?? '30d'
  const rangeB = useMemo(() => (presetB === 'custom' ? { from: params.get('fromB') ?? period.prev.from, to: params.get('toB') ?? period.prev.to } : presetB === 'prev' as string ? period.prev : resolvePreset(presetB, params.get('anchorB') ?? period.prev.to)), [presetB, params, period.prev])

  const setP = (k: string, v: string) => setParams((p) => { const n = new URLSearchParams(p); n.set(k, v); return n }, { replace: true })

  const avA = useAvailability(a || undefined)
  const avB = useAvailability(modeParam === 'people' ? b || undefined : a || undefined)
  const available = useMemo(() => {
    const keys = new Set<string>()
    for (const tb of avA.data?.tables ?? []) if (tb.metric_key) keys.add(tb.metric_key)
    const other = new Set<string>()
    for (const tb of avB.data?.tables ?? []) if (tb.metric_key) other.add(tb.metric_key)
    return (metrics.data ?? []).filter((m) => keys.has(m.key) || other.has(m.key))
  }, [avA.data, avB.data, metrics.data])

  if (people.isLoading) return <Loading />
  if (!people.data?.length) return <div className="card empty">Add people first.</div>

  const left = { id: a, label: pa?.name ?? '', person: pa, range: period.range }
  const right = modeParam === 'people' ? { id: b, label: pb?.name ?? '', person: pb, range: period.range } : { id: a, label: `${pa?.name ?? ''} · ${rangeB.from} → ${rangeB.to}`, person: pa, range: rangeB }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Compare</h1>
          <div className="sub">Two people over the same period, or one person across two periods.</div>
        </div>
        <div className="row">
          <div className="seg">
            <button className={modeParam === 'people' ? 'active' : ''} onClick={() => setP('mode', 'people')}>Two people</button>
            <button className={modeParam === 'periods' ? 'active' : ''} onClick={() => setP('mode', 'periods')}>Two periods</button>
          </div>
        </div>
      </div>
      <div className="card row" style={{ marginBottom: 16, gap: 16 }}>
        <PersonSelect label="A" people={people.data} value={a} onChange={(v) => setP('a', v)} />
        {modeParam === 'people' ? <PersonSelect label="B" people={people.data} value={b} onChange={(v) => setP('b', v)} /> : null}
        <div className="row" style={{ flex: 1 }}>
          <span className="muted small">{modeParam === 'people' ? 'Period' : 'Period A'}</span>
          <PeriodPicker state={period} set={period.set} showCompare={false} last={anchor} />
        </div>
        {modeParam === 'periods' && (
          <div className="row">
            <span className="muted small">Period B</span>
            <div className="seg">
              <button className={presetB === ('prev' as Preset) ? 'active' : ''} onClick={() => setP('presetB', 'prev')}>previous</button>
              {PRESETS.filter((p) => p.key !== 'all').map((p) => (
                <button key={p.key} className={presetB === p.key ? 'active' : ''} onClick={() => setP('presetB', p.key)}>{p.label} ending</button>
              ))}
            </div>
            {presetB !== ('prev' as Preset) && <input type="date" value={params.get('anchorB') ?? period.prev.to} onChange={(e) => setP('anchorB', e.target.value)} />}
          </div>
        )}
      </div>

      <div className="chips" style={{ marginBottom: 16 }}>
        {available.map((m) => (
          <button key={m.key} className={`chip ${selected.includes(m.key) ? 'active' : ''}`} onClick={() => setSelected((s) => (s.includes(m.key) ? s.filter((x) => x !== m.key) : [...s, m.key]))}>{m.name}</button>
        ))}
      </div>

      <div className="grid cols-2">
        {selected.filter((k) => available.some((m) => m.key === k)).map((key) => (
          <CompareMetric key={key} metricKey={key} left={left} right={right} t={t} sameRange={modeParam === 'people'} />
        ))}
      </div>
      {selected.length > 0 && available.length === 0 && <div className="muted">Neither selection has data yet.</div>}
    </>
  )
}

function PersonSelect({ label, people, value, onChange }: { label: string; people: Person[]; value: string; onChange: (v: string) => void }) {
  const p = people.find((x) => x.id === value)
  return (
    <label className="row">
      <span className="muted small">{label}</span>
      {p && <Avatar p={p} />}
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {people.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
      </select>
    </label>
  )
}

type Side = { id: string; label: string; person?: Person; range: { from: string; to: string } }

function headlineOf(s: Series | undefined): string {
  if (!s || !s.points.length) return '–'
  if (s.metric === 'sleep') return fmtHours(s.points.reduce((a, p) => a + p.v, 0) / s.points.length) + ' / night'
  if (s.agg === 'cumulative' || s.agg === 'event') return fmtValue(s.points.reduce((a, p) => a + p.v, 0) / s.points.length, s.unit) + ' / day'
  const n = s.points.reduce((a, p) => a + p.n, 0)
  return fmtValue(s.points.reduce((a, p) => a + p.v * p.n, 0) / Math.max(1, n), s.unit) + ' avg'
}

function CompareMetric({ metricKey, left, right, t, sameRange }: { metricKey: string; left: Side; right: Side; t: ReturnType<typeof tokens>; sameRange: boolean }) {
  const bucket = autoBucket(left.range.from, left.range.to)
  const l = useSeries(left.id, metricKey, left.range, bucket)
  const r = useSeries(right.id, metricKey, right.range, bucket)
  const opt = useMemo(() => {
    if (!l.data && !r.data) return null
    const lp = l.data?.points ?? []
    let rp = r.data?.points ?? []
    if (!sameRange) {
      // Align period B onto period A's dates by index so the lines overlay.
      rp = rp.map((p, i) => ({ ...p, t: lp[i]?.t ?? p.t }))
    }
    return multiLine(t, [{ name: left.label, pts: lp, color: left.person?.color ?? t.palette[0] }, { name: right.label, pts: rp, color: sameRange ? right.person?.color ?? t.palette[1] : t.palette[1], dashed: !sameRange }], l.data?.unit ?? r.data?.unit ?? '', bucket)
  }, [l.data, r.data, t, left, right, sameRange, bucket])
  const name = l.data?.name ?? r.data?.name ?? metricKey
  if (!l.isLoading && !r.isLoading && !l.data?.points.length && !r.data?.points.length) return null
  return (
    <Section title={name}>
      <div className="grid cols-2" style={{ marginBottom: 8 }}>
        <div><div className="muted small">{left.label}</div><div style={{ fontSize: 20, fontWeight: 650, color: left.person?.color }}>{headlineOf(l.data)}</div><div className="faint small">{l.data?.points.length ?? 0} {bucket}s with data</div></div>
        <div><div className="muted small">{right.label}</div><div style={{ fontSize: 20, fontWeight: 650, color: sameRange ? right.person?.color : undefined }}>{headlineOf(r.data)}</div><div className="faint small">{r.data?.points.length ?? 0} {bucket}s with data</div></div>
      </div>
      {opt && <Chart option={opt} height={220} />}
    </Section>
  )
}
