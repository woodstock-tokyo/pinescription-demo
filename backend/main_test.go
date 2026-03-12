package main

import (
	"math"
	"testing"
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

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
