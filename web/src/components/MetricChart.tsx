import { useMemo } from 'react'
import { useSeries } from '../api/hooks'
import type { Series } from '../api/types'
import { Chart } from '../charts/Chart'
import { bandLine, barsSeries } from '../charts/options'
import type { tokens } from '../charts/theme'
import { fmtValue } from '../format'
import { ErrorNote, Loading, Section } from './Section'

type Tok = ReturnType<typeof tokens>

interface Props {
  t: Tok
  personId: string
  metric: string
  title?: string
  range: { from: string; to: string }
  prev?: { from: string; to: string }
  bucket: 'day' | 'week' | 'month'
  color?: string
  goal?: number
  refLines?: { y: number; label: string }[]
  height?: number
  /** Hide the whole card when there is no data (default) instead of showing an empty note. */
  hideEmpty?: boolean
  summary?: (s: Series) => string
}

export function seriesSummary(s: Series): string {
  if (!s.points.length) return ''
  if (s.agg === 'cumulative' || s.agg === 'event') {
    const total = s.points.reduce((a, p) => a + p.v, 0)
    return `${fmtValue(total / s.points.length, s.unit)} / ${s.bucket} avg · ${fmtValue(total, s.unit)} total · ${s.points.length} ${s.bucket}s with data`
  }
  const n = s.points.reduce((a, p) => a + p.n, 0)
  const mean = s.points.reduce((a, p) => a + p.v * p.n, 0) / Math.max(1, n)
  const mins = s.points.map((p) => p.min ?? p.v)
  const maxs = s.points.map((p) => p.max ?? p.v)
  return `avg ${fmtValue(mean, s.unit)} · range ${fmtValue(Math.min(...mins), s.unit)} – ${fmtValue(Math.max(...maxs), s.unit)} · ${n} samples`
}

/** Fetches a metric series and renders bars (cumulative/event) or a band line (sample). */
export function MetricChart({ t, personId, metric, title, range, prev, bucket, color, goal, refLines, height = 240, hideEmpty = true, summary }: Props) {
  const cur = useSeries(personId, metric, range, bucket)
  const prevQ = useSeries(personId, metric, prev ?? range, bucket, !!prev)
  const option = useMemo(() => {
    const s = cur.data
    if (!s) return null
    if (s.agg === 'sample' || (s.agg === 'duration' && s.metric === 'sleep')) return bandLine(t, s.points, s.unit, bucket, color, refLines, range)
    return barsSeries(t, s.points, s.unit, bucket, prev ? prevQ.data?.points : undefined, goal, color, range)
  }, [cur.data, prevQ.data, t, bucket, color, goal, prev, refLines, range])

  if (cur.isLoading) return <Section title={title ?? metric}><Loading /></Section>
  if (cur.error) return <Section title={title ?? metric}><ErrorNote error={cur.error} /></Section>
  if (!cur.data || cur.data.points.length === 0) {
    if (hideEmpty) return null
    return <Section title={title ?? metric}><div className="muted small">No data in this period.</div></Section>
  }
  return (
    <Section title={title ?? cur.data.name} hint={(summary ?? seriesSummary)(cur.data)}>
      {option && <Chart option={option} height={height} />}
    </Section>
  )
}
