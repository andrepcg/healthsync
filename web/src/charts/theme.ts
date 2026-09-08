import { echarts } from './echarts'

/** Read the CSS tokens so charts always match the current theme. */
export function tokens() {
  const s = getComputedStyle(document.documentElement)
  const v = (n: string) => s.getPropertyValue(n).trim()
  return {
    text: v('--text'),
    muted: v('--muted'),
    faint: v('--faint'),
    grid: v('--grid'),
    border: v('--border'),
    surface: v('--surface'),
    accent: v('--accent'),
    good: v('--good'),
    bad: v('--bad'),
    warn: v('--warn'),
    palette: [v('--c1'), v('--c2'), v('--c3'), v('--c4'), v('--c5'), v('--c6'), v('--c7'), v('--c8')],
    sleep: { deep: v('--sleep-deep'), core: v('--sleep-core'), rem: v('--sleep-rem'), awake: v('--sleep-awake'), inbed: v('--sleep-inbed') },
    rings: { move: v('--ring-move'), exercise: v('--ring-exercise'), stand: v('--ring-stand') },
  }
}

let registered = ''

/** (Re)register the ECharts theme from the current tokens; returns its name. */
export function ensureTheme(mode: 'light' | 'dark'): string {
  const name = `hs-${mode}`
  if (registered === name) return name
  const t = tokens()
  echarts.registerTheme(name, {
    color: t.palette,
    backgroundColor: 'transparent',
    textStyle: { color: t.text, fontFamily: getComputedStyle(document.body).fontFamily },
    title: { textStyle: { color: t.text } },
    legend: { textStyle: { color: t.muted } },
    tooltip: {
      backgroundColor: t.surface,
      borderColor: t.border,
      textStyle: { color: t.text, fontSize: 12 },
      extraCssText: 'box-shadow: 0 6px 20px rgba(0,0,0,.15); border-radius: 8px;',
    },
    categoryAxis: {
      axisLine: { lineStyle: { color: t.border } },
      axisTick: { show: false },
      axisLabel: { color: t.muted },
      splitLine: { show: false },
    },
    valueAxis: {
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: t.muted },
      splitLine: { lineStyle: { color: t.grid } },
    },
    timeAxis: {
      axisLine: { lineStyle: { color: t.border } },
      axisTick: { show: false },
      axisLabel: { color: t.muted },
      splitLine: { show: false },
    },
    line: { symbol: 'none', smooth: false, lineStyle: { width: 2 } },
    bar: { itemStyle: { borderRadius: [3, 3, 0, 0] } },
  })
  registered = name
  return name
}
