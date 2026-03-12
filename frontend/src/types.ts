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

export interface IndicatorOutput {
  indicator_id: string
  name:         string
  plots:        Record<string, PlotPoint[]>  // plot-name → series
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
