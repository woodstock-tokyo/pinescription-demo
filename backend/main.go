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

type PlotRenderOptions struct {
	Color        string   `json:"color,omitempty"`
	LineWidth    int      `json:"linewidth,omitempty"`
	LineStyle    int      `json:"linestyle,omitempty"`
	TrackPrice   *bool    `json:"trackprice,omitempty"`
	Display      *float64 `json:"display,omitempty"`
	Format       string   `json:"format,omitempty"`
	Precision    *int     `json:"precision,omitempty"`
	ForceOverlay *bool    `json:"force_overlay,omitempty"`
}

type DrawingPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type DrawingPolyline struct {
	Points    []DrawingPoint `json:"points"`
	Color     string         `json:"color"`
	LineWidth int            `json:"line_width"`
	Opacity   float64        `json:"opacity,omitempty"`
}

type DrawingBox struct {
	Left        int64   `json:"left"`
	Right       int64   `json:"right"`
	Top         float64 `json:"top"`
	Bottom      float64 `json:"bottom"`
	Color       string  `json:"color"`
	BorderColor string  `json:"border_color,omitempty"`
	Opacity     float64 `json:"opacity,omitempty"`
}

type DrawingLabel struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
	Color string  `json:"color"`
	Size  int     `json:"size,omitempty"`
}

type DashboardRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Color string `json:"color,omitempty"`
}

type DrawingDashboard struct {
	Title    string         `json:"title"`
	Rows     []DashboardRow `json:"rows"`
	Position string         `json:"position,omitempty"`
	Size     string         `json:"size,omitempty"`
}

type DrawingOutput struct {
	Polylines []DrawingPolyline `json:"polylines,omitempty"`
	Boxes     []DrawingBox      `json:"boxes,omitempty"`
	Labels    []DrawingLabel    `json:"labels,omitempty"`
	Dashboard *DrawingDashboard `json:"dashboard,omitempty"`
}

type IndicatorOutput struct {
	IndicatorID string                       `json:"indicator_id"`
	Name        string                       `json:"name"`
	Overlay     bool                         `json:"overlay"`
	Plots       map[string][]PlotPoint       `json:"plots"`
	PlotOptions map[string]PlotRenderOptions `json:"plot_options,omitempty"`
	Drawings    DrawingOutput                `json:"drawings,omitempty"`
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
		"trackprice",
		"histbase",
		"offset",
		"join",
		"editable",
		"show_last",
		"display",
		"format",
		"precision",
		"force_overlay",
		"linestyle",
	}

	chartPointFromIndexParamNames = []string{"index", "price"}
	polylineNewParamNames         = []string{"points", "curved", "closed", "xloc", "line_color", "fill_color", "line_style", "line_width", "force_overlay"}
	boxNewParamNames              = []string{"left", "top", "right", "bottom", "border_color", "border_width", "border_style", "extend", "xloc", "bgcolor", "text", "text_size", "text_color", "text_halign", "text_valign", "text_wrap", "force_overlay"}
	labelNewParamNames            = []string{"x", "y", "text", "xloc", "yloc", "color", "style", "textcolor", "size", "textalign", "tooltip", "force_overlay"}
	tableNewParamNames            = []string{"position", "columns", "rows", "bgcolor", "frame_color", "frame_width", "border_color", "border_width", "force_overlay"}
	tableCellParamNames           = []string{"table_id", "column", "row", "text", "width", "height", "text_color", "text_halign", "text_valign", "text_size", "bgcolor", "tooltip", "text_font_family"}
	tableClearParamNames          = []string{"table_id", "start_column", "start_row", "end_column", "end_row"}
	tableMergeCellsParamNames     = []string{"table_id", "start_column", "start_row", "end_column", "end_row"}
)

const (
	maxDrawingPolylines     = 160
	maxDrawingBoxes         = 160
	maxDrawingLabels        = 120
	maxDrawingPointsPerLine = 1200
	maxDrawingTableCells    = 120
	maxDashboardTextLength  = 96
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
	plotOptions   map[string]PlotRenderOptions
	nextBarByPlot map[string]int
	mu            sync.Mutex
}

