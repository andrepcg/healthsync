import { Link } from 'react-router-dom'
import type { Observation } from '../api/types'

export const SEVERITY_LABEL: Record<Observation['severity'], string> = {
  alert: 'Alert',
  warning: 'Warning',
  notice: 'Notice',
  info: 'Info',
}

export const CATEGORY_LABEL: Record<string, string> = {
  recovery: 'Recovery',
  training: 'Training load',
  sleep: 'Sleep',
  activity: 'Activity habits',
  fitness: 'Fitness',
  heart: 'Heart',
  hearing: 'Hearing',
  body: 'Body',
  data: 'Data quality',
}

export function severityClass(o: Observation): string {
  if (o.severity === 'alert') return 'bad'
  if (o.severity === 'warning') return 'warn'
  if (o.tone === 'good') return 'good'
  if (o.severity === 'notice') return 'info'
  return ''
}

export function ObservationCard({ o, personId, compact }: { o: Observation; personId: string; compact?: boolean }) {
  const cls = severityClass(o)
  return (
    <div className={`obs ${cls}`}>
      <div className="row spread" style={{ alignItems: 'flex-start' }}>
        <div style={{ minWidth: 0 }}>
          <div className="row" style={{ gap: 6 }}>
            <span className={`badge ${cls}`}>{SEVERITY_LABEL[o.severity]}</span>
            <span className="faint small">{CATEGORY_LABEL[o.category] ?? o.category}</span>
          </div>
          <div className="obs-title">{o.link ? <Link to={o.link === 'people' ? '/people' : `/p/${personId}/${o.link}`}>{o.title}</Link> : o.title}</div>
        </div>
        <span className="faint small nowrap">{o.window}</span>
      </div>
      <p className="obs-detail">{o.detail}</p>
      {!compact && o.advice && <p className="obs-advice">{o.advice}</p>}
      {!compact && o.evidence?.length > 0 && (
        <dl className="obs-evidence">
          {o.evidence.map((e) => (
            <div key={e.label}>
              <dt>{e.label}</dt>
              <dd>{e.value}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  )
}
