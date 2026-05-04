import { useEffect, useRef, useCallback } from 'react'
import {
  createChart,
  ColorType,
  CrosshairMode,
} from 'lightweight-charts'
import type {
  IChartApi,
  ISeriesApi,
  UTCTimestamp,
  CandlestickData,
  LineData,
  LogicalRange,
  WhitespaceData,
} from 'lightweight-charts'
import type { Bar, IndicatorPane, PlotPoint } from './types'

// ── Colour palette ────────────────────────────────────────────────────────────
const PALETTE = [
  '#2962ff', '#ff6d00', '#00bcd4', '#e040fb',
  '#ffeb3b', '#69f0ae', '#ff4081', '#40c4ff',
  '#b2ff59', '#ff6e40', '#ea80fc', '#18ffff',
]

// indicatorId → plotName → colour
const colourMap: Record<string, Record<string, string>> = {}
let colourIdx = 0

export function getColourMap() { return colourMap }

function assignColour(indId: string, plotName: string): string {
  if (!colourMap[indId]) colourMap[indId] = {}
  if (!colourMap[indId][plotName]) {
    colourMap[indId][plotName] = PALETTE[colourIdx++ % PALETTE.length]
  }
  return colourMap[indId][plotName]
}

export function clearColours(indId: string) {
  delete colourMap[indId]
}

// ── Hook ──────────────────────────────────────────────────────────────────────

interface ChartHandle {
  loadHistory:         (bars: Bar[]) => void
  addBar:              (bar: Bar) => void
  loadIndicator:       (indId: string, plots: Record<string, PlotPoint[]>, pane: IndicatorPane) => void
  updateIndicatorTick: (indId: string, values: Record<string, number>, time: number) => void
  setIndicatorVisible: (indId: string, visible: boolean) => void
  removeIndicator:     (indId: string) => void
}

type LineSeries = ISeriesApi<'Line'>
type LineSeriesData = LineData | WhitespaceData

interface IndicatorEntry {
  pane: IndicatorPane
  chart: IChartApi
  el?: HTMLDivElement
  series: Record<string, LineSeries>
  visible: boolean
  unsubscribeSync?: () => void
}

function chartOptions() {
  return {
    layout: {
      background: { type: ColorType.Solid, color: '#0b0e17' },
      textColor: '#5d6b8a',
      fontFamily: "'Inter', sans-serif",
      fontSize: 11,
    },
    grid: {
      vertLines: { color: '#1c2133' },
      horzLines: { color: '#1c2133' },
    },
    crosshair: { mode: CrosshairMode.Normal },
    rightPriceScale: { borderColor: '#2a3350' },
    timeScale: {
      borderColor: '#2a3350',
      timeVisible: true,
      secondsVisible: false,
    },
  }
}

function lineData(points: PlotPoint[], timeline?: UTCTimestamp[]): LineSeriesData[] {
  const validPoints = points.filter(p => isFinite(p.value))
  if (!timeline) return validPoints.map(p => ({ time: p.time as UTCTimestamp, value: p.value }))
  if (validPoints.length === 0) return timeline.map(time => ({ time }))

  const firstValueTime = validPoints[0].time
  const timelineSet = new Set<number>(timeline)
  const leftPadding = timeline
    .filter(time => time < firstValueTime)
    .map(time => ({ time }))
  const values = validPoints
    .filter(p => timelineSet.has(p.time))
    .map(p => ({ time: p.time as UTCTimestamp, value: p.value }))

  return [...leftPadding, ...values]
}

