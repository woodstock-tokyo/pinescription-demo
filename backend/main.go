package main

import (
	"encoding/json"
	"errors"
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
	pine "github.com/woodstock-tokyo/pinescription"
	pseries "github.com/woodstock-tokyo/pinescription/series"
)

type Bar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type IndicatorScript struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

type PlotPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type IndicatorOutput struct {
	IndicatorID string                 `json:"indicator_id"`
	Name        string                 `json:"name"`
	Plots       map[string][]PlotPoint `json:"plots"`
}

type IndicatorUpdate struct {
	IndicatorID string             `json:"indicator_id"`
	Name        string             `json:"name"`
	Values      map[string]float64 `json:"values"`
}

type WSEnvelope struct {
	Type             string            `json:"type"`
	Bar              *Bar              `json:"bar,omitempty"`
	Bars             []Bar             `json:"bars,omitempty"`
	Indicator        *IndicatorScript  `json:"indicator,omitempty"`
	IndicatorID      string            `json:"indicator_id,omitempty"`
	IndicatorOutput  *IndicatorOutput  `json:"indicator_output,omitempty"`
	IndicatorUpdates []IndicatorUpdate `json:"indicator_updates,omitempty"`
	ID               string            `json:"id,omitempty"`
	Error            string            `json:"error,omitempty"`
}

var (
	bars   []Bar
	barsMu sync.RWMutex

	indicators = map[string]*IndicatorScript{}
	indMu      sync.RWMutex

	clients   = map[*websocket.Conn]bool{}
	clientsMu sync.RWMutex

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(_ *http.Request) bool { return true },
	}

	plotParamNames = []string{
		"series",
		"title",
		"color",
		"linewidth",
		"style",
		"transp",
		"trackprice",
		"histbase",
		"offset",
		"join",
		"editable",
		"show_last",
		"display",
		"scale",
	}
)

func seedHistory(n int) []Bar {
	if n < 2 {
		n = 2
	}
	out := make([]Bar, 0, n)
	px := 45000.0
	t := time.Now().UTC().Add(-time.Duration(n) * time.Minute)

	for i := 0; i < n; i++ {
		open := px
		move := (rand.Float64() - 0.5) * 320.0
		close := math.Max(100, open+move)
		high := math.Max(open, close) + rand.Float64()*90.0
		low := math.Min(open, close) - rand.Float64()*90.0
		if low < 0 {
			low = 0
		}
		volume := 100 + rand.Float64()*900

		out = append(out, Bar{
			Time:   t.Unix(),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: volume,
		})

		px = close
		t = t.Add(time.Minute)
	}

	return out
}

func nextBar(prev Bar) Bar {
	open := prev.Close
	move := (rand.Float64() - 0.5) * 260.0
	close := math.Max(100, open+move)
	high := math.Max(open, close) + rand.Float64()*70.0
	low := math.Min(open, close) - rand.Float64()*70.0
	if low < 0 {
		low = 0
	}
	volume := 100 + rand.Float64()*900

	return Bar{
		Time:   prev.Time + 60,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}
}

type barProvider struct {
	symbol    string
	bars      []Bar
	timeframe string
	session   string
}

func newBarProvider(symbol string, bs []Bar) *barProvider {
	if symbol == "" {
		symbol = "DEMO"
	}
	return &barProvider{
		symbol:    symbol,
		bars:      bs,
		timeframe: "1",
		session:   "regular",
	}
}

func (p *barProvider) GetSeries(seriesKey string) (pine.SeriesExtended, error) {
	symbol, valueType, err := parseSeriesKey(seriesKey)
	if err != nil {
		return nil, err
	}
	if symbol != p.symbol {
		return nil, fmt.Errorf("unknown symbol: %s", symbol)
	}

	size := len(p.bars)
	if size < 1 {
		size = 1
	}
	q := pseries.NewQueue(size)
	for _, b := range p.bars {
		v, err := valueFromBar(b, valueType)
		if err != nil {
			return nil, err
		}
		q.Update(v)
	}
	return q, nil
}

func (p *barProvider) GetSymbols() ([]string, error) {
	return []string{p.symbol}, nil
}

