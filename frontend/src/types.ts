// ── Wire types matching the Go backend ──────────────────────────────────────

export interface Bar {
  time:   number   // unix seconds
  open:   number
  high:   number
  low:    number
  close:  number
  volume: number
}

export interface PlotPoint {
  time:  number
  value: number
}

export interface PlotRenderOptions {
  color?:         string
  linewidth?:     number
  linestyle?:     number
  trackprice?:    boolean
  display?:       number
  format?:        string
  precision?:     number
  force_overlay?: boolean
}

export interface DrawingPoint {
  time:  number
  value: number
}

export interface DrawingPolyline {
  points:     DrawingPoint[]
  color:      string
  line_width: number
  opacity?:   number
}

export interface DrawingBox {
  left:          number
  right:         number
  top:           number
  bottom:        number
  color:         string
  border_color?: string
  opacity?:      number
}

export interface DrawingLabel {
  time:  number
  value: number
  color: string
  size?: number
}

export interface DashboardRow {
  label: string
  value: string
  color?: string
}

export interface DrawingDashboard {
  title:     string
  rows:      DashboardRow[]
  position?: string
  size?:     string
}

export interface DrawingOutput {
  polylines?: DrawingPolyline[]
  boxes?:     DrawingBox[]
  labels?:    DrawingLabel[]
  dashboard?: DrawingDashboard
}

export interface IndicatorOutput {
  indicator_id: string
  name:         string
  overlay:      boolean
  plots:        Record<string, PlotPoint[]>  // plot-name → series
  plot_options?: Record<string, PlotRenderOptions>
  drawings?:     DrawingOutput
}

export interface IndicatorUpdate {
  indicator_id: string
  name:         string
  values:       Record<string, number>       // plot-name → latest value
}

export interface IndicatorScript {
  id:     string
  name:   string
  script: string
}

export type IndicatorPane = 'price' | 'separate'

// ── Server → Client ──────────────────────────────────────────────────────────

export type ServerMsg =
  | { type: 'history';           bars: Bar[] }
  | { type: 'tick';              bar: Bar; indicator_updates: IndicatorUpdate[] }
  | { type: 'indicator_loaded';  indicator_output: IndicatorOutput }
  | { type: 'indicator_removed'; indicator_id: string }
  | { type: 'error';             error: string }

// ── Client → Server ──────────────────────────────────────────────────────────

export type ClientMsg =
  | { type: 'add_indicator';    indicator: IndicatorScript }
  | { type: 'remove_indicator'; id: string }