export function useChart(containerRef: React.RefObject<HTMLDivElement>): ChartHandle {
  const priceChartRef = useRef<IChartApi | null>(null)
  const candleRef     = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const indicatorsRef = useRef<Record<string, IndicatorEntry>>({})
  const timelineRef   = useRef<UTCTimestamp[]>([])
  const syncingRef    = useRef(false)
  const resizeRef     = useRef<() => void>(() => {})

  const syncCharts = useCallback((source: IChartApi, range: LogicalRange | null) => {
    if (!range || syncingRef.current) return
    syncingRef.current = true
    try {
      const charts = [priceChartRef.current, ...Object.values(indicatorsRef.current).map(entry => entry.chart)]
      charts.forEach(chart => {
        if (chart && chart !== source) chart.timeScale().setVisibleLogicalRange(range)
      })
    } finally {
      syncingRef.current = false
    }
  }, [])

  const subscribeSync = useCallback((chart: IChartApi) => {
    const handler = (range: LogicalRange | null) => syncCharts(chart, range)
    chart.timeScale().subscribeVisibleLogicalRangeChange(handler)
    return () => chart.timeScale().unsubscribeVisibleLogicalRangeChange(handler)
  }, [syncCharts])

  const removeIndicatorEntry = useCallback((indId: string) => {
    const entry = indicatorsRef.current[indId]
    if (!entry) return
    Object.values(entry.series).forEach(series => entry.chart.removeSeries(series))
    entry.unsubscribeSync?.()
    if (entry.pane === 'separate') {
      entry.chart.remove()
      entry.el?.remove()
    }
    delete indicatorsRef.current[indId]
    clearColours(indId)
    resizeRef.current()
  }, [])

  // ── init chart ──────────────────────────────────────────────────────────────
  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    el.classList.add('chart-stack')
    const pricePane = document.createElement('div')
    pricePane.className = 'chart-pane chart-pane-price'
    el.appendChild(pricePane)

    const chart = createChart(pricePane, chartOptions())
    const candles = chart.addCandlestickSeries({
      upColor:        '#26a69a',
      downColor:      '#ef5350',
      borderUpColor:  '#26a69a',
      borderDownColor:'#ef5350',
      wickUpColor:    '#26a69a',
      wickDownColor:  '#ef5350',
    })

    priceChartRef.current = chart
    candleRef.current     = candles

    const unsubscribePriceSync = subscribeSync(chart)

    const resizeCharts = () => {
      const resize = (targetChart: IChartApi, targetEl: HTMLElement) => {
        targetChart.applyOptions({
          width:  Math.max(1, targetEl.clientWidth),
          height: Math.max(1, targetEl.clientHeight),
        })
      }
      resize(chart, pricePane)
      Object.values(indicatorsRef.current).forEach(entry => {
        if (entry.el) resize(entry.chart, entry.el)
      })
    }
    resizeRef.current = resizeCharts

    const ro = new ResizeObserver(resizeCharts)
    ro.observe(el)
    resizeCharts()

    return () => {
      ro.disconnect()
      unsubscribePriceSync()
      Object.keys(indicatorsRef.current).forEach(indId => removeIndicatorEntry(indId))
      chart.remove()
      pricePane.remove()
      priceChartRef.current = null
      candleRef.current = null
    }
  }, [containerRef, removeIndicatorEntry, subscribeSync])

  // ── handlers ─────────────────────────────────────────────────────────────────

  const loadHistory = useCallback((bars: Bar[]) => {
    const s = candleRef.current
    if (!s) return
    const data: CandlestickData[] = bars.map(b => ({
      time:  b.time as UTCTimestamp,
      open:  b.open,
      high:  b.high,
      low:   b.low,
      close: b.close,
    }))
    timelineRef.current = data.map(d => d.time as UTCTimestamp)
    s.setData(data)
    priceChartRef.current?.timeScale().fitContent()
  }, [])

  const addBar = useCallback((bar: Bar) => {
    const time = bar.time as UTCTimestamp
    const timeline = timelineRef.current
    if (timeline[timeline.length - 1] !== time) timelineRef.current = [...timeline, time]
    candleRef.current?.update({
      time,
      open:  bar.open,
      high:  bar.high,
      low:   bar.low,
      close: bar.close,
    })
  }, [])

  const loadIndicator = useCallback((indId: string, plots: Record<string, PlotPoint[]>, pane: IndicatorPane) => {
    const priceChart = priceChartRef.current
    const container = containerRef.current
    if (!priceChart || !container) return

    removeIndicatorEntry(indId)

    const entry: IndicatorEntry = { pane, chart: priceChart, series: {}, visible: true }

    if (pane === 'separate') {
      const subPane = document.createElement('div')
      subPane.className = 'chart-pane chart-pane-indicator'
      container.appendChild(subPane)

      entry.chart = createChart(subPane, chartOptions())
      entry.el = subPane
      entry.unsubscribeSync = subscribeSync(entry.chart)

      const range = priceChart.timeScale().getVisibleLogicalRange()
      if (range) entry.chart.timeScale().setVisibleLogicalRange(range)
    }

    Object.entries(plots).forEach(([plotName, pts]) => {
      const colour = assignColour(indId, plotName)
      const ls = entry.chart.addLineSeries({
        color:             colour,
        lineWidth:         2,
        priceLineVisible:  false,
        lastValueVisible:  true,
        title:             plotName,
      })
      ls.setData(lineData(pts, pane === 'separate' ? timelineRef.current : undefined))
      entry.series[plotName] = ls
    })

    indicatorsRef.current[indId] = entry
    resizeRef.current()
  }, [containerRef, removeIndicatorEntry, subscribeSync])

  const updateIndicatorTick = useCallback((indId: string, values: Record<string, number>, time: number) => {
    const entry = indicatorsRef.current[indId]
    if (!entry) return
    Object.entries(values).forEach(([plotName, value]) => {
      const ls = entry.series[plotName]
      if (ls && isFinite(value)) {
        ls.update({ time: time as UTCTimestamp, value })
      }
    })
  }, [])

  const setIndicatorVisible = useCallback((indId: string, visible: boolean) => {
    const entry = indicatorsRef.current[indId]
    if (!entry || entry.visible === visible) return
    entry.visible = visible
    Object.values(entry.series).forEach(series => {
      series.applyOptions({ visible })
    })
    if (entry.el) entry.el.classList.toggle('chart-pane-hidden', !visible)
    resizeRef.current()
  }, [])

  const removeIndicator = useCallback((indId: string) => {
    removeIndicatorEntry(indId)
  }, [removeIndicatorEntry])

  return { loadHistory, addBar, loadIndicator, updateIndicatorTick, setIndicatorVisible, removeIndicator }
}
