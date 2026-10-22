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
const { InvestigationPanel, IncidentEvidence, AuditHistory, Overview, Journey } = module.exports
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

test('Incident Lab labels a deterministic diagnosis as a fallback', () => {
  const markup = renderToStaticMarkup(React.createElement(InvestigationPanel, {
    incident: { id: scriptedDiagnosis.incident_id, status: 'open' },
    result: { ...scriptedDiagnosis, origin: 'deterministic_fallback' },
    loading: false, error: '', onInvestigate() {},
  }))
  assert.ok(markup.includes('Deterministic fallback diagnosis'))
  assert.ok(markup.includes('AI analysis was unavailable or invalid'))
  assert.ok(!markup.includes('>AI diagnosis<'))
})

test('Incident Lab shows why a proposed correction was rejected', () => {
  const markup = renderToStaticMarkup(React.createElement(InvestigationPanel, {
    incident: { id: scriptedDiagnosis.incident_id, status: 'open' },
    result: scriptedDiagnosis,
    loading: false, error: '', onInvestigate() {},
    approving: false, approvalError: 'PayPal payout item is FAILED, not SUCCESS', onApprove() {},
  }))
  assert.ok(markup.includes('Approve record correction'))
  assert.ok(markup.includes('PayPal payout item is FAILED, not SUCCESS'))
})

test('Incident Lab shows a resolved correction and audit history', () => {
  const incident = { id: scriptedDiagnosis.incident_id, status: 'resolved' }
  const investigation = renderToStaticMarkup(React.createElement(InvestigationPanel, {
    incident, result: scriptedDiagnosis, loading: false, error: '', onInvestigate() {}, onApprove() {},
  }))
  assert.ok(investigation.includes('Record correction approved'))
  assert.ok(!investigation.includes('Approve record correction'))
  const evidence = renderToStaticMarkup(React.createElement(IncidentEvidence, {
    incident, payout: { status: 'corrected', provider_status: 'SUCCESS', item_id: scriptedDiagnosis.item_id },
    events: [
      { id: 1, source: 'paypal', kind: 'provider_verification', details: 'Fresh item', occurred_at: '2026-10-21T10:00:00Z' },
      { id: 2, source: 'system', kind: 'operator_approval', details: 'Approved', occurred_at: '2026-10-21T10:00:01Z' },
      { id: 3, source: 'system', kind: 'record_corrected', details: 'Corrected', occurred_at: '2026-10-21T10:00:02Z' },
    ],
  }))
  for (const label of ['Fresh item verification', 'Reconciled payout record']) {
    assert.ok(evidence.includes(label), `missing ${label}`)
  }
  const audit = renderToStaticMarkup(React.createElement(AuditHistory, { events: [
    { id: 1, source: 'paypal', kind: 'provider_verification', details: 'Fresh item', occurred_at: '2026-10-21T10:00:00Z' },
  ] }))
  assert.match(audit, /Audit trail/)
  assert.match(audit, /Event history/)
})

test('Overview leads to the active incident and journey shows resolution', () => {
  const snapshot = {
    session: { id: 'session-1', expires_at: '2026-10-22T10:00:00Z' },
    payout: { status: 'corrected', provider_status: 'SUCCESS', item_id: 'item-1', origin: 'live' },
    incident: { id: 'incident-1', status: 'resolved' }, investigation: scriptedDiagnosis, events: [],
  }
  const overview = renderToStaticMarkup(React.createElement(Overview, { snapshot, onOpen() {} }))
  assert.match(overview, /Open Incident Lab/)
  assert.match(overview, /Payout record corrected/)
  assert.match(overview, /item-1/)
  const journey = renderToStaticMarkup(React.createElement(Journey, { snapshot, investigation: scriptedDiagnosis }))
  assert.match(journey, /Recovery complete/)
  assert.match(journey, /Approve and verify correction/)
  assert.match(journey, /fresh PayPal check/i)
})

test('a no-change diagnosis invites review without implying approval is available', () => {
  const snapshot = {
    session: { id: 'session-1', expires_at: '2026-10-22T10:00:00Z' },
    payout: { status: 'pending', provider_status: 'SUCCESS', item_id: 'item-1', origin: 'live' },
    incident: { id: 'incident-1', status: 'open' },
    investigation: { ...scriptedDiagnosis, proposed_action: 'no_change' }, events: [],
  }
  const overview = renderToStaticMarkup(React.createElement(Overview, { snapshot, onOpen() {} }))
  const journey = renderToStaticMarkup(React.createElement(Journey, { snapshot, investigation: snapshot.investigation }))
  assert.match(overview, /Review diagnosis and evidence/)
  assert.match(journey, /No record correction proposed/)
  assert.doesNotMatch(overview + journey, /Approve the proposal|Review and approve/)
})
