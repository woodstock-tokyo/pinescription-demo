package main

import (
	"fmt"
	"time"

	pine "github.com/woodstock-tokyo/pinescription"
)

type InputOption struct {
	Value interface{} `json:"value"`
	Label string      `json:"label"`
}

// Keep the wire format independent of the runtime's Go field names and enum
// structs. Qualified enum strings round-trip through SetInputParameters.
type ScriptInput struct {
	ID      string        `json:"id"`
	Kind    string        `json:"kind"`
	Title   string        `json:"title"`
	Tooltip string        `json:"tooltip,omitempty"`
	Group   string        `json:"group,omitempty"`
	Inline  string        `json:"inline,omitempty"`
	Default interface{}   `json:"default"`
	Options []InputOption `json:"options,omitempty"`
	Min     *float64      `json:"min,omitempty"`
	Max     *float64      `json:"max,omitempty"`
	Step    *float64      `json:"step,omitempty"`
	Confirm bool          `json:"confirm,omitempty"`
}

func inputWireValue(value interface{}) interface{} {
	switch v := value.(type) {
	case pine.EnumValue:
		return v.Type + "." + v.Name
	case pine.InputSource:
		return string(v)
	default:
		return value
	}
}

func compileIndicator(script string) (*pine.Engine, []byte, []ScriptInput, error) {
	normalized, err := normalizeScript(script)
	if err != nil {
		return nil, nil, nil, err
	}
	engine := pine.NewEngine()
	bytecode, err := engine.Compile(normalized)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compile failed: %w", err)
	}
	// No provider or runtime execution is needed to inspect input metadata.
	defs, err := engine.InputParameters(bytecode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("input discovery failed: %w", err)
	}
	inputs := make([]ScriptInput, 0, len(defs))
	for _, def := range defs {
		input := ScriptInput{
			ID: def.ID, Kind: def.Kind, Title: def.Title, Tooltip: def.Tooltip,
			Group: def.Group, Inline: def.Inline, Default: inputWireValue(def.Default),
			Min: def.Min, Max: def.Max, Step: def.Step, Confirm: def.Confirm,
		}
		for _, option := range def.Options {
			input.Options = append(input.Options, InputOption{Value: inputWireValue(option), Label: fmt.Sprint(option)})
		}
		inputs = append(inputs, input)
	}
	return engine, bytecode, inputs, nil
}

func cloneInputValues(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	out := make(map[string]interface{}, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func (p *barProvider) GetSymbolInfo(symbol string) (pine.SymbolInfo, error) {
	if symbol != p.symbol {
		return nil, fmt.Errorf("unknown symbol: %s", symbol)
	}
	return pine.SymbolInfo{
		"timezone": "UTC", "currency": "USDT", "basecurrency": "BTC",
		"mintick": 0.01, "pointvalue": 1.0, "mincontract": 0.000001,
		"type": "crypto", "description": "Simulated BTC/USDT",
	}, nil
}

func (p *barProvider) GetBarTimes(symbol, timeframe string) ([]pine.BarTime, error) {
	if symbol != p.symbol || timeframe != "1" {
		return nil, fmt.Errorf("unsupported bar context: %s/%s", symbol, timeframe)
	}
	times := make([]pine.BarTime, len(p.bars))
	for i, bar := range p.bars {
		open := time.Unix(bar.Time, 0).UTC()
		times[i] = pine.BarTime{Open: open, Close: open.Add(time.Minute)}
	}
	return times, nil
}

func (p *barProvider) GetMarketSessions(symbol string) (pine.MarketSessions, error) {
	if symbol != p.symbol {
		return pine.MarketSessions{}, fmt.Errorf("unknown symbol: %s", symbol)
	}
	return pine.MarketSessions{Regular: "24x7"}, nil
}
