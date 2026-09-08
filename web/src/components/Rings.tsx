import type { ActivityDay } from '../api/types'
import { fmtDate, fmtInt } from '../format'

function arc(cx: number, cy: number, r: number, frac: number, color: string, track: string, width: number) {
  const f = Math.max(0, Math.min(1, frac))
  const c = 2 * Math.PI * r
  return (
    <>
      <circle cx={cx} cy={cy} r={r} fill="none" stroke={track} strokeWidth={width} />
      <circle
        cx={cx}
        cy={cy}
        r={r}
        fill="none"
        stroke={color}
        strokeWidth={width}
        strokeLinecap="round"
        strokeDasharray={`${c * f} ${c}`}
        transform={`rotate(-90 ${cx} ${cy})`}
        opacity={f === 0 ? 0 : 1}
      />
    </>
  )
}

export function RingGlyph({ day, size = 44 }: { day: ActivityDay; size?: number }) {
  const w = size * 0.13
  const c = size / 2
  const frac = (v: number | null, g: number | null) => (v && g ? v / g : 0)
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-label={`${fmtDate(day.date)}: move ${fmtInt(day.energy)}/${fmtInt(day.energy_goal)}`}>
      {arc(c, c, c - w / 2 - 1, frac(day.energy, day.energy_goal), 'var(--ring-move)', 'var(--surface-2)', w)}
      {arc(c, c, c - w * 1.5 - 2, frac(day.exercise, day.exercise_goal), 'var(--ring-exercise)', 'var(--surface-2)', w)}
      {arc(c, c, c - w * 2.5 - 3, frac(day.stand, day.stand_goal), 'var(--ring-stand)', 'var(--surface-2)', w)}
    </svg>
  )
}

export function RingsStrip({ days }: { days: ActivityDay[] }) {
  return (
    <div className="rings-strip">
      {days.map((d) => (
        <div className="ring-day" key={d.date} title={`${fmtDate(d.date)} · move ${fmtInt(d.energy)}/${fmtInt(d.energy_goal)} kcal · exercise ${fmtInt(d.exercise)}/${fmtInt(d.exercise_goal)} min · stand ${fmtInt(d.stand)}/${fmtInt(d.stand_goal)} h`}>
          <RingGlyph day={d} />
          <span>{fmtDate(d.date, 'EEEEE d')}</span>
        </div>
      ))}
    </div>
  )
}
