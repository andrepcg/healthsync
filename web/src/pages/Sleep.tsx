import { useMemo } from 'react'
import { useSleepNights } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { donut, multiLine, onsetWake, sleepStages } from '../charts/options'
import { MetricChart } from '../components/MetricChart'
import { ErrorNote, Loading, Section } from '../components/Section'
import { fmtHours, fmtNum } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

function stddev(xs: number[]) {
  if (xs.length < 2) return 0
  const m = xs.reduce((a, b) => a + b, 0) / xs.length
  return Math.sqrt(xs.reduce((a, b) => a + (b - m) ** 2, 0) / (xs.length - 1))
}

function minutesOfDay(s: string, wrapBefore = 12) {
  const [h, m] = s.slice(11).split(':').map(Number)
  let mins = h * 60 + m
  if (h < wrapBefore) mins += 24 * 60
  return mins
}

export function SleepPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const q = useSleepNights(personId, period.range)

  const stats = useMemo(() => {
    const nights = (q.data?.nights ?? []).filter((n) => n.onset)
    if (!nights.length) return null
    const hours = nights.map((n) => n.hours)
    const onsets = nights.map((n) => minutesOfDay(n.onset))
    const wakes = nights.map((n) => minutesOfDay(n.wake, 4))
    const avgMin = (xs: number[]) => xs.reduce((a, b) => a + b, 0) / xs.length
    const clock = (mins: number) => `${String(Math.floor((mins / 60) % 24)).padStart(2, '0')}:${String(Math.round(mins % 60)).padStart(2, '0')}`
    const stages = nights.reduce(
      (a, n) => ({ deep: a.deep + n.stages.deep, core: a.core + n.stages.core, rem: a.rem + n.stages.rem, awake: a.awake + n.stages.awake, unspecified: a.unspecified + n.stages.unspecified }),
      { deep: 0, core: 0, rem: 0, awake: 0, unspecified: 0 },
    )
    return {
      n: nights.length,
      avg: avgMin(hours),
      over7: nights.filter((n) => n.hours >= 7).length,
      onset: clock(avgMin(onsets)),
      wake: clock(avgMin(wakes)),
      onsetSd: stddev(onsets),
      wakeSd: stddev(wakes),
      naps: nights.reduce((a, n) => a + n.naps, 0),
      stages,
    }
  }, [q.data])

  const vitals = useMemo(() => {
    const nights = q.data?.nights ?? []
    const pick = (k: 'wrist_temp' | 'resp_rate' | 'spo2_min' | 'spo2_avg' | 'breathing_disturbances' | 'hr_min' | 'hrv') =>
      nights.filter((n) => n.vitals[k] !== undefined).map((n) => ({ t: n.night, v: n.vitals[k] as number, n: 1 }))
    return {
      temp: pick('wrist_temp'),
      resp: pick('resp_rate'),
      spo2min: pick('spo2_min'),
      spo2avg: pick('spo2_avg'),
      bd: pick('breathing_disturbances'),
      hrmin: pick('hr_min'),
      hrv: pick('hrv'),
    }
  }, [q.data])

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Sleep" page={page} /><NoData page={page} /></>

  const nights = q.data?.nights ?? []
  const missing = (q.data?.days_in_period ?? 0) - nights.filter((n) => n.onset).length

  return (
    <>
      <PageHead title="Sleep" sub="Nights are grouped by session, never by a clock hour. Nights with no data are gaps, not zeros." page={page} compare={false} />
      {q.isLoading && <Loading />}
      {q.error && <ErrorNote error={q.error} />}
      {q.data && nights.length === 0 && <div className="card muted">No sleep recorded in this period.</div>}
      {stats && (
        <div className="stack">
          <div className="stat-strip card">
            <div className="stat"><div className="l">Average night</div><div className="v">{fmtHours(stats.avg)}</div><div className="muted small">{stats.n} nights recorded{missing > 0 ? ` · ${missing} without data` : ''}</div></div>
            <div className="stat"><div className="l">Nights ≥ 7 h</div><div className="v">{Math.round((stats.over7 / stats.n) * 100)} %</div><div className="muted small">{stats.over7} of {stats.n}</div></div>
            <div className="stat"><div className="l">Typical bedtime</div><div className="v">{stats.onset}</div><div className="muted small">± {Math.round(stats.onsetSd)} min</div></div>
            <div className="stat"><div className="l">Typical wake</div><div className="v">{stats.wake}</div><div className="muted small">± {Math.round(stats.wakeSd)} min</div></div>
            <div className="stat"><div className="l">Deep</div><div className="v">{stats.stages.deep + stats.stages.core + stats.stages.rem > 0 ? `${Math.round((stats.stages.deep / (stats.stages.deep + stats.stages.core + stats.stages.rem + stats.stages.unspecified)) * 100)} %` : '–'}</div><div className="muted small">{fmtHours(stats.stages.deep / stats.n)} / night</div></div>
            <div className="stat"><div className="l">REM</div><div className="v">{stats.stages.deep + stats.stages.core + stats.stages.rem > 0 ? `${Math.round((stats.stages.rem / (stats.stages.deep + stats.stages.core + stats.stages.rem + stats.stages.unspecified)) * 100)} %` : '–'}</div><div className="muted small">{fmtHours(stats.stages.rem / stats.n)} / night</div></div>
            {stats.naps > 0 && <div className="stat"><div className="l">Naps</div><div className="v">{fmtHours(stats.naps)}</div><div className="muted small">total, kept separate</div></div>}
          </div>

          <Section title="Sleep stages per night" hint={`${stats.n} nights`}>
            <Chart option={sleepStages(t, nights.filter((n) => n.onset))} height={300} />
          </Section>

          <div className="grid cols-3">
            <Section title="Bedtime → wake" className="span-2">
              <Chart option={onsetWake(t, nights)} height={260} />
            </Section>
            <Section title="Stage share" hint="whole period">
              <Chart
                option={donut(t, [
                  { name: 'Deep', value: stats.stages.deep, color: t.sleep.deep },
                  { name: 'Core', value: stats.stages.core, color: t.sleep.core },
                  { name: 'REM', value: stats.stages.rem, color: t.sleep.rem },
                  { name: 'Asleep', value: stats.stages.unspecified, color: t.faint },
                  { name: 'Awake', value: stats.stages.awake, color: t.sleep.awake },
                ].filter((p) => p.value > 0), 'hr')}
                height={260}
              />
            </Section>
          </div>

          <h2 style={{ marginTop: 8 }}>Overnight vitals</h2>
          <div className="grid cols-2">
            {vitals.hrmin.length > 0 && (
              <Section title="Sleeping heart rate" hint="minimum per night">
                <Chart option={multiLine(t, [{ name: 'Min HR', pts: vitals.hrmin, color: t.palette[1] }], 'count/min', 'day', period.range)} height={200} />
              </Section>
            )}
            {vitals.hrv.length > 0 && (
              <Section title="Overnight HRV" hint="mean per night">
                <Chart option={multiLine(t, [{ name: 'HRV', pts: vitals.hrv, color: t.palette[4] }], 'ms', 'day', period.range)} height={200} />
              </Section>
            )}
            {vitals.resp.length > 0 && (
              <Section title="Respiratory rate" hint="breaths per minute while asleep">
                <Chart option={multiLine(t, [{ name: 'Resp. rate', pts: vitals.resp, color: t.palette[2] }], 'count/min', 'day', period.range)} height={200} />
              </Section>
            )}
            {vitals.spo2avg.length > 0 && (
              <Section title="Blood oxygen while asleep" hint="avg and min per night">
                <Chart option={multiLine(t, [{ name: 'Avg', pts: vitals.spo2avg, color: t.palette[7] }, { name: 'Min', pts: vitals.spo2min, color: t.palette[1], dashed: true }], '%', 'day', period.range)} height={200} />
              </Section>
            )}
            {vitals.temp.length > 0 && (
              <Section title="Wrist temperature" hint="deviation from baseline">
                <Chart option={multiLine(t, [{ name: 'Wrist temp', pts: vitals.temp, color: t.palette[3] }], 'degC', 'day', period.range)} height={200} />
              </Section>
            )}
            {vitals.bd.length > 0 && (
              <Section title="Breathing disturbances" hint="Apple's sleep apnoea indicator · lower is better">
                <Chart option={multiLine(t, [{ name: 'Disturbances', pts: vitals.bd, color: t.palette[6] }], 'count', 'day', period.range)} height={200} />
              </Section>
            )}
            <MetricChart t={t} personId={personId} metric="sleep-duration-goal" title="Sleep goal" range={period.range} bucket={period.bucket} />
          </div>
          <div className="muted small">Averages are over the {stats.n} nights that have data. A night without data means the watch was not worn, not that {person?.name ?? 'they'} did not sleep. Average stage split: deep {fmtNum((stats.stages.deep / stats.n), 1)} h · core {fmtNum(stats.stages.core / stats.n, 1)} h · REM {fmtNum(stats.stages.rem / stats.n, 1)} h · awake {fmtNum(stats.stages.awake / stats.n, 1)} h.</div>
        </div>
      )}
    </>
  )
}
