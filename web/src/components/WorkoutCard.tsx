import { Link } from 'react-router-dom'
import type { WorkoutItem } from '../api/types'
import { activityEmoji, activityName, fmtDateTime, fmtDuration, fmtInt, fmtNum } from '../format'

export function WorkoutCard({ w, personId }: { w: WorkoutItem; personId: string }) {
  const bits: string[] = []
  if (w.duration_min !== null) bits.push(fmtDuration(w.duration_min))
  if (w.distance_km) bits.push(`${fmtNum(w.distance_km, 2)} km`)
  if (w.energy_kcal) bits.push(`${fmtInt(w.energy_kcal)} kcal`)
  if (w.avg_hr) bits.push(`${fmtInt(w.avg_hr)} bpm avg`)
  return (
    <Link className="workout-card" to={`/p/${personId}/workouts/${w.id}`}>
      <span className="emoji">{activityEmoji(w.activity_type)}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="row spread">
          <b>{activityName(w.activity_type)}</b>
          <span className="meta">{fmtDateTime(w.start)}</span>
        </div>
        <div className="meta">
          {bits.join(' · ')}
          {w.has_route && ' · 🗺️'}
          {w.effort_score !== undefined && ` · effort ${fmtInt(w.effort_score)}`}
        </div>
      </div>
    </Link>
  )
}
