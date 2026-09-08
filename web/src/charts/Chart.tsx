import { useEffect, useRef } from 'react'
import { echarts, type EChartsOption } from './echarts'
import { ensureTheme } from './theme'
import { useThemeMode } from '../state/ThemeContext'

interface Props {
  option: EChartsOption
  height?: number | string
  className?: string
  onClick?: (params: unknown) => void
  /** Called with the instance after creation (for connect/group). */
  onReady?: (chart: echarts.ECharts) => void
}

/**
 * Thin ECharts wrapper: one instance per mount, resize on container change,
 * re-init on theme switch, notMerge so option builders stay pure.
 */
export function Chart({ option, height = 260, className, onClick, onReady }: Props) {
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

  return <div ref={ref} className={className} style={{ width: '100%', height }} />
}
