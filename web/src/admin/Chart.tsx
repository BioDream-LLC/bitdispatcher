import { useEffect, useRef } from 'react'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

export type ChartOption = echarts.EChartsCoreOption

/** An ECharts chart that resizes with its box. onClick gets the clicked category index. */
export default function Chart({ option, height = 180, onClick }: { option: ChartOption; height?: number; onClick?: (index: number, series: number) => void }) {
  const el = useRef<HTMLDivElement>(null)
  const chart = useRef<echarts.ECharts | null>(null)
  const click = useRef(onClick)
  click.current = onClick
  useEffect(() => {
    if (!el.current) return
    const c = echarts.init(el.current, undefined, { renderer: 'canvas' })
    chart.current = c
    c.on('click', (p) => click.current?.(p.dataIndex ?? 0, p.seriesIndex ?? 0))
    const ro = new ResizeObserver(() => c.resize())
    ro.observe(el.current)
    return () => {
      ro.disconnect()
      c.dispose()
      chart.current = null
    }
  }, [])
  useEffect(() => {
    chart.current?.setOption(option, true)
  }, [option])
  return <div ref={el} style={{ height }} className="w-full" />
}

/** Shared look: dark, quiet axes, tabular numbers. */
export const base = {
  backgroundColor: 'transparent',
  textStyle: { color: '#94a3b8', fontFamily: 'inherit', fontSize: 10 },
  grid: { left: 36, right: 12, top: 18, bottom: 22 },
  tooltip: { trigger: 'axis', backgroundColor: '#0f172a', borderColor: '#334155', textStyle: { color: '#e2e8f0', fontSize: 11 } },
}
