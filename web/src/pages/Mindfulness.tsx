import { useSeries, useTable } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { barsSeries, calendarHeatmap } from '../charts/options'
import { DataTable } from '../components/DataTable'
import { Loading, Section } from '../components/Section'
import { fmtDuration, fmtInt } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

export function MindfulnessPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const s = useSeries(personId, 'mindful-sessions', period.range, period.bucket)
  const daily = useSeries(personId, 'mindful-sessions', period.range, 'day')
  const rows = useTable(personId, 'mindful_sessions', { ...period.range, limit: 100, order: 'desc' })
  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Mindfulness" page={page} /><NoData page={page} /></>
  const pts = s.data?.points ?? []
  const total = pts.reduce((a, p) => a + p.v, 0)
  const sessions = pts.reduce((a, p) => a + p.n, 0)
  return (
    <>
      <PageHead title="Mindfulness" sub="Mindful minutes from Breathe, meditation and third-party apps" page={page} compare={false} />
      {s.isLoading && <Loading />}
      {s.data && pts.length === 0 && <div className="card muted">No mindful sessions in this period.</div>}
      {pts.length > 0 && (
        <div className="stack">
          <div className="stat-strip card">
            <div className="stat"><div className="l">Total</div><div className="v">{fmtDuration(total)}</div></div>
            <div className="stat"><div className="l">Sessions</div><div className="v">{fmtInt(sessions)}</div></div>
            <div className="stat"><div className="l">Days with a session</div><div className="v">{daily.data?.points.length ?? '–'}</div></div>
            <div className="stat"><div className="l">Avg session</div><div className="v">{fmtDuration(total / Math.max(1, sessions))}</div></div>
          </div>
          <Section title="Mindful minutes">
            <Chart option={barsSeries(t, pts, 'min', period.bucket, undefined, undefined, t.palette[4], period.range)} height={240} />
          </Section>
          {daily.data && daily.data.points.length > 20 && (
            <Section title="Calendar">
              <Chart option={calendarHeatmap(t, daily.data.points, period.from, period.to, 'min')} height={Math.max(160, 40 + 130 * Math.ceil(daily.data.points.length / 365))} />
            </Section>
          )}
          {rows.data && rows.data.rows.length > 0 && (
            <Section title="Sessions" hint={`latest ${rows.data.rows.length} of ${rows.data.total}`}>
              <div style={{ maxHeight: 360, overflowY: 'auto' }}>
                <DataTable columns={['start_date', 'end_date', 'source_name']} rows={rows.data.rows} />
              </div>
            </Section>
          )}
        </div>
      )}
    </>
  )
}
