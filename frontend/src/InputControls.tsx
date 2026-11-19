import type { ScriptInput } from './types'
import type { InputDrafts } from './inputs'

const sources = ['open', 'high', 'low', 'close', 'volume', 'hl2', 'hlc3', 'hlcc4', 'ohlc4']

export function InputControls({ parameters, drafts, onChange }: {
  parameters: ScriptInput[]
  drafts: InputDrafts
  onChange: (id: string, value: string | boolean) => void
}) {
  return <div className="input-parameters">
    {parameters.map(parameter => {
      const id = `pine-input-${parameter.id}`
      const value = drafts[parameter.id] ?? parameter.default
      const options = parameter.kind === 'source'
        ? sources.map(source => ({ label: source, value: source }))
        : parameter.options
      const numeric = ['int', 'float', 'price', 'time'].includes(parameter.kind)
      return <div className="field" key={parameter.id}>
        <label className="field-label" htmlFor={id}>
          {parameter.group && `${parameter.group} · `}{parameter.title || parameter.id}
        </label>
        {parameter.kind === 'bool'
          ? <input id={id} type="checkbox" checked={value === true}
              onChange={event => onChange(parameter.id, event.target.checked)} />
          : options?.length
            ? <select id={id} className="field-input" value={String(value)}
                onChange={event => onChange(parameter.id, event.target.value)}>
                {options.map(option => <option key={String(option.value)} value={String(option.value)}>
                  {option.label}
                </option>)}
              </select>
            : parameter.kind === 'text_area'
              ? <textarea id={id} className="field-input" rows={3} value={String(value)}
                  onChange={event => onChange(parameter.id, event.target.value)} />
              : <input id={id} className="field-input" type={numeric ? 'number' : 'text'}
                  value={String(value)} min={parameter.min} max={parameter.max}
                  step={parameter.step ?? (['int', 'time'].includes(parameter.kind) ? 1 : 'any')}
                  onChange={event => onChange(parameter.id, event.target.value)} />}
        {parameter.tooltip && <small className="field-hint">{parameter.tooltip}</small>}
        {parameter.kind === 'time' && <small className="field-hint">Unix timestamp in milliseconds</small>}
      </div>
    })}
  </div>
}
