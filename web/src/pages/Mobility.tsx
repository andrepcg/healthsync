import { MetricChart } from '../components/MetricChart'
import { Loading } from '../components/Section'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

const WALKING = ['walking-speed', 'walking-step-length', 'walking-asymmetry', 'walking-double-support', 'walking-steadiness', 'stair-ascent-speed', 'stair-descent-speed', 'six-minute-walk']
const RUNNING = ['running-speed', 'running-power', 'running-stride-length', 'running-ground-contact-time', 'running-vertical-oscillation']
const CYCLING = ['cycling-speed', 'cycling-power', 'cycling-cadence', 'cycling-ftp']

export function MobilityPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Mobility & Running" page={page} /><NoData page={page} /></>
  const group = (title: string, keys: string[], color: string) =>
    keys.some((k) => page.has(k)) ? (
      <>
        <h2 style={{ marginTop: 8 }}>{title}</h2>
        <div className="grid cols-2">
          {keys.map((k) => (
            <MetricChart key={k} t={t} personId={personId} metric={k} range={period.range} bucket={period.bucket} color={color} height={210} refLines={k === 'walking-steadiness' ? [{ y: 50, label: 'low' }, { y: 30, label: 'very low' }] : undefined} />
          ))}
        </div>
      </>
    ) : null
  return (
    <>
      <PageHead title="Mobility & Running" sub="Gait metrics from the iPhone and Watch, plus running and cycling form" page={page} compare={false} />
      <div className="stack">
        {group('Walking', WALKING, t.palette[7])}
        {group('Running', RUNNING, t.palette[1])}
        {group('Cycling', CYCLING, t.palette[6])}
        {![...WALKING, ...RUNNING, ...CYCLING].some((k) => page.has(k)) && <div className="card muted">No mobility metrics recorded yet.</div>}
      </div>
    </>
  )
}
