package main

import (
	"math"
	"os"
	"strings"
	"testing"

	pine "github.com/woodstock-tokyo/pinescription"
)

func testBars() []Bar {
	return []Bar{
		{Time: 1000, Open: 10, High: 14, Low: 8, Close: 12, Volume: 100},
		{Time: 1060, Open: 12, High: 15, Low: 11, Close: 14, Volume: 120},
		{Time: 1120, Open: 14, High: 17, Low: 13, Close: 16, Volume: 140},
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestEvalExample2EmitsDrawingOutput(t *testing.T) {
	script, err := os.ReadFile("../example2.pine")
	if err != nil {
		t.Fatalf("read example2.pine: %v", err)
	}

	mutatedTitleScript := strings.Replace(string(script), "Volumetric Regression Heatmap [LuxAlgo]", "Runtime Drawing Hook Smoke", 1)
	output, err := evalIndicatorOutput(&IndicatorScript{
		ID:     "example2",
		Name:   "Runtime Drawing Hook Smoke",
		Script: mutatedTitleScript,
	}, seedHistory(1200))
	if err != nil {
		t.Fatalf("evalIndicatorOutput failed: %v", err)
	}
	if !output.Overlay {
		t.Fatalf("overlay = false, want true")
	}
	if len(output.Drawings.Polylines) == 0 {
		t.Fatalf("expected example2 to emit heatmap polylines")
	}
	if len(output.Drawings.Boxes) == 0 {
		t.Fatalf("expected example2 to emit volume profile boxes")
	}
	if output.Drawings.Dashboard == nil {
		t.Fatalf("expected example2 to emit dashboard table")
	}
	if output.Drawings.Dashboard.Position != "top_right" {
		t.Fatalf("dashboard position = %q, want top_right", output.Drawings.Dashboard.Position)
	}
	if output.Drawings.Dashboard.Size != "small" {
		t.Fatalf("dashboard size = %q, want small", output.Drawings.Dashboard.Size)
	}
}

func TestEvalScriptCollectsTablePositionSizeAndClear(t *testing.T) {
	script := `indicator("Table Clear", overlay=true)
t = table.new(position.bottom_left, 2, 2)
table.cell(t, 0, 0, "keep", text_size=size.large)
table.cell(t, 1, 1, "drop")
t.clear(1, 1, 1, 1)
1`

	output, err := evalIndicatorOutput(&IndicatorScript{ID: "table", Name: "Table Clear", Script: script}, testBars())
	if err != nil {
		t.Fatalf("evalIndicatorOutput failed: %v", err)
	}
	if output.Drawings.Dashboard == nil {
		t.Fatalf("expected dashboard output")
	}
	if output.Drawings.Dashboard.Position != "bottom_left" {
		t.Fatalf("dashboard position = %q, want bottom_left", output.Drawings.Dashboard.Position)
	}
	if output.Drawings.Dashboard.Size != "large" {
		t.Fatalf("dashboard size = %q, want large", output.Drawings.Dashboard.Size)
	}
	if output.Drawings.Dashboard.Title != "keep" {
		t.Fatalf("dashboard title = %q, want keep", output.Drawings.Dashboard.Title)
	}
	for _, row := range output.Drawings.Dashboard.Rows {
		if row.Label == "drop" || row.Value == "drop" {
			t.Fatalf("table.clear did not remove cleared cell: %+v", output.Drawings.Dashboard.Rows)
		}
	}
}

func TestEvalScriptSupportsParenthesizedSourceExpression(t *testing.T) {
	bars := testBars()
	script := `indicator("Manual Mid", overlay=true)
plot((high + low) / 2, title="manual_hl2")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	pts, ok := plots["manual_hl2"]
	if !ok {
		t.Fatalf("expected plot 'manual_hl2', got keys: %v", mapKeys(plots))
	}
	if len(pts) != len(bars) {
		t.Fatalf("expected %d points, got %d", len(bars), len(pts))
	}

	for i := range bars {
		want := (bars[i].High + bars[i].Low) / 2
		if !almostEqual(pts[i].Value, want) {
			t.Fatalf("bar %d: expected %f, got %f", i, want, pts[i].Value)
		}
		if pts[i].Time != bars[i].Time {
			t.Fatalf("bar %d: expected time %d, got %d", i, bars[i].Time, pts[i].Time)
		}
	}
}

func TestEvalScriptSupportsOperatorExpressionsWithoutSpaces(t *testing.T) {
	bars := testBars()
	script := `indicator("Manual Mid", overlay=true)
plot((high+low)/2, title="manual_hl2_no_spaces")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	pts, ok := plots["manual_hl2_no_spaces"]
	if !ok {
		t.Fatalf("expected plot 'manual_hl2_no_spaces', got keys: %v", mapKeys(plots))
	}
	if len(pts) != len(bars) {
		t.Fatalf("expected %d points, got %d", len(bars), len(pts))
	}

	for i := range bars {
		want := (bars[i].High + bars[i].Low) / 2
		if !almostEqual(pts[i].Value, want) {
			t.Fatalf("bar %d: expected %f, got %f", i, want, pts[i].Value)
		}
		if pts[i].Time != bars[i].Time {
			t.Fatalf("bar %d: expected time %d, got %d", i, bars[i].Time, pts[i].Time)
		}
	}
}

func TestEvalScriptSupportsIndicatorAndPlotOnSameLine(t *testing.T) {
	bars := testBars()
	script := `indicator("Assigned", overlay=true); plot((close+high)/2, title="manual_ch2")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	pts, ok := plots["manual_ch2"]
	if !ok {
		t.Fatalf("expected plot 'manual_ch2', got keys: %v", mapKeys(plots))
	}
	if len(pts) != len(bars) {
		t.Fatalf("expected %d points, got %d", len(bars), len(pts))
	}

	for i := range bars {
		want := (bars[i].Close + bars[i].High) / 2
		if !almostEqual(pts[i].Value, want) {
			t.Fatalf("bar %d: expected %f, got %f", i, want, pts[i].Value)
		}
		if pts[i].Time != bars[i].Time {
			t.Fatalf("bar %d: expected time %d, got %d", i, bars[i].Time, pts[i].Time)
		}
	}
}

func TestEvalScriptSupportsOHLCVIdentifiers(t *testing.T) {
	bars := testBars()
	script := `plot(open, title="open")
plot(high, title="high")
plot(low, title="low")
plot(close, title="close")
plot(volume, title="volume")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	cases := []struct {
		name string
		want func(Bar) float64
	}{
		{name: "open", want: func(b Bar) float64 { return b.Open }},
		{name: "high", want: func(b Bar) float64 { return b.High }},
		{name: "low", want: func(b Bar) float64 { return b.Low }},
		{name: "close", want: func(b Bar) float64 { return b.Close }},
		{name: "volume", want: func(b Bar) float64 { return b.Volume }},
	}

	for _, tc := range cases {
		pts, ok := plots[tc.name]
		if !ok {
			t.Fatalf("expected plot %q, got keys: %v", tc.name, mapKeys(plots))
		}
		if len(pts) != len(bars) {
			t.Fatalf("plot %q: expected %d points, got %d", tc.name, len(bars), len(pts))
		}
		for i := range bars {
			want := tc.want(bars[i])
			if !almostEqual(pts[i].Value, want) {
				t.Fatalf("plot %q bar %d: expected %f, got %f", tc.name, i, want, pts[i].Value)
			}
			if pts[i].Time != bars[i].Time {
				t.Fatalf("plot %q bar %d: expected time %d, got %d", tc.name, i, bars[i].Time, pts[i].Time)
			}
		}
	}
}

func TestEvalScriptSupportsHL2AndHLC3(t *testing.T) {
	bars := testBars()
	script := `plot(hl2, title="hl2")
plot(hlc3, title="hlc3")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	hl2Points, ok := plots["hl2"]
	if !ok {
		t.Fatalf("expected plot 'hl2', got keys: %v", mapKeys(plots))
	}
	hlc3Points, ok := plots["hlc3"]
	if !ok {
		t.Fatalf("expected plot 'hlc3', got keys: %v", mapKeys(plots))
	}

	if len(hl2Points) != len(bars) || len(hlc3Points) != len(bars) {
		t.Fatalf("unexpected point counts: hl2=%d hlc3=%d bars=%d", len(hl2Points), len(hlc3Points), len(bars))
	}

	for i := range bars {
		wantHL2 := (bars[i].High + bars[i].Low) / 2
		wantHLC3 := (bars[i].High + bars[i].Low + bars[i].Close) / 3

		if !almostEqual(hl2Points[i].Value, wantHL2) {
			t.Fatalf("hl2 bar %d: expected %f, got %f", i, wantHL2, hl2Points[i].Value)
		}
		if !almostEqual(hlc3Points[i].Value, wantHLC3) {
			t.Fatalf("hlc3 bar %d: expected %f, got %f", i, wantHLC3, hlc3Points[i].Value)
		}
	}
}

func TestPlotCollectorAdvancesBarIndexForWarmupNaN(t *testing.T) {
	bars := testBars()
	collector := newPlotCollector(bars)

	if _, err := collector.capture(math.NaN(), "warmup"); err != nil {
		t.Fatalf("capture NaN failed: %v", err)
	}
	if _, err := collector.capture(42.0, "warmup"); err != nil {
		t.Fatalf("capture value failed: %v", err)
	}

	plots := collector.snapshot()
	pts := plots["warmup"]
	if len(pts) != 1 {
		t.Fatalf("expected one drawable point after warmup, got %d", len(pts))
	}
	if pts[0].Time != bars[1].Time {
		t.Fatalf("expected first drawable point at warmed-up bar time %d, got %d", bars[1].Time, pts[0].Time)
	}
	if !almostEqual(pts[0].Value, 42.0) {
		t.Fatalf("expected value 42, got %f", pts[0].Value)
	}
}

func TestEvalScriptATRUsesHistoricalOHLCV(t *testing.T) {
	bars := []Bar{
		{Time: 1000, Open: 10, High: 12, Low: 9, Close: 11, Volume: 100},
		{Time: 1060, Open: 11, High: 14, Low: 10, Close: 13, Volume: 100},
		{Time: 1120, Open: 13, High: 15, Low: 12, Close: 14, Volume: 100},
		{Time: 1180, Open: 14, High: 20, Low: 13, Close: 18, Volume: 100},
	}
	script := `indicator("ATR 3")
plot(ta.atr(3), title="ATR")`

	plots, err := evalScript(script, bars)
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}

	pts := plots["ATR"]
	if len(pts) != 2 {
		t.Fatalf("expected two ATR points after warmup, got %d", len(pts))
	}
	if pts[0].Time != bars[2].Time {
		t.Fatalf("expected first ATR point at third bar time %d, got %d", bars[2].Time, pts[0].Time)
	}
	if !almostEqual(pts[0].Value, 10.0/3.0) {
		t.Fatalf("expected first ATR value %f, got %f", 10.0/3.0, pts[0].Value)
	}
	if pts[1].Time != bars[3].Time {
		t.Fatalf("expected second ATR point at fourth bar time %d, got %d", bars[3].Time, pts[1].Time)
	}
	if !almostEqual(pts[1].Value, 41.0/9.0) {
		t.Fatalf("expected second ATR value %f, got %f", 41.0/9.0, pts[1].Value)
	}
}

func TestPlotParamNamesBindKeywordArgsToLatestSignature(t *testing.T) {
	bars := testBars()
	provider := newBarProvider("DEMO", bars)
	engine := pine.NewEngine()
	engine.RegisterMarketDataProvider(provider)
	engine.SetDefaultSymbol(provider.symbol)
	engine.SetDefaultValueType("close")

	var lastArgs []interface{}
	if err := engine.RegisterFunctionWithParamNames("plot", plotParamNames, func(args ...interface{}) (interface{}, error) {
		lastArgs = append([]interface{}(nil), args...)
		return args[0], nil
	}); err != nil {
		t.Fatalf("register plot function: %v", err)
	}

	bytecode, err := engine.Compile(`plot(close, title="Close", format=format.price, precision=4, force_overlay=true, linestyle=line.style_dashed)`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if _, err := engine.Execute(bytecode); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(lastArgs) != len(plotParamNames) {
		t.Fatalf("expected %d bound args, got %d: %#v", len(plotParamNames), len(lastArgs), lastArgs)
	}
	if title, ok := lastArgs[1].(string); !ok || title != "Close" {
		t.Fatalf("title bound to arg[1] = %#v, want Close", lastArgs[1])
	}
	if format, ok := lastArgs[12].(string); !ok || format != "price" {
		t.Fatalf("format bound to arg[12] = %#v, want price", lastArgs[12])
	}
	precision, ok := toFloat64(lastArgs[13])
	if !ok || !almostEqual(precision, 4) {
		t.Fatalf("precision bound to arg[13] = %#v, want 4", lastArgs[13])
	}
	forceOverlay, ok := lastArgs[14].(bool)
	if !ok || !forceOverlay {
		t.Fatalf("force_overlay bound to arg[14] = %#v, want true", lastArgs[14])
	}
	linestyle, ok := toFloat64(lastArgs[15])
	if !ok || !almostEqual(linestyle, 1) {
		t.Fatalf("linestyle bound to arg[15] = %#v, want line.style_dashed", lastArgs[15])
	}
}

func TestEvalIndicatorOutputIncludesPlotRenderOptions(t *testing.T) {
	ind := &IndicatorScript{
		ID:   "styled",
		Name: "Styled",
		Script: `indicator("Styled")
plot(close, title="Styled Close", color=color.red, linewidth=3, trackprice=true, display=display.all, format=format.price, precision=4, force_overlay=true, linestyle=line.style_dotted)`,
	}

	output, err := evalIndicatorOutput(ind, testBars())
	if err != nil {
		t.Fatalf("evalIndicatorOutput failed: %v", err)
	}

	options, ok := output.PlotOptions["Styled Close"]
	if !ok {
		t.Fatalf("expected plot options for Styled Close, got %#v", output.PlotOptions)
	}
	if options.Color != "#ff0000" {
		t.Fatalf("color = %q, want #ff0000", options.Color)
	}
	if options.LineWidth != 3 {
		t.Fatalf("linewidth = %d, want 3", options.LineWidth)
	}
	if options.LineStyle != 2 {
		t.Fatalf("linestyle = %d, want line.style_dotted", options.LineStyle)
	}
	if options.TrackPrice == nil || !*options.TrackPrice {
		t.Fatalf("trackprice = %#v, want true", options.TrackPrice)
	}
	if options.Display == nil || !almostEqual(*options.Display, 1) {
		t.Fatalf("display = %#v, want display.all", options.Display)
	}
	if options.Format != "price" {
		t.Fatalf("format = %q, want price", options.Format)
	}
	if options.Precision == nil || *options.Precision != 4 {
		t.Fatalf("precision = %#v, want 4", options.Precision)
	}
	if options.ForceOverlay == nil || !*options.ForceOverlay {
		t.Fatalf("force_overlay = %#v, want true", options.ForceOverlay)
	}
}

func TestIndicatorOverlayParsesScriptDeclaration(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   bool
	}{
		{
			name: "keyword true",
			script: `//@version=6
indicator("Overlay", overlay=true)
plot(close)`,
			want: true,
		},
		{
			name: "keyword false with spaces",
			script: `indicator("Separate", shorttitle = "Sep", overlay = false)
plot(close)`,
			want: false,
		},
		{
			name: "positional third argument",
			script: `indicator("Overlay", "Ov", true)
plot(close)`,
			want: true,
		},
		{
			name: "multiline keyword true",
			script: `indicator(
    "Overlay",
    shorttitle = "Ov",
    overlay = true
)
plot(close)`,
			want: true,
		},
		{
			name: "default false when omitted",
			script: `indicator("Default Separate")
plot(close)`,
			want: false,
		},
		{
			name:   "default false without declaration",
			script: `plot(close)`,
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := indicatorOverlay(tc.script); got != tc.want {
				t.Fatalf("indicatorOverlay() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvalScriptSupportsMultilineIndicatorDeclaration(t *testing.T) {
	plots, err := evalScript(`indicator(
    "Overlay",
    shorttitle = "Ov",
    overlay = true
)
plot(close, title="Close")`, testBars())
	if err != nil {
		t.Fatalf("evalScript failed: %v", err)
	}
	if _, ok := plots["Close"]; !ok {
		t.Fatalf("expected plot Close, got keys: %v", mapKeys(plots))
	}
}

func TestEvalIndicatorOutputIncludesOverlayMetadata(t *testing.T) {
	output, err := evalIndicatorOutput(&IndicatorScript{
		ID:   "overlay",
		Name: "Overlay",
		Script: `indicator("Overlay", overlay=true)
plot(close, title="Close")`,
	}, testBars())
	if err != nil {
		t.Fatalf("evalIndicatorOutput failed: %v", err)
	}
	if !output.Overlay {
		t.Fatalf("overlay = false, want true")
	}
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
