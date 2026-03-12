package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pine "github.com/tsuz/go-pine/pine"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain types
// ─────────────────────────────────────────────────────────────────────────────

// Bar is a single OHLCV candle sent to the browser.
type Bar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// IndicatorScript is a named PineScript program stored server-side.
type IndicatorScript struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

// PlotPoint is one named plot value at a given timestamp.
type PlotPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

// IndicatorOutput is returned to the browser after evaluating a script.
type IndicatorOutput struct {
	IndicatorID string                 `json:"indicator_id"`
	Name        string                 `json:"name"`
	Plots       map[string][]PlotPoint `json:"plots"`
}

// IndicatorUpdate is a lightweight per-bar update for all active indicators.
type IndicatorUpdate struct {
	IndicatorID string             `json:"indicator_id"`
	Name        string             `json:"name"`
	Values      map[string]float64 `json:"values"`
}

// WSEnvelope is the top-level WebSocket message wrapper.
type WSEnvelope struct {
	Type             string            `json:"type"`
	Bars             []Bar             `json:"bars,omitempty"`
	Bar              *Bar              `json:"bar,omitempty"`
	IndicatorUpdates []IndicatorUpdate `json:"indicator_updates,omitempty"`
	IndicatorOutput  *IndicatorOutput  `json:"indicator_output,omitempty"`
	IndicatorID      string            `json:"indicator_id,omitempty"`
	Error            string            `json:"error,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Server state
// ─────────────────────────────────────────────────────────────────────────────

var (
	barsMu     sync.RWMutex
	bars       []Bar
	indMu      sync.RWMutex
	indicators = map[string]*IndicatorScript{}
	clientsMu  sync.RWMutex
	clients    = map[*websocket.Conn]bool{}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ─────────────────────────────────────────────────────────────────────────────
// Market data simulation
// ─────────────────────────────────────────────────────────────────────────────

func r2(v float64) float64 { return math.Round(v*100) / 100 }

func seedHistory(n int) []Bar {
	rng := rand.New(rand.NewSource(42))
	out := make([]Bar, 0, n)
	price := 65000.0
	now := time.Now().UTC().Truncate(time.Minute)
	start := now.Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		t := start.Add(time.Duration(i) * time.Minute)
		o := price
		chg := (rng.Float64()*2 - 1) * price * 0.004
		c := r2(o + chg)
		h := r2(math.Max(o, c) + rng.Float64()*price*0.002)
		l := r2(math.Min(o, c) - rng.Float64()*price*0.002)
		v := r2(rng.Float64()*15 + 1)
		out = append(out, Bar{Time: t.Unix(), Open: r2(o), High: h, Low: l, Close: c, Volume: v})
		price = c
	}
	return out
}

func nextBar(prev Bar, rng *rand.Rand) Bar {
	o := prev.Close
	chg := (rng.Float64()*2 - 1) * o * 0.004
	c := r2(o + chg)
	h := r2(math.Max(o, c) + rng.Float64()*o*0.002)
	l := r2(math.Min(o, c) - rng.Float64()*o*0.002)
	v := r2(rng.Float64()*15 + 1)
	return Bar{Time: prev.Time + 60, Open: r2(o), High: h, Low: l, Close: c, Volume: v}
}

// ─────────────────────────────────────────────────────────────────────────────
// go-pine bridge
// ─────────────────────────────────────────────────────────────────────────────

// barsToOHLCV converts our Bar slice to go-pine's OHLCV slice.
func barsToOHLCV(bs []Bar) []pine.OHLCV {
	out := make([]pine.OHLCV, len(bs))
	for i, b := range bs {
		out[i] = pine.OHLCV{
			O: b.Open, H: b.High, L: b.Low, C: b.Close, V: b.Volume,
			S: time.Unix(b.Time, 0).UTC(),
		}
	}
	return out
}

// collectSeries walks the OHLCVSeries in order and, for each bar time,
// looks up the ValueSeries value using SetCurrent+Val.
// This is the correct pattern for go-pine: Value fields are unexported;
// the only public accessor is ValueSeries.Val() which returns the *current* value.
func collectSeries(vs pine.ValueSeries, bs []Bar) []PlotPoint {
	var pts []PlotPoint
	for _, b := range bs {
		t := time.Unix(b.Time, 0).UTC()
		if vs.SetCurrent(t) {
			if val := vs.Val(); val != nil && !math.IsNaN(*val) && !math.IsInf(*val, 0) {
				pts = append(pts, PlotPoint{Time: b.Time, Value: r2(*val)})
			}
		}
	}
	return pts
}

// latestVal returns only the last value in a ValueSeries for the given bars.
func latestVal(vs pine.ValueSeries, bs []Bar) (float64, bool) {
	if len(bs) == 0 {
		return 0, false
	}
	t := time.Unix(bs[len(bs)-1].Time, 0).UTC()
	if vs.SetCurrent(t) {
		if val := vs.Val(); val != nil && !math.IsNaN(*val) && !math.IsInf(*val, 0) {
			return r2(*val), true
		}
	}
	return 0, false
}

// ─────────────────────────────────────────────────────────────────────────────
// PineScript evaluator
// ─────────────────────────────────────────────────────────────────────────────

// evalScript parses a PineScript-like program and evaluates it over the given
// bars using the go-pine / pinescription library.
// Returns a map of plot-name → time-series of PlotPoints.
func evalScript(script string, bs []Bar) (map[string][]PlotPoint, error) {
	if len(bs) == 0 {
		return nil, fmt.Errorf("no bars provided")
	}

	ohlcvs := barsToOHLCV(bs)
	series, err := pine.NewOHLCVSeries(ohlcvs)
	if err != nil {
		return nil, fmt.Errorf("NewOHLCVSeries: %w", err)
	}

	// Advance the series to the end so all indicator caches are populated.
	for {
		v, err := series.Next()
		if err != nil {
			return nil, fmt.Errorf("series.Next: %w", err)
		}
		if v == nil {
			break
		}
	}

	// Parse and evaluate the script.
	namedSeries, err := parsePinePlots(script, series)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]PlotPoint)
	for name, vs := range namedSeries {
		result[name] = collectSeries(vs, bs)
	}
	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// PineScript DSL parser / evaluator
// ─────────────────────────────────────────────────────────────────────────────
//
// Supported PineScript v5 subset:
//
//   indicator("title", overlay=true)
//
//   // variable assignment
//   myVar = ta.sma(close, 20)
//   [macdLine, signal, hist] = ta.macd(close, 12, 26, 9)
//   [mid, upper, lower] = ta.bb(close, 20, 2.0)
//   [mid, upper, lower] = ta.kc(close, 20, 1.5)
//   [adx, diPlus, diMinus] = ta.dmi(14, 14)
//
//   // plots
//   plot(myVar, title="SMA 20")
//   plot(ta.rsi(close, 14), title="RSI")
//
// Supported ta.* functions:
//   ta.sma, ta.ema, ta.rma, ta.rsi, ta.macd, ta.bb / ta.bbands,
//   ta.atr, ta.cci, ta.mfi, ta.dmi, ta.kc, ta.stdev, ta.variance,
//   ta.roc, ta.change

type evalContext struct {
	series pine.OHLCVSeries
	vars   map[string]pine.ValueSeries
}

func newEvalContext(series pine.OHLCVSeries) *evalContext {
	return &evalContext{series: series, vars: make(map[string]pine.ValueSeries)}
}

func parsePinePlots(script string, series pine.OHLCVSeries) (map[string]pine.ValueSeries, error) {
	ctx := newEvalContext(series)
	plots := make(map[string]pine.ValueSeries)

	for _, rawLine := range strings.Split(script, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "indicator(") {
			continue
		}

		// plot(expr, title="Name", ...)
		if strings.HasPrefix(line, "plot(") {
			name, vs, err := parsePlotLine(line, ctx)
			if err != nil {
				return nil, fmt.Errorf("plot() error: %w", err)
			}
			if vs != nil {
				plots[name] = vs
			}
			continue
		}

		// Multi-return: [a, b, c] = ta.func(...)
		if strings.HasPrefix(line, "[") {
			if err := parseMultiAssign(line, ctx); err != nil {
				return nil, fmt.Errorf("multi-assign %q: %w", line, err)
			}
			continue
		}

		// Single assignment
		if isAssignment(line) {
			if err := parseSingleAssign(line, ctx); err != nil {
				return nil, fmt.Errorf("assign %q: %w", line, err)
			}
		}
	}

	// Auto-plot all variables if no explicit plot() calls.
	if len(plots) == 0 {
		for name, vs := range ctx.vars {
			plots[name] = vs
		}
	}
	return plots, nil
}

// isAssignment returns true if line contains a bare = (not ==, !=, <=, >=, :=).
func isAssignment(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			if i > 0 {
				p := line[i-1]
				if p == '!' || p == '<' || p == '>' || p == '=' || p == ':' {
					continue
				}
			}
			if i+1 < len(line) && line[i+1] == '=' {
				continue
			}
			return true
		}
	}
	return false
}

func parsePlotLine(line string, ctx *evalContext) (string, pine.ValueSeries, error) {
	inner := extractBetweenParens(line[4:]) // "plot" is 4 chars
	args := splitTopLevel(inner, ',')
	if len(args) == 0 {
		return "", nil, fmt.Errorf("empty plot()")
	}
	expr := strings.TrimSpace(args[0])
	name := expr
	for _, arg := range args[1:] {
		arg = strings.TrimSpace(arg)
		if strings.HasPrefix(arg, "title=") {
			t := strings.Trim(strings.TrimPrefix(arg, "title="), `"' `)
			if t != "" {
				name = t
			}
		}
	}
	vs, err := evalExpr(expr, ctx)
	if err != nil {
		return "", nil, err
	}
	return name, vs, nil
}

