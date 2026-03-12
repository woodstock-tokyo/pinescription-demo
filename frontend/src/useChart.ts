import { useEffect, useRef, useCallback } from 'react'
import {
  createChart,
  IChartApi,
  ISeriesApi,
  ColorType,
  CrosshairMode,
  UTCTimestamp,
  CandlestickData,
  LineData,
} from 'lightweight-charts'
import { Bar, PlotPoint } from './types'

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
  loadIndicator:       (indId: string, plots: Record<string, PlotPoint[]>) => void
  updateIndicatorTick: (indId: string, values: Record<string, number>, time: number) => void
  removeIndicator:     (indId: string) => void
}

export function useChart(containerRef: React.RefObject<HTMLDivElement>): ChartHandle {
  const chartRef    = useRef<IChartApi | null>(null)
  const candleRef   = useRef<ISeriesApi<'Candlestick'> | null>(null)
  // indId → plotName → LineSeries
  const lineMap     = useRef<Record<string, Record<string, ISeriesApi<'Line'>>>>({})

  // ── init chart ──────────────────────────────────────────────────────────────
  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    const chart = createChart(el, {
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
    })

    const candles = chart.addCandlestickSeries({
      upColor:        '#26a69a',
      downColor:      '#ef5350',
      borderUpColor:  '#26a69a',
      borderDownColor:'#ef5350',
      wickUpColor:    '#26a69a',
      wickDownColor:  '#ef5350',
    })

    chartRef.current  = chart
    candleRef.current = candles

    const ro = new ResizeObserver(() => {
      chart.applyOptions({ width: el.clientWidth, height: el.clientHeight })
    })
    ro.observe(el)

    return () => {
      ro.disconnect()
      chart.remove()
    }
  }, [containerRef])

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
    s.setData(data)
    chartRef.current?.timeScale().fitContent()
  }, [])

  const addBar = useCallback((bar: Bar) => {
    candleRef.current?.update({
      time:  bar.time as UTCTimestamp,
      open:  bar.open,
      high:  bar.high,
      low:   bar.low,
      close: bar.close,
    })
  }, [])

  const loadIndicator = useCallback((indId: string, plots: Record<string, PlotPoint[]>) => {
    const chart = chartRef.current
    if (!chart) return

    // Remove any existing series for this indicator.
    const existing = lineMap.current[indId] || {}
    Object.values(existing).forEach(s => chart.removeSeries(s))
    lineMap.current[indId] = {}

    Object.entries(plots).forEach(([plotName, pts]) => {
      const colour = assignColour(indId, plotName)
      const ls = chart.addLineSeries({
        color:             colour,
        lineWidth:         2,
        priceLineVisible:  false,
        lastValueVisible:  true,
        title:             plotName,
      })
      const data: LineData[] = pts
        .filter(p => p.value !== 0 && isFinite(p.value))
        .map(p => ({ time: p.time as UTCTimestamp, value: p.value }))
      ls.setData(data)
      lineMap.current[indId][plotName] = ls
    })
  }, [])

  const updateIndicatorTick = useCallback((indId: string, values: Record<string, number>, time: number) => {
    const seriesMap = lineMap.current[indId]
    if (!seriesMap) return
    Object.entries(values).forEach(([plotName, value]) => {
      const ls = seriesMap[plotName]
      if (ls && value !== 0 && isFinite(value)) {
        ls.update({ time: time as UTCTimestamp, value })
      }
    })
  }, [])

  const removeIndicator = useCallback((indId: string) => {
    const chart = chartRef.current
    if (!chart) return
    const existing = lineMap.current[indId] || {}
    Object.values(existing).forEach(s => chart.removeSeries(s))
    delete lineMap.current[indId]
    clearColours(indId)
  }, [])

  return { loadHistory, addBar, loadIndicator, updateIndicatorTick, removeIndicator }
}