func newPlotCollector(bs []Bar) *plotCollector {
	return &plotCollector{
		bars:          bs,
		plots:         make(map[string][]PlotPoint),
		plotOptions:   make(map[string]PlotRenderOptions),
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
	c.plotOptions[name] = plotOptionsFromArgs(args)
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

func (c *plotCollector) optionsSnapshot() map[string]PlotRenderOptions {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string]PlotRenderOptions, len(c.plotOptions))
	for name, options := range c.plotOptions {
		out[name] = options
	}
	return out
}

func plotOptionsFromArgs(args []interface{}) PlotRenderOptions {
	var options PlotRenderOptions
	if len(args) > 2 {
		options.Color = colorString(args[2])
	}
	if len(args) > 3 {
		if width, ok := toInt(args[3]); ok && width > 0 {
			options.LineWidth = width
		}
	}
	if len(args) > 5 {
		options.TrackPrice = boolOption(args[5])
	}
	if len(args) > 11 {
		if display, ok := toFloat64(args[11]); ok {
			options.Display = &display
		}
	}
	if len(args) > 12 {
		options.Format = toOptionalString(args[12])
	}
	if len(args) > 13 {
		if precision, ok := toInt(args[13]); ok {
			options.Precision = &precision
		}
	}
	if len(args) > 14 {
		options.ForceOverlay = boolOption(args[14])
	}
	if len(args) > 15 {
		if style, ok := toInt(args[15]); ok {
			options.LineStyle = style
		}
	}
	return options
}

type drawingCollector struct {
	nextTableID int
	bars        []Bar
	polylines   []DrawingPolyline
	boxes       []DrawingBox
	labels      []DrawingLabel
	tables      map[string]tableState
	tableCells  []tableCell
	mu          sync.Mutex
}

type tableState struct {
	ID       string
	Position string
}

type tableCell struct {
	TableID  string
	Column   int
	Row      int
	Text     string
	Color    string
	TextSize string
}

func newDrawingCollector(bs []Bar) *drawingCollector {
	return &drawingCollector{bars: bs, tables: map[string]tableState{}}
}

func (c *drawingCollector) register(engine *pine.Engine) error {
	registrations := []struct {
		name   string
		params []string
		fn     pine.UserFunction
	}{
		{"chart.point.from_index", chartPointFromIndexParamNames, c.chartPointFromIndex},
		{"polyline.new", polylineNewParamNames, c.polylineNew},
		{"polyline.delete", []string{"id"}, noopDrawingHook},
		{"box.new", boxNewParamNames, c.boxNew},
		{"box.delete", []string{"id"}, noopDrawingHook},
		{"label.new", labelNewParamNames, c.labelNew},
		{"label.delete", []string{"id"}, noopDrawingHook},
		{"table.new", tableNewParamNames, c.tableNew},
		{"table.cell", tableCellParamNames, c.tableCell},
		{"table.clear", tableClearParamNames, c.tableClear},
		{"table.merge_cells", tableMergeCellsParamNames, noopDrawingHook},
	}
	for _, reg := range registrations {
		if err := engine.RegisterFunctionWithParamNames(reg.name, reg.params, reg.fn); err != nil {
			return fmt.Errorf("register %s function: %w", reg.name, err)
		}
	}
	return nil
}

func noopDrawingHook(args ...interface{}) (interface{}, error) { return nil, nil }

func (c *drawingCollector) chartPointFromIndex(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, errors.New("chart.point.from_index expects index and price")
	}
	idx, ok := toFloat64(args[0])
	if !ok {
		return nil, nil
	}
	price, ok := toFloat64(args[1])
	if !ok || math.IsNaN(price) || math.IsInf(price, 0) {
		return nil, nil
	}
	return map[string]interface{}{"type": "chart.point", "index": idx, "price": price, "time": c.timeForIndex(int(math.Round(idx)))}, nil
}

func (c *drawingCollector) polylineNew(args ...interface{}) (interface{}, error) {
	points := drawingPointsFromValue(firstArg(args, 0))
	if len(points) > maxDrawingPointsPerLine {
		points = points[:maxDrawingPointsPerLine]
	}
	color := colorString(firstArg(args, 4))
	if color == "" {
		color = "rgba(91, 156, 246, 0.55)"
	}
	lineWidth := 1
	if width, ok := toInt(firstArg(args, 7)); ok && width > 0 {
		lineWidth = width
	}
	polyline := DrawingPolyline{Points: points, Color: color, LineWidth: lineWidth, Opacity: 0.85}
	c.mu.Lock()
	if len(c.polylines) < maxDrawingPolylines {
		c.polylines = append(c.polylines, polyline)
	}
	c.mu.Unlock()
	return map[string]interface{}{"type": "polyline", "points": firstArg(args, 0)}, nil
}

