export interface Preset {
  id:          string
  name:        string
  description: string
  script:      string
}

export const PRESETS: Preset[] = [
  {
    id: 'sma_20',
    name: 'SMA 20',
    description: 'Simple Moving Average (20)',
    script:
`indicator("SMA 20", overlay=true)
plot(ta.sma(close, 20), title="SMA 20")`,
  },
  {
    id: 'ema_50',
    name: 'EMA 50',
    description: 'Exponential Moving Average (50)',
    script:
`indicator("EMA 50", overlay=true)
plot(ta.ema(close, 50), title="EMA 50")`,
  },
  {
    id: 'rsi_14',
    name: 'RSI 14',
    description: 'Relative Strength Index (14)',
    script:
`indicator("RSI 14")
plot(ta.rsi(close, 14), title="RSI")`,
  },
  {
    id: 'macd',
    name: 'MACD',
    description: 'MACD (12, 26, 9)',
    script:
`indicator("MACD")
[macdLine, signal, hist] = ta.macd(close, 12, 26, 9)
plot(macdLine, title="MACD")
plot(signal,   title="Signal")
plot(hist,     title="Histogram")`,
  },
  {
    id: 'bb_20',
    name: 'Bollinger Bands',
    description: 'Bollinger Bands (20, 2σ)',
    script:
`indicator("Bollinger Bands", overlay=true)
[mid, upper, lower] = ta.bb(close, 20, 2.0)
plot(mid,   title="BB Mid")
plot(upper, title="BB Upper")
plot(lower, title="BB Lower")`,
  },
  {
    id: 'atr_14',
    name: 'ATR 14',
    description: 'Average True Range (14)',
    script:
`indicator("ATR 14")
plot(ta.atr(14), title="ATR")`,
  },
  {
    id: 'cci_20',
    name: 'CCI 20',
    description: 'Commodity Channel Index (20)',
    script:
`indicator("CCI 20")
plot(ta.cci(20), title="CCI")`,
  },
  {
    id: 'mfi_14',
    name: 'MFI 14',
    description: 'Money Flow Index (14)',
    script:
`indicator("MFI 14")
plot(ta.mfi(14), title="MFI")`,
  },
  {
    id: 'dmi_14',
    name: 'DMI',
    description: 'Directional Movement Index',
    script:
`indicator("DMI")
[adx, diPlus, diMinus] = ta.dmi(14, 14)
plot(adx,     title="ADX")
plot(diPlus,  title="DI+")
plot(diMinus, title="DI-")`,
  },
  {
    id: 'kc_20',
    name: 'Keltner Channel',
    description: 'Keltner Channel (20, 1.5)',
    script:
`indicator("Keltner Channel", overlay=true)
[mid, upper, lower] = ta.kc(close, 20, 1.5)
plot(mid,   title="KC Mid")
plot(upper, title="KC Upper")
plot(lower, title="KC Lower")`,
  },
  {
    id: 'roc_12',
    name: 'ROC 12',
    description: 'Rate of Change (12)',
    script:
`indicator("ROC 12")
plot(ta.roc(close, 12), title="ROC")`,
  },
  {
    id: 'stdev_20',
    name: 'Std Dev 20',
    description: 'Standard Deviation (20)',
    script:
`indicator("Std Dev 20")
plot(ta.stdev(close, 20), title="StdDev")`,
  },
]
