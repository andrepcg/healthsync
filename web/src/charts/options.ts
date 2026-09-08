import type { EChartsOption } from './echarts'
import type { SeriesPoint, SleepNight } from '../api/types'
import { tokens } from './theme'
import { fmtDate, fmtNum, fmtUnit, fmtValue } from '../format'

type Tok = ReturnType<typeof tokens>

const baseGrid = { left: 12, right: 16, top: 28, bottom: 8, containLabel: true }

export type Range = { from: string; to: string }

/** End of the inclusive `to` day, so the last bar has room. */
function endOfDay(to: string) {
  return `${to}T23:59:59`
}

function timeAxis(bucket: 'day' | 'week' | 'month' = 'day', range?: Range) {
  return {
    type: 'time' as const,
    // Pin the axis to the selected period so single points and sparse
    // series still sit in the right place, and all cards align.
    min: range ? `${range.from}T00:00:00` : undefined,
    max: range ? endOfDay(range.to) : undefined,
    boundaryGap: bucket === 'day' ? false : (['5%', '5%'] as [string, string]),
    axisLabel: {
      hideOverlap: true,
      formatter: (v: number) => fmtDate(new Date(v).toISOString(), bucket === 'month' ? 'MMM yy' : 'd MMM'),
    },
  }
}

function valueAxis(unit?: string, min?: number | 'dataMin', max?: number | 'dataMax') {
  return {
    type: 'value' as const,
    min,
    max,
    scale: true,
    axisLabel: { formatter: (v: number) => fmtNum(v) },
    name: unit ? fmtUnit(unit) : undefined,
    nameGap: 8,
    nameTextStyle: { align: 'right' as const, color: undefined },
  }
}

const tip = (unit?: string) => ({
  trigger: 'axis' as const,
  valueFormatter: (v: unknown) => (typeof v === 'number' ? fmtValue(v, unit) : '–'),
})

/** Daily/weekly/monthly bars, optionally with the previous period as ghost bars aligned by index. */
export function barsSeries(
  t: Tok,
  cur: SeriesPoint[],
  unit: string,
  bucket: 'day' | 'week' | 'month',
  prev?: SeriesPoint[],
  goal?: number,
  color?: string,
  range?: Range,
): EChartsOption {
  const c = color ?? t.accent
  const series: Record<string, unknown>[] = []
  if (prev && prev.length && cur.length) {
    // Previous period aligned by position (day 1 ↔ day 1) as a faint dashed
    // line, so the comparison reads without competing with the bars.
    const shifted = prev.slice(0, cur.length).map((p, i) => [cur[i].t, p.v])
    series.push({
      name: 'Previous period',
      type: 'line',
      data: shifted,
      symbol: 'none',
      lineStyle: { color: t.faint, width: 1.5, type: 'dashed' },
      itemStyle: { color: t.faint },
      z: 3,
      tooltip: { valueFormatter: (v: unknown) => (typeof v === 'number' ? fmtValue(v, unit) : '–') },
    })
  }
  series.push({
    name: 'This period',
    type: 'bar',
    data: cur.map((p) => [p.t, p.v]),
    itemStyle: { color: c },
    barCategoryGap: '35%',
    z: 2,
    markLine:
      goal && goal > 0
        ? { silent: true, symbol: 'none', data: [{ yAxis: goal, label: { formatter: 'goal', position: 'insideEndTop' } }], lineStyle: { color: t.muted, type: 'dashed' } }
        : undefined,
  })
  return {
    grid: baseGrid,
    tooltip: tip(unit),
    xAxis: { ...timeAxis(bucket, range), boundaryGap: ['3%', '3%'] },
    yAxis: valueAxis(unit, 0),
    series,
  }
}

