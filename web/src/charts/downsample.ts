/** Largest-Triangle-Three-Buckets downsampling for long [x, y] series. */
export function lttb(points: [number, number][], threshold: number): [number, number][] {
  const n = points.length
  if (threshold >= n || threshold < 3) return points
  const out: [number, number][] = [points[0]]
  const every = (n - 2) / (threshold - 2)
  let a = 0
  for (let i = 0; i < threshold - 2; i++) {
    const rs = Math.floor((i + 1) * every) + 1
    const re = Math.min(Math.floor((i + 2) * every) + 1, n)
    let ax = 0
    let ay = 0
    for (let j = rs; j < re; j++) {
      ax += points[j][0]
      ay += points[j][1]
    }
    ax /= re - rs
    ay /= re - rs
    const bs = Math.floor(i * every) + 1
    const be = Math.floor((i + 1) * every) + 1
    let maxArea = -1
    let next = bs
    for (let j = bs; j < be; j++) {
      const area = Math.abs((points[a][0] - ax) * (points[j][1] - points[a][1]) - (points[a][0] - points[j][0]) * (ay - points[a][1]))
      if (area > maxArea) {
        maxArea = area
        next = j
      }
    }
    out.push(points[next])
    a = next
  }
  out.push(points[n - 1])
  return out
}
