import { useMemo } from 'react'
import { Link, useParams } from 'react-router-dom'
import { personPath, withParams } from '../api/client'
import { useECG, useECGAnalysis, useECGs } from '../api/hooks'
import type { ECGAnalysis } from '../api/types'
import { Chart } from '../charts/Chart'
import { beatTemplate, ecgThumb, ecgTrace, rrTachogram } from '../charts/options'
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
  const an = useECGAnalysis(personId, eid)
  const a = an.data?.analysis
  const opt = useMemo(() => {
    if (!q.data?.samples) return null
    const peaks = a?.r_peaks_s.map((s, i) => ({ s, mv: a.r_peaks_mv[i] }))
    return ecgTrace(t, q.data.samples, q.data.sample_rate, true, peaks)
  }, [q.data, t, a])
  const rrOpt = useMemo(() => (a && a.rr_ms.length > 1 ? rrTachogram(t, a.rr_ms, a.premature_beats > 0) : null), [a, t])
  const tplOpt = useMemo(() => (a && a.template_mv.length > 0 ? beatTemplate(t, a.template_mv, a.template_t0_ms, a.template_dt_ms) : null), [a, t])
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
      <Section title={`${e.lead || 'Lead I'} · ${fmtNum(e.duration_s, 0)} s at ${fmtInt(e.sample_rate)} Hz`} hint="grid: 1 s (minor 200 ms) × 0.5 mV · dots mark detected beats · drag or scroll to zoom">
        {opt && <Chart option={opt} height={420} />}
      </Section>

      {a && <Analysis a={a} classification={e.classification} rrOpt={rrOpt} tplOpt={tplOpt} />}
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

const CLASSIFICATIONS: Record<string, string> = {
  'sinus rhythm': 'The heart is beating in a uniform pattern between 50 and 100 bpm, driven by its natural pacemaker. This is the normal result. It does not rule out other conditions, and it says nothing about the heart outside these 30 seconds.',
  'atrial fibrillation': "The upper chambers are quivering rather than contracting in step, so beats arrive at irregular intervals and the P wave is usually missing. AFib raises the risk of stroke and is treatable; the watch's algorithm is reliable enough that a doctor should review this recording, ideally together with the others from the same day.",
  'high heart rate': 'The rhythm looked regular but above 100 bpm, so the watch did not classify it. Exercise, stress, caffeine, fever or dehydration all do this; a resting recording above 100 bpm that repeats is worth mentioning to a doctor.',
  'low heart rate': 'The rhythm looked regular but below 50 bpm, so the watch did not classify it. Common in fit people at rest and during sleep; dizziness or fainting alongside it is not normal.',
  inconclusive: 'The watch could not classify the recording, usually because of movement, a poor contact, or a rhythm it is not designed to recognise (for example a pacemaker). Try again sitting still with the arm on a table.',
  'poor recording': 'Too much noise to read. Rest your arms on a table, keep the wrist and finger still and dry, and record again.',
}

function classificationText(c: string): string {
  const l = c.toLowerCase()
  for (const k of Object.keys(CLASSIFICATIONS)) if (l.includes(k)) return CLASSIFICATIONS[k]
  return 'Unrecognised classification label from the device.'
}

