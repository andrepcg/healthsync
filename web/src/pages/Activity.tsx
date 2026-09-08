import { useMemo } from 'react'
import { useRings, useSeries } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { calendarHeatmap, weekdayBars } from '../charts/options'
import { MetricChart } from '../components/MetricChart'
import { RingsStrip } from '../components/Rings'
import { Loading, Section } from '../components/Section'
import { fmtInt } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'
import { getDay, parseISO } from 'date-fns'

export function ActivityPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const prev = period.compare ? period.prev : undefined
  const rings = useRings(personId, period.range)
  const stepsDay = useSeries(personId, 'steps', period.range, 'day')
  const stepsPrev = useSeries(personId, 'steps', period.prev, 'day', period.compare)

  const weekday = useMemo(() => {
    const agg = (pts?: { t: string; v: number }[]) => {
      if (!pts) return undefined
      const sums = Array(7).fill(0)
      const counts = Array(7).fill(0)
      for (const p of pts) {
        const d = (getDay(parseISO(p.t)) + 6) % 7
        sums[d] += p.v
        counts[d]++
      }
      return sums.map((s, i) => (counts[i] ? Math.round(s / counts[i]) : 0))
    }
    return { cur: agg(stepsDay.data?.points), prev: agg(stepsPrev.data?.points) }
  }, [stepsDay.data, stepsPrev.data])

  const ringsClosed = useMemo(() => (rings.data?.days ?? []).map((d) => ({ t: d.date, v: (d.closed.energy ? 1 : 0) + (d.closed.exercise ? 1 : 0) + (d.closed.stand ? 1 : 0) })), [rings.data])
  const goal = useMemo(() => {
    const days = rings.data?.days ?? []
    const last = days[days.length - 1]
    return last?.energy_goal ?? undefined
  }, [rings.data])

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Activity" page={page} /><NoData page={page} /></>

  return (
    <>
      <PageHead title="Activity" sub="Movement, energy and Activity rings" page={page} />
      <div className="stack">
        <div className="grid cols-2">
          <MetricChart t={t} personId={personId} metric="steps" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[0]} />
          <MetricChart t={t} personId={personId} metric="active-energy" range={period.range} prev={prev} bucket={period.bucket} color={t.rings.move} goal={period.bucket === 'day' ? goal : undefined} />
          <MetricChart t={t} personId={personId} metric="exercise-time" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[5]} />
          <MetricChart t={t} personId={personId} metric="stand-hours" title="Stand hours" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[2]} />
          <MetricChart t={t} personId={personId} metric="distance-walking-running" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[7]} />
          <MetricChart t={t} personId={personId} metric="distance-cycling" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[6]} />
          <MetricChart t={t} personId={personId} metric="flights-climbed" range={period.range} prev={prev} bucket={period.bucket} color={t.palette[4]} />
          <MetricChart t={t} personId={personId} metric="basal-energy" range={period.range} prev={prev} bucket={period.bucket} color={t.faint} />
          <MetricChart t={t} personId={personId} metric="distance-swimming" range={period.range} prev={prev} bucket={period.bucket} />
          <MetricChart t={t} personId={personId} metric="physical-effort" range={period.range} bucket={period.bucket} />
          <MetricChart t={t} personId={personId} metric="workout-effort-score" range={period.range} bucket={period.bucket} />
          <MetricChart t={t} personId={personId} metric="stand-time" range={period.range} prev={prev} bucket={period.bucket} />
          <MetricChart t={t} personId={personId} metric="move-time" range={period.range} prev={prev} bucket={period.bucket} />
        </div>

        {weekday.cur && stepsDay.data && stepsDay.data.points.length >= 7 && (
          <Section title="Steps by weekday" hint="mean per day with data">
            <Chart option={weekdayBars(t, weekday.cur, weekday.prev)} height={220} />
          </Section>
        )}

        {rings.data && rings.data.days.length > 0 && (
          <>
            <Section
              title="Activity rings"
              hint={`all three closed on ${rings.data.streaks.days_all_closed} of ${rings.data.streaks.days} days · longest streak ${rings.data.streaks.longest_all}`}
              right={rings.data.streaks.current_all > 0 ? <span className="badge good">current streak {rings.data.streaks.current_all}</span> : undefined}
            >
              {rings.data.days.length <= 60 ? (
                <RingsStrip days={rings.data.days} />
              ) : (
                <Chart option={calendarHeatmap(t, ringsClosed, period.from, period.to, 'count', 3)} height={Math.max(160, 40 + 130 * Math.ceil(ringsClosed.length / 365))} />
              )}
              <div className="legend" style={{ marginTop: 10 }}>
                <span style={{ ['--sw' as string]: 'var(--ring-move)' }}>Move {goal ? `(goal ${fmtInt(goal)} kcal)` : ''}</span>
                <span style={{ ['--sw' as string]: 'var(--ring-exercise)' }}>Exercise</span>
                <span style={{ ['--sw' as string]: 'var(--ring-stand)' }}>Stand</span>
              </div>
            </Section>
            {stepsDay.data && stepsDay.data.points.length > 60 && (
              <Section title="Steps calendar">
                <Chart option={calendarHeatmap(t, stepsDay.data.points, period.from, period.to, 'count')} height={Math.max(160, 40 + 130 * Math.ceil(stepsDay.data.points.length / 365))} />
              </Section>
            )}
          </>
        )}
      </div>
    </>
  )
}