func parseMultiAssign(line string, ctx *evalContext) error {
	end := strings.Index(line, "]")
	if end < 0 {
		return fmt.Errorf("missing ]")
	}
	names := strings.Split(line[1:end], ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	rest := strings.TrimSpace(line[end+1:])
	rest = strings.TrimPrefix(rest, ":=")
	rest = strings.TrimPrefix(rest, "=")
	rest = strings.TrimSpace(rest)

	results, err := evalMultiExpr(rest, ctx)
	if err != nil {
		return err
	}
	for i, name := range names {
		if i < len(results) && name != "_" && name != "" {
			ctx.vars[name] = results[i]
		}
	}
	return nil
}

func parseSingleAssign(line string, ctx *evalContext) error {
	eqIdx := -1
	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			if i > 0 {
				p := line[i-1]
				if p == '!' || p == '<' || p == '>' || p == '=' || p == ':' {
					continue
				}
			}
			if i+1 < len(line) && line[i+1] == '=' {
				continue
			}
			eqIdx = i
			break
		}
	}
	if eqIdx < 0 {
		return nil
	}
	lhs := strings.TrimSpace(line[:eqIdx])
	lhs = strings.TrimSuffix(lhs, ":")
	lhs = strings.TrimPrefix(lhs, "var float ")
	lhs = strings.TrimPrefix(lhs, "float ")
	lhs = strings.TrimPrefix(lhs, "var ")
	rhs := strings.TrimSpace(line[eqIdx+1:])

	vs, err := evalExpr(rhs, ctx)
	if err != nil {
		return err
	}
	if vs != nil {
		ctx.vars[lhs] = vs
	}
	return nil
}

