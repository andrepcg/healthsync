import { useEffect, useRef } from 'react'
import { format } from 'date-fns'
import { echarts, type EChartsOption } from './echarts'
import { ensureTheme } from './theme'
import { useThemeMode } from '../state/ThemeContext'

interface Props {
  option: EChartsOption
  height?: number | string
  className?: string
  onClick?: (params: unknown) => void
  /** Called with the day (YYYY-MM-DD) nearest to a click anywhere in the plot of a time-axis chart. */
  onDayClick?: (date: string) => void
  /** Called with the instance after creation (for connect/group). */
  onReady?: (chart: echarts.ECharts) => void
}

/**
 * Thin ECharts wrapper: one instance per mount, resize on container change,
 * re-init on theme switch, notMerge so option builders stay pure.
 */
export function Chart({ option, height = 260, className, onClick, onDayClick, onReady }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  const inst = useRef<echarts.ECharts | null>(null)
  const mode = useThemeMode()

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const chart = echarts.init(el, ensureTheme(mode), { renderer: 'canvas' })
    inst.current = chart
    onReady?.(chart)
    const ro = new ResizeObserver(() => chart.resize())
    ro.observe(el)
    return () => {
      ro.disconnect()
      chart.dispose()
      inst.current = null
    }
  }, [mode]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    inst.current?.setOption(option, { notMerge: true, lazyUpdate: true })
  }, [option, mode])

  useEffect(() => {
    const c = inst.current
    if (!c || !onClick) return
    c.on('click', onClick)
    return () => {
      c.off('click', onClick)
    }
  }, [onClick, mode])

  useEffect(() => {
    const c = inst.current
    if (!c || !onDayClick) return
    const zr = c.getZr()
    const h = (e: { offsetX: number; offsetY: number }) => {
      const px = [e.offsetX, e.offsetY]
      if (!c.containPixel({ gridIndex: 0 }, px)) return
      const [x] = c.convertFromPixel({ gridIndex: 0 }, px) as number[]
      // Daily points sit at local midnight: half a day either side belongs to it.
      if (Number.isFinite(x)) onDayClick(format(new Date(x + 12 * 3600 * 1000), 'yyyy-MM-dd'))
    }
    zr.on('click', h)
    return () => {
      zr.off('click', h)
    }
  }, [onDayClick, mode])

  return <div ref={ref} className={className} style={{ width: '100%', height, cursor: onDayClick ? 'pointer' : undefined }} />
}