func (c *drawingCollector) boxNew(args ...interface{}) (interface{}, error) {
	left, lok := toFloat64(firstArg(args, 0))
	top, tok := toFloat64(firstArg(args, 1))
	right, rok := toFloat64(firstArg(args, 2))
	bottom, bok := toFloat64(firstArg(args, 3))
	if !lok || !tok || !rok || !bok || math.IsNaN(top) || math.IsNaN(bottom) {
		return map[string]interface{}{"type": "box"}, nil
	}
	fill := colorString(firstArg(args, 9))
	if fill == "" {
		fill = colorString(firstArg(args, 4))
	}
	if fill == "" {
		fill = "rgba(91, 156, 246, 0.25)"
	}
	border := colorString(firstArg(args, 4))
	if border == "" {
		border = fill
	}
	box := DrawingBox{Left: c.timeForIndex(int(math.Round(left))), Right: c.timeForIndex(int(math.Round(right))), Top: top, Bottom: bottom, Color: fill, BorderColor: border, Opacity: 0.5}
	c.mu.Lock()
	if len(c.boxes) < maxDrawingBoxes {
		c.boxes = append(c.boxes, box)
	}
	c.mu.Unlock()
	return map[string]interface{}{"type": "box", "left": left, "top": top, "right": right, "bottom": bottom}, nil
}

func (c *drawingCollector) labelNew(args ...interface{}) (interface{}, error) {
	x, xok := toFloat64(firstArg(args, 0))
	y, yok := toFloat64(firstArg(args, 1))
	if !xok || !yok || math.IsNaN(y) || math.IsInf(y, 0) {
		return map[string]interface{}{"type": "label"}, nil
	}
	color := colorString(firstArg(args, 5))
	if color == "" {
		color = colorString(firstArg(args, 7))
	}
	if color == "" {
		color = "#ffffff"
	}
	size := 6
	if rawSize, ok := toInt(firstArg(args, 8)); ok {
		size = 5 + rawSize*2
	}
	label := DrawingLabel{Time: c.timeForIndex(int(math.Round(x))), Value: y, Color: color, Size: size}
	c.mu.Lock()
	if len(c.labels) < maxDrawingLabels {
		c.labels = append(c.labels, label)
	}
	c.mu.Unlock()
	return map[string]interface{}{"type": "label", "x": x, "y": y}, nil
}

func (c *drawingCollector) tableNew(args ...interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextTableID++
	id := fmt.Sprintf("table-%d", c.nextTableID)
	c.tables[id] = tableState{ID: id, Position: tablePositionString(firstArg(args, 0))}
	return map[string]interface{}{"type": "table", "id": id, "position": c.tables[id].Position}, nil
}

func (c *drawingCollector) tableCell(args ...interface{}) (interface{}, error) {
	column, cok := toInt(firstArg(args, 1))
	row, rok := toInt(firstArg(args, 2))
	if !cok || !rok {
		return nil, nil
	}
	cell := tableCell{TableID: tableIDFromValue(firstArg(args, 0)), Column: column, Row: row, Text: truncateText(toOptionalString(firstArg(args, 3)), maxDashboardTextLength), Color: colorString(firstArg(args, 6)), TextSize: tableSizeString(firstArg(args, 9))}
	c.mu.Lock()
	if len(c.tableCells) < maxDrawingTableCells {
		c.tableCells = append(c.tableCells, cell)
	}
	c.mu.Unlock()
	return nil, nil
}

func (c *drawingCollector) tableClear(args ...interface{}) (interface{}, error) {
	tableID := tableIDFromValue(firstArg(args, 0))
	startColumn, hasStartColumn := toInt(firstArg(args, 1))
	startRow, hasStartRow := toInt(firstArg(args, 2))
	endColumn, hasEndColumn := toInt(firstArg(args, 3))
	endRow, hasEndRow := toInt(firstArg(args, 4))
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.tableCells[:0]
	for _, cell := range c.tableCells {
		if tableID != "" && cell.TableID != tableID {
			kept = append(kept, cell)
			continue
		}
		inColumn := !hasStartColumn || cell.Column >= startColumn
		inRow := !hasStartRow || cell.Row >= startRow
		if hasEndColumn {
			inColumn = inColumn && cell.Column <= endColumn
		}
		if hasEndRow {
			inRow = inRow && cell.Row <= endRow
		}
		if !(inColumn && inRow) {
			kept = append(kept, cell)
		}
	}
	c.tableCells = kept
	return nil, nil
}

