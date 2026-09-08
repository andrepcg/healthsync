import { addDays, differenceInCalendarDays, format, isValid, parseISO, subDays, subMonths, subYears } from 'date-fns'

export type Preset = '7d' | '30d' | '90d' | '1y' | 'all' | 'custom'

export interface PeriodState {
  preset: Preset
  from: string
  to: string
  compare: boolean
}

export const PRESETS: { key: Preset; label: string }[] = [
  { key: '7d', label: '7 days' },
  { key: '30d', label: '30 days' },
  { key: '90d', label: '90 days' },
  { key: '1y', label: '1 year' },
  { key: 'all', label: 'All' },
]

const DAY = 'yyyy-MM-dd'

export function fmtDay(d: Date): string {
  return format(d, DAY)
}

export function parseDay(s: string | undefined | null): Date | null {
  if (!s) return null
  const d = parseISO(s.slice(0, 10))
  return isValid(d) ? d : null
}

/**
 * Resolve a preset into concrete inclusive dates. The anchor is the last day
 * with data rather than today, so a person whose export is a week old still
 * sees a full window. `first` bounds the "all" preset.
 */
export function resolvePreset(preset: Preset, anchor: string, first?: string): { from: string; to: string } {
  const end = parseDay(anchor) ?? new Date()
  const to = fmtDay(end)
  switch (preset) {
    case '7d':
      return { from: fmtDay(subDays(end, 6)), to }
    case '30d':
      return { from: fmtDay(subDays(end, 29)), to }
    case '90d':
      return { from: fmtDay(subDays(end, 89)), to }
    case '1y':
      return { from: fmtDay(addDays(subYears(end, 1), 1)), to }
    case 'all':
      return { from: first ?? fmtDay(subYears(end, 10)), to }
    default:
      return { from: fmtDay(subMonths(end, 1)), to }
  }
}

export function readPeriod(params: URLSearchParams, anchor: string, first?: string): PeriodState {
  const compare = params.get('compare') !== 'none'
  const from = params.get('from')
  const to = params.get('to')
  if (from && to && parseDay(from) && parseDay(to) && from <= to) {
    return { preset: 'custom', from, to, compare }
  }
  const presetParam = params.get('preset') as Preset | null
  const preset: Preset = presetParam && PRESETS.some((p) => p.key === presetParam) ? presetParam : '30d'
  return { preset, ...resolvePreset(preset, anchor, first), compare }
}

export function writePeriod(params: URLSearchParams, next: Partial<PeriodState>): URLSearchParams {
  const p = new URLSearchParams(params)
  if (next.preset && next.preset !== 'custom') {
    p.set('preset', next.preset)
    p.delete('from')
    p.delete('to')
  }
  if (next.preset === 'custom' || (next.from && next.to)) {
    if (next.from) p.set('from', next.from)
    if (next.to) p.set('to', next.to)
    p.delete('preset')
  }
  if (next.compare !== undefined) {
    if (next.compare) p.delete('compare')
    else p.set('compare', 'none')
  }
  return p
}

export function periodDays(from: string, to: string): number {
  const a = parseDay(from)
  const b = parseDay(to)
  if (!a || !b) return 0
  return differenceInCalendarDays(b, a) + 1
}

export function previousPeriod(from: string, to: string): { from: string; to: string } {
  const a = parseDay(from)!
  const n = periodDays(from, to)
  return { from: fmtDay(subDays(a, n)), to: fmtDay(subDays(a, 1)) }
}

/** Day buckets up to 90 days, weeks up to ~15 months, months beyond. */
export function autoBucket(from: string, to: string): 'day' | 'week' | 'month' {
  const n = periodDays(from, to)
  if (n <= 92) return 'day'
  if (n <= 460) return 'week'
  return 'month'
}