func (p *barProvider) GetValuesTypes() ([]string, error) {
	return []string{"open", "high", "low", "close", "volume", "hl2", "hlc3"}, nil
}

func (p *barProvider) SetTimeframe(tf string) error {
	tf = strings.TrimSpace(tf)
	if tf == "" {
		return nil
	}
	p.timeframe = tf
	return nil
}

func (p *barProvider) GetTimeframe() string {
	if p.timeframe == "" {
		return "1"
	}
	return p.timeframe
}

func (p *barProvider) SetSession(session string) error {
	session = strings.TrimSpace(session)
	if session == "" {
		return nil
	}
	p.session = session
	return nil
}

func (p *barProvider) GetSession() string {
	if p.session == "" {
		return "regular"
	}
	return p.session
}

func parseSeriesKey(seriesKey string) (string, string, error) {
	parts := strings.Split(seriesKey, "|")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid series key %q (expected symbol|value_type)", seriesKey)
	}
	symbol := strings.TrimSpace(parts[0])
	valueType := strings.ToLower(strings.TrimSpace(parts[1]))
	if symbol == "" || valueType == "" {
		return "", "", fmt.Errorf("invalid series key %q", seriesKey)
	}
	return symbol, valueType, nil
}

func valueFromBar(b Bar, valueType string) (float64, error) {
	switch strings.ToLower(valueType) {
	case "open":
		return b.Open, nil
	case "high":
		return b.High, nil
	case "low":
		return b.Low, nil
	case "close":
		return b.Close, nil
	case "volume":
		return b.Volume, nil
	case "hl2":
		return (b.High + b.Low) / 2.0, nil
	case "hlc3":
		return (b.High + b.Low + b.Close) / 3.0, nil
	default:
		return 0, fmt.Errorf("unsupported value_type %q", valueType)
	}
}

type plotCollector struct {
	bars          []Bar
	plots         map[string][]PlotPoint
	nextBarByPlot map[string]int
	mu            sync.Mutex
}

func newPlotCollector(bs []Bar) *plotCollector {
	return &plotCollector{
		bars:          bs,
		plots:         make(map[string][]PlotPoint),
		nextBarByPlot: make(map[string]int),
	}
}

func (c *plotCollector) capture(args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return math.NaN(), errors.New("plot expects at least 1 argument")
	}

	name := "plot"
	if len(args) >= 2 {
		name = toName(args[1], name)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	barIdx := c.nextBarByPlot[name]
	c.nextBarByPlot[name] = barIdx + 1
	if barIdx < 0 || barIdx >= len(c.bars) {
		return args[0], nil
	}

	v, ok := toFloat64(args[0])
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return args[0], nil
	}

	point := PlotPoint{Time: c.bars[barIdx].Time, Value: v}
	c.plots[name] = append(c.plots[name], point)

	return args[0], nil
}

func (c *plotCollector) snapshot() map[string][]PlotPoint {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string][]PlotPoint, len(c.plots))
	for name, pts := range c.plots {
		copied := make([]PlotPoint, len(pts))
		copy(copied, pts)
		out[name] = copied
	}
	return out
}

func normalizeScript(script string) (string, error) {
	lines := strings.Split(script, "\n")
	cleaned := make([]string, 0, len(lines))

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		stripped, strippedIndicator, err := stripLeadingCall(line, "indicator")
		if err != nil {
			return "", err
		}
		if strippedIndicator {
			line = stripped
		}
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		cleaned = append(cleaned, line)
	}

	if len(cleaned) == 0 {
		return "", errors.New("script has no executable statements")
	}

	return strings.Join(cleaned, "\n"), nil
}

func stripLeadingCall(line, name string) (string, bool, error) {
	if !startsWithCall(line, name) {
		return line, false, nil
	}

	openIdx := -1
	for i := len(name); i < len(line); i++ {
		if line[i] == '(' {
			openIdx = i
			break
		}
	}
	if openIdx < 0 {
		return "", true, fmt.Errorf("invalid %s(...) call", name)
	}

	closeIdx, err := findMatchingParen(line, openIdx)
	if err != nil {
		return "", true, err
	}

	remainder := strings.TrimSpace(line[closeIdx+1:])
	if strings.HasPrefix(remainder, ";") {
		remainder = strings.TrimSpace(remainder[1:])
	}

	return remainder, true, nil
}

