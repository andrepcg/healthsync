import { useMemo, useState } from 'react'
import { useWorkoutTypes, useWorkouts } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { stackedByType } from '../charts/options'
import { Pagination } from '../components/DataTable'
import { ErrorNote, Loading, Section } from '../components/Section'
import { WorkoutCard } from '../components/WorkoutCard'
import { activityEmoji, activityName, fmtDuration, fmtInt, fmtNum } from '../format'
import { bucketKey } from './weeks'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

const PAGE = 25

export function WorkoutsPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const [type, setType] = useState('')
  const [offset, setOffset] = useState(0)
  const types = useWorkoutTypes(personId)
  const list = useWorkouts(personId, { ...period.range, type, limit: PAGE, offset })
  const all = useWorkouts(personId, { ...period.range, limit: 1000 })

  const volume = useMemo(() => {
    const items = all.data?.items ?? []
    if (!items.length) return null
    const bucket = period.bucket === 'day' ? 'week' : period.bucket
    const byKey = new Map<string, Record<string, number>>()
    const typeSet = new Set<string>()
    for (const w of items) {
      const k = bucketKey(w.start.slice(0, 10), bucket)
      const name = activityName(w.activity_type)
      typeSet.add(name)
      const row = byKey.get(k) ?? {}
      row[name] = (row[name] ?? 0) + (w.duration_min ?? 0)
      byKey.set(k, row)
    }
    const weeks = [...byKey.keys()].sort()
    const typesArr = [...typeSet]
    const data: Record<string, number[]> = {}
    for (const ty of typesArr) data[ty] = weeks.map((wk) => Math.round(byKey.get(wk)?.[ty] ?? 0))
    const perType = typesArr.map((ty) => {
      const ws = items.filter((w) => activityName(w.activity_type) === ty)
      return { ty, count: ws.length, minutes: ws.reduce((a, w) => a + (w.duration_min ?? 0), 0), km: ws.reduce((a, w) => a + (w.distance_km ?? 0), 0), kcal: ws.reduce((a, w) => a + (w.energy_kcal ?? 0), 0), raw: ws[0].activity_type }
    }).sort((a, b) => b.minutes - a.minutes)
    return { weeks, types: typesArr, data, perType, bucket }
  }, [all.data, period.bucket])

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Workouts" page={page} /><NoData page={page} /></>

  return (
    <>
      <PageHead title="Workouts" sub={all.data ? `${all.data.total} workouts in period` : undefined} page={page} compare={false} />
      <div className="stack">
        {volume && (
          <>
            <div className="stat-strip card">
              {volume.perType.slice(0, 6).map((p) => (
                <div className="stat" key={p.ty}>
                  <div className="l">{activityEmoji(p.raw)} {p.ty}</div>
                  <div className="v">{p.count}</div>
                  <div className="muted small">
                    {fmtDuration(p.minutes)}
                    {p.km > 0 && ` · ${fmtNum(p.km, 1)} km`}
                    {p.kcal > 0 && ` · ${fmtInt(p.kcal)} kcal`}
                  </div>
                </div>
              ))}
            </div>
            <Section title={`Training volume per ${volume.bucket}`} hint="minutes, stacked by activity">
              <Chart option={stackedByType(t, volume.weeks, volume.types, volume.data)} height={260} />
            </Section>
          </>
        )}

        <Section
          title="All workouts"
          right={
            <div className="chips">
              <button className={`chip ${type === '' ? 'active' : ''}`} onClick={() => { setType(''); setOffset(0) }}>All</button>
              {(types.data ?? []).map((ty) => (
                <button key={ty.type} className={`chip ${type === ty.type ? 'active' : ''}`} onClick={() => { setType(ty.type); setOffset(0) }}>
                  {activityEmoji(ty.type)} {activityName(ty.type)} <span className="faint">{ty.count}</span>
                </button>
              ))}
            </div>
          }
        >
          {list.isLoading && <Loading />}
          {list.error && <ErrorNote error={list.error} />}
          {list.data && list.data.items.length === 0 && <div className="muted small">No workouts match.</div>}
          {list.data?.items.map((w) => <WorkoutCard key={w.id} w={w} personId={personId} />)}
          {list.data && list.data.total > PAGE && <Pagination total={list.data.total} limit={PAGE} offset={offset} onChange={setOffset} />}
        </Section>
      </div>
    </>
  )
}
