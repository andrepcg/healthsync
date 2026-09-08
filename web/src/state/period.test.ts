import { describe, expect, it } from 'vitest'
import { autoBucket, periodDays, previousPeriod, readPeriod, resolvePreset, writePeriod } from './period'

describe('period', () => {
  it('resolves presets against the anchor, not today', () => {
    expect(resolvePreset('7d', '2026-09-08')).toEqual({ from: '2026-09-02', to: '2026-09-08' })
    expect(resolvePreset('30d', '2026-09-08')).toEqual({ from: '2026-08-10', to: '2026-09-08' })
    expect(resolvePreset('1y', '2026-09-08')).toEqual({ from: '2025-09-09', to: '2026-09-08' })
    expect(resolvePreset('all', '2026-09-08', '2017-01-01')).toEqual({ from: '2017-01-01', to: '2026-09-08' })
  })
  it('computes the previous period of equal length', () => {
    expect(periodDays('2026-09-01', '2026-09-30')).toBe(30)
    expect(previousPeriod('2026-09-01', '2026-09-30')).toEqual({ from: '2026-08-02', to: '2026-08-31' })
  })
  it('reads custom ranges and falls back to a 30 day preset', () => {
    const p = readPeriod(new URLSearchParams('from=2026-01-01&to=2026-01-31&compare=none'), '2026-09-08')
    expect(p).toEqual({ preset: 'custom', from: '2026-01-01', to: '2026-01-31', compare: false })
    const d = readPeriod(new URLSearchParams('from=bad'), '2026-09-08')
    expect(d.preset).toBe('30d')
    expect(d.compare).toBe(true)
  })
  it('writes presets and clears custom bounds', () => {
    const q = writePeriod(new URLSearchParams('from=2026-01-01&to=2026-01-31'), { preset: '7d' })
    expect(q.get('preset')).toBe('7d')
    expect(q.get('from')).toBeNull()
    const c = writePeriod(new URLSearchParams(), { preset: 'custom', from: '2026-01-01', to: '2026-01-02', compare: false })
    expect(c.toString()).toBe('from=2026-01-01&to=2026-01-02&compare=none')
  })
  it('picks buckets by span', () => {
    expect(autoBucket('2026-08-10', '2026-09-08')).toBe('day')
    expect(autoBucket('2026-01-01', '2026-09-08')).toBe('week')
    expect(autoBucket('2023-01-01', '2026-09-08')).toBe('month')
  })
})