func startsWithCall(line, name string) bool {
	if !strings.HasPrefix(line, name) {
		return false
	}
	i := len(name)
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i < len(line) && line[i] == '('
}

func findMatchingParen(s string, openIdx int) (int, error) {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '(' {
		return -1, errors.New("invalid opening parenthesis")
	}

	depth := 0
	var quote byte
	escaped := false

	for i := openIdx; i < len(s); i++ {
		ch := s[i]

		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}

	return -1, errors.New("unmatched parentheses")
}

func toName(v interface{}, fallback string) string {
	if v == nil {
		return fallback
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return fallback
		}
		return s
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return fallback
	}
	return s
}

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func toInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

var requiredOHLCVValueTypes = []string{"open", "high", "low", "close", "volume"}

func ensureRequiredValueTypes(provider *barProvider, required []string) error {
	valueTypes, err := provider.GetValuesTypes()
	if err != nil {
		return fmt.Errorf("get provider value types: %w", err)
	}

	available := make(map[string]struct{}, len(valueTypes))
	for _, valueType := range valueTypes {
		available[strings.ToLower(strings.TrimSpace(valueType))] = struct{}{}
	}

	for _, valueType := range required {
		key := strings.ToLower(strings.TrimSpace(valueType))
		if key == "" {
			continue
		}
		if _, ok := available[key]; !ok {
			return fmt.Errorf("market data provider missing required value_type %q", key)
		}
	}

	return nil
}

func evalScript(script string, bs []Bar) (map[string][]PlotPoint, error) {
	normalized, err := normalizeScript(script)
	if err != nil {
		return nil, err
	}

	provider := newBarProvider("DEMO", bs)
	if err := ensureRequiredValueTypes(provider, requiredOHLCVValueTypes); err != nil {
		return nil, err
	}
	collector := newPlotCollector(bs)

	engine := pine.NewEngine()
	engine.RegisterMarketDataProvider(provider)
	engine.SetDefaultSymbol(provider.symbol)
	engine.SetDefaultValueType("close")
	engine.SetTimeframe(provider.GetTimeframe())
	engine.SetSession(provider.GetSession())
	if len(bs) > 0 {
		engine.SetStartTime(time.Unix(bs[0].Time, 0).UTC())
		engine.SetCurrentTime(time.Unix(bs[len(bs)-1].Time, 0).UTC())
	}

	if err := engine.RegisterFunctionWithParamNames("plot", plotParamNames, collector.capture); err != nil {
		return nil, fmt.Errorf("register plot function: %w", err)
	}

	bytecode, err := engine.Compile(normalized)
	if err != nil {
		return nil, fmt.Errorf("compile failed: %w", err)
	}

	v, err := engine.Execute(bytecode)
	if err != nil {
		return nil, fmt.Errorf("execute failed: %w", err)
	}

	plots := collector.snapshot()
	if len(plots) == 0 && len(bs) > 0 {
		if fv, ok := toFloat64(v); ok && !math.IsNaN(fv) && !math.IsInf(fv, 0) {
			plots["result"] = []PlotPoint{{Time: bs[len(bs)-1].Time, Value: fv}}
		}
	}

	return plots, nil
}

func evalIndicatorOutput(ind *IndicatorScript, bs []Bar) (*IndicatorOutput, error) {
	plots, err := evalScript(ind.Script, bs)
	if err != nil {
		return nil, err
	}
	return &IndicatorOutput{
		IndicatorID: ind.ID,
		Name:        ind.Name,
		Plots:       plots,
	}, nil
}

func barsSnapshot() []Bar {
	barsMu.RLock()
	defer barsMu.RUnlock()
	out := make([]Bar, len(bars))
	copy(out, bars)
	return out
}