/** Line with a min–max band (sample metrics). */
export function bandLine(t: Tok, pts: SeriesPoint[], unit: string, bucket: 'day' | 'week' | 'month', color?: string, refLines?: { y: number; label: string }[], range?: Range): EChartsOption {
  const c = color ?? t.accent
  const hasBand = pts.some((p) => p.min !== undefined && p.max !== undefined)
  const series: Record<string, unknown>[] = []
  if (hasBand) {
    series.push(
      { name: 'min', type: 'line', data: pts.map((p) => [p.t, p.min]), lineStyle: { opacity: 0 }, stack: 'band', symbol: 'none', silent: true, tooltip: { show: false } },
      {
        name: 'range',
        type: 'line',
        data: pts.map((p) => [p.t, (p.max ?? 0) - (p.min ?? 0)]),
        lineStyle: { opacity: 0 },
        areaStyle: { color: c, opacity: 0.14 },
        stack: 'band',
        symbol: 'none',
        silent: true,
        tooltip: { show: false },
      },
    )
  }
  series.push({
    name: 'avg',
    type: 'line',
    data: pts.map((p) => [p.t, p.v]),
    itemStyle: { color: c },
    lineStyle: { color: c, width: 2 },
    symbol: pts.length <= 60 ? 'circle' : 'none',
    symbolSize: pts.length <= 3 ? 9 : 5,
    connectNulls: false,
    markLine: refLines?.length
      ? { silent: true, symbol: 'none', data: refLines.map((r) => ({ yAxis: r.y, label: { formatter: r.label, position: 'insideEndTop' } })), lineStyle: { color: t.faint, type: 'dashed' } }
      : undefined,
  })
  return {
    grid: baseGrid,
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const arr = params as { axisValue: number; seriesName: string; data: [string, number] }[]
        const avg = arr.find((p) => p.seriesName === 'avg')
        if (!avg) return ''
        const pt = pts.find((p) => p.t === avg.data[0])
        let s = `<b>${fmtDate(avg.data[0])}</b><br/>avg ${fmtValue(avg.data[1], unit)}`
        if (pt?.min !== undefined && pt.max !== undefined) s += `<br/><span style="color:${t.muted}">min ${fmtValue(pt.min, unit)} · max ${fmtValue(pt.max, unit)} · ${pt.n} samples</span>`
        return s
      },
    },
    xAxis: timeAxis(bucket, range),
    yAxis: valueAxis(unit, 'dataMin', 'dataMax'),
    series,
  }
}

/** Multiple lines on a shared time axis. */
export function multiLine(t: Tok, lines: { name: string; pts: SeriesPoint[]; color?: string; dashed?: boolean }[], unit: string, bucket: 'day' | 'week' | 'month', range?: Range): EChartsOption {
  return {
    grid: { ...baseGrid, top: 34 },
    legend: { top: 0, right: 0, icon: 'roundRect', itemWidth: 10, itemHeight: 10 },
    tooltip: tip(unit),
    xAxis: timeAxis(bucket, range),
    yAxis: valueAxis(unit, 'dataMin', 'dataMax'),
    series: lines.map((l, i) => ({
      name: l.name,
      type: 'line',
      data: l.pts.map((p) => [p.t, p.v]),
      itemStyle: { color: l.color ?? t.palette[i % t.palette.length] },
      lineStyle: { width: 2, type: l.dashed ? 'dashed' : 'solid' },
      symbol: l.pts.length <= 3 ? 'circle' : 'none',
      symbolSize: 8,
      connectNulls: false,
    })),
  }
}

/** Tiny sparkline for KPI tiles. */
export function sparkline(t: Tok, pts: SeriesPoint[], color?: string): EChartsOption {
  const c = color ?? t.accent
  return {
    grid: { left: 0, right: 0, top: 2, bottom: 2 },
    xAxis: { type: 'time', show: false },
    yAxis: { type: 'value', show: false, scale: true },
    tooltip: { show: false },
    animation: false,
    series: [
      {
        type: 'line',
        data: pts.map((p) => [p.t, p.v]),
        symbol: 'none',
        lineStyle: { color: c, width: 1.5 },
        areaStyle: { color: c, opacity: 0.12 },
      },
    ],
  }
}