func (c *drawingCollector) snapshot() DrawingOutput {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := DrawingOutput{
		Polylines: append([]DrawingPolyline(nil), c.polylines...),
		Boxes:     append([]DrawingBox(nil), c.boxes...),
		Labels:    append([]DrawingLabel(nil), c.labels...),
	}
	if dashboard := dashboardFromTableCells(c.tables, c.tableCells); dashboard != nil {
		out.Dashboard = dashboard
	}
	return out
}

func (c *drawingCollector) timeForIndex(index int) int64 {
	if len(c.bars) == 0 {
		return int64(index * 60)
	}
	if index >= 0 && index < len(c.bars) {
		return c.bars[index].Time
	}
	return c.bars[0].Time + int64(index)*60
}

func drawingPointsFromValue(v interface{}) []DrawingPoint {
	items := arrayItems(v)
	points := make([]DrawingPoint, 0, len(items))
	for _, item := range items {
		pointMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		t, tok := toFloat64(pointMap["time"])
		price, pok := toFloat64(pointMap["price"])
		if !tok || !pok || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}
		points = append(points, DrawingPoint{Time: int64(math.Round(t)), Value: price})
	}
	return points
}

func arrayItems(v interface{}) []interface{} {
	switch arr := v.(type) {
	case nil:
		return nil
	case []interface{}:
		return arr
	case pine.ArrayValue:
		return arr.PineArrayItems()
	default:
		return nil
	}
}

func dashboardFromTableCells(tables map[string]tableState, cells []tableCell) *DrawingDashboard {
	if len(cells) == 0 {
		return nil
	}
	sorted := append([]tableCell(nil), cells...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].TableID != sorted[j].TableID {
			return sorted[i].TableID < sorted[j].TableID
		}
		if sorted[i].Row == sorted[j].Row {
			return sorted[i].Column < sorted[j].Column
		}
		return sorted[i].Row < sorted[j].Row
	})
	selectedTableID := sorted[0].TableID
	filtered := sorted[:0]
	for _, cell := range sorted {
		if cell.TableID == selectedTableID {
			filtered = append(filtered, cell)
		}
	}
	rowsByIndex := map[int]map[int]tableCell{}
	rowIndexes := make([]int, 0)
	dashboardSize := "normal"
	for _, cell := range filtered {
		if cell.Text == "" {
			continue
		}
		if cell.TextSize != "" && dashboardSize == "normal" {
			dashboardSize = cell.TextSize
		}
		if _, ok := rowsByIndex[cell.Row]; !ok {
			rowsByIndex[cell.Row] = map[int]tableCell{}
			rowIndexes = append(rowIndexes, cell.Row)
		}
		rowsByIndex[cell.Row][cell.Column] = cell
	}
	if len(rowIndexes) == 0 {
		return nil
	}
	dashboard := &DrawingDashboard{Position: tablePositionString(nil), Size: dashboardSize}
	if table, ok := tables[selectedTableID]; ok && table.Position != "" {
		dashboard.Position = table.Position
	}
	for _, rowIdx := range rowIndexes {
		row := rowsByIndex[rowIdx]
		left := row[0]
		right := row[1]
		if dashboard.Title == "" && right.Text == "" {
			dashboard.Title = left.Text
			continue
		}
		if left.Text == "" && right.Text == "" {
			continue
		}
		dashboard.Rows = append(dashboard.Rows, DashboardRow{Label: left.Text, Value: right.Text, Color: right.Color})
	}
	if dashboard.Title == "" {
		dashboard.Title = "Indicator Table"
	}
	if len(dashboard.Rows) == 0 && dashboard.Title == "" {
		return nil
	}
	return dashboard
}

func tableIDFromValue(v interface{}) string {
	if tableMap, ok := v.(map[string]interface{}); ok {
		if id, ok := tableMap["id"].(string); ok {
			return id
		}
	}
	return ""
}

