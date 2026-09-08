import { useMemo } from 'react'
import type { Tile } from '../api/types'
import { Chart } from '../charts/Chart'
import { sparkline } from '../charts/options'
import { tokens } from '../charts/theme'
import { useThemeMode } from '../state/ThemeContext'
import { fmtHours, fmtInt, fmtNum, fmtPct, fmtUnit, fmtValue } from '../format'

export function DeltaBadge({ delta, direction, good }: { delta?: number; direction: string; good: string }) {
  if (delta === undefined || direction === '') return null
  let cls = ''
  if (direction !== 'flat' && good) {
    cls = direction === good ? 'good' : 'bad'
  }
  const arrow = direction === 'up' ? '▲' : direction === 'down' ? '▼' : '■'
  return (
    <span className={`delta ${cls}`} title="vs previous period">
      {arrow} {fmtPct(Math.abs(delta) < 0.05 ? 0 : delta, Math.abs(delta) < 10 ? 1 : 0).replace(/^[+-]/, '')}
    </span>
  )
}

function headline(t: Tile): { main: string; unit: string } {
  if (t.metric === 'sleep') return { main: fmtHours(t.value), unit: '/ night' }
  switch (t.unit) {
    case 'count':
      return { main: fmtInt(t.value), unit: t.kind === 'cumulative' || t.kind === 'event' ? '/ day' : '' }
    case 'min':
      return { main: fmtInt(t.value), unit: 'min / day' }
    case 'kcal':
      return { main: fmtInt(t.value), unit: 'kcal / day' }
    case 'km':
      return { main: fmtNum(t.value, 1), unit: 'km / day' }
    case 'count/min':
      return { main: fmtInt(t.value), unit: 'bpm' }
    case 'hr':
      return { main: fmtHours(t.value), unit: '' }
    case '%':
      return { main: fmtNum(t.value, 1), unit: '%' }
    case 'degC':
      return { main: fmtNum(t.value, 2), unit: '°C' }
    default:
      return { main: fmtNum(t.value), unit: fmtUnit(t.unit) + (t.kind === 'cumulative' ? ' / day' : '') }
  }
}

export function KpiTile({ tile, onClick }: { tile: Tile; onClick?: () => void }) {
  const mode = useThemeMode()
  const t = useMemo(() => tokens(), [mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const opt = useMemo(() => sparkline(t, tile.sparkline), [t, tile.sparkline])
  const h = headline(tile)
  const denom = tile.metric === 'sleep' ? `${tile.days} nights` : tile.kind === 'latest' ? 'latest reading' : `${tile.days} days with data`
  return (
    <div className={`card tile ${onClick ? 'clickable' : ''}`} onClick={onClick} style={onClick ? { cursor: 'pointer' } : undefined}>
      <div className="row spread">
        <span className="label">{tile.name}</span>
        <DeltaBadge delta={tile.delta_pct} direction={tile.direction} good={tile.good} />
      </div>
      <div className="value">
        {h.main}
        {h.unit && <span className="unit">{h.unit}</span>}
      </div>
      {tile.sparkline.length > 1 && (
        <div className="spark">
          <Chart option={opt} height={34} />
        </div>
      )}
      <div className="foot">
        <span>{denom}</span>
        {tile.total !== undefined && tile.kind === 'cumulative' && tile.metric !== 'sleep' && <span>total {fmtValue(tile.total, tile.unit)}</span>}
        {tile.previous !== undefined && tile.kind !== 'cumulative' && <span>prev {fmtValue(tile.previous, tile.unit)}</span>}
      </div>
    </div>
  )
}
