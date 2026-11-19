# pinescription-demo

A PineScript-style trading indicator demo built with Go + React.

## Overview

This project demonstrates a PineScript-inspired DSL for defining technical indicators, powered by [`woodstock-tokyo/pinescription`](https://github.com/woodstock-tokyo/pinescription) on the backend and a React/TypeScript frontend with real-time charting. It supports both traditional `plot(...)` indicators and drawing-object output captured from Pine runtime hooks.

## Architecture

```
pinescription-demo/
├── backend/          # Go WebSocket server + indicator engine
│   ├── main.go       # Server, script normalization, plot/drawing hooks
│   ├── runtime.go    # Pre-execution input discovery and market context
│   ├── go.mod
│   └── Dockerfile
├── frontend/         # React + TypeScript + Lightweight Charts
│   ├── src/
│   │   ├── App.tsx
│   │   ├── useChart.ts
│   │   ├── ws.ts
│   │   ├── presets.ts
│   │   └── types.ts
│   ├── index.html
│   └── Dockerfile
├── indicators/       # Sample PineScript-style indicator files
│   ├── sma_cross.pine
│   ├── rsi.pine
│   └── macd.pine
├── example.pine      # Larger plot-oriented example
├── example2.pine     # Volumetric Regression Heatmap drawing example
└── docker-compose.yml
```

## Features

- **PineScript-style DSL** — define indicators with familiar syntax (`indicator()`, `plot()`, `sma()`, `ema()`, `rsi()`, `macd()`, etc.)
- **Real-time WebSocket streaming** — live OHLCV bar updates pushed to the browser
- **Interactive chart** — candlestick chart with overlaid indicator plots using Lightweight Charts
- **Preset indicators** — built-in SMA Cross, RSI, MACD, Bollinger Bands, and more
- **Drawing-object examples** — renders runtime-captured `polyline`, `box`, `label`, and `table` output for scripts such as `example2.pine`
- **Custom scripts** — write and evaluate your own indicator scripts in the browser
- **Pre-execution inputs** — discover input metadata, edit typed values, and apply validated overrides without changing source
- **Market context** — actual bar timestamps, UTC timezone, a continuous `24x7` session, and simulated contract metadata
- **Updated runtime presets** — configurable EMA, anchored VWAP/bands, RCI, and historical strategy equity with metric snapshots

The backend pins pinescription revision `000ace7` from
[pinescription PR #15](https://github.com/woodstock-tokyo/pinescription/pull/15).
That PR is an upstream dependency, not a claim that the API expansion is already
released on the compiler's main branch. No local module replacement is required.

## Quick Start

### Using Docker Compose

The backend build downloads the pinned module and uses BuildKit SSH forwarding
for GitHub access. Ensure your SSH agent can access the compiler repository.
The frontend uses the repository-root build context so its shared `.pine`
preset imports are available.

```bash
docker compose up --build
```

Then open http://localhost:3000

### Manual

Install Go 1.25.6 or newer and Node.js 20 or newer. A sibling compiler checkout is
optional; the checked-in `go.mod` and `go.sum` resolve the pinned revision.
If your GitHub setup requires it, configure `GOPRIVATE` and Git credentials before
downloading `github.com/woodstock-tokyo/pinescription`.

**Backend:**
```bash
cd backend
go run .
# Listens on :8080
```

**Frontend:**
```bash
cd frontend
npm ci
npm run dev
# Opens on :3000
```

### Configure Script Inputs

1. Select a preset or paste a script.
2. Click **Inspect / Reset Inputs**. The backend compiles and discovers input
   definitions without executing the script or fetching provider data.
3. Edit numeric, checkbox, source, enum, text, or option-list controls, then click
   **Add to Chart**. Without inspection, validated script defaults are used.
4. To change inputs on an existing indicator, keep its ID and add it again.
   Failed validation leaves the previously working indicator intact.

Overrides persist during each historical recomputation on new bars. Editing the
script discards its old input form, and stale inspection responses are ignored.
Time inputs use Unix milliseconds; colors accept literal color strings, including
RGBA hex values. This demo does not implement TradingView's chart-click price/time
pickers or full input-layout/confirmation behavior.

Input metadata must use constants supported by the compiler's pre-execution
evaluator. For example, use `const string group = "Settings"` for an input group.
The heatmap sample uses explicit group/option constants and equivalent RGBA
literal defaults instead of runtime `color.new(...)` calls.

### WebSocket Input Protocol

Existing `add_indicator` and `remove_indicator` requests remain supported.
`add_indicator.indicator` additionally accepts an `inputs` object keyed by the
IDs returned by inspection:

```json
{"type":"inspect_inputs","request_id":"preview-1","indicator":{"id":"ema","name":"EMA","script":"length=input.int(20)\nplot(ta.ema(close,length))"}}
```

The reply has type `inputs_discovered`, the same `request_id`, and an
`input_parameters` array (omitted when empty). Inspection errors carry the same
request ID. Enum option values are qualified names such as `"Side.buy"`; source
values are identifiers such as `"close"`.

```json
{"type":"add_indicator","indicator":{"id":"ema","name":"EMA","script":"length=input.int(20)\nplot(ta.ema(close,length))","inputs":{"length":50}}}
```

## Sample Indicators

See the [`indicators/`](./indicators/) directory and the root-level `example.pine` / `example2.pine` scripts for examples. Presets are wired explicitly in [`frontend/src/presets.ts`](./frontend/src/presets.ts); dropping a `.pine` file into the repository does not automatically add it to the UI.

| Preset | Demonstrates |
| --- | --- |
| Configurable EMA | Constant input defaults, numeric bounds, and selectable source series |
| Daily Anchored VWAP | UTC daily anchors and volume-weighted deviation bands |
| Rank Correlation Index | `ta.rci` with a configurable lookback |
| Historical SMA Strategy | Native historical orders, explicit zero margins, equity and `[1]` metric snapshots |

### SMA Crossover
```pine
indicator("SMA Cross", overlay=true)
fast = sma(close, 10)
slow = sma(close, 30)
plot(fast, "Fast SMA", color=#2196F3)
plot(slow, "Slow SMA", color=#FF5722)
```

### RSI
```pine
indicator("RSI", overlay=false)
r = rsi(close, 14)
plot(r, "RSI", color=#9C27B0)
hline(70, "Overbought", color=#FF0000)
hline(30, "Oversold", color=#00FF00)
```

### MACD
```pine
indicator("MACD", overlay=false)
[macdLine, signalLine, hist] = macd(close, 12, 26, 9)
plot(macdLine, "MACD", color=#2196F3)
plot(signalLine, "Signal", color=#FF9800)
plot(hist, "Histogram", color=#4CAF50)
```

### Larger Example Script

`example.pine` in the root folder demonstrates a larger plot-oriented script that still renders through regular `plot(...)` output.

### Volumetric Regression Heatmap

`example2.pine` is a larger Pine v6 drawing-object example based on the LuxAlgo Volumetric Regression Heatmap. Unlike the smaller examples, it does not use `plot(...)`; it emits visual output through Pine drawing APIs such as `polyline.new`, `box.new`, `label.new`, `chart.point.from_index`, and `table.cell`.

The demo still uses the same preset → WebSocket → backend evaluation → chart rendering pipeline:

1. The preset loads the raw `example2.pine` source into the editor.
2. The backend normalizes, compiles, and executes the script with pinescription.
3. Registered runtime hooks collect plot output and drawing-object output generically.
4. The frontend renders captured drawings as an SVG overlay and dashboard table on the price pane.

## Runtime Scope and Limitations

- The data source is a **simulated** one-minute `DEMO` market, not an exchange
  feed. Bars retain their actual timestamps; gaps are not silently replaced with
  consecutive minute timestamps. Contract settings are UTC, `USDT`, tick size
  `0.01`, point value `1`, minimum contract `0.000001`, and a `24x7` regular session.
- Native strategies use the compiler's limited historical broker and require
  explicit `margin_long=0, margin_short=0`. Slippage affects fills rather than
  the chart mark; historical scalar metrics use prior calculation snapshots.
  New simulated bars trigger full historical recomputation, **not** live-tick
  rollback, fill-triggered recalculation, or real trading.
- Strategy output uses explicit `plot(...)` hooks. This demo does not add an
  order-marker renderer, trade-ledger dashboard, or complete TradingView broker
  support. Advanced broker/risk options remain explicit runtime errors.
- Same-context security expressions can use `DEMO`/`"1"`. No cross-context
  security provider, library resolver, footprint/order-flow provider, or
  alternate-chart data source is configured here. Scripts needing these
  capabilities are not silently supplied fake data; missing footprints may
  return `na` according to the runtime API.
- Drawing and plotting hooks remain presentation adapters. No algorithm-specific
  drawing behavior or new blanket argument-binding layer is introduced.

See the compiler's
[feature reference](https://github.com/woodstock-tokyo/pinescription/blob/000ace7e2b2e2c0a2f5b970f70949995c08a704d/docs/features.md)
for exact API coverage and unsupported behavior.

## Supported Functions

| Function | Description |
|----------|-------------|
| `sma(src, length)` | Simple Moving Average |
| `ema(src, length)` | Exponential Moving Average |
| `rsi(src, length)` | Relative Strength Index |
| `macd(src, fast, slow, signal)` | MACD (returns [macd, signal, hist]) |
| `atr(length)` | Average True Range |
| `cci(length)` | Commodity Channel Index |
| `mfi(length)` | Money Flow Index |
| `roc(src, length)` | Rate of Change |
| `stdev(src, length)` | Standard Deviation |
| `bb(src, length, mult)` | Bollinger Bands |

## Verification

Run the same checks used by the demo integration tests:

```bash
# demo backend
cd backend
go test -race ./...
go vet ./...

# demo frontend
cd ../frontend
npm test
npm run build
```

The backend tests cover input discovery without execution, typed overrides and
tick persistence, actual timestamps/session boundaries, native strategy history,
the new presets, WebSocket inspection, and failed replacement safety. Existing
heatmap/drawing/table regressions remain in place. Frontend tests exercise
input-value conversion, validation, enum/source round-trips, and stale overrides.

## License

Application code is MIT licensed. `example2.pine` retains its original LuxAlgo
CC BY-NC-SA 4.0 notice; the input-metadata adaptations do not change that license.
