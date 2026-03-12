import { useState, useEffect, useRef, useCallback } from 'react'
import { WSClient } from './ws'
import { Bar, IndicatorScript } from './types'
import { PRESETS } from './presets'
import { useChart, getColourMap } from './useChart'

// ── helpers ───────────────────────────────────────────────────────────────────

function wsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/ws`
}

function fmt(n: number, d = 2) {
  return n.toLocaleString('en-US', { minimumFractionDigits: d, maximumFractionDigits: d })
}

// ── App ───────────────────────────────────────────────────────────────────────

export default function App() {
  const containerRef = useRef<HTMLDivElement>(null)
  const chart = useChart(containerRef)
  const wsRef = useRef<WSClient | null>(null)

  const [connected,   setConnected]   = useState(false)
  const [activeTab,   setActiveTab]   = useState<'active' | 'add'>('add')
  const [activeInds,  setActiveInds]  = useState<IndicatorScript[]>([])
  const [lastBar,     setLastBar]     = useState<Bar | null>(null)
  const [prevClose,   setPrevClose]   = useState<number | null>(null)
  const [formId,      setFormId]      = useState('')
  const [formName,    setFormName]    = useState('')
  const [formScript,  setFormScript]  = useState(PRESETS[0].script)
  const [formError,   setFormError]   = useState('')
  const [, forceRender] = useState(0)

  // ── WebSocket ────────────────────────────────────────────────────────────────
  useEffect(() => {
    const ws = new WSClient(wsUrl())
    wsRef.current = ws

    const off = ws.on((msg) => {
      if (msg.type === '__connected')   { setConnected(true);  return }
      if (msg.type === '__disconnected'){ setConnected(false); return }

      switch (msg.type) {
        case 'history': {
          chart.loadHistory(msg.bars)
          const n = msg.bars.length
          if (n > 0) setLastBar(msg.bars[n - 1])
          if (n > 1) setPrevClose(msg.bars[n - 2].close)
          break
        }

        case 'tick': {
          chart.addBar(msg.bar)
          setPrevClose(prev => (lastBar ? lastBar.close : prev))
          setLastBar(msg.bar)
          msg.indicator_updates?.forEach(u =>
            chart.updateIndicatorTick(u.indicator_id, u.values, msg.bar.time)
          )
          break
        }

        case 'indicator_loaded': {
          const o = msg.indicator_output
          chart.loadIndicator(o.indicator_id, o.plots)
          setActiveInds(prev => {
            const filtered = prev.filter(i => i.id !== o.indicator_id)
            return [...filtered, { id: o.indicator_id, name: o.name, script: '' }]
          })
          forceRender(n => n + 1)
          break
        }

        case 'indicator_removed': {
          chart.removeIndicator(msg.indicator_id)
          setActiveInds(prev => prev.filter(i => i.id !== msg.indicator_id))
          forceRender(n => n + 1)
          break
        }

        case 'error':
          setFormError(msg.error)
          break
      }
    })

    return () => { off(); ws.destroy() }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── actions ──────────────────────────────────────────────────────────────────

  const addIndicator = useCallback(() => {
    const ws = wsRef.current
    if (!ws) return
    const id     = formId.trim()   || `ind_${Date.now()}`
    const name   = formName.trim() || id
    const script = formScript.trim()
    if (!script) { setFormError('Script cannot be empty'); return }
    setFormError('')
    ws.send({ type: 'add_indicator', indicator: { id, name, script } })
    setFormId('')
    setFormName('')
    setActiveTab('active')
  }, [formId, formName, formScript])

  const removeIndicator = useCallback((id: string) => {
    wsRef.current?.send({ type: 'remove_indicator', id })
  }, [])

  const loadPreset = useCallback((id: string) => {
    const p = PRESETS.find(x => x.id === id)
    if (!p) return
    setFormId(p.id)
    setFormName(p.name)
    setFormScript(p.script)
    setFormError('')
    setActiveTab('add')
  }, [])

  // ── derived ──────────────────────────────────────────────────────────────────

  const pct = lastBar && prevClose
    ? ((lastBar.close - prevClose) / prevClose * 100)
    : null

  const colours = getColourMap()

  // ── render ───────────────────────────────────────────────────────────────────

  return (
    <div className="app">

      {/* ── Topbar ── */}
      <header className="topbar">
        <div className="topbar-logo">
          📈 <span className="brand">pinescription</span>
          <span className="topbar-badge">demo</span>
        </div>
        <span className="topbar-pair">BTC / USDT · 1m · Simulated</span>

        <div className="topbar-right">
          {lastBar && (
            <>
              <span className="price-val">{fmt(lastBar.close)}</span>
              {pct !== null && (
                <span className={`price-chg ${pct >= 0 ? 'up' : 'dn'}`}>
                  {pct >= 0 ? '▲' : '▼'} {Math.abs(pct).toFixed(2)}%
                </span>
              )}
            </>
          )}
          <div
            className={`ws-dot ${connected ? 'on' : 'off'}`}
            title={connected ? 'Connected' : 'Reconnecting…'}
          />
        </div>
      </header>

      {/* ── Chart ── */}
      <section className="chart-panel">
        <div className="chart-container">
          <div id="kline-chart" ref={containerRef} />

          {/* floating legend */}
          <div className="chart-legend">
            {activeInds.map(ind =>
              Object.entries(colours[ind.id] || {}).map(([plotName, colour]) => (
                <div
                  key={`${ind.id}:${plotName}`}
                  className="legend-row"
                  style={{ '--c': colour } as React.CSSProperties}
                >
                  <div className="legend-dot" />
                  {ind.name}: {plotName}
                </div>
              ))
            )}
          </div>
        </div>

        {/* OHLCV strip */}
        {lastBar && (
          <div className="ohlcv-strip">
            <span>O <strong>{fmt(lastBar.open)}</strong></span>
            <span style={{ color: '#26a69a' }}>H <strong>{fmt(lastBar.high)}</strong></span>
            <span style={{ color: '#ef5350' }}>L <strong>{fmt(lastBar.low)}</strong></span>
            <span>C <strong>{fmt(lastBar.close)}</strong></span>
            <span>V <strong>{fmt(lastBar.volume, 2)}</strong></span>
          </div>
        )}
      </section>

      {/* ── Sidebar ── */}
      <aside className="sidebar">
        <div className="tabs">
          <button
            className={`tab ${activeTab === 'active' ? 'active' : ''}`}
            onClick={() => setActiveTab('active')}
          >
            Active ({activeInds.length})
          </button>
          <button
            className={`tab ${activeTab === 'add' ? 'active' : ''}`}
            onClick={() => setActiveTab('add')}
          >
            + Add Indicator
          </button>
        </div>

        <div className="tab-body">

          {/* ── Active indicators ── */}
          {activeTab === 'active' && (
            activeInds.length === 0
              ? (
                <div className="empty">
                  <div className="empty-icon">📊</div>
                  <div>No active indicators yet.</div>
                  <div>Switch to <strong>+ Add Indicator</strong> to get started.</div>
                </div>
              )
              : activeInds.map(ind => {
                  const indColours = colours[ind.id] || {}
                  const firstColour = Object.values(indColours)[0] ?? '#fff'
                  return (
                    <div key={ind.id} className="ind-card">
                      <div className="ind-card-head">
                        <div className="ind-swatch" style={{ background: firstColour }} />
                        <span className="ind-name">{ind.name}</span>
                        <button className="ind-remove" onClick={() => removeIndicator(ind.id)} title="Remove">✕</button>
                      </div>
                      <div className="ind-plots">
                        {Object.entries(indColours).map(([plotName, colour]) => (
                          <span
                            key={plotName}
                            className="plot-pill"
                            style={{ color: colour, borderColor: colour + '44' }}
                          >
                            {plotName}
                          </span>
                        ))}
                      </div>
                    </div>
                  )
                })
          )}

          {/* ── Add indicator ── */}
          {activeTab === 'add' && (
            <div className="add-form">

              {/* Presets */}
              <div>
                <div className="presets-label">Quick Presets</div>
                <div className="presets-grid">
                  {PRESETS.map(p => (
                    <button
                      key={p.id}
                      className="btn-preset"
                      title={p.description}
                      onClick={() => loadPreset(p.id)}
                    >
                      {p.name}
                    </button>
                  ))}
                </div>
              </div>

              <hr className="divider" />

              <div className="field">
                <label className="field-label">Indicator ID</label>
                <input
                  className="field-input"
                  placeholder="e.g. my_sma"
                  value={formId}
                  onChange={e => setFormId(e.target.value)}
                />
              </div>

              <div className="field">
                <label className="field-label">Display Name</label>
                <input
                  className="field-input"
                  placeholder="e.g. My SMA"
                  value={formName}
                  onChange={e => setFormName(e.target.value)}
                />
              </div>

              <div className="field">
                <label className="field-label">PineScript</label>
                <textarea
                  className="script-editor"
                  value={formScript}
                  onChange={e => setFormScript(e.target.value)}
                  spellCheck={false}
                  placeholder={`indicator("My Indicator", overlay=true)\nplot(ta.sma(close, 20), title="SMA 20")`}
                />
              </div>

              {formError && <div className="err-msg">⚠ {formError}</div>}

              <button
                className="btn-add"
                onClick={addIndicator}
                disabled={!connected}
              >
                {connected ? 'Add to Chart' : 'Connecting…'}
              </button>
            </div>
          )}

        </div>
      </aside>
    </div>
  )
}
