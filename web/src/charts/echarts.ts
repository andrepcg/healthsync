import * as echarts from 'echarts/core'
import { BarChart, CustomChart, HeatmapChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import {
  CalendarComponent,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  MarkAreaComponent,
  MarkLineComponent,
  MarkPointComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  BarChart,
  LineChart,
  ScatterChart,
  HeatmapChart,
  PieChart,
  CustomChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  MarkLineComponent,
  MarkAreaComponent,
  MarkPointComponent,
  CalendarComponent,
  VisualMapComponent,
  CanvasRenderer,
])

export { echarts }
export type { EChartsCoreOption as EChartsOption } from 'echarts/core'