/** Stacked sleep stages per night. Missing nights are simply absent. */
export function sleepStages(t: Tok, nights: SleepNight[]): EChartsOption {
  const cats = nights.map((n) => n.night)
  const stage = (name: string, key: keyof SleepNight['stages'], color: string) => ({
    name,
    type: 'bar',
    stack: 'sleep',
    data: nights.map((n) => n.stages[key] || null),
    itemStyle: { color, borderRadius: 0 },
    barCategoryGap: '30%',
    emphasis: { focus: 'series' },
  })
  return {
    grid: { ...baseGrid, top: 34 },
    legend: { top: 0, right: 0, icon: 'roundRect', itemWidth: 10, itemHeight: 10 },
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const arr = params as { axisValue: string; seriesName: string; data: number | null; color: string }[]
        const n = nights.find((x) => x.night === arr[0]?.axisValue)
        if (!n) return ''
        const rows = arr
          .filter((p) => p.data)
          .map((p) => `<span style="color:${p.color}">■</span> ${p.seriesName}: ${fmtNum(p.data as number, 1)} h`)
          .join('<br/>')
        return `<b>Night of ${fmtDate(n.night)}</b><br/>${fmtNum(n.hours, 1)} h asleep${n.naps ? ` · naps ${fmtNum(n.naps, 1)} h` : ''}<br/>${n.onset ? `${n.onset.slice(11)} → ${n.wake.slice(11)}<br/>` : ''}${rows}`
      },
    },
    xAxis: { type: 'category', data: cats, axisLabel: { hideOverlap: true, formatter: (v: string) => fmtDate(v, 'd MMM') } },
    yAxis: { type: 'value', name: 'h', min: 0, axisLabel: { formatter: (v: number) => `${v}` } },
    series: [
      stage('Deep', 'deep', t.sleep.deep),
      stage('Core', 'core', t.sleep.core),
      stage('REM', 'rem', t.sleep.rem),
      stage('Asleep (unspecified)', 'unspecified', t.faint),
      stage('Awake', 'awake', t.sleep.awake),
    ],
  }
}

/** Bedtime → wake bars on a clock axis (18:00 → 12:00 next day). */
export function onsetWake(t: Tok, nights: SleepNight[]): EChartsOption {
  const withData = nights.filter((n) => n.onset && n.wake)
  const toHours = (s: string, night: string) => {
    // hours since 18:00 on the night date
    const d = new Date(s.replace(' ', 'T'))
    const base = new Date(`${night}T18:00:00`)
    return (d.getTime() - base.getTime()) / 3600000
  }
  const data = withData.map((n) => {
    const a = toHours(n.onset, n.night)
    const b = toHours(n.wake, n.night)
    return { value: [n.night, a, b], n }
  })
  const label = (h: number) => {
    const hh = (18 + h) % 24
    return `${String(Math.floor(hh)).padStart(2, '0')}:00`
  }
  return {
    grid: baseGrid,
    tooltip: {
      trigger: 'item',
      formatter: (p: unknown) => {
        const d = (p as { data: { n: SleepNight } }).data.n
        return `<b>Night of ${fmtDate(d.night)}</b><br/>${d.onset.slice(11)} → ${d.wake.slice(11)}<br/>${fmtNum(d.hours, 1)} h asleep`
      },
    },
    xAxis: { type: 'category', data: withData.map((n) => n.night), axisLabel: { hideOverlap: true, formatter: (v: string) => fmtDate(v, 'd MMM') } },
    yAxis: { type: 'value', min: 0, max: 18, interval: 3, inverse: true, axisLabel: { formatter: (v: number) => label(v) } },
    series: [
      {
        type: 'custom',
        data,
        renderItem: (_params: unknown, api: { value: (i: number) => number; coord: (v: [number, number]) => [number, number]; size: (v: [number, number]) => [number, number] }) => {
          const x = api.value(0)
          const start = api.coord([x, api.value(1)])
          const end = api.coord([x, api.value(2)])
          const w = api.size([1, 0])[0] * 0.55
          return {
            type: 'rect',
            shape: { x: start[0] - w / 2, y: Math.min(start[1], end[1]), width: w, height: Math.abs(end[1] - start[1]) },
            style: { fill: t.sleep.core, opacity: 0.9 },
          }
        },
      },
    ],
  }
}

