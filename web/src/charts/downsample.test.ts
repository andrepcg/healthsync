import { describe, expect, it } from 'vitest'
import { lttb } from './downsample'

describe('lttb', () => {
  it('keeps endpoints and hits the threshold', () => {
    const pts: [number, number][] = Array.from({ length: 1000 }, (_, i) => [i, Math.sin(i / 10)])
    const out = lttb(pts, 100)
    expect(out.length).toBe(100)
    expect(out[0]).toEqual(pts[0])
    expect(out[99]).toEqual(pts[999])
  })
  it('returns input when already small', () => {
    const pts: [number, number][] = [[0, 1], [1, 2]]
    expect(lttb(pts, 10)).toBe(pts)
  })
})
