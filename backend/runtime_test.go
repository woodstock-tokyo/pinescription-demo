package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestInputDiscoveryDoesNotExecuteScript(t *testing.T) {
	_, _, inputs, err := compileIndicator(`enum Side
    buy = "Buy"
    sell = "Sell"
const int n = 2
length = input.int(n, minval=1, maxval=10)
source = input.source(close)
side = input.enum(Side.buy)
note = input.text_area("one; two", tooltip="before; execution")
unused(x = input.bool(true)) => x
runtime.error("must not execute during inspection")`)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 5 {
		t.Fatalf("got %d inputs, want 5", len(inputs))
	}
	if inputs[0].Default != float64(2) || inputs[1].Default != "close" || inputs[2].Default != "Side.buy" {
		t.Fatalf("unexpected defaults: %+v", inputs)
	}
	if inputs[2].Options[1].Value != "Side.sell" || inputs[2].Options[1].Label != "Sell" {
		t.Fatalf("enum options do not round-trip: %+v", inputs[2])
	}
	raw, err := json.Marshal(inputs)
	if err != nil || !strings.Contains(string(raw), `"default":"one; two"`) {
		t.Fatalf("invalid metadata serialization: %s, %v", raw, err)
	}
}

func TestInputOverridesPersistDuringTickRecalculation(t *testing.T) {
	ind := &IndicatorScript{
		ID: "configured", Script: `length = input.int(2,minval=1,maxval=4)
source = input.source(close)
plot(source*length,title="Configured")`,
		Inputs: map[string]interface{}{"length": float64(3), "source": "high"},
	}
	output, err := evalIndicatorOutput(ind, testBars())
	if err != nil {
		t.Fatal(err)
	}
	points := output.Plots["Configured"]
	if len(points) != 3 || points[2].Value != 51 {
		t.Fatalf("overrides not applied: %v", points)
	}
	indMu.Lock()
	previous := indicators
	indicators = map[string]*IndicatorScript{ind.ID: ind}
	indMu.Unlock()
	t.Cleanup(func() {
		indMu.Lock()
		indicators = previous
		indMu.Unlock()
	})
	updates := latestIndicatorValues(testBars())
	if len(updates) != 1 || updates[0].Values["Configured"] != 51 {
		t.Fatalf("tick lost input overrides: %v", updates)
	}
	for _, values := range []map[string]interface{}{
		{"length": 0}, {"length": 1.5}, {"length": "three"},
		{"source": "unknown"}, {"unknown": 3},
	} {
		ind.Inputs = values
		if _, err := evalIndicatorOutput(ind, testBars()); err == nil {
			t.Fatalf("invalid override accepted: %v", values)
		}
	}
}

func TestMarketContextUsesActualBarsAndContinuousSession(t *testing.T) {
	bs := testBars()
	start := time.Date(2025, 1, 1, 23, 59, 0, 0, time.UTC).Unix()
	for i, offset := range []int64{0, 60, 240} {
		bs[i].Time = start + offset
	}
	plots, err := evalScript(`plot(time,title="Open")
plot(time_close,title="Close")
plot(session.isfirstbar?1:0,title="First")
plot(session.islastbar?1:0,title="Last")
plot(syminfo.mintick,title="Tick")`, bs)
	if err != nil {
		t.Fatal(err)
	}
	for i, bar := range bs {
		if plots["Open"][i].Value != float64(bar.Time*1000) || plots["Close"][i].Value != float64((bar.Time+60)*1000) {
			t.Fatalf("bar %d was synthesized instead of using its timestamp: %v", i, plots)
		}
		if plots["First"][i].Value != []float64{0, 1, 0}[i] || plots["Last"][i].Value != []float64{1, 0, 0}[i] {
			t.Fatalf("bar %d continuous-session boundary mismatch: %v", i, plots)
		}
		if plots["Tick"][i].Value != 0.01 {
			t.Fatal("missing simulated-market tick metadata")
		}
	}
}

func TestNativeStrategyHistoryAndUnsupportedCapabilities(t *testing.T) {
	output, err := evalIndicatorOutput(&IndicatorScript{Script: `strategy("History",overlay=true,initial_capital=1000,margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long,qty=1)
plot(strategy.position_size,title="Current")
plot(strategy.position_size[1],title="Previous")`}, testBars())
	if err != nil {
		t.Fatal(err)
	}
	if !output.Overlay || output.Plots["Current"][1].Value != 1 || output.Plots["Previous"][0].Value != 0 {
		t.Fatalf("strategy overlay/history did not reach the demo: %+v", output)
	}
	if output.Plots["Previous"][0].Time != testBars()[1].Time {
		t.Fatal("missing strategy history shifted plot timestamps")
	}
	for _, script := range []string{
		`strategy("Unsupported margins",margin_long=100,margin_short=100)`,
		`request.security("OTHER","120",close)`,
		`import me/Library/1 as lib`,
	} {
		if _, err := evalScript(script, testBars()); err == nil {
			t.Fatalf("unsupported capability silently accepted: %s", script)
		}
	}
}

func TestNewRuntimePresets(t *testing.T) {
	bs := seedHistory(400)
	for _, name := range []string{"inputs_ema", "anchored_vwap", "rci", "strategy_history"} {
		t.Run(name, func(t *testing.T) {
			script, err := os.ReadFile("../indicators/" + name + ".pine")
			if err != nil {
				t.Fatal(err)
			}
			output, err := evalIndicatorOutput(&IndicatorScript{Script: string(script)}, bs)
			if err != nil || len(output.Plots) == 0 {
				t.Fatalf("preset did not produce plots: %v, %v", output, err)
			}
		})
	}
}

func TestWebSocketInspectionAndFailedReplacement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsHandler))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	read := func() WSEnvelope {
		t.Helper()
		var message WSEnvelope
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	read() // Initial history.
	if err := conn.WriteJSON(WSEnvelope{Type: "inspect_inputs", RequestID: "preview-1",
		Indicator: &IndicatorScript{Script: "length = input.int(2)\nruntime.error(\"not inspection\")"}}); err != nil {
		t.Fatal(err)
	}
	response := read()
	if response.Type != "inputs_discovered" || response.RequestID != "preview-1" || len(response.InputParameters) != 1 {
		t.Fatalf("unexpected inspection response: %+v", response)
	}
	indMu.Lock()
	previous := indicators
	original := &IndicatorScript{ID: "kept", Script: "plot(close)"}
	indicators = map[string]*IndicatorScript{"kept": original}
	indMu.Unlock()
	t.Cleanup(func() {
		indMu.Lock()
		indicators = previous
		indMu.Unlock()
	})
	if err := conn.WriteJSON(WSEnvelope{Type: "add_indicator", Indicator: &IndicatorScript{
		ID: "kept", Script: "length=input.int(2,minval=1)\nplot(close*length)", Inputs: map[string]interface{}{"length": 0},
	}}); err != nil {
		t.Fatal(err)
	}
	if response := read(); response.Type != "error" {
		t.Fatalf("expected invalid input error, got %+v", response)
	}
	indMu.RLock()
	kept := indicators["kept"]
	indMu.RUnlock()
	if kept != original {
		t.Fatal("failed replacement removed the working indicator")
	}
}