func tablePositionString(v interface{}) string {
	position, ok := toInt(v)
	if !ok {
		return "top_right"
	}
	switch position {
	case 1:
		return "bottom_right"
	case 2:
		return "top_left"
	case 3:
		return "bottom_left"
	default:
		return "top_right"
	}
}

func tableSizeString(v interface{}) string {
	size, ok := toInt(v)
	if !ok {
		return ""
	}
	switch size {
	case 0:
		return "tiny"
	case 1:
		return "small"
	case 3:
		return "large"
	case 4:
		return "huge"
	case 5:
		return "auto"
	default:
		return "normal"
	}
}

func truncateText(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen-1]) + "…"
}

func firstArg(args []interface{}, index int) interface{} {
	if index < 0 || index >= len(args) {
		return nil
	}
	return args[index]
}

func normalizeScript(script string) (string, error) {
	lines := strings.Split(script, "\n")
	cleaned := make([]string, 0, len(lines))

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t\r")
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "//") {
			continue
		}
		// split line by ; into lines and trim each part, ignoring empty parts
		parts := strings.Split(line, ";")
		for _, part := range parts {
			trimmed := strings.TrimRight(part, " \t\r")
			if len(parts) > 1 {
				trimmed = strings.TrimSpace(trimmed)
			}
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
	}

	if len(cleaned) == 0 {
		return "", errors.New("script has no executable statements")
	}

	return strings.Join(cleaned, "\n"), nil
}

func indicatorOverlay(script string) bool {
	const pineDefaultOverlay = false

	call, ok, err := leadingCallText(script, "indicator")
	if err != nil || !ok {
		return pineDefaultOverlay
	}

	openIdx := strings.IndexByte(call, '(')
	if openIdx < 0 {
		return pineDefaultOverlay
	}
	closeIdx, err := findMatchingParen(call, openIdx)
	if err != nil {
		return pineDefaultOverlay
	}

	args := splitTopLevelArgs(call[openIdx+1 : closeIdx])
	positional := 0
	for _, arg := range args {
		key, value, hasKey := splitKeywordArg(arg)
		if hasKey {
			if strings.EqualFold(strings.TrimSpace(key), "overlay") {
				return parseBoolLiteral(value, pineDefaultOverlay)
			}
			continue
		}

		positional++
		if positional == 3 {
			return parseBoolLiteral(arg, pineDefaultOverlay)
		}
	}

	return pineDefaultOverlay
}

func stripLeadingCallFromScript(script, name string) (string, error) {
	callStart, openIdx, ok := leadingCallBounds(script, name)
	if !ok {
		return script, nil
	}

	closeIdx, err := findMatchingParen(script, openIdx)
	if err != nil {
		return "", err
	}

	remainder := strings.TrimSpace(script[closeIdx+1:])
	if strings.HasPrefix(remainder, ";") {
		remainder = strings.TrimSpace(remainder[1:])
	}

	return script[:callStart] + remainder, nil
}

func leadingCallText(script, name string) (string, bool, error) {
	_, openIdx, ok := leadingCallBounds(script, name)
	if !ok {
		return "", false, nil
	}

	closeIdx, err := findMatchingParen(script, openIdx)
	if err != nil {
		return "", false, err
	}

	start := openIdx
	for start > 0 && (script[start-1] == ' ' || script[start-1] == '\t') {
		start--
	}
	for start > 0 && isIdentifierByte(script[start-1]) {
		start--
	}

	return script[start : closeIdx+1], true, nil
}

func leadingCallBounds(script, name string) (int, int, bool) {
	offset := 0
	for _, rawLine := range strings.SplitAfter(script, "\n") {
		line := strings.TrimRight(rawLine, "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			offset += len(rawLine)
			continue
		}
		if !startsWithCall(trimmed, name) {
			return 0, 0, false
		}

		lineBody := strings.TrimLeft(line, " \t")
		callStart := offset + len(line) - len(lineBody)
		openInTrimmed := strings.IndexByte(trimmed, '(')
		if openInTrimmed < 0 {
			return 0, 0, false
		}
		return callStart, callStart + openInTrimmed, true
	}

	return 0, 0, false
}

