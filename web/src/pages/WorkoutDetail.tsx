import { useMemo } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useRoute, useWorkout } from '../api/hooks'
import { personPath, withParams } from '../api/client'
import { Chart } from '../charts/Chart'
import { routeProfile, workoutHR, zonesBar } from '../charts/options'
import { ErrorNote, Loading, Section } from '../components/Section'
import { DataTable } from '../components/DataTable'
import { RouteMap } from '../components/RouteMap'
import { activityEmoji, activityName, fmtDateTime, fmtDuration, fmtInt, fmtNum, fmtValue } from '../format'
import { usePage } from './usePage'

function haversine(a: [number, number], b: [number, number]) {
  const R = 6371000
  const toR = (x: number) => (x * Math.PI) / 180
  const dLat = toR(b[0] - a[0])
  const dLon = toR(b[1] - a[1])
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(toR(a[0])) * Math.cos(toR(b[0])) * Math.sin(dLon / 2) ** 2
  return 2 * R * Math.asin(Math.sqrt(h))
}

export default function WorkoutDetailPage() {
  const { wid } = useParams()
  const page = usePage()
  const { personId, t } = page
  const q = useWorkout(personId, wid)
  const w = q.data
  const route = useRoute(personId, wid, !!w?.has_route)

  const zones = useMemo(() => {
    const zs = (w?.zones ?? []).filter((z) => (z.group_type as string)?.includes('HeartRate'))
    return zs.map((z, i) => {
      const min = z.minimum as number | null
      const max = z.maximum as number | null
      const label = min && max ? `Z${i + 1} ${fmtInt(min)}–${fmtInt(max)}` : min ? `Z${i + 1} ${fmtInt(min)}+` : `Z${i + 1} <${fmtInt(max)}`
      return { label, minutes: (z.duration as number) ?? 0, min: min ?? undefined, max: max ?? undefined }
    })
  }, [w])

  const pauses = useMemo(() => {
    const evs = (w?.events ?? []) as { type: string; date: string }[]
    const out: [string, string][] = []
    let open: string | null = null
    for (const e of evs) {
      if (e.type.endsWith('Pause')) open = e.date
      if (e.type.endsWith('Resume') && open) {
        out.push([open.replace(' ', 'T'), e.date.replace(' ', 'T')])
        open = null
      }
    }
    return out
  }, [w])

  const segments = useMemo(() => {
    const evs = (w?.events ?? []) as { type: string; date: string; duration: number | null }[]
    return evs.filter((e) => e.type.endsWith('Segment') && e.duration).map((e, i) => ({ i: i + 1, start: e.date, duration: e.duration as number }))
  }, [w])

  const profile = useMemo(() => {
    const pts = route.data?.points ?? []
    let dist = 0
    return pts.map((p, i) => {
      if (i > 0) dist += haversine([pts[i - 1].lat, pts[i - 1].lon], [p.lat, p.lon])
      return { ele: p.ele, speed: p.speed, dist }
    })
  }, [route.data])

  const stats = useMemo(() => {
    const out: { label: string; value: string }[] = []
    if (!w) return out
    if (w.duration_min !== null) out.push({ label: 'Duration', value: fmtDuration(w.duration_min) })
    if (w.distance_km) out.push({ label: 'Distance', value: `${fmtNum(w.distance_km, 2)} km` })
    if (w.distance_km && w.duration_min) {
      const paceMin = w.duration_min / w.distance_km
      const isRun = /Running|Walking|Hiking/.test(w.activity_type)
      out.push(isRun ? { label: 'Pace', value: `${Math.floor(paceMin)}:${String(Math.round((paceMin % 1) * 60)).padStart(2, '0')} /km` } : { label: 'Avg speed', value: `${fmtNum((w.distance_km / w.duration_min) * 60, 1)} km/h` })
    }
    if (w.energy_kcal) out.push({ label: 'Active energy', value: `${fmtInt(w.energy_kcal)} kcal` })
    if (w.avg_hr) out.push({ label: 'Avg HR', value: `${fmtInt(w.avg_hr)} bpm` })
    if (w.max_hr) out.push({ label: 'Max HR', value: `${fmtInt(w.max_hr)} bpm` })
    if (w.effort_score !== undefined) out.push({ label: 'Effort', value: `${fmtInt(w.effort_score)} / 10` })
    const mets = w.metadata?.HKAverageMETs
    if (mets) out.push({ label: 'Avg METs', value: fmtNum(parseFloat(mets), 1) })
    const temp = w.metadata?.HKWeatherTemperature
    if (temp) {
      const f = parseFloat(temp)
      out.push({ label: 'Weather', value: `${fmtNum(temp.includes('degF') ? ((f - 32) * 5) / 9 : f, 0)} °C${w.metadata?.HKWeatherHumidity ? ` · ${fmtInt(parseFloat(w.metadata.HKWeatherHumidity) / 100)} % rh` : ''}` })
    }
    const asc = w.metadata?.HKElevationAscended
    if (asc) out.push({ label: 'Ascent', value: `${fmtNum(parseFloat(asc) / 100, 0)} m` })
    for (const s of w.statistics as { type: string; average: number | null; sum: number | null; unit: string }[]) {
      const ty = s.type.replace('HKQuantityTypeIdentifier', '')
      if (['HeartRate', 'ActiveEnergyBurned', 'BasalEnergyBurned'].includes(ty) || ty.startsWith('Distance')) continue
      if (s.average !== null) out.push({ label: ty.replace(/([a-z])([A-Z])/g, '$1 $2'), value: `${fmtValue(s.average, s.unit)} avg` })
      else if (s.sum !== null) out.push({ label: ty.replace(/([a-z])([A-Z])/g, '$1 $2'), value: fmtValue(s.sum, s.unit) })
    }
    return out
  }, [w])

  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorNote error={q.error} />
  if (!w) return null

  return (
    <>
      <div className="page-head">
        <div>
          <div className="small"><Link to={`/p/${personId}/workouts`}>← Workouts</Link></div>
          <h1>{activityEmoji(w.activity_type)} {activityName(w.activity_type)}</h1>
          <div className="sub">
            {fmtDateTime(w.start)} → {w.end.slice(11, 16)} · {w.source}
            {w.indoor !== undefined && <span className="badge" style={{ marginLeft: 8 }}>{w.indoor ? 'indoor' : 'outdoor'}</span>}
          </div>
        </div>
        {w.has_route && (
          <a className="btn sm" href={withParams(personPath(personId, `/workouts/${w.id}/route`), { format: 'gpx' })}>
            Download GPX
          </a>
        )}
      </div>

      <div className="stack">
        <div className="stat-strip card">
          {stats.map((s) => (
            <div className="stat" key={s.label}>
              <div className="l">{s.label}</div>
              <div className="v">{s.value}</div>
            </div>
          ))}
        </div>

        {w.hr_samples.length > 1 && (
          <div className="grid cols-3">
            <Section title="Heart rate" hint={`${w.hr_samples.length} samples`} className={zones.length ? 'span-2' : 'span-2'}>
              <Chart option={workoutHR(t, w.hr_samples, zones, pauses)} height={280} />
            </Section>
            {zones.length > 0 && (
              <Section title="Heart rate zones" hint="minutes per zone">
                <Chart option={zonesBar(t, zones)} height={280} />
              </Section>
            )}
          </div>
        )}

        {w.has_route && (
          <div className="grid cols-3">
            <Section title="Route" hint={route.data ? `${route.data.points.length} GPS points · ${w.route ? fmtNum((w.route.distance_m as number) / 1000, 2) + ' km · ↑ ' + fmtNum(w.route.elevation_gain_m as number, 0) + ' m' : ''}` : undefined} className="span-2">
              {route.isLoading && <Loading />}
              {route.data && <RouteMap points={route.data.points} />}
            </Section>
            <Section title="Elevation & speed">
              {profile.length > 1 && <Chart option={routeProfile(t, profile)} height={360} />}
            </Section>
          </div>
        )}

        {segments.length > 0 && (
          <Section title="Segments" hint="from workout events">
            <div className="table-wrap">
              <table className="data">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Start</th>
                    <th className="num">Duration</th>
                  </tr>
                </thead>
                <tbody>
                  {segments.map((s) => (
                    <tr key={s.i}>
                      <td>{s.i}</td>
                      <td>{s.start.slice(11, 19)}</td>
                      <td className="num">{fmtDuration(s.duration)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>
        )}

        <details className="card">
          <summary style={{ cursor: 'pointer', fontWeight: 600 }}>Raw data: statistics, events, metadata</summary>
          <div className="stack" style={{ marginTop: 12 }}>
            {w.statistics.length > 0 && <><h3>Statistics</h3><DataTable columns={['type', 'sum', 'average', 'minimum', 'maximum', 'unit']} rows={w.statistics} /></>}
            {w.events.length > 0 && <><h3>Events</h3><DataTable columns={['type', 'date', 'duration', 'duration_unit']} rows={w.events} /></>}
            {Object.keys(w.metadata).length > 0 && (
              <>
                <h3>Metadata</h3>
                <dl className="kv">
                  {Object.entries(w.metadata).map(([k, v]) => (
                    <div key={k} style={{ display: 'contents' }}>
                      <dt>{k}</dt>
                      <dd className="mono">{v}</dd>
                    </div>
                  ))}
                </dl>
              </>
            )}
            {w.device && <div className="muted small">Device: {w.device}{w.source_version ? ` · software ${w.source_version}` : ''}</div>}
          </div>
        </details>
      </div>
    </>
  )
}