// evalExpr evaluates a single-value expression.
func evalExpr(expr string, ctx *evalContext) (pine.ValueSeries, error) {
	expr = strings.TrimSpace(expr)

	// Variable reference
	if vs, ok := ctx.vars[expr]; ok {
		return vs, nil
	}

	// Source series
	switch expr {
	case "close":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropClose), nil
	case "open":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropOpen), nil
	case "high":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropHigh), nil
	case "low":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropLow), nil
	case "volume":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropVolume), nil
	case "hl2":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropHL2), nil
	case "hlc3":
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropHLC3), nil
	}

	// ta.xxx(...) call
	if strings.HasPrefix(expr, "ta.") {
		return evalTACall(expr, ctx)
	}

	// Numeric literal
	if f, err := strconv.ParseFloat(expr, 64); err == nil {
		return constSeries(f, ctx), nil
	}

	// Arithmetic: a op b
	if vs, err := evalArithmetic(expr, ctx); err == nil {
		return vs, nil
	}

	return nil, fmt.Errorf("unknown expression: %q", expr)
}

// constSeries creates a ValueSeries filled with a constant.
func constSeries(f float64, ctx *evalContext) pine.ValueSeries {
	vs := pine.NewValueSeries()
	cur := ctx.series.GoToFirst()
	for cur != nil {
		vs.Set(cur.S, f)
		next, err := ctx.series.Next()
		if err != nil || next == nil {
			break
		}
		cur = next
	}
	if cur := ctx.series.Current(); cur != nil {
		vs.SetCurrent(cur.S)
	}
	return vs
}

