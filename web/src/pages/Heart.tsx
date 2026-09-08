import { useMemo, useState } from 'react'
import { fetchJson, personPath } from '../api/client'
import { useHeart } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { bandLine, beatsLine, bloodPressure, multiLine } from '../charts/options'
import { MetricChart } from '../components/MetricChart'
import { Dialog } from '../components/Dialog'
import { ErrorNote, Loading, Section } from '../components/Section'
import { DataTable } from '../components/DataTable'
import { categoryValue, fmtDateTime, fmtInt, fmtNum, fmtValue } from '../format'
import { useQuery } from '@tanstack/react-query'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

export function HeartPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const heart = useHeart(personId, period.range, period.bucket)
  const hrvReadings = useQuery({
    queryKey: ['hrv-readings', personId, period.from, period.to],
    queryFn: () => fetchJson<Record<string, unknown>[]>(personPath(personId, '/heart/hrv'), period.range),
    enabled: !!personId,
  })
  const [beatsFor, setBeatsFor] = useState<Record<string, unknown> | null>(null)

  const rhrMA = useMemo(() => {
    const pts = heart.data?.resting_heart_rate?.points ?? []
    return pts.map((p, i) => {
      const win = pts.slice(Math.max(0, i - 6), i + 1)
      return { ...p, v: win.reduce((a, b) => a + b.v, 0) / win.length }
    })
  }, [heart.data])

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Heart" page={page} /><NoData page={page} /></>
  const h = heart.data

  return (
    <>
      <PageHead title="Heart" sub="Heart rate, variability, fitness and rhythm notifications" page={page} />
      {heart.isLoading && <Loading />}
      {heart.error && <ErrorNote error={heart.error} />}
      {h && (
        <div className="stack">
          <div className="grid cols-2">
            {h.heart_rate && (
              <Section title="Heart rate" hint="daily min · avg · max" className="span-2">
                <Chart option={bandLine(t, h.heart_rate.points, 'count/min', period.bucket, t.palette[1], undefined, period.range)} height={260} />
              </Section>
            )}
            {h.resting_heart_rate && (
              <Section title="Resting heart rate" hint="with 7-day average">
                <Chart option={multiLine(t, [{ name: 'Resting HR', pts: h.resting_heart_rate.points, color: t.palette[1] }, { name: '7-day avg', pts: rhrMA, color: t.palette[0], dashed: true }], 'count/min', period.bucket, period.range)} height={240} />
              </Section>
            )}
            {h.hrv && (
              <Section title="Heart rate variability" hint="SDNN · daily avg with range">
                <Chart option={bandLine(t, h.hrv.points, 'ms', period.bucket, t.palette[4], undefined, period.range)} height={240} />
              </Section>
            )}
            {h.walking_heart_rate && (
              <Section title="Walking heart rate">
                <Chart option={bandLine(t, h.walking_heart_rate.points, 'count/min', period.bucket, t.palette[6], undefined, period.range)} height={220} />
              </Section>
            )}
            {h.heart_rate_recovery && (
              <Section title="Heart rate recovery" hint="1 minute after workouts">
                <Chart option={bandLine(t, h.heart_rate_recovery.points, 'count/min', period.bucket, t.palette[2], undefined, period.range)} height={220} />
              </Section>
            )}
            {h.vo2max && (
              <Section title="VO₂ max" hint="cardio fitness estimate">
                <Chart option={bandLine(t, h.vo2max.points, 'mL/min·kg', period.bucket, t.palette[5], undefined, period.range)} height={220} />
              </Section>
            )}
            {h.spo2 && (
              <Section title="Blood oxygen" hint="SpO₂">
                <Chart option={bandLine(t, h.spo2.points, '%', period.bucket, t.palette[7], undefined, period.range)} height={220} />
              </Section>
            )}
            {h.respiratory_rate && (
              <Section title="Respiratory rate">
                <Chart option={bandLine(t, h.respiratory_rate.points, 'count/min', period.bucket, t.palette[3], undefined, period.range)} height={220} />
              </Section>
            )}
            {h.afib_burden && (
              <Section title="AFib burden">
                <Chart option={bandLine(t, h.afib_burden.points, '%', period.bucket, t.bad, undefined, period.range)} height={220} />
              </Section>
            )}
            <MetricChart t={t} personId={personId} metric="peripheral-perfusion-index" range={period.range} bucket={period.bucket} />
          </div>

          {h.blood_pressure.length > 0 && (
            <Section title="Blood pressure" hint={`${h.blood_pressure.length} readings`}>
              <Chart option={bloodPressure(t, h.blood_pressure as { start_date: string; systolic: number; diastolic: number }[])} height={240} />
              <DataTable columns={['start_date', 'systolic', 'diastolic', 'unit', 'source_name']} rows={h.blood_pressure} />
            </Section>
          )}

          {h.events.length > 0 && (
            <Section title="Heart rhythm notifications" hint={`${h.events.length} in period`}>
              <div className="table-wrap">
                <table className="data">
                  <thead>
                    <tr>
                      <th>When</th>
                      <th>Type</th>
                      <th>Detail</th>
                      <th>Source</th>
                    </tr>
                  </thead>
                  <tbody>
                    {h.events.map((e, i) => {
                      const meta = (e.metadata as Record<string, string>) ?? {}
                      const kind = e.kind as string
                      return (
                        <tr key={i}>
                          <td>{fmtDateTime(e.start_date as string)}</td>
                          <td>
                            <span className={`badge ${kind === 'irregular' ? 'bad' : 'warn'}`}>{kind === 'high' ? 'High heart rate' : kind === 'low' ? 'Low heart rate' : 'Irregular rhythm'}</span>
                          </td>
                          <td className="muted">
                            {meta.HKHeartRateEventThreshold ? `threshold ${meta.HKHeartRateEventThreshold}` : categoryValue(e.value) !== 'Not Applicable' ? categoryValue(e.value) : ''}
                          </td>
                          <td className="muted">{e.source_name as string}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </Section>
          )}

          {hrvReadings.data && hrvReadings.data.length > 0 && (
            <Section title="HRV readings" hint="click a reading for beat-to-beat detail">
              <div className="table-wrap" style={{ maxHeight: 320, overflowY: 'auto' }}>
                <table className="data">
                  <thead>
                    <tr>
                      <th>When</th>
                      <th className="num">SDNN</th>
                      <th className="num">Beats</th>
                      <th>Source</th>
                    </tr>
                  </thead>
                  <tbody>
                    {hrvReadings.data.slice(0, 200).map((r) => (
                      <tr key={r.id as number} className="clickable" onClick={() => setBeatsFor(r)}>
                        <td>{fmtDateTime(r.start_date as string)}</td>
                        <td className="num">{fmtValue(r.value as number, 'ms')}</td>
                        <td className="num">{fmtInt(r.beats as number)}</td>
                        <td className="muted">{r.source_name as string}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Section>
          )}
        </div>
      )}
      {beatsFor && <BeatsDialog personId={personId} reading={beatsFor} onClose={() => setBeatsFor(null)} t={t} />}
    </>
  )
}

function BeatsDialog({ personId, reading, onClose, t }: { personId: string; reading: Record<string, unknown>; onClose: () => void; t: ReturnType<typeof usePage>['t'] }) {
  const beats = useQuery({
    queryKey: ['hrv-beats', personId, reading.id],
    queryFn: () => fetchJson<{ seq: number; time: string; bpm: number }[]>(personPath(personId, `/heart/hrv/${reading.id}/beats`)),
  })
  return (
    <Dialog title={`HRV ${fmtNum(reading.value as number, 1)} ms · ${fmtDateTime(reading.start_date as string)}`} onClose={onClose}>
      {beats.isLoading && <Loading />}
      {beats.data && beats.data.length > 0 ? (
        <>
          <Chart option={beatsLine(t, beats.data)} height={220} />
          <div className="muted small">{beats.data.length} instantaneous beats over the reading window (about a minute).</div>
        </>
      ) : (
        !beats.isLoading && <div className="muted">No beat-to-beat data stored for this reading.</div>
      )}
      <div className="row" style={{ justifyContent: 'flex-end', marginTop: 12 }}>
        <button className="btn" onClick={onClose}>
          Close
        </button>
      </div>
    </Dialog>
  )
}