function Analysis({ a, classification, rrOpt, tplOpt }: { a: ECGAnalysis; classification: string; rrOpt: ReturnType<typeof rrTachogram> | null; tplOpt: ReturnType<typeof beatTemplate> | null }) {
  const irregularClass = a.irregularity === 'regular' ? 'good' : a.irregularity === 'mildly irregular' ? '' : a.irregularity === 'irregular' ? 'warn' : 'bad'
  const qualityClass = a.quality === 'good' ? 'good' : a.quality === 'fair' ? 'warn' : 'bad'
  const poor = a.quality === 'poor' || a.beats < 4
  return (
    <div className="stack" style={{ marginTop: 16 }}>
      <Section title="What the watch concluded">
        <div className="row" style={{ alignItems: 'flex-start', gap: 12 }}>
          {classBadge(classification)}
          <p className="muted" style={{ flex: 1, minWidth: 240 }}>{classificationText(classification)}</p>
        </div>
      </Section>

      <Section title="Rhythm analysis" hint="derived here from the raw trace, independently of the watch">
        {poor ? (
          <div className="muted">Not enough clean beats to analyse this recording ({a.beats} detected). {a.notes.join(' ')}</div>
        ) : (
          <>
            <div className="stat-strip" style={{ marginBottom: 14 }}>
              <div className="stat"><div className="l">Heart rate</div><div className="v">{fmtInt(a.hr_mean)} bpm</div><div className="muted small">range {fmtInt(a.hr_min)}–{fmtInt(a.hr_max)} · {a.beats} beats in {fmtNum(a.duration_s, 0)} s</div></div>
              <div className="stat"><div className="l">Rhythm</div><div className="v"><span className={`badge ${irregularClass}`} style={{ fontSize: 14 }}>{a.irregularity}</span></div><div className="muted small">beat-to-beat variation {fmtNum(a.rr_cv_pct, 1)} %</div></div>
              <div className="stat"><div className="l">Premature beats</div><div className="v">{a.premature_beats}</div><div className="muted small">early beat followed by a pause</div></div>
              <div className="stat"><div className="l">RMSSD</div><div className="v">{fmtInt(a.rmssd_ms)} ms</div><div className="muted small">short-term variability</div></div>
              <div className="stat"><div className="l">SDNN</div><div className="v">{fmtInt(a.sdnn_ms)} ms</div><div className="muted small">pNN50 {fmtNum(a.pnn50_pct, 0)} %</div></div>
              <div className="stat"><div className="l">Signal quality</div><div className="v"><span className={`badge ${qualityClass}`} style={{ fontSize: 14 }}>{a.quality}</span></div><div className="muted small">R {fmtNum(a.r_amplitude_mv, 2)} mV · noise {fmtNum(a.noise_mv * 1000, 0)} µV</div></div>
            </div>
            {a.notes.length > 0 && <ul className="muted small" style={{ margin: '0 0 12px', paddingLeft: 18 }}>{a.notes.map((n) => <li key={n}>{n}</li>)}</ul>}
            <div className="grid cols-2">
              <div>
                <h3 style={{ marginBottom: 4 }}>Beat-to-beat intervals</h3>
                <p className="muted small" style={{ marginBottom: 6 }}>Each dot is the time between two beats. A flat line is a steady rhythm; a gentle wave that follows breathing is normal; scattered dots with no pattern is what atrial fibrillation looks like. Orange dots are premature beats.</p>
                {rrOpt && <Chart option={rrOpt} height={220} />}
              </div>
              <div>
                <h3 style={{ marginBottom: 4 }}>Averaged beat</h3>
                <p className="muted small" style={{ marginBottom: 6 }}>All regular beats overlaid and averaged, so noise cancels out. The shaded bands show where the P wave (atria contracting), the QRS spike (ventricles contracting) and the T wave (ventricles resetting) normally sit in a healthy beat. A missing or chaotic P wave with an irregular rhythm is typical of AFib.</p>
                {tplOpt && <Chart option={tplOpt} height={220} />}
              </div>
            </div>
          </>
        )}
      </Section>

      <Section title="How to read these numbers">
        <dl className="kv" style={{ gap: '8px 20px' }}>
          <dt>Heart rate</dt><dd>Beats per minute over the recording, from the detected beats. Resting adults are commonly 50–90 bpm; fit people often sit lower. The range shows how much it moved within the 30 seconds.</dd>
          <dt>Rhythm regularity</dt><dd>How much the time between beats varies (coefficient of variation of the RR intervals). Under about 7 % is a regular rhythm; breathing alone causes a few percent. Above 20 % on a resting recording is markedly irregular and, together with the watch's classification, points towards atrial fibrillation or frequent extra beats.</dd>
          <dt>Premature beats</dt><dd>A beat arriving clearly early, followed by a longer pause. Occasional ones are very common and usually harmless; many per minute, or ones that come with symptoms, are worth a conversation with a doctor.</dd>
          <dt>RMSSD and SDNN</dt><dd>Two measures of heart-rate variability in milliseconds. Over a 30-second resting recording, values of roughly 20–80 ms are typical and higher generally reflects a relaxed, well-recovered state. Only compare with other 30-second recordings of the same person; the watch's daily HRV figure uses a different window. During atrial fibrillation these numbers become very large and stop meaning "recovery".</dd>
          <dt>pNN50</dt><dd>Share of successive beats that differ by more than 50 ms. Another variability measure; it rises with relaxation and breathing depth, and shoots up in irregular rhythms.</dd>
          <dt>Signal quality</dt><dd>How large the beats are compared with the noise between them. "Fair" or "poor" means small waves (P, T) may be lost in noise; resting the arms on a table and keeping still fixes most recordings.</dd>
          <dt>Not shown: PR, QRS, QT intervals</dt><dd>These conduction timings matter clinically but need a clean, calibrated multi-lead ECG to measure to the tens of milliseconds that count. A wrist recording cannot, so they are deliberately left out rather than shown wrong.</dd>
        </dl>
      </Section>
    </div>
  )
}
