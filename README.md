# pinescription-demo

A PineScript-style trading indicator demo built with Go + React.

## Overview

This project demonstrates a PineScript-inspired DSL for defining technical indicators, powered by [`woodstock-tokyo/pinescription`](https://github.com/woodstock-tokyo/pinescription) on the backend and a React/TypeScript frontend with real-time charting.

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
└── docker-compose.yml
```

## Features

- **PineScript-style DSL** — define indicators with familiar syntax (`indicator()`, `plot()`, `sma()`, `ema()`, `rsi()`, `macd()`, etc.)
- **Real-time WebSocket streaming** — live OHLCV bar updates pushed to the browser
- **Interactive chart** — candlestick chart with overlaid indicator plots using Lightweight Charts
- **Preset indicators** — built-in SMA Cross, RSI, MACD, Bollinger Bands, and more
- **Custom scripts** — write and evaluate your own indicator scripts in the browser

## Quick Start

### Using Docker Compose

```bash
docker-compose up --build
```

Then open http://localhost:3000

### Manual

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

See the [`indicators/`](./indicators/) directory for example scripts.

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

## License

MIT
