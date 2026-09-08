import { useEffect, useMemo, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { RoutePoint } from '../api/types'
import { tokens } from '../charts/theme'

/** Polyline coloured by speed on OSM tiles; still draws without tiles (offline). */
export function RouteMap({ points }: { points: RoutePoint[] }) {
  const ref = useRef<HTMLDivElement>(null)
  const t = useMemo(() => tokens(), [])

  useEffect(() => {
    const el = ref.current
    if (!el || points.length < 2) return
    const map = L.map(el, { zoomControl: true, attributionControl: true })
    // Leaflet needs a view before vector layers can be clipped/rendered.
    const bounds = L.latLngBounds(points.map((p) => [p.lat, p.lon] as [number, number]))
    map.fitBounds(bounds, { padding: [20, 20] })
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      errorTileUrl: 'data:image/gif;base64,R0lGODlhAQABAAAAACw=',
    }).addTo(map)

    const speeds = points.map((p) => p.speed ?? 0).filter((s) => s > 0)
    const lo = Math.min(...speeds, 0)
    const hi = Math.max(...speeds, 1)
    const color = (s: number | null) => {
      if (s === null) return t.accent
      const f = Math.max(0, Math.min(1, (s - lo) / (hi - lo || 1)))
      const hue = 220 - f * 180 // blue → red
      return `hsl(${hue} 80% 50%)`
    }
    const layers: L.Layer[] = []
    for (let i = 1; i < points.length; i++) {
      const a = points[i - 1]
      const b = points[i]
      layers.push(L.polyline([[a.lat, a.lon], [b.lat, b.lon]], { color: color(b.speed), weight: 4, opacity: 0.9 }))
    }
    L.featureGroup(layers).addTo(map)
    L.circleMarker([points[0].lat, points[0].lon], { radius: 6, color: '#fff', fillColor: t.good, fillOpacity: 1, weight: 2 }).addTo(map).bindTooltip('Start')
    const last = points[points.length - 1]
    L.circleMarker([last.lat, last.lon], { radius: 6, color: '#fff', fillColor: t.bad, fillOpacity: 1, weight: 2 }).addTo(map).bindTooltip('End')
    return () => {
      map.remove()
    }
  }, [points, t])

  return (
    <>
      <div ref={ref} className="leaflet-container" />
      <div className="legend" style={{ marginTop: 8 }}>
        <span style={{ ['--sw' as string]: 'hsl(220 80% 50%)' }}>slower</span>
        <span style={{ ['--sw' as string]: 'hsl(130 80% 45%)' }}>·</span>
        <span style={{ ['--sw' as string]: 'hsl(40 80% 50%)' }}>faster</span>
      </div>
    </>
  )
}