/** Calendar heatmap of a daily series (rings closed count, steps, …). */
export function calendarHeatmap(t: Tok, pts: { t: string; v: number }[], from: string, to: string, unit: string, maxOverride?: number): EChartsOption {
  const max = maxOverride ?? Math.max(1, ...pts.map((p) => p.v))
  return {
    tooltip: { formatter: (p: unknown) => `${fmtDate((p as { value: [string, number] }).value[0])}<br/>${fmtValue((p as { value: [string, number] }).value[1], unit)}` },
    visualMap: { min: 0, max, show: false, inRange: { color: [t.grid, t.accent] } },
    calendar: {
      range: [from, to],
      cellSize: ['auto', 14],
      left: 40,
      right: 10,
      top: 24,
      itemStyle: { color: 'transparent', borderColor: t.surface, borderWidth: 2 },
      splitLine: { show: false },
      dayLabel: { firstDay: 1, nameMap: ['S', 'M', 'T', 'W', 'T', 'F', 'S'], color: t.faint },
      monthLabel: { color: t.muted },
      yearLabel: { show: false },
    },
    series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: pts.map((p) => [p.t, p.v]) }],
  }
}

/** Horizontal bars for HR zones. */
export function zonesBar(t: Tok, zones: { label: string; minutes: number }[]): EChartsOption {
  const colors = [t.palette[7], t.palette[2], t.palette[5], t.palette[6], t.palette[1]]
  return {
    grid: { left: 90, right: 40, top: 8, bottom: 8 },
    xAxis: { type: 'value', show: false },
    yAxis: { type: 'category', data: zones.map((z) => z.label), inverse: true, axisLine: { show: false } },
    tooltip: { trigger: 'item', valueFormatter: (v: unknown) => `${fmtNum(v as number, 1)} min` },
    series: [
      {
        type: 'bar',
        data: zones.map((z, i) => ({ value: z.minutes, itemStyle: { color: colors[i % colors.length], borderRadius: 3 } })),
        label: { show: true, position: 'right', formatter: (p: { value: number }) => `${fmtNum(p.value, 0)} min` },
        barCategoryGap: '30%',
      },
    ],
  }
}

/** Heart-rate over a workout with zone bands and pause shading. */
export function workoutHR(t: Tok, samples: SeriesPoint[], zones: { min?: number; max?: number }[], pauses: [string, string][]): EChartsOption {
  const zoneColors = [t.palette[7], t.palette[2], t.palette[5], t.palette[6], t.palette[1]]
  const areas = zones.map((z, i) => [{ yAxis: z.min ?? 0, itemStyle: { color: zoneColors[i % zoneColors.length], opacity: 0.08 } }, { yAxis: z.max ?? 250 }])
  const pauseAreas = pauses.map(([a, b]) => [{ xAxis: a, itemStyle: { color: t.faint, opacity: 0.18 } }, { xAxis: b }])
  return {
    grid: baseGrid,
    tooltip: { trigger: 'axis', valueFormatter: (v: unknown) => `${fmtNum(v as number, 0)} bpm` },
    xAxis: { type: 'time', axisLabel: { formatter: (v: number) => fmtDate(new Date(v).toISOString(), 'HH:mm') } },
    yAxis: valueAxis('count/min', 'dataMin', 'dataMax'),
    series: [
      {
        name: 'Heart rate',
        type: 'line',
        data: samples.map((p) => [p.t.replace(' ', 'T'), p.v]),
        symbol: 'none',
        lineStyle: { color: t.palette[1], width: 1.6 },
        sampling: 'lttb',
        markArea: { silent: true, data: [...areas, ...pauseAreas] },
      },
    ],
  }
}

