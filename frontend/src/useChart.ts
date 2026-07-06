import { useEffect, useRef, useCallback } from 'react'
import {
  createChart,
  ColorType,
  CrosshairMode,
  LineStyle,
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
import type { Bar, DrawingOutput, IndicatorPane, PlotPoint, PlotRenderOptions } from './types'

// ── Colour palette ────────────────────────────────────────────────────────────
const PALETTE = [
  '#2962ff', '#ff6d00', '#00bcd4', '#e040fb',
  '#ffeb3b', '#69f0ae', '#ff4081', '#40c4ff',
  '#b2ff59', '#ff6e40', '#ea80fc', '#18ffff',
]
const RIGHT_PRICE_SCALE_MIN_WIDTH = 88
const MAX_RENDER_POLYLINES = 160
const MAX_RENDER_BOXES = 160
const MAX_RENDER_LABELS = 120
const MAX_RENDER_POINTS_PER_LINE = 1200
const MAX_RENDER_DASHBOARD_ROWS = 20
const MAX_RENDER_TEXT_LENGTH = 96

// indicatorId → plotName → colour
const colourMap: Record<string, Record<string, string>> = {}
let colourIdx = 0

export function getColourMap() { return colourMap }

function assignColour(indId: string, plotName: string, preferred?: string): string {
  if (!colourMap[indId]) colourMap[indId] = {}
  if (!colourMap[indId][plotName]) {
    colourMap[indId][plotName] = preferred || PALETTE[colourIdx++ % PALETTE.length]
  }
  return colourMap[indId][plotName]
}

export function clearColours(indId: string) {
  delete colourMap[indId]
}

export function hasIndicatorColours(indId: string): boolean {
  return Boolean(colourMap[indId] && Object.keys(colourMap[indId]).length > 0)
}

// ── Hook ──────────────────────────────────────────────────────────────────────

interface ChartHandle {
  loadHistory:         (bars: Bar[]) => void
  addBar:              (bar: Bar) => void
  loadIndicator:       (indId: string, plots: Record<string, PlotPoint[]>, pane: IndicatorPane, plotOptions?: Record<string, PlotRenderOptions>, drawings?: DrawingOutput) => void
  updateIndicatorTick: (indId: string, values: Record<string, number>, time: number) => void
  setIndicatorVisible: (indId: string, visible: boolean) => void
  removeIndicator:     (indId: string) => void
}

type LineSeries = ISeriesApi<'Line'>
type LineSeriesData = LineData | WhitespaceData
type PlotSeriesEntry = { chart: IChartApi; series: LineSeries }

interface IndicatorEntry {
  pane: IndicatorPane
  chart: IChartApi
  el?: HTMLDivElement
  series: Record<string, PlotSeriesEntry>
  drawings?: DrawingOutput
  drawingBackGroup?: SVGGElement
  drawingFrontGroup?: SVGGElement
  visible: boolean
  unsubscribeSync?: () => void
}

function hasDrawingOutput(drawings?: DrawingOutput): boolean {
  return Boolean(
    drawings &&
    ((drawings.polylines?.length ?? 0) > 0 ||
      (drawings.boxes?.length ?? 0) > 0 ||
      (drawings.labels?.length ?? 0) > 0 ||
      drawings.dashboard)
  )
}

function svgEl<K extends keyof SVGElementTagNameMap>(tag: K): SVGElementTagNameMap[K] {
  return document.createElementNS('http://www.w3.org/2000/svg', tag)
}

function sizeDrawingSvg(svg: SVGSVGElement, width: number, height: number) {
  svg.setAttribute('width', String(width))
  svg.setAttribute('height', String(height))
  svg.setAttribute('viewBox', `0 0 ${width} ${height}`)
}

function clampText(value: string, maxLength = MAX_RENDER_TEXT_LENGTH): string {
  return value.length > maxLength ? `${value.slice(0, maxLength - 1)}…` : value
}

function dashboardMetrics(size?: string) {
  switch (size) {
    case 'tiny': return { width: 208, title: 10, row: 9, rowHeight: 19, padding: 10 }
    case 'small': return { width: 228, title: 11, row: 10, rowHeight: 21, padding: 11 }
    case 'large': return { width: 274, title: 14, row: 13, rowHeight: 27, padding: 14 }
    case 'huge': return { width: 304, title: 16, row: 14, rowHeight: 30, padding: 16 }
    default: return { width: 248, title: 12, row: 11, rowHeight: 24, padding: 12 }
  }
}

function dashboardOrigin(position: string | undefined, panelWidth: number, panelHeight: number, chartWidth: number, chartHeight: number) {
  const margin = 14
  const right = Math.max(margin, chartWidth - panelWidth - margin - RIGHT_PRICE_SCALE_MIN_WIDTH)
  const bottom = Math.max(margin, chartHeight - panelHeight - margin)
  switch (position) {
    case 'bottom_left': return { x: margin, y: bottom }
    case 'bottom_right': return { x: right, y: bottom }
    case 'top_left': return { x: margin, y: margin }
    default: return { x: right, y: margin }
  }
}

function chartOptions() {
  return {
    layout: {
      background: { type: ColorType.Solid, color: 'transparent' },
      textColor: '#5d6b8a',
      fontFamily: "'Inter', sans-serif",
      fontSize: 11,
    },
    grid: {
      vertLines: { color: '#1c2133' },
      horzLines: { color: '#1c2133' },
    },
    crosshair: { mode: CrosshairMode.Normal },
    rightPriceScale: {
      borderColor: '#2a3350',
      minimumWidth: RIGHT_PRICE_SCALE_MIN_WIDTH,
    },
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

function lineWidth(width?: number): 1 | 2 | 3 | 4 {
  if (!width || !isFinite(width)) return 2
  return Math.min(4, Math.max(1, Math.round(width))) as 1 | 2 | 3 | 4
}

function lineStyle(style?: number): LineStyle {
  if (style === 1) return LineStyle.Dashed
  if (style === 2) return LineStyle.Dotted
  return LineStyle.Solid
}

function priceFormat(options?: PlotRenderOptions): { type: 'price' | 'volume' | 'percent'; precision: number; minMove: number } | undefined {
  const type = options?.format
  if (type !== 'price' && type !== 'volume' && type !== 'percent') return undefined
  const precision = options?.precision ?? (type === 'volume' ? 0 : 2)
  return {
    type,
    precision,
    minMove: 10 ** -Math.max(0, precision),
  }
}

function isDisplayVisible(options?: PlotRenderOptions): boolean {
  return options?.display === undefined || options.display !== 0
}

export function useChart(containerRef: React.RefObject<HTMLDivElement>): ChartHandle {
  const priceChartRef = useRef<IChartApi | null>(null)
  const candleRef     = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const pricePaneRef  = useRef<HTMLDivElement | null>(null)
  const drawingBackSvgRef = useRef<SVGSVGElement | null>(null)
  const drawingFrontSvgRef = useRef<SVGSVGElement | null>(null)
  const indicatorsRef = useRef<Record<string, IndicatorEntry>>({})
  const timelineRef   = useRef<UTCTimestamp[]>([])
  const syncingRef    = useRef(false)
  const resizeRef     = useRef<() => void>(() => {})

  const redrawDrawings = useCallback(() => {
    const chart = priceChartRef.current
    const candles = candleRef.current
    const pricePane = pricePaneRef.current
    const backSvg = drawingBackSvgRef.current
    const frontSvg = drawingFrontSvgRef.current
    if (!chart || !candles || !pricePane || !backSvg || !frontSvg) return

    const width = Math.max(1, pricePane.clientWidth)
    const height = Math.max(1, pricePane.clientHeight)
    sizeDrawingSvg(backSvg, width, height)
    sizeDrawingSvg(frontSvg, width, height)

    const timeline = timelineRef.current.map(Number)
    const lastTime = timeline[timeline.length - 1]
    const prevTime = timeline[timeline.length - 2]
    const lastCoord = lastTime === undefined ? null : chart.timeScale().timeToCoordinate(lastTime as UTCTimestamp)
    const prevCoord = prevTime === undefined ? null : chart.timeScale().timeToCoordinate(prevTime as UTCTimestamp)
    const spacing = lastCoord !== null && prevCoord !== null
      ? Math.max(1, lastCoord - prevCoord)
      : 8

    const xForTime = (time: number): number | null => {
      const direct = chart.timeScale().timeToCoordinate(time as UTCTimestamp)
      if (direct !== null) return direct
      if (lastTime !== undefined && lastCoord !== null) {
        return lastCoord + ((time - lastTime) / 60) * spacing
      }
      return null
    }

    const yForPrice = (price: number): number | null => {
      if (!isFinite(price)) return null
      return candles.priceToCoordinate(price)
    }

    Object.values(indicatorsRef.current).forEach(entry => {
      const backGroup = entry.drawingBackGroup
      const frontGroup = entry.drawingFrontGroup
      if (!backGroup || !frontGroup || !entry.drawings) return
      backGroup.replaceChildren()
      frontGroup.replaceChildren()
      const display = entry.visible ? '' : 'none'
      backGroup.style.display = display
      frontGroup.style.display = display

      entry.drawings.boxes?.slice(0, MAX_RENDER_BOXES).forEach(box => {
        const x1 = xForTime(box.left)
        const x2 = xForTime(box.right)
        const y1 = yForPrice(box.top)
        const y2 = yForPrice(box.bottom)
        if (x1 === null || x2 === null || y1 === null || y2 === null) return
        const rect = svgEl('rect')
        rect.setAttribute('x', String(Math.min(x1, x2)))
        rect.setAttribute('y', String(Math.min(y1, y2)))
        rect.setAttribute('width', String(Math.abs(x2 - x1)))
        rect.setAttribute('height', String(Math.max(1, Math.abs(y2 - y1))))
        rect.setAttribute('fill', box.color)
        rect.setAttribute('stroke', box.border_color || box.color)
        rect.setAttribute('stroke-width', '1')
        rect.setAttribute('opacity', String(box.opacity ?? 1))
        backGroup.appendChild(rect)
      })

      entry.drawings.polylines?.slice(0, MAX_RENDER_POLYLINES).forEach(line => {
        const commands: string[] = []
        line.points.slice(0, MAX_RENDER_POINTS_PER_LINE).forEach((point, index) => {
          const x = xForTime(point.time)
          const y = yForPrice(point.value)
          if (x === null || y === null || !isFinite(x) || !isFinite(y)) return
          commands.push(`${index === 0 || commands.length === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`)
        })
        if (commands.length < 2) return
        const path = svgEl('path')
        path.setAttribute('d', commands.join(' '))
        path.setAttribute('fill', 'none')
        path.setAttribute('stroke', line.color)
        path.setAttribute('stroke-width', String(Math.max(1, line.line_width || 1)))
        path.setAttribute('stroke-linecap', 'round')
        path.setAttribute('stroke-linejoin', 'round')
        path.setAttribute('opacity', String(line.opacity ?? 1))
        backGroup.appendChild(path)
      })

      entry.drawings.labels?.slice(0, MAX_RENDER_LABELS).forEach(label => {
        const x = xForTime(label.time)
        const y = yForPrice(label.value)
        if (x === null || y === null) return
        const dot = svgEl('circle')
        dot.setAttribute('cx', String(x))
        dot.setAttribute('cy', String(y))
        dot.setAttribute('r', String(label.size ?? 5))
        dot.setAttribute('fill', label.color)
        dot.setAttribute('stroke', 'rgba(11,14,23,.85)')
        dot.setAttribute('stroke-width', '2')
        frontGroup.appendChild(dot)
      })

      if (entry.drawings.dashboard) {
        const dashboard = entry.drawings.dashboard
        const rows = dashboard.rows.slice(0, MAX_RENDER_DASHBOARD_ROWS)
        const metrics = dashboardMetrics(dashboard.size)
        const panelHeight = metrics.padding * 2 + metrics.title + rows.length * metrics.rowHeight + 10
        const origin = dashboardOrigin(dashboard.position, metrics.width, panelHeight, width, height)
        const g = svgEl('g')
        g.setAttribute('transform', `translate(${origin.x}, ${origin.y})`)
        const bg = svgEl('rect')
        bg.setAttribute('width', String(metrics.width))
        bg.setAttribute('height', String(panelHeight))
        bg.setAttribute('rx', '8')
        bg.setAttribute('fill', 'rgba(11,14,23,.84)')
        bg.setAttribute('stroke', 'rgba(92,138,255,.24)')
        g.appendChild(bg)

        const title = svgEl('text')
        title.textContent = clampText(dashboard.title)
        title.setAttribute('x', String(metrics.padding))
        title.setAttribute('y', String(metrics.padding + metrics.title))
        title.setAttribute('fill', '#d1d4dc')
        title.setAttribute('font-size', String(metrics.title))
        title.setAttribute('font-weight', '700')
        g.appendChild(title)

        rows.forEach((row, index) => {
          const y = metrics.padding + metrics.title + 14 + index * metrics.rowHeight
          const key = svgEl('text')
          key.textContent = clampText(row.label)
          key.setAttribute('x', String(metrics.padding))
          key.setAttribute('y', String(y))
          key.setAttribute('fill', '#5d6b8a')
          key.setAttribute('font-size', String(metrics.row))
          g.appendChild(key)

          const value = svgEl('text')
          value.textContent = clampText(row.value)
          value.setAttribute('x', String(metrics.width - metrics.padding))
          value.setAttribute('y', String(y))
          value.setAttribute('text-anchor', 'end')
          value.setAttribute('fill', row.color || '#d1d4dc')
          value.setAttribute('font-size', String(metrics.row))
          value.setAttribute('font-weight', '700')
          g.appendChild(value)
        })
        frontGroup.appendChild(g)
      }
    })
  }, [])

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
    requestAnimationFrame(redrawDrawings)
  }, [redrawDrawings])

  const subscribeSync = useCallback((chart: IChartApi) => {
    const handler = (range: LogicalRange | null) => syncCharts(chart, range)
    chart.timeScale().subscribeVisibleLogicalRangeChange(handler)
    return () => chart.timeScale().unsubscribeVisibleLogicalRangeChange(handler)
  }, [syncCharts])

  const removeIndicatorEntry = useCallback((indId: string) => {
    const entry = indicatorsRef.current[indId]
    if (!entry) return
    Object.values(entry.series).forEach(plot => plot.chart.removeSeries(plot.series))
    entry.drawingBackGroup?.remove()
    entry.drawingFrontGroup?.remove()
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

    const drawingBackSvg = svgEl('svg')
    drawingBackSvg.classList.add('drawing-overlay', 'drawing-overlay-back')
    pricePane.appendChild(drawingBackSvg)

    const chart = createChart(pricePane, chartOptions())
    const candles = chart.addCandlestickSeries({
      upColor:        '#26a69a',
      downColor:      '#ef5350',
      borderUpColor:  '#26a69a',
      borderDownColor:'#ef5350',
      wickUpColor:    '#26a69a',
      wickDownColor:  '#ef5350',
    })

    const drawingFrontSvg = svgEl('svg')
    drawingFrontSvg.classList.add('drawing-overlay', 'drawing-overlay-front')
    pricePane.appendChild(drawingFrontSvg)

    priceChartRef.current = chart
    candleRef.current     = candles
    pricePaneRef.current  = pricePane
    drawingBackSvgRef.current = drawingBackSvg
    drawingFrontSvgRef.current = drawingFrontSvg

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
      redrawDrawings()
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
      pricePaneRef.current = null
      drawingBackSvgRef.current = null
      drawingFrontSvgRef.current = null
    }
  }, [containerRef, redrawDrawings, removeIndicatorEntry, subscribeSync])

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
    requestAnimationFrame(redrawDrawings)
  }, [redrawDrawings])

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
    requestAnimationFrame(redrawDrawings)
  }, [redrawDrawings])

  const loadIndicator = useCallback((indId: string, plots: Record<string, PlotPoint[]>, pane: IndicatorPane, plotOptions: Record<string, PlotRenderOptions> = {}, drawings?: DrawingOutput) => {
    const priceChart = priceChartRef.current
    const container = containerRef.current
    const drawingBackSvg = drawingBackSvgRef.current
    const drawingFrontSvg = drawingFrontSvgRef.current
    if (!priceChart || !container) return

    removeIndicatorEntry(indId)

    const needsSeparatePane = pane === 'separate'
      && Object.keys(plots).some(plotName => !plotOptions[plotName]?.force_overlay)
    const entry: IndicatorEntry = {
      pane: needsSeparatePane ? 'separate' : 'price',
      chart: priceChart,
      series: {},
      drawings: hasDrawingOutput(drawings) ? drawings : undefined,
      visible: true,
    }

    if (entry.drawings && drawingBackSvg && drawingFrontSvg) {
      entry.drawingBackGroup = svgEl('g')
      entry.drawingFrontGroup = svgEl('g')
      entry.drawingBackGroup.dataset.indicatorId = indId
      entry.drawingFrontGroup.dataset.indicatorId = indId
      drawingBackSvg.appendChild(entry.drawingBackGroup)
      drawingFrontSvg.appendChild(entry.drawingFrontGroup)
      assignColour(indId, 'Drawings', '#ff9800')
    }

    if (needsSeparatePane) {
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
      const options = plotOptions[plotName]
      const targetChart = options?.force_overlay ? priceChart : entry.chart
      const colour = assignColour(indId, plotName, options?.color)
      const ls = targetChart.addLineSeries({
        color:             colour,
        lineWidth:         lineWidth(options?.linewidth),
        lineStyle:         lineStyle(options?.linestyle),
        priceLineVisible:  options?.trackprice ?? false,
        lastValueVisible:  true,
        title:             plotName,
        visible:           isDisplayVisible(options),
        priceFormat:       priceFormat(options),
      })
      ls.setData(lineData(pts, targetChart === entry.chart && entry.pane === 'separate' ? timelineRef.current : undefined))
      entry.series[plotName] = { chart: targetChart, series: ls }
    })

    indicatorsRef.current[indId] = entry
    resizeRef.current()
    redrawDrawings()
  }, [containerRef, redrawDrawings, removeIndicatorEntry, subscribeSync])

  const updateIndicatorTick = useCallback((indId: string, values: Record<string, number>, time: number) => {
    const entry = indicatorsRef.current[indId]
    if (!entry) return
    Object.entries(values).forEach(([plotName, value]) => {
      const ls = entry.series[plotName]
      if (ls && isFinite(value)) {
        ls.series.update({ time: time as UTCTimestamp, value })
      }
    })
    requestAnimationFrame(redrawDrawings)
  }, [redrawDrawings])

  const setIndicatorVisible = useCallback((indId: string, visible: boolean) => {
    const entry = indicatorsRef.current[indId]
    if (!entry || entry.visible === visible) return
    entry.visible = visible
    Object.values(entry.series).forEach(plot => {
      plot.series.applyOptions({ visible })
    })
    if (entry.drawingBackGroup) entry.drawingBackGroup.style.display = visible ? '' : 'none'
    if (entry.drawingFrontGroup) entry.drawingFrontGroup.style.display = visible ? '' : 'none'
    if (entry.el) entry.el.classList.toggle('chart-pane-hidden', !visible)
    resizeRef.current()
  }, [])

  const removeIndicator = useCallback((indId: string) => {
    removeIndicatorEntry(indId)
  }, [removeIndicatorEntry])

  return { loadHistory, addBar, loadIndicator, updateIndicatorTick, setIndicatorVisible, removeIndicator }
}
