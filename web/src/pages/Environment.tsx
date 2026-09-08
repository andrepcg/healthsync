import { useEnvironment } from '../api/hooks'
import { Chart } from '../charts/Chart'
import { bandLine, barsSeries } from '../charts/options'
import { DataTable } from '../components/DataTable'
import { ErrorNote, Loading, Section } from '../components/Section'
import { categoryValue } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

export function EnvironmentPage() {
  const page = usePage()
  const { personId, period, t, person } = page
  const q = useEnvironment(personId, period.range, period.bucket)
  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Environment & Hearing" page={page} /><NoData page={page} /></>
  const d = q.data
  const empty = d && !d.headphone_audio_exposure && !d.environmental_audio_exposure && !d.time_in_daylight && !d.uv_exposure && !d.water_temperature
  return (
    <>
      <PageHead title="Environment & Hearing" sub="Sound exposure, daylight and surroundings" page={page} compare={false} />
      {q.isLoading && <Loading />}
      {q.error && <ErrorNote error={q.error} />}
      {empty && <div className="card muted">No environment data in this period.</div>}
      {d && (
        <div className="grid cols-2">
          {d.headphone_audio_exposure && (
            <Section title="Headphone audio levels" hint="daily avg with range · WHO recommends staying under 80 dB">
              <Chart option={bandLine(t, d.headphone_audio_exposure.points, 'dBASPL', period.bucket, t.palette[4], [{ y: 80, label: '80 dB' }], period.range)} height={240} />
            </Section>
          )}
          {d.environmental_audio_exposure && (
            <Section title="Environmental sound levels" hint="measured by the Watch">
              <Chart option={bandLine(t, d.environmental_audio_exposure.points, 'dBASPL', period.bucket, t.palette[2], [{ y: 80, label: '80 dB' }], period.range)} height={240} />
            </Section>
          )}
          {d.time_in_daylight && (
            <Section title="Time in daylight" hint="minutes per day · 20 min is a common target">
              <Chart option={barsSeries(t, d.time_in_daylight.points, 'min', period.bucket, undefined, 20, t.palette[3], period.range)} height={240} />
            </Section>
          )}
          {d.uv_exposure && (
            <Section title="UV exposure">
              <Chart option={bandLine(t, d.uv_exposure.points, 'count', period.bucket, t.palette[6], undefined, period.range)} height={240} />
            </Section>
          )}
          {d.water_temperature && (
            <Section title="Water temperature" hint="from swims and dives">
              <Chart option={bandLine(t, d.water_temperature.points, 'degC', period.bucket, t.palette[7], undefined, period.range)} height={240} />
            </Section>
          )}
          {d.events.length > 0 && (
            <Section title="Loud environment notifications" hint={`${d.events.length}`} className="span-2">
              <DataTable columns={['start_date', 'value', 'metadata', 'source_name']} rows={d.events.map((e) => ({ ...e, value: categoryValue(e.value), metadata: (e.metadata as Record<string, string>)?.HKMetadataKeyAudioExposureLevel ?? JSON.stringify(e.metadata) }))} />
            </Section>
          )}
        </div>
      )}
    </>
  )
}
