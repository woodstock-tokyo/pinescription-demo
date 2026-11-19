import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import ts from 'typescript'

// Use the existing TypeScript compiler, without a separate test framework or a
// Node-specific TypeScript runtime. npm run build also type-checks this module.
const source = readFileSync(new URL('../src/inputs.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 },
})
const { defaultInputDrafts, buildInputValues } = await import(
  `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`
)

const parameters = [
  { id: 'length', kind: 'int', title: 'Length', default: 2, min: 1, max: 10 },
  { id: 'price', kind: 'price', title: 'Price', default: 10.5 },
  { id: 'enabled', kind: 'bool', title: 'Enabled', default: true },
  { id: 'source', kind: 'source', title: 'Source', default: 'close' },
  { id: 'side', kind: 'enum', title: 'Side', default: 'Side.buy',
    options: [{ value: 'Side.buy', label: 'Buy' }, { value: 'Side.sell', label: 'Sell' }] },
  { id: 'note', kind: 'text_area', title: 'Note', default: 'one; two' },
]

test('defaults preserve booleans and serialize numeric drafts as numbers', () => {
  const drafts = defaultInputDrafts(parameters)
  assert.equal(drafts.length, '2')
  assert.equal(drafts.enabled, true)
  assert.deepEqual(buildInputValues(parameters, drafts), {
    length: 2, price: 10.5, enabled: true, source: 'close', side: 'Side.buy', note: 'one; two',
  })
})

test('overrides retain source and qualified enum selections', () => {
  const drafts = { ...defaultInputDrafts(parameters), length: '5', enabled: false, source: 'high', side: 'Side.sell' }
  const values = buildInputValues(parameters, drafts)
  assert.equal(values.length, 5)
  assert.equal(values.enabled, false)
  assert.equal(values.source, 'high')
  assert.equal(values.side, 'Side.sell')
})

test('invalid numeric drafts do not silently become zero or JSON null', () => {
  for (const value of ['', ' ', 'NaN', 'Infinity', 'two', '1.5', '0', '11', true]) {
    assert.throws(() => buildInputValues(parameters, { ...defaultInputDrafts(parameters), length: value }))
  }
})

test('invalid option and boolean values are rejected', () => {
  assert.throws(() => buildInputValues(parameters, { ...defaultInputDrafts(parameters), side: 'Buy' }))
  assert.throws(() => buildInputValues(parameters, { ...defaultInputDrafts(parameters), enabled: 'false' }))
})

test('empty input metadata sends no stale overrides', () => {
  assert.deepEqual(buildInputValues([], { length: '5' }), {})
})
