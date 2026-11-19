import type { InputValues, ScriptInput } from './types'

export type InputDrafts = Record<string, string | boolean>

export function defaultInputDrafts(parameters: ScriptInput[]): InputDrafts {
  return Object.fromEntries(parameters.map(p => [
    p.id, typeof p.default === 'boolean' ? p.default : String(p.default),
  ]))
}

export function buildInputValues(parameters: ScriptInput[], drafts: InputDrafts): InputValues {
  const values: InputValues = {}
  for (const parameter of parameters) {
    const value = drafts[parameter.id] ?? parameter.default
    const title = parameter.title || parameter.id
    if (parameter.kind === 'bool') {
      if (typeof value !== 'boolean') throw new Error(`${title} must be true or false`)
      values[parameter.id] = value
    } else if (['int', 'float', 'price', 'time'].includes(parameter.kind)) {
      const n = typeof value === 'number' ? value : Number(value)
      if (typeof value === 'boolean' || String(value).trim() === '' || !Number.isFinite(n)) {
        throw new Error(`${title} must be a finite number`)
      }
      if (['int', 'time'].includes(parameter.kind) && !Number.isInteger(n)) {
        throw new Error(`${title} must be an integer`)
      }
      if (parameter.min !== undefined && n < parameter.min) {
        throw new Error(`${title} must be at least ${parameter.min}`)
      }
      if (parameter.max !== undefined && n > parameter.max) {
        throw new Error(`${title} must be at most ${parameter.max}`)
      }
      values[parameter.id] = n
    } else {
      values[parameter.id] = String(value)
    }
    if (parameter.options?.length && !parameter.options.some(option => option.value === values[parameter.id])) {
      throw new Error(`${title} must match one of the available options`)
    }
  }
  return values
}
