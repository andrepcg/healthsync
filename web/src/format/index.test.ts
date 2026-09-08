import { describe, expect, it } from 'vitest'
import { activityName, fmtDuration, fmtHours, fmtNum, fmtPct, fmtValue } from './index'

describe('format', () => {
  it('formats numbers by magnitude', () => {
    expect(fmtNum(12345.6)).toBe('12,346')
    expect(fmtNum(12.345)).toBe('12.3')
    expect(fmtNum(1.2345)).toBe('1.23')
    expect(fmtNum(null)).toBe('–')
  })
  it('formats durations', () => {
    expect(fmtDuration(45)).toBe('45 min')
    expect(fmtDuration(83)).toBe('1 h 23 min')
    expect(fmtHours(7.2)).toBe('7 h 12 min')
  })
  it('formats deltas and units', () => {
    expect(fmtPct(12.3)).toBe('+12 %')
    expect(fmtPct(-4.5, 1)).toBe('-4.5 %')
    expect(fmtValue(72, 'count/min')).toBe('72 bpm')
    expect(fmtValue(8123, 'count')).toBe('8,123')
    expect(fmtValue(97.5, '%')).toBe('97.5 %')
  })
  it('names activities', () => {
    expect(activityName('HKWorkoutActivityTypeHighIntensityIntervalTraining')).toBe('High Intensity Interval Training')
    expect(activityName('HKWorkoutActivityTypeCycling')).toBe('Cycling')
  })
})