/** Elevation + speed profile along a route by point index. */
export function routeProfile(t: Tok, pts: { ele: number | null; speed: number | null; dist: number }[]): EChartsOption {
  return {
    grid: { left: 12, right: 12, top: 28, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const arr = params as { axisValue: number; seriesName: string; data: [number, number]; color: string }[]
        return `${fmtNum(arr[0].axisValue / 1000, 2)} km<br/>` + arr.map((p) => `<span style="color:${p.color}">■</span> ${p.seriesName}: ${p.seriesName === 'Speed' ? fmtNum(p.data[1], 1) + ' km/h' : fmtNum(p.data[1], 0) + ' m'}`).join('<br/>')
      },
    },
    xAxis: { type: 'value', axisLabel: { formatter: (v: number) => `${fmtNum(v / 1000, 1)} km` } },
    yAxis: [
      { type: 'value', name: 'm', scale: true, axisLabel: { formatter: (v: number) => fmtNum(v, 0) } },
      { type: 'value', name: 'km/h', scale: true, splitLine: { show: false }, axisLabel: { formatter: (v: number) => fmtNum(v, 0) } },
    ],
    series: [
      { name: 'Elevation', type: 'line', data: pts.filter((p) => p.ele !== null).map((p) => [p.dist, p.ele]), areaStyle: { opacity: 0.15 }, symbol: 'none', lineStyle: { color: t.palette[5] }, itemStyle: { color: t.palette[5] } },
      { name: 'Speed', type: 'line', yAxisIndex: 1, data: pts.filter((p) => p.speed !== null).map((p) => [p.dist, (p.speed ?? 0) * 3.6]), symbol: 'none', lineStyle: { color: t.palette[0], width: 1.2 }, itemStyle: { color: t.palette[0] } },
    ],
  }
}

/** ECG trace on a paper-like grid: 200 ms / 0.5 mV. Optional R-peak markers (seconds, mV). */
export function ecgTrace(t: Tok, samples: number[], rate: number, zoom = true, rPeaks?: { s: number; mv: number }[]): EChartsOption {
  const data = samples.map((v, i) => [i / rate, v / 1000]) // seconds, mV
  const dur = samples.length / rate
  const markPoint = rPeaks?.length
    ? {
        symbol: 'circle',
        symbolSize: 7,
        itemStyle: { color: t.accent, borderColor: t.surface, borderWidth: 1.5 },
        label: { show: false },
        data: rPeaks.map((p) => ({ coord: [p.s, p.mv] })),
        silent: true,
      }
    : undefined
  return {
    grid: { left: 64, right: 16, top: 10, bottom: zoom ? 60 : 20 },
    tooltip: { trigger: 'axis', formatter: (p: unknown) => `${fmtNum((p as { data: [number, number] }[])[0].data[0], 3)} s · ${fmtNum((p as { data: [number, number] }[])[0].data[1], 3)} mV` },
    xAxis: {
      type: 'value',
      min: 0,
      max: dur,
      interval: 1,
      axisLabel: { formatter: (v: number) => `${v.toFixed(0)} s`, color: t.faint },
      splitLine: { show: true, lineStyle: { color: t.grid } },
      minorTick: { show: true, splitNumber: 5 },
      minorSplitLine: { show: true, lineStyle: { color: t.grid, opacity: 0.35 } },
    },
    yAxis: { type: 'value', min: -1.5, max: 1.5, interval: 0.5, axisLabel: { formatter: (v: number) => `${v.toFixed(1)} mV`, color: t.faint }, splitLine: { show: true, lineStyle: { color: t.grid } } },
    dataZoom: zoom ? [{ type: 'inside', filterMode: 'none' }, { type: 'slider', height: 22, bottom: 8, filterMode: 'none', borderColor: t.border, fillerColor: t.accent + '22' }] : undefined,
    animation: false,
    series: [{ type: 'line', data, symbol: 'none', lineStyle: { color: t.bad, width: 1.2 }, sampling: 'none', large: true, markPoint }],
  }
}

