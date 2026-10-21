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
const { PayoutPanel } = module.exports

const common = { creating: false, checking: false, pollingPaused: false, stateUnverified: false,
  dailyLimitReached: false, now: 0, switchingBackup: false, error: '',
  onCreate() {}, onVerify() {}, onCheck() {}, onBackup() {} }

test('slow live payout offers a clearly labeled pre-created backup', () => {
  const payout = { id: 'live-1', creation_key: 'new-key', batch_id: 'live-batch', status: 'pending', provider_status: 'PENDING', origin: 'live', created_at: '2026-10-21T17:00:00Z' }
  const started = Date.parse(payout.created_at)
  const before = renderToStaticMarkup(React.createElement(PayoutPanel, { ...common, payout, now: started + 29_999 }))
  assert.doesNotMatch(before, /Use pre-created Sandbox payout/)
  const markup = renderToStaticMarkup(React.createElement(PayoutPanel, { ...common, payout, now: started + 30_000 }))
  assert.match(markup, /Use pre-created Sandbox payout/)
  assert.match(markup, /This opens a new demo session/)
})

test('daily limit offers backup without another create action', () => {
  const markup = renderToStaticMarkup(React.createElement(PayoutPanel, { ...common, dailyLimitReached: true }))
  assert.match(markup, /Use pre-created Sandbox payout/)
  assert.match(markup, /Daily Sandbox limit reached/)
})

test('backup payout is presented as pre-created', () => {
  const payout = { id: 'backup-1', creation_key: 'old-key', batch_id: 'old-batch', item_id: 'old-item', status: 'pending', provider_status: 'SUCCESS', origin: 'precreated' }
  const markup = renderToStaticMarkup(React.createElement(PayoutPanel, { ...common, payout }))
  assert.match(markup, /Pre-created Sandbox payout/)
  assert.doesNotMatch(markup, /Start one genuine Sandbox payout/)
})
