# pinescription-demo

A PineScript-style trading indicator demo built with Go + React.

## Overview

This project demonstrates a PineScript-inspired DSL for defining technical indicators, powered by [`woodstock-tokyo/pinescription`](https://github.com/woodstock-tokyo/pinescription) on the backend and a React/TypeScript frontend with real-time charting. It supports both traditional `plot(...)` indicators and drawing-object output captured from Pine runtime hooks.

## Architecture

```
pinescription-demo/
├── backend/          # Go WebSocket server + indicator engine
│   ├── main.go       # Server, DSL parser, indicator evaluation
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

## Quick Start

### Using Docker Compose

Docker Compose is useful for the released-module setup. While `backend/go.mod` contains the local `../../pinescription` replace used by this workspace, prefer the manual development flow below; the backend Docker build context is `./backend` and cannot see the sibling checkout unless the Docker context/build files are adjusted.

```bash
docker-compose up --build
```

Then open http://localhost:3000

### Manual

The backend currently uses a local workspace patch of pinescription:

```go
replace github.com/woodstock-tokyo/pinescription => ../../pinescription
```

For manual development, keep this repository checked out next to the sibling pinescription repository:

```text
src/
├── pinescription/
└── pinescription-demo/
```

**Backend:**
```bash
cd backend
go run main.go
# Listens on :8080
```

**Frontend:**
```bash
cd frontend
npm install
npm run dev
# Opens on :3000
```

## Sample Indicators

See the [`indicators/`](./indicators/) directory and the root-level `example.pine` / `example2.pine` scripts for examples. Presets are wired explicitly in [`frontend/src/presets.ts`](./frontend/src/presets.ts); dropping a `.pine` file into the repository does not automatically add it to the UI.

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

The backend intentionally does not special-case the indicator title or implement the heatmap algorithm in Go. Regression tests mutate the original script title before evaluation to ensure the script executes through the generic runtime path.

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
# sibling runtime package
cd ../pinescription
rtk go test ./...

# demo backend
cd ../pinescription-demo/backend
rtk go test ./...

# demo frontend
cd ../frontend
rtk npm run build
```

The backend test suite includes coverage for `example2.pine` drawing output, title-independent execution, table dashboard position/size preservation, and `table.clear` handling.

## License

MIT