func indicatorsSnapshot() []*IndicatorScript {
	indMu.RLock()
	defer indMu.RUnlock()

	out := make([]*IndicatorScript, 0, len(indicators))
	for _, ind := range indicators {
		if ind == nil {
			continue
		}
		cp := *ind
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func latestIndicatorValues(bs []Bar) []IndicatorUpdate {
	inds := indicatorsSnapshot()
	updates := make([]IndicatorUpdate, 0, len(inds))

	for _, ind := range inds {
		plots, err := evalScript(ind.Script, bs)
		if err != nil {
			log.Printf("eval error %s: %v", ind.ID, err)
			continue
		}

		vals := make(map[string]float64)
		for name, pts := range plots {
			if len(pts) == 0 {
				continue
			}
			vals[name] = pts[len(pts)-1].Value
		}
		if len(vals) == 0 {
			continue
		}

		updates = append(updates, IndicatorUpdate{
			IndicatorID: ind.ID,
			Name:        ind.Name,
			Values:      vals,
		})
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].IndicatorID < updates[j].IndicatorID
	})

	return updates
}

func broadcast(env WSEnvelope) {
	b, _ := json.Marshal(env)

	clientsMu.RLock()
	conns := make([]*websocket.Conn, 0, len(clients))
	for c := range clients {
		conns = append(conns, c)
	}
	clientsMu.RUnlock()

	for _, c := range conns {
		if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			clientsMu.Lock()
			delete(clients, c)
			clientsMu.Unlock()
			_ = c.Close()
		}
	}
}

func writeError(c *websocket.Conn, msg string) {
	_ = c.WriteJSON(WSEnvelope{Type: "error", Error: msg})
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer conn.Close()

	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	_ = conn.WriteJSON(WSEnvelope{Type: "history", Bars: barsSnapshot()})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			return
		}

		var req WSEnvelope
		if err := json.Unmarshal(raw, &req); err != nil {
			writeError(conn, "bad request JSON")
			continue
		}

		switch req.Type {
		case "add_indicator":
			if req.Indicator == nil {
				writeError(conn, "missing indicator payload")
				continue
			}

			ind := &IndicatorScript{
				ID:     strings.TrimSpace(req.Indicator.ID),
				Name:   strings.TrimSpace(req.Indicator.Name),
				Script: req.Indicator.Script,
			}
			if ind.ID == "" {
				writeError(conn, "indicator.id is required")
				continue
			}
			if strings.TrimSpace(ind.Script) == "" {
				writeError(conn, "indicator.script is required")
				continue
			}
			if ind.Name == "" {
				ind.Name = ind.ID
			}

			indMu.Lock()
			indicators[ind.ID] = ind
			indMu.Unlock()

			output, err := evalIndicatorOutput(ind, barsSnapshot())
			if err != nil {
				indMu.Lock()
				delete(indicators, ind.ID)
				indMu.Unlock()
				writeError(conn, fmt.Sprintf("eval error: %v", err))
				continue
			}

			_ = conn.WriteJSON(WSEnvelope{Type: "indicator_loaded", IndicatorOutput: output})

		case "remove_indicator":
			id := strings.TrimSpace(req.ID)
			if id == "" {
				writeError(conn, "id is required")
				continue
			}

			indMu.Lock()
			delete(indicators, id)
			indMu.Unlock()

			_ = conn.WriteJSON(WSEnvelope{Type: "indicator_removed", IndicatorID: id})

		default:
			writeError(conn, "unknown type: "+req.Type)
		}
	}
}

func tickLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		barsMu.Lock()
		if len(bars) == 0 {
			bars = seedHistory(300)
		}
		next := nextBar(bars[len(bars)-1])
		bars = append(bars, next)
		if len(bars) > 500 {
			bars = bars[len(bars)-500:]
		}
		bsCopy := make([]Bar, len(bars))
		copy(bsCopy, bars)
		barsMu.Unlock()

		updates := latestIndicatorValues(bsCopy)

		broadcast(WSEnvelope{
			Type:             "tick",
			Bar:              &next,
			IndicatorUpdates: updates,
		})
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	barsMu.Lock()
	bars = seedHistory(300)
	barsMu.Unlock()

	go tickLoop()

	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":8080"
	log.Printf("backend listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
