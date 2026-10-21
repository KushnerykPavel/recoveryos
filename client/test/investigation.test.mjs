import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { rolldown } from 'rolldown'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const require = createRequire(import.meta.url)
const bundle = await rolldown({ input: new URL('../src/App.tsx', import.meta.url).pathname, platform: 'node', external: ['react', 'react/jsx-runtime'] })
const compiled = (await bundle.generate({ format: 'cjs' })).output[0].code
await bundle.close()
const module = { exports: {} }
new Function('require', 'module', 'exports', compiled)(require, module, module.exports)
const { InvestigationPanel } = module.exports
const scriptedDiagnosis = JSON.parse(readFileSync(new URL('./fixtures/investigation.json', import.meta.url), 'utf8'))

test('Incident Lab shows the diagnosis, proposal, and provenance of cited evidence', () => {
  const markup = renderToStaticMarkup(React.createElement(InvestigationPanel, {
    incident: { id: scriptedDiagnosis.incident_id, status: 'open' },
    result: scriptedDiagnosis,
    loading: false, error: '', onInvestigate() {},
  }))
  for (const visible of ['AI diagnosis', scriptedDiagnosis.diagnosis, 'Correct the internal payout record', 'PayPal provider', 'Internal record', 'Injected event', 'Recovery policy']) {
    assert.ok(markup.includes(visible), `missing visible evidence: ${visible}`)
  }
})
