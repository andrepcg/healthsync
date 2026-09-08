import { Link } from 'react-router-dom'
import { useUploadStatus } from '../api/hooks'
import { fmtInt } from '../format'

export function ImportBanner({ personId }: { personId: string }) {
  const { data } = useUploadStatus(personId, false)
  if (!data || data.status === 'idle') return null
  if (data.status === 'running') {
    const p = data.progress
    return (
      <div className="banner">
        <span>⏳</span>
        <span>
          Importing <b>{data.filename}</b>: {fmtInt(p.records)} records · {p.workouts} workouts · {p.routes} routes · {p.ecgs} ECGs
        </span>
        <span className="faint small" style={{ marginLeft: 'auto' }}>
          {data.elapsed}
        </span>
      </div>
    )
  }
  if (data.status === 'failed') {
    return (
      <div className="banner bad">
        <span>⚠️</span>
        <span>
          Last import failed: {data.error} · <Link to="/people">details</Link>
        </span>
      </div>
    )
  }
  return null
}
