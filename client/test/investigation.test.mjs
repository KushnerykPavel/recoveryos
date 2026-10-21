import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
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

test('Incident Lab shows the diagnosis, proposal, and provenance of cited evidence', () => {
  const markup = renderToStaticMarkup(React.createElement(InvestigationPanel, {
    incident: { id: 'incident-1', status: 'open' },
    result: {
      incident_id: 'incident-1', item_id: 'paypal-item-1', item_status: 'SUCCESS',
      diagnosis: 'PayPal confirms SUCCESS while the payout record remains pending.',
      proposed_action: 'correct_payout_record', origin: 'ai',
      evidence: [
        { source: 'provider', reference: 'paypal-item-1', finding: 'PayPal item succeeded' },
        { source: 'internal', reference: 'payout-1', finding: 'Payout record pending' },
        { source: 'injected', reference: '5', finding: 'Internal update skipped' },
        { source: 'policy', reference: 'record-correction-policy', finding: 'Fresh verification required' },
      ],
    },
    loading: false, error: '', onInvestigate() {},
  }))
  for (const visible of ['AI diagnosis', 'PayPal confirms SUCCESS', 'Correct the internal payout record', 'PayPal provider', 'Internal record', 'Injected event', 'Recovery policy']) {
    assert.ok(markup.includes(visible), `missing visible evidence: ${visible}`)
  }
})