// evalArithmetic handles simple binary operations.
func evalArithmetic(expr string, ctx *evalContext) (pine.ValueSeries, error) {
	ops := []byte{'+', '-', '*', '/'}
	for _, op := range ops {
		depth := 0
		for i := len(expr) - 1; i > 0; i-- {
			switch expr[i] {
			case ')':
				depth++
			case '(':
				depth--
			}
			if depth == 0 && expr[i] == op {
				a, err := evalExpr(expr[:i], ctx)
				if err != nil {
					return nil, err
				}
				b, err := evalExpr(expr[i+1:], ctx)
				if err != nil {
					return nil, err
				}
				switch op {
				case '+':
					return pine.Add(a, b), nil
				case '-':
					return pine.Sub(a, b), nil
				case '*':
					return pine.Mul(a, b), nil
				case '/':
					return pine.Div(a, b), nil
				}
			}
		}
	}
	return nil, fmt.Errorf("not arithmetic")
}

// evalTACall evaluates a ta.xxx(...) function call.
func evalTACall(expr string, ctx *evalContext) (pine.ValueSeries, error) {
	parenIdx := strings.Index(expr, "(")
	if parenIdx < 0 {
		return nil, fmt.Errorf("missing ( in %q", expr)
	}
	fn := expr[:parenIdx]
	argsStr := extractBetweenParens(expr[parenIdx:])
	args := splitTopLevel(argsStr, ',')

	switch fn {
	case "ta.sma":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.SMA(src, int64(l)), nil

	case "ta.ema":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.EMA(src, int64(l)), nil

	case "ta.rma":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.RMA(src, int64(l)), nil

	case "ta.rsi":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.RSI(src, int64(l)), nil

	case "ta.atr":
		l := intArg(args, 0, 14)
		tr := pine.OHLCVAttr(ctx.series, pine.OHLCPropTR)
		return pine.ATR(tr, int64(l)), nil

	case "ta.cci":
		l := intArg(args, 0, 20)
		tp := pine.OHLCVAttr(ctx.series, pine.OHLCPropHLC3)
		return pine.CCI(tp, int64(l)), nil

	case "ta.mfi":
		l := intArg(args, 0, 14)
		return pine.MFI(ctx.series, int64(l)), nil

	case "ta.stdev":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.Stdev(src, int64(l)), nil

	case "ta.variance":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.Variance(src, int64(l)), nil

	case "ta.roc":
		src, l, err := srcAndLen(args, 0, 1, 14, ctx)
		if err != nil {
			return nil, err
		}
		return pine.ROC(src, l), nil

	case "ta.change":
		src, l, err := srcAndLen(args, 0, 1, 1, ctx)
		if err != nil {
			return nil, err
		}
		return pine.Change(src, l), nil

	// Multi-return functions — return first value when used as single expr.
	case "ta.macd":
		results, err := evalMultiExpr(expr, ctx)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results[0], nil
		}

	case "ta.bb", "ta.bbands":
		results, err := evalMultiExpr(expr, ctx)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results[0], nil
		}

	case "ta.dmi":
		results, err := evalMultiExpr(expr, ctx)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results[0], nil
		}

	case "ta.kc":
		results, err := evalMultiExpr(expr, ctx)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results[0], nil
		}

	default:
		return nil, fmt.Errorf("unsupported function: %s", fn)
	}
	return nil, fmt.Errorf("function %s returned no results", fn)
}