/** RR intervals beat by beat: the quickest way to see rhythm regularity. */
export function rrTachogram(t: Tok, rr: number[], premature: boolean): EChartsOption {
  const med = [...rr].sort((a, b) => a - b)[Math.floor(rr.length / 2)] ?? 0
  return {
    grid: { left: 12, right: 16, top: 28, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', formatter: (p: unknown) => { const q = (p as { dataIndex: number; data: number }[])[0]; return `beat ${q.dataIndex + 1}<br/>RR ${fmtNum(q.data, 0)} ms · ${fmtNum(60000 / q.data, 0)} bpm` } },
    xAxis: { type: 'category', data: rr.map((_, i) => i + 1), name: 'beat', axisLabel: { color: t.faint } },
    yAxis: { type: 'value', name: 'ms', scale: true, axisLabel: { formatter: (v: number) => fmtNum(v, 0) } },
    series: [
      {
        type: 'line',
        data: rr,
        symbol: 'circle',
        symbolSize: 6,
        lineStyle: { color: t.palette[1], width: 1.5 },
        itemStyle: { color: (p: { data: number }) => (premature && p.data < 0.8 * med ? t.warn : t.palette[1]) },
        markLine: { silent: true, symbol: 'none', lineStyle: { color: t.faint, type: 'dashed' }, data: [{ yAxis: med, label: { formatter: 'median', position: 'insideEndTop' } }] },
      },
    ],
  }
}

/** Averaged beat with the regions where P, QRS and T normally sit. */
export function beatTemplate(t: Tok, tpl: number[], t0: number, dt: number): EChartsOption {
  const data = tpl.map((v, i) => [t0 + i * dt, v])
  const area = (from: number, to: number, label: string, color: string) => [{ xAxis: from, name: label, itemStyle: { color, opacity: 0.1 }, label: { show: true, position: 'insideTop', color: t.muted, fontSize: 11 } }, { xAxis: to }]
  return {
    grid: { left: 12, right: 16, top: 28, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', formatter: (p: unknown) => { const q = (p as { data: [number, number] }[])[0]; return `${fmtNum(q.data[0], 0)} ms · ${fmtNum(q.data[1], 3)} mV` } },
    xAxis: { type: 'value', min: Math.floor(t0 / 50) * 50, max: Math.ceil((t0 + tpl.length * dt) / 50) * 50, interval: 100, name: 'ms from R', axisLabel: { formatter: (v: number) => `${v}` , color: t.faint }, splitLine: { show: true, lineStyle: { color: t.grid } } },
    yAxis: { type: 'value', name: 'mV', scale: true, axisLabel: { formatter: (v: number) => fmtNum(v, 1) } },
    series: [
      {
        type: 'line',
        data,
        symbol: 'none',
        lineStyle: { color: t.bad, width: 2 },
        markArea: { silent: true, data: [area(-240, -80, 'P wave', t.palette[0]), area(-60, 60, 'QRS', t.palette[1]), area(120, 420, 'T wave', t.palette[5])] },
        markLine: { silent: true, symbol: 'none', lineStyle: { color: t.faint }, data: [{ xAxis: 0, label: { formatter: 'R', position: 'insideStartBottom' } }] },
      },
    ],
  }
}

export function ecgThumb(t: Tok, env: [number, number][]): EChartsOption {
  return {
    grid: { left: 0, right: 0, top: 4, bottom: 4 },
    xAxis: { type: 'category', show: false, data: env.map((_, i) => i) },
    yAxis: { type: 'value', show: false, min: -1500, max: 1500 },
    animation: false,
    series: [
      { type: 'line', data: env.map((e) => e[0]), symbol: 'none', lineStyle: { opacity: 0 }, stack: 'e', silent: true },
      { type: 'line', data: env.map((e) => e[1] - e[0]), symbol: 'none', lineStyle: { opacity: 0 }, areaStyle: { color: t.bad, opacity: 0.8 }, stack: 'e', silent: true },
    ],
  }
}

/** Weekday means (Mon..Sun). */
export function weekdayBars(t: Tok, cur: number[], prev?: number[], unit = 'count'): EChartsOption {
  const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
  const series: Record<string, unknown>[] = []
  if (prev) series.push({ name: 'Previous', type: 'bar', data: prev, itemStyle: { color: t.border }, barGap: '-100%', z: 1 })
  series.push({ name: 'This period', type: 'bar', data: cur, itemStyle: { color: t.accent }, z: 2 })
  return {
    grid: baseGrid,
    tooltip: tip(unit),
    xAxis: { type: 'category', data: days },
    yAxis: valueAxis(unit, 0),
    series,
  }
}

/** Donut. */
export function donut(t: Tok, parts: { name: string; value: number; color?: string }[], unit = ''): EChartsOption {
  return {
    tooltip: { trigger: 'item', valueFormatter: (v: unknown) => fmtValue(v as number, unit) },
    legend: { bottom: 0, icon: 'circle', itemWidth: 8, itemHeight: 8, textStyle: { color: t.muted } },
    series: [
      {
        type: 'pie',
        radius: ['52%', '78%'],
        center: ['50%', '42%'],
        data: parts.map((p, i) => ({ ...p, itemStyle: { color: p.color ?? t.palette[i] } })),
        label: { show: false },
        itemStyle: { borderColor: t.surface, borderWidth: 2 },
      },
    ],
  }
}

/** Scatter of blood pressure readings. */
export function bloodPressure(t: Tok, rows: { start_date: string; systolic: number; diastolic: number }[]): EChartsOption {
  return {
    grid: { ...baseGrid, top: 34 },
    legend: { top: 0, right: 0 },
    tooltip: { trigger: 'axis', valueFormatter: (v: unknown) => `${fmtNum(v as number, 0)} mmHg` },
    xAxis: timeAxis('day'),
    yAxis: { ...valueAxis('mmHg', 40, 200) },
    series: [
      { name: 'Systolic', type: 'line', data: rows.map((r) => [r.start_date.replace(' ', 'T'), r.systolic]), symbol: 'circle', symbolSize: 7, lineStyle: { width: 1, color: t.bad }, itemStyle: { color: t.bad }, markLine: { silent: true, symbol: 'none', lineStyle: { type: 'dashed', color: t.faint }, data: [{ yAxis: 120, label: { formatter: '120' } }] } },
      { name: 'Diastolic', type: 'line', data: rows.map((r) => [r.start_date.replace(' ', 'T'), r.diastolic]), symbol: 'circle', symbolSize: 7, lineStyle: { width: 1, color: t.accent }, itemStyle: { color: t.accent }, markLine: { silent: true, symbol: 'none', lineStyle: { type: 'dashed', color: t.faint }, data: [{ yAxis: 80, label: { formatter: '80' } }] } },
    ],
  }
}

/** Per-type weekly workout volume, stacked. */
export function stackedByType(t: Tok, weeks: string[], types: string[], data: Record<string, number[]>, unit = 'min'): EChartsOption {
  return {
    grid: { ...baseGrid, top: 34 },
    legend: { top: 0, right: 0, type: 'scroll', icon: 'roundRect', itemWidth: 10, itemHeight: 10 },
    tooltip: tip(unit),
    xAxis: { type: 'category', data: weeks, axisLabel: { hideOverlap: true, formatter: (v: string) => fmtDate(v, 'd MMM') } },
    yAxis: valueAxis(unit, 0),
    series: types.map((ty, i) => ({ name: ty, type: 'bar', stack: 'w', data: data[ty], itemStyle: { color: t.palette[i % t.palette.length], borderRadius: 0 } })),
  }
}

/** Simple line of arbitrary points with a beat-to-beat feel (HRV drill-down). */
export function beatsLine(t: Tok, beats: { seq: number; bpm: number }[]): EChartsOption {
  return {
    grid: baseGrid,
    tooltip: { trigger: 'axis', valueFormatter: (v: unknown) => `${fmtNum(v as number, 0)} bpm` },
    xAxis: { type: 'category', data: beats.map((b) => b.seq), axisLabel: { show: false } },
    yAxis: valueAxis('count/min', 'dataMin', 'dataMax'),
    series: [{ type: 'line', data: beats.map((b) => b.bpm), symbol: 'circle', symbolSize: 4, lineStyle: { color: t.palette[1] }, itemStyle: { color: t.palette[1] } }],
  }
}
