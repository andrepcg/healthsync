import { useTable } from '../api/hooks'
import { MetricChart } from '../components/MetricChart'
import { DataTable } from '../components/DataTable'
import { Loading, Section } from '../components/Section'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

const METRICS = [
  { key: 'body-mass', refs: undefined },
  { key: 'bmi', refs: [{ y: 18.5, label: 'underweight' }, { y: 25, label: 'overweight' }, { y: 30, label: 'obese' }] },
  { key: 'body-fat' },
  { key: 'lean-body-mass' },
  { key: 'waist-circumference' },
  { key: 'height' },
  { key: 'body-temperature' },
  { key: 'basal-body-temperature' },
  { key: 'blood-glucose' },
  { key: 'insulin-delivery' },
  { key: 'blood-alcohol' },
  { key: 'electrodermal-activity' },
]

export function BodyPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const weights = useTable(personId, 'body_mass', { ...period.range, limit: 200, order: 'desc' })
  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Body" page={page} /><NoData page={page} /></>
  const any = METRICS.some((m) => page.has(m.key))
  return (
    <>
      <PageHead title="Body" sub="Weight, composition and other measurements" page={page} compare={false} />
      {!any && <div className="card muted">No body measurements recorded. Weight from a smart scale or the Health app will appear here.</div>}
      <div className="grid cols-2">
        {METRICS.map((m) => (
          <MetricChart key={m.key} t={t} personId={personId} metric={m.key} range={period.range} bucket={period.bucket} refLines={m.refs} color={t.palette[5]} />
        ))}
      </div>
      {weights.data && weights.data.total > 0 && (
        <Section title="Weight entries" hint={`${weights.data.total} in period`} className="" >
          <div style={{ maxHeight: 360, overflowY: 'auto' }}>
            <DataTable columns={['start_date', 'value', 'unit', 'source_name']} rows={weights.data.rows} />
          </div>
        </Section>
      )}
    </>
  )
}