func isIdentifierByte(ch byte) bool {
	return ch == '_' || ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func splitTopLevelArgs(s string) []string {
	args := []string{}
	start := 0
	depth := 0
	var quote byte
	escaped := false

	for i := 0; i < len(s); i++ {
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
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(s[start:])
	if last != "" || len(args) > 0 {
		args = append(args, last)
	}
	return args
}

func splitKeywordArg(arg string) (string, string, bool) {
	depth := 0
	var quote byte
	escaped := false

	for i := 0; i < len(arg); i++ {
		ch := arg[i]
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
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '=':
			if depth == 0 {
				return strings.TrimSpace(arg[:i]), strings.TrimSpace(arg[i+1:]), true
			}
		}
	}

	return "", "", false
}

func parseBoolLiteral(s string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.Trim(strings.TrimSpace(s), "'\""))
	if err != nil {
		return fallback
	}
	return parsed
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

func toOptionalString(v interface{}) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func boolOption(v interface{}) *bool {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case bool:
		return &x
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(x))
		if err == nil {
			return &parsed
		}
	}
	if f, ok := toFloat64(v); ok {
		parsed := f != 0
		return &parsed
	}
	return nil
}

func colorString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	if base, ok := m["base"]; ok {
		baseColor := colorString(base)
		if baseColor == "" {
			return ""
		}
		transp, _ := toFloat64(m["transp"])
		alpha := math.Max(0, math.Min(1, 1-transp/100))
		return hexWithAlpha(baseColor, alpha)
	}
	r, rok := toFloat64(m["r"])
	g, gok := toFloat64(m["g"])
	b, bok := toFloat64(m["b"])
	if !rok || !gok || !bok {
		return ""
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", clampByte(r), clampByte(g), clampByte(b))
}

func clampByte(v float64) int {
	return int(math.Max(0, math.Min(255, math.Round(v))))
}

func hexWithAlpha(color string, alpha float64) string {
	if !strings.HasPrefix(color, "#") || len(color) != 7 {
		return color
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", hexByte(color[1:3]), hexByte(color[3:5]), hexByte(color[5:7]), alpha)
}

func hexByte(s string) int {
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0
	}
	return int(v)
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
	plots, _, _, err := evalScriptWithOptions(script, bs)
	return plots, err
}

func evalScriptWithOptions(script string, bs []Bar) (map[string][]PlotPoint, map[string]PlotRenderOptions, DrawingOutput, error) {
	normalized, err := normalizeScript(script)
	if err != nil {
		return nil, nil, DrawingOutput{}, err
	}

	provider := newBarProvider("DEMO", bs)
	if err := ensureRequiredValueTypes(provider, requiredOHLCVValueTypes); err != nil {
		return nil, nil, DrawingOutput{}, err
	}
	collector := newPlotCollector(bs)
	drawings := newDrawingCollector(bs)

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
		return nil, nil, DrawingOutput{}, fmt.Errorf("register plot function: %w", err)
	}
	if err := drawings.register(engine); err != nil {
		return nil, nil, DrawingOutput{}, err
	}

	bytecode, err := engine.Compile(normalized)
	if err != nil {
		return nil, nil, DrawingOutput{}, fmt.Errorf("compile failed: %w", err)
	}

	v, err := engine.Execute(bytecode)
	if err != nil {
		return nil, nil, DrawingOutput{}, fmt.Errorf("execute failed: %w", err)
	}

	plots := collector.snapshot()
	plotOptions := collector.optionsSnapshot()
	if len(plots) == 0 && len(bs) > 0 {
		if fv, ok := toFloat64(v); ok && !math.IsNaN(fv) && !math.IsInf(fv, 0) {
			plots["result"] = []PlotPoint{{Time: bs[len(bs)-1].Time, Value: fv}}
		}
	}

	return plots, plotOptions, drawings.snapshot(), nil
}

func evalIndicatorOutput(ind *IndicatorScript, bs []Bar) (*IndicatorOutput, error) {
	plots, plotOptions, drawings, err := evalScriptWithOptions(ind.Script, bs)
	if err != nil {
		return nil, err
	}
	return &IndicatorOutput{
		IndicatorID: ind.ID,
		Name:        ind.Name,
		Overlay:     indicatorOverlay(ind.Script),
		Plots:       plots,
		PlotOptions: plotOptions,
		Drawings:    drawings,
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
			bars = seedHistory(1200)
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
	bars = seedHistory(1200)
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