// evalMultiExpr evaluates functions that return multiple ValueSeries.
func evalMultiExpr(expr string, ctx *evalContext) ([]pine.ValueSeries, error) {
	expr = strings.TrimSpace(expr)
	parenIdx := strings.Index(expr, "(")
	if parenIdx < 0 {
		return nil, fmt.Errorf("missing ( in %q", expr)
	}
	fn := expr[:parenIdx]
	argsStr := extractBetweenParens(expr[parenIdx:])
	args := splitTopLevel(argsStr, ',')

	switch fn {
	case "ta.macd":
		// ta.macd(source, fast, slow, signal)
		src, err := resolveSource(args, 0, ctx)
		if err != nil {
			return nil, err
		}
		fast := int64(intArg(args, 1, 12))
		slow := int64(intArg(args, 2, 26))
		sig := int64(intArg(args, 3, 9))
		macdLine, sigLine, histLine := pine.MACD(src, fast, slow, sig)
		return []pine.ValueSeries{macdLine, sigLine, histLine}, nil

	case "ta.bb", "ta.bbands":
		// ta.bb(source, length, mult)
		src, err := resolveSource(args, 0, ctx)
		if err != nil {
			return nil, err
		}
		l := int64(intArg(args, 1, 20))
		mult := floatArg(args, 2, 2.0)
		mid := pine.SMA(src, l)
		stdev := pine.Stdev(src, l)
		upper := pine.Add(mid, pine.MulConst(stdev, mult))
		lower := pine.Sub(mid, pine.MulConst(stdev, mult))
		return []pine.ValueSeries{mid, upper, lower}, nil

	case "ta.dmi":
		// ta.dmi(length, smooth)
		l := intArg(args, 0, 14)
		smooth := intArg(args, 1, 14)
		adx, plus, minus := pine.DMI(ctx.series, l, smooth)
		return []pine.ValueSeries{adx, plus, minus}, nil

	case "ta.kc":
		// ta.kc(source, length, mult)
		src, err := resolveSource(args, 0, ctx)
		if err != nil {
			return nil, err
		}
		l := int64(intArg(args, 1, 20))
		mult := floatArg(args, 2, 1.5)
		mid, upper, lower := pine.KC(src, ctx.series, l, mult, true)
		return []pine.ValueSeries{mid, upper, lower}, nil
	}
	return nil, fmt.Errorf("unknown multi-return function: %s", fn)
}

// ─── argument helpers ─────────────────────────────────────────────────────────

func srcAndLen(args []string, srcIdx, lenIdx, defLen int, ctx *evalContext) (pine.ValueSeries, int, error) {
	src, err := resolveSource(args, srcIdx, ctx)
	if err != nil {
		return nil, 0, err
	}
	return src, intArg(args, lenIdx, defLen), nil
}

func resolveSource(args []string, idx int, ctx *evalContext) (pine.ValueSeries, error) {
	if idx >= len(args) {
		return pine.OHLCVAttr(ctx.series, pine.OHLCPropClose), nil
	}
	return evalExpr(strings.TrimSpace(args[idx]), ctx)
}

func intArg(args []string, idx, def int) int {
	if idx >= len(args) {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(args[idx])); err == nil {
		return n
	}
	return def
}

func floatArg(args []string, idx int, def float64) float64 {
	if idx >= len(args) {
		return def
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(args[idx]), 64); err == nil {
		return f
	}
	return def
}

// ─── string helpers ───────────────────────────────────────────────────────────

// extractBetweenParens returns the content inside the outermost () pair.
func extractBetweenParens(s string) string {
	start := strings.Index(s, "(")
	if start < 0 {
		return s
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[start+1 : i]
			}
		}
	}
	return s[start+1:]
}

// splitTopLevel splits s by sep, ignoring separators inside brackets/parens.
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		default:
			if ch == sep && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// ─────────────────────────────────────────────────────────────────────────────
// Indicator evaluation helpers
// ─────────────────────────────────────────────────────────────────────────────

