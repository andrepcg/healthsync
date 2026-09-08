import { useMemo } from 'react'
import { Link, useParams } from 'react-router-dom'
import { personPath, withParams } from '../api/client'
import { useECG, useECGs } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { ecgThumb, ecgTrace } from '../charts/options'
import { ErrorNote, Loading, Section } from '../components/Section'
import { fmtDateTime, fmtInt, fmtNum } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

function classBadge(c: string) {
  const l = c.toLowerCase()
  const cls = l.includes('sinus') ? 'good' : l.includes('fibrillation') ? 'bad' : l.includes('inconclusive') || l.includes('poor') ? '' : 'warn'
  return <span className={`badge ${cls}`}>{c || 'Unknown'}</span>
}

export function EcgPage() {
  const page = usePage()
  const { personId, person } = page
  const list = useECGs(personId)
  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="ECG" page={page} /><NoData page={page} /></>
  return (
    <>
      <div className="page-head">
        <div>
          <h1>Electrocardiograms</h1>
          <div className="sub">{list.data ? `${list.data.length} recordings from the Apple Watch ECG app` : ''}</div>
        </div>
      </div>
      {list.isLoading && <Loading />}
      {list.error && <ErrorNote error={list.error} />}
      {list.data && list.data.length === 0 && <div className="card muted">No ECG recordings in this export.</div>}
      <div className="grid auto-wide">
        {list.data?.map((e) => (
          <Link key={e.id} to={`/p/${personId}/ecg/${e.id}`} className="card" style={{ color: 'inherit' }}>
            <div className="row spread">
              <b>{fmtDateTime(e.recorded_at)}</b>
              {classBadge(e.classification)}
            </div>
            <Thumb personId={personId} id={e.id} />
            <div className="muted small row" style={{ gap: 12 }}>
              <span>{fmtNum(e.sample_count / e.sample_rate, 0)} s · {fmtInt(e.sample_rate)} Hz</span>
              {e.avg_hr && <span>{fmtInt(e.avg_hr)} bpm</span>}
              {e.symptoms && <span>symptoms: {e.symptoms}</span>}
              <span>{e.device}</span>
            </div>
          </Link>
        ))}
      </div>
    </>
  )
}

function Thumb({ personId, id }: { personId: string; id: number }) {
  const page = usePage()
  const q = useECG(personId, id, 300)
  const opt = useMemo(() => (q.data?.envelope ? ecgThumb(page.t, q.data.envelope) : null), [q.data, page.t])
  return <div style={{ height: 60, margin: '8px 0' }}>{opt && <Chart option={opt} height={60} />}</div>
}

export function EcgDetailPage() {
  const { eid } = useParams()
  const page = usePage()
  const { personId, t } = page
  const q = useECG(personId, eid)
  const opt = useMemo(() => (q.data?.samples ? ecgTrace(t, q.data.samples, q.data.sample_rate) : null), [q.data, t])
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorNote error={q.error} />
  const e = q.data
  if (!e) return null
  return (
    <>
      <div className="page-head">
        <div>
          <div className="small"><Link to={`/p/${personId}/ecg`}>← ECG</Link></div>
          <h1>ECG · {fmtDateTime(e.recorded_at)}</h1>
          <div className="sub row">
            {classBadge(e.classification)}
            {e.avg_hr && <span>{fmtInt(e.avg_hr)} bpm average</span>}
            {e.symptoms && <span>· symptoms: {e.symptoms}</span>}
          </div>
        </div>
        <a className="btn sm" href={withParams(personPath(personId, `/ecg/${e.id}`), { format: 'csv' })}>Download CSV</a>
      </div>
      <Section title={`${e.lead || 'Lead I'} · ${fmtNum(e.duration_s, 0)} s at ${fmtInt(e.sample_rate)} Hz`} hint="grid: 1 s (minor 200 ms) × 0.5 mV · drag or scroll to zoom">
        {opt && <Chart option={opt} height={420} />}
      </Section>
      <dl className="kv card" style={{ marginTop: 16 }}>
        <dt>Classification</dt><dd>{e.classification}</dd>
        <dt>Recorded</dt><dd>{fmtDateTime(e.recorded_at)}</dd>
        <dt>Device</dt><dd>{e.device} · ECG app {e.software_version}</dd>
        <dt>Samples</dt><dd>{fmtInt(e.sample_count)} ({e.unit})</dd>
        <dt>Source file</dt><dd className="mono">{e.file_name}</dd>
      </dl>
      <p className="muted small" style={{ marginTop: 12 }}>The Apple Watch records a single-lead (Lead I) ECG. It is not a diagnosis; discuss anything unusual with a doctor.</p>
    </>
  )
}
