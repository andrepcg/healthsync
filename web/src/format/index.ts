import { format, parseISO } from 'date-fns'

const LOCALE = 'en-GB'

const nf = new Map<string, Intl.NumberFormat>()
function numberFormat(maxFrac: number, minFrac = 0): Intl.NumberFormat {
  const k = `${minFrac}-${maxFrac}`
  let f = nf.get(k)
  if (!f) {
    f = new Intl.NumberFormat(LOCALE, { maximumFractionDigits: maxFrac, minimumFractionDigits: minFrac })
    nf.set(k, f)
  }
  return f
}

/** Sensible precision by magnitude: 12 345 · 1 234 · 123 · 12.3 · 1.23 */
export function fmtNum(v: number | null | undefined, digits?: number): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '–'
  if (digits !== undefined) return numberFormat(digits, digits).format(v)
  const a = Math.abs(v)
  if (a >= 100) return numberFormat(0).format(v)
  if (a >= 10) return numberFormat(1).format(v)
  return numberFormat(2).format(v)
}

export function fmtInt(v: number | null | undefined): string {
  if (v === null || v === undefined) return '–'
  return numberFormat(0).format(v)
}

export function fmtPct(v: number | null | undefined, digits = 0): string {
  if (v === null || v === undefined) return '–'
  return `${v > 0 ? '+' : ''}${numberFormat(digits, digits).format(v)} %`
}

/** Minutes → "1 h 23 min" / "45 min" */
export function fmtDuration(min: number | null | undefined): string {
  if (min === null || min === undefined) return '–'
  const total = Math.round(min)
  const h = Math.floor(total / 60)
  const m = total % 60
  if (h === 0) return `${m} min`
  return `${h} h ${String(m).padStart(2, '0')} min`
}

/** Hours (decimal) → "7 h 12 min" */
export function fmtHours(h: number | null | undefined): string {
  if (h === null || h === undefined) return '–'
  return fmtDuration(h * 60)
}

export function fmtDate(d: string | null | undefined, pattern = 'd MMM yyyy'): string {
  if (!d) return '–'
  try {
    return format(parseISO(d.replace(' ', 'T')), pattern)
  } catch {
    return d
  }
}

export function fmtDateTime(d: string | null | undefined): string {
  return fmtDate(d, 'd MMM yyyy, HH:mm')
}

export function fmtTime(d: string | null | undefined): string {
  return fmtDate(d, 'HH:mm')
}

export function fmtShortDate(d: string): string {
  return fmtDate(d, 'd MMM')
}

export function fmtBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}

/** Display unit names the way people read them. */
export function fmtUnit(u: string | undefined): string {
  switch (u) {
    case 'count/min':
      return 'bpm'
    case 'degC':
      return '°C'
    case 'degF':
      return '°F'
    case 'count':
      return ''
    case 'hr':
      return 'h'
    case 'mL/min·kg':
      return 'mL/kg·min'
    case 'dBASPL':
      return 'dB'
    case 'appleEffortScore':
      return '/10'
    default:
      return u ?? ''
  }
}

/** Value with unit, choosing precision by unit. */
export function fmtValue(v: number | null | undefined, unit?: string): string {
  if (v === null || v === undefined) return '–'
  switch (unit) {
    case 'count':
    case 'count/min':
    case 'kcal':
    case 'min':
    case 'ms':
    case 'W':
    case 'm':
      return `${fmtInt(v)}${unit === 'count' ? '' : ' ' + fmtUnit(unit)}`
    case 'hr':
      return fmtHours(v)
    case '%':
      return `${fmtNum(v, 1)} %`
    default:
      return `${fmtNum(v)}${unit ? ' ' + fmtUnit(unit) : ''}`
  }
}

/** HKWorkoutActivityTypeHighIntensityIntervalTraining → High Intensity Interval Training */
export function activityName(t: string | undefined): string {
  if (!t) return ''
  const s = t.replace(/^HKWorkoutActivityType/, '')
  return s.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
}

export function activityEmoji(t: string | undefined): string {
  const s = (t ?? '').toLowerCase()
  if (s.includes('run')) return '🏃'
  if (s.includes('cycl')) return '🚴'
  if (s.includes('walk') || s.includes('hik')) return '🚶'
  if (s.includes('swim') || s.includes('water') || s.includes('div')) return '🏊'
  if (s.includes('climb')) return '🧗'
  if (s.includes('yoga') || s.includes('mind')) return '🧘'
  if (s.includes('strength') || s.includes('functional') || s.includes('core')) return '🏋️'
  if (s.includes('interval') || s.includes('cross')) return '⚡'
  if (s.includes('jump')) return '🪢'
  if (s.includes('row')) return '🚣'
  if (s.includes('ski') || s.includes('snow')) return '⛷️'
  if (s.includes('soccer') || s.includes('football')) return '⚽'
  if (s.includes('tennis') || s.includes('padel') || s.includes('badminton')) return '🎾'
  if (s.includes('dance')) return '💃'
  return '🏅'
}

/** HKCategoryValueSleepAnalysisAsleepCore → Asleep Core */
export function categoryValue(v: unknown): string {
  if (typeof v !== 'string') return String(v ?? '')
  return v
    .replace(/^HKCategoryValue[A-Za-z]*?(?=[A-Z][a-z]+$|Asleep|InBed|Awake|NotApplicable)/, '')
    .replace(/^HKCategoryValue/, '')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
}

/** Human label for an hk metric key or table name */
export function metricLabel(key: string): string {
  return key
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .replace('Hrv', 'HRV')
    .replace('Spo2', 'SpO₂')
    .replace('Vo2max', 'VO₂ Max')
    .replace('Bmi', 'BMI')
}