func evalIndicatorOutput(ind *IndicatorScript, bs []Bar) (*IndicatorOutput, error) {
	plots, err := evalScript(ind.Script, bs)
	if err != nil {
		return nil, err
	}
	return &IndicatorOutput{IndicatorID: ind.ID, Name: ind.Name, Plots: plots}, nil
}

func latestIndicatorValues(bs []Bar) []IndicatorUpdate {
	indMu.RLock()
	defs := make([]*IndicatorScript, 0, len(indicators))
	for _, d := range indicators {
		defs = append(defs, d)
	}
	indMu.RUnlock()
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })

	var updates []IndicatorUpdate
	for _, def := range defs {
		ohlcvs := barsToOHLCV(bs)
		series, err := pine.NewOHLCVSeries(ohlcvs)
		if err != nil {
			continue
		}
		for {
			v, err := series.Next()
			if err != nil || v == nil {
				break
			}
		}
		namedSeries, err := parsePinePlots(def.Script, series)
		if err != nil {
			log.Printf("eval error %s: %v", def.ID, err)
			continue
		}
		values := make(map[string]float64)
		for name, vs := range namedSeries {
			if val, ok := latestVal(vs, bs); ok {
				values[name] = val
			}
		}
		updates = append(updates, IndicatorUpdate{
			IndicatorID: def.ID,
			Name:        def.Name,
			Values:      values,
		})
	}
	return updates
}

// ─────────────────────────────────────────────────────────────────────────────
// WebSocket
// ─────────────────────────────────────────────────────────────────────────────

func broadcast(env WSEnvelope) {
	data, _ := json.Marshal(env)
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for conn := range clients {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}
}

func writeError(conn *websocket.Conn, msg string) {
	_ = conn.WriteJSON(WSEnvelope{Type: "error", Error: msg})
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws upgrade:", err)
		return
	}
	defer conn.Close()

	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
	}()

	// Send history on connect.
	barsMu.RLock()
	histCopy := make([]Bar, len(bars))
	copy(histCopy, bars)
	barsMu.RUnlock()
	if err := conn.WriteJSON(WSEnvelope{Type: "history", Bars: histCopy}); err != nil {
		return
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		var msgType string
		if err := json.Unmarshal(msg["type"], &msgType); err != nil {
			continue
		}

		switch msgType {
		case "add_indicator":
			var ind IndicatorScript
			if err := json.Unmarshal(msg["indicator"], &ind); err != nil {
				writeError(conn, "invalid indicator payload")
				continue
			}
			if ind.ID == "" || ind.Script == "" {
				writeError(conn, "id and script are required")
				continue
			}
			if ind.Name == "" {
				ind.Name = ind.ID
			}
			indMu.Lock()
			indicators[ind.ID] = &ind
			indMu.Unlock()

			barsMu.RLock()
			bsCopy := make([]Bar, len(bars))
			copy(bsCopy, bars)
			barsMu.RUnlock()

			out, err := evalIndicatorOutput(&ind, bsCopy)
			if err != nil {
				writeError(conn, fmt.Sprintf("eval error: %v", err))
				continue
			}
			_ = conn.WriteJSON(WSEnvelope{Type: "indicator_loaded", IndicatorOutput: out})

		case "remove_indicator":
			var id string
			if err := json.Unmarshal(msg["id"], &id); err != nil {
				continue
			}
			indMu.Lock()
			delete(indicators, id)
			indMu.Unlock()
			_ = conn.WriteJSON(WSEnvelope{Type: "indicator_removed", IndicatorID: id})
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tick loop
// ─────────────────────────────────────────────────────────────────────────────

func tickLoop() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		barsMu.Lock()
		prev := bars[len(bars)-1]
		nb := nextBar(prev, rng)
		bars = append(bars, nb)
		if len(bars) > 500 {
			bars = bars[len(bars)-500:]
		}
		bsCopy := make([]Bar, len(bars))
		copy(bsCopy, bars)
		barsMu.Unlock()

		updates := latestIndicatorValues(bsCopy)
		broadcast(WSEnvelope{Type: "tick", Bar: &nb, IndicatorUpdates: updates})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	barsMu.Lock()
	bars = seedHistory(300)
	barsMu.Unlock()

	go tickLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Println("pinescription-demo backend listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
