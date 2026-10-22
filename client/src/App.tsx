import { useEffect, useState } from 'react'
import { AllCommunityModule, ModuleRegistry, themeQuartz, type ColDef } from 'ag-grid-community'
import { AgGridReact } from 'ag-grid-react'

ModuleRegistry.registerModules([AllCommunityModule])

type Session = { id: string; created_at: string; expires_at: string }
type Payout = { id: string; creation_key: string; batch_id?: string; item_id?: string; status: string; provider_status?: string; origin: 'live' | 'precreated'; created_at: string }
type Incident = { id: string; status: string }
type Event = { id: number; source: string; kind: string; details: string; occurred_at: string }
type Snapshot = { session: Session; payout?: Payout; incident?: Incident; investigation?: Investigation; events: Event[] }
type Investigation = { incident_id: string; item_id: string; item_status: string; diagnosis: string; proposed_action: 'correct_payout_record' | 'no_change'; evidence: { source: 'provider' | 'internal' | 'injected' | 'policy'; reference: string; finding: string }[]; origin: 'ai' | 'deterministic_fallback' }
type View = 'overview' | 'lab'

const eventNames: Record<string, string> = {
  provider_verification: 'Provider verification', operator_approval: 'Operator approval',
  record_corrected: 'Record correction', record_correction_rejected: 'Correction rejected',
  investigation_completed: 'Investigation completed', internal_update_skipped: 'Internal update skipped',
  item_observed: 'Payout item observed', batch_created: 'Payout batch created',
  precreated_payout_attached: 'Pre-created payout attached',
}

function eventPresentation(event: Event) {
  if (event.source === 'paypal') return { label: 'PayPal response', className: 'grid-provider' }
  if (event.source === 'injected') return { label: 'Injected fault', className: 'grid-injected' }
  if (event.kind === 'investigation_completed') return event.details.includes('deterministic_fallback')
    ? { label: 'Fallback diagnosis', className: 'grid-fallback' }
    : { label: 'AI conclusion', className: 'grid-conclusion' }
  return { label: 'RecoveryOS', className: '' }
}

function apiPath(path: string) {
  return `${import.meta.env.BASE_URL}${path.replace(/^\//, '')}`
}

async function getSnapshot(path: string, method: 'GET' | 'POST' = 'GET', headers?: HeadersInit): Promise<Snapshot | null> {
  const response = await fetch(apiPath(path), { method, headers, credentials: 'same-origin', cache: 'no-store' })
  if (response.status === 401) return null
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error || `Request failed (${response.status})`)
  }
  return response.json() as Promise<Snapshot>
}

export function App() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [view, setView] = useState<View>('overview')
  const [loading, setLoading] = useState(true)
  const [starting, setStarting] = useState(false)
  const [creatingPayout, setCreatingPayout] = useState(false)
  const [checkingPayout, setCheckingPayout] = useState(false)
  const [pollingPaused, setPollingPaused] = useState(false)
  const [payoutStateUnverified, setPayoutStateUnverified] = useState(false)
  const [dailyLimitReached, setDailyLimitReached] = useState(false)
  const [switchingBackup, setSwitchingBackup] = useState(false)
  const [now, setNow] = useState(Date.now())
  const [error, setError] = useState('')
  const [investigation, setInvestigation] = useState<Investigation | null>(null)
  const [investigating, setInvestigating] = useState(false)
  const [investigationError, setInvestigationError] = useState('')
  const [approving, setApproving] = useState(false)
  const [approvalError, setApprovalError] = useState('')

  useEffect(() => {
    let active = true
    getSnapshot('/api/sessions/current')
      .then(value => { if (active) { setSnapshot(value); setInvestigation(value?.investigation ?? null) } })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : 'Could not reach the API') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const payout = snapshot?.payout
  const awaitingBatch = payout?.status === 'creating'
  const awaitingItem = payout?.status === 'pending' && !!payout.batch_id && !snapshot?.incident && (!payout.provider_status || payout.provider_status === 'PENDING')
  const backupWaiting = !!payout && payout.origin === 'live' && !snapshot?.incident && payout.provider_status !== 'SUCCESS'

  useEffect(() => {
    if (!backupWaiting) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [backupWaiting])

  useEffect(() => {
    if (creatingPayout || pollingPaused || (!awaitingBatch && !awaitingItem)) return
    let active = true
    let timer: number
    async function poll() {
      let failed = false
      setCheckingPayout(true)
      try {
        const next = await getSnapshot(awaitingBatch ? '/api/sessions/current' : '/api/payouts/reconcile', awaitingBatch ? 'GET' : 'POST')
        if (active) {
          setSnapshot(next)
          setError('')
        }
      } catch (cause) {
        if (active) {
          failed = true
          setPollingPaused(true)
          setError(cause instanceof Error ? cause.message : 'Could not check the payout item')
        }
      } finally {
        if (active) {
          setCheckingPayout(false)
          if (!failed) timer = window.setTimeout(poll, 3000)
        }
      }
    }
    timer = window.setTimeout(poll, 3000)
    return () => { active = false; window.clearTimeout(timer) }
  }, [awaitingBatch, awaitingItem, creatingPayout, pollingPaused, payout?.id])

  async function startSession() {
    setStarting(true)
    setError('')
    try {
      const next = await getSnapshot('/api/sessions', 'POST')
      if (!next) throw new Error('Session could not be created')
      setSnapshot(next)
      setPollingPaused(false)
      setPayoutStateUnverified(false)
      setDailyLimitReached(false)
      setView('lab')
      setInvestigation(null)
      setApprovalError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not start session')
    } finally {
      setStarting(false)
    }
  }

  async function startPayout() {
    if (!snapshot || snapshot.payout || creatingPayout) return
    setCreatingPayout(true)
    setPayoutStateUnverified(false)
    setError('')
    try {
      const next = await getSnapshot('/api/payouts', 'POST', { 'X-RecoveryOS-Scenario': 'missed-update' })
      if (!next) throw new Error('Session expired. Start a new demo session.')
      setSnapshot(next)
      setPollingPaused(false)
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Could not start the payout'
      if (message === 'daily payout creation limit reached') setDailyLimitReached(true)
      // A lost response may still mean PayPal received the request. Read the
      // reserved record before offering the action again.
      try {
        setSnapshot(await getSnapshot('/api/sessions/current'))
      } catch {
        setPayoutStateUnverified(true)
      }
      setError(message)
    } finally {
      setCreatingPayout(false)
    }
  }

  async function switchToBackup() {
    if (switchingBackup) return
    setSwitchingBackup(true)
    setError('')
    try {
      const next = await getSnapshot('/api/sessions/backup', 'POST')
      if (!next) throw new Error('Session expired. Start a new demo session.')
      setSnapshot(next)
      setInvestigation(null)
      setDailyLimitReached(false)
      setPollingPaused(false)
      setPayoutStateUnverified(false)
      setApprovalError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not open the backup payout')
    } finally {
      setSwitchingBackup(false)
    }
  }

  async function verifyPayoutState() {
    setError('')
    try {
      setSnapshot(await getSnapshot('/api/sessions/current'))
      setPayoutStateUnverified(false)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not verify payout state')
    }
  }

  async function checkPayout() {
    setCheckingPayout(true)
    setError('')
    try {
      const next = await getSnapshot(awaitingBatch ? '/api/sessions/current' : '/api/payouts/reconcile', awaitingBatch ? 'GET' : 'POST')
      setSnapshot(next)
      setPollingPaused(false)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not check the payout item')
    } finally {
      setCheckingPayout(false)
    }
  }

  async function investigate() {
    setInvestigating(true)
    setInvestigationError('')
    setApprovalError('')
    try {
      const response = await fetch(apiPath('/api/incidents/investigate'), { method: 'POST', credentials: 'same-origin', cache: 'no-store' })
      if (!response.ok) {
        const body = await response.json().catch(() => ({})) as { error?: string }
        throw new Error(body.error || `Investigation failed (${response.status})`)
      }
      setInvestigation(await response.json() as Investigation)
      try {
        setSnapshot(await getSnapshot('/api/sessions/current'))
      } catch {
        // The diagnosis is still available even if refreshing the event history fails.
      }
    } catch (cause) {
      setInvestigationError(cause instanceof Error ? cause.message : 'Investigation unavailable')
    } finally {
      setInvestigating(false)
    }
  }

  async function approveCorrection() {
    if (!snapshot?.incident || approving) return
    setApproving(true)
    setApprovalError('')
    try {
      const response = await fetch(apiPath('/api/incidents/approve'), {
        method: 'POST', credentials: 'same-origin', cache: 'no-store',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ incident_id: snapshot.incident.id }),
      })
      const body = await response.json().catch(() => ({})) as { error?: string }
      if (!response.ok) setApprovalError(body.error || `Approval could not proceed (${response.status})`)
    } catch (cause) {
      setApprovalError(cause instanceof Error ? cause.message : 'Could not check record correction')
    } finally {
      setApproving(false)
      try { setSnapshot(await getSnapshot('/api/sessions/current')) } catch { /* Keep the rejection visible. */ }
    }
  }

  return <div className="app-shell">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">R</span>RecoveryOS</div>
      <div className="topbar-right"><span className="environment"><span className="pulse" /> Sandbox environment</span>{snapshot && <span className="session-pill">Session {snapshot.session.id.slice(0, 8)}</span>}</div>
    </header>
    <div className="workspace">
      <aside className="sidebar" aria-label="Main navigation">
        <span className="sidebar-label">WORKSPACE</span>
        <button aria-label="Overview" className={view === 'overview' ? 'nav active' : 'nav'} onClick={() => setView('overview')}>▦ <span>Overview</span></button>
        <button aria-label="Incident Lab" className={view === 'lab' ? 'nav active' : 'nav'} onClick={() => setView('lab')}>◇ <span>Incident Lab</span></button>
        <div className="sidebar-bottom">● &nbsp;Demo workspace</div>
      </aside>
      <main className="content">
        {loading ? <p className="loading" role="status">Loading your workspace…</p> : !snapshot ?
          <section className="welcome"><div className="eyebrow">WELCOME TO RECOVERYOS</div><h1>See recovery in action.</h1><p>Start a private demo workspace to explore how an incident moves from detection to a safe resolution.</p><div className="welcome-card"><span className="welcome-icon">◇</span><div><strong>Your own Incident Lab</strong><small>No account or credentials needed. Your session and activity are separate from other visitors.</small></div></div><button className="primary" onClick={startSession} disabled={starting}>{starting ? 'Starting…' : 'Start demo session'} ↗</button>{error && <p className="error" role="alert">{error}</p>}</section>
          : view === 'overview' ? <Overview snapshot={snapshot} onOpen={() => setView('lab')} />
          : <section className="page">
            <div className="eyebrow">DEMO WORKSPACE / INCIDENT LAB</div>
            <h1>Incident Lab</h1>
            <p className="intro">Follow one payout from PayPal evidence to a verified record correction. Each action and conclusion stays in this session.</p>
            <div className="lab-layout">
              <div className="lab-main">
                <Journey snapshot={snapshot} investigation={investigation} />
                <PayoutPanel payout={payout} creating={creatingPayout} checking={checkingPayout} pollingPaused={pollingPaused} stateUnverified={payoutStateUnverified} dailyLimitReached={dailyLimitReached} now={now} switchingBackup={switchingBackup} error={error} onCreate={startPayout} onVerify={verifyPayoutState} onCheck={checkPayout} onBackup={switchToBackup} />
                {snapshot.incident && payout && <>
                  <IncidentEvidence payout={payout} incident={snapshot.incident} events={snapshot.events} />
                  <InvestigationPanel incident={snapshot.incident} result={investigation?.incident_id === snapshot.incident.id ? investigation : null} loading={investigating} error={investigationError} onInvestigate={investigate} approving={approving} approvalError={approvalError} onApprove={approveCorrection} />
                </>}
                <AuditHistory events={snapshot.events} />
              </div>
              <aside className="lab-aside">
                <div className="panel"><span className="eyebrow">SESSION STATE</span><h2>At a glance</h2><State label="Payout record" value={payout?.status ?? 'None yet'} /><State label="Provider item" value={payout?.provider_status ?? '—'} /><State label="Incident" value={snapshot.incident?.status ?? 'None'} /><State label="Events" value={String(snapshot.events.length)} /></div>
                <div className="aside-note">ⓘ &nbsp;PayPal credentials stay on the server. This browser receives only its own session state.</div>
              </aside>
            </div>
          </section>}

      </main>
    </div>
  </div>
}

export function Overview({ snapshot, onOpen }: { snapshot: Snapshot; onOpen: () => void }) {
  const { payout, incident, investigation } = snapshot
  const resolved = incident?.status === 'resolved'
  let next = 'Create a Sandbox payout'
  if (payout) next = 'Wait for PayPal item evidence'
  if (incident) next = 'Investigate the discrepancy'
  if (investigation) next = investigation.proposed_action === 'correct_payout_record' ? 'Review and approve the proposed correction' : 'Review diagnosis and evidence'
  if (resolved) next = 'Review the result and audit history'
  return <section className="page">
    <div className="eyebrow">YOUR WORKSPACE</div><h1>Overview</h1>
    <p className="intro">One private demo session. The Incident Lab holds the evidence and recovery actions.</p>
    <div className="metrics">
      <Metric label="Payout record" value={payout?.status ?? 'Not started'} detail={payout ? (payout.origin === 'precreated' ? 'Pre-created Sandbox payout' : 'Live Sandbox payout') : 'No payout in this session'} />
      <Metric label="PayPal item" value={payout?.provider_status ?? 'Not observed'} detail={payout?.item_id ?? 'Item-level status'} />
      <Metric label="Incident" value={incident?.status ?? 'None'} detail={incident ? `Case ${incident.id.slice(0, 8)}` : 'No discrepancy detected'} />
    </div>
    <div className="feature-card"><div><div className="eyebrow">NEXT STEP</div><h2>{next}</h2><p>{resolved ? 'Inspect the verified PayPal state, corrected record, and complete event history.' : 'Open Incident Lab to continue the guided recovery journey.'}</p></div><button className="primary" onClick={onOpen}>Open Incident Lab ↗</button></div>
    {incident && <div className="overview-case"><span className="eyebrow">{resolved ? 'RESOLVED CASE' : 'ACTIVE CASE'}</span><strong>{resolved ? 'Payout record corrected' : 'PayPal payout completed, record still pending'}</strong><span>{payout?.item_id ?? 'Item pending'} · {incident.status}</span><button className="text-action" onClick={onOpen}>View case and evidence →</button></div>}
    <p className="session-expiry">Session expires {new Date(snapshot.session.expires_at).toLocaleString()}.</p>
  </section>
}

export function Journey({ snapshot, investigation }: { snapshot: Snapshot; investigation: Investigation | null }) {
  const { payout, incident } = snapshot
  return <div className="panel journey-panel"><span className="eyebrow">GUIDED JOURNEY</span><h2>{incident?.status === 'resolved' ? 'Recovery complete' : 'Follow the discrepancy to resolution'}</h2>
    <Step number="01" title="Start a private session" detail="Your isolated workspace is ready." done />
    <Step number="02" title="Observe a Sandbox payout" detail={payout?.item_id ? `PayPal item ${payout.item_id}: ${payout.provider_status ?? 'awaiting status'}` : 'Create a payout and wait for its specific PayPal item.'} done={payout?.provider_status === 'SUCCESS'} />
    <Step number="03" title="Detect the discrepancy" detail={incident ? 'The injected missed update left the record pending; reconciliation opened this incident.' : 'Detection runs automatically when the PayPal item succeeds.'} done={!!incident} />
    <Step number="04" title="Investigate and review" detail={investigation ? `${investigation.origin === 'ai' ? 'AI' : 'Deterministic fallback'} diagnosis is ready for your review.` : 'Start the investigation when the incident appears.'} done={!!investigation} />
    <Step number="05" title={investigation?.proposed_action === 'no_change' ? 'Review the decision' : 'Approve and verify correction'} detail={incident?.status === 'resolved' ? 'A fresh PayPal check confirmed SUCCESS; the record was corrected once.' : investigation?.proposed_action === 'no_change' ? 'No record correction proposed. Review the evidence or run the investigation again.' : 'Approve the proposal to trigger a fresh PayPal check and safe record correction.'} done={incident?.status === 'resolved'} />
  </div>
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <div className="metric"><small>{label}</small><strong>{value}</strong><small>{detail}</small></div>
}
function Step({ number, title, detail, done }: { number: string; title: string; detail: string; done: boolean }) {
  return <div className="step"><span className={done ? 'step-number done' : 'step-number'}>{done ? '✓' : number}</span><div><strong>{title}</strong><p>{detail}</p></div></div>
}
function State({ label, value }: { label: string; value: string }) {
  return <div className="state-row"><span>{label}</span><strong>{value}</strong></div>
}

export function IncidentEvidence({ payout, incident, events }: { payout: Payout; incident: Incident; events: Event[] }) {
  const verified = events.some(event => event.kind === 'provider_verification')
  return <div className="panel incident-evidence">
    <span className="eyebrow">{incident.status.toUpperCase()} INCIDENT · {incident.id.slice(0, 8)}</span>
    <h2>{incident.status === 'resolved' ? 'Reconciled payout record' : 'PayPal payout completed, internal record pending'}</h2>
    <p>{incident.status === 'resolved' ? 'The payout record now agrees with the verified PayPal item. This incident is resolved.' : 'Reconciliation confirmed the PayPal item succeeded while the payout record still says pending.'}</p>
    <div className="evidence-columns">
      <div className="provider-evidence"><span className="event-source paypal">{payout.origin === 'precreated' ? 'Pre-created PayPal Sandbox evidence' : 'Genuine PayPal response'}</span><State label="Item ID" value={payout.item_id ?? '—'} /><State label="Item status" value={payout.provider_status ?? '—'} />{verified && <p>Fresh item verification was recorded at approval.</p>}</div>
      <div><strong>Internal payout record</strong><State label="Record status" value={payout.status} /><p className="injected-note">{incident.status === 'resolved' ? 'The approved correction repaired the deliberately skipped record update.' : 'Injected fault: RecoveryOS deliberately skipped the record update.'}</p></div>
    </div>
  </div>
}

export function AuditHistory({ events }: { events: Event[] }) {
  const columns: ColDef<Event>[] = [
    { headerName: 'Source', valueGetter: params => params.data ? eventPresentation(params.data).label : '', minWidth: 150, flex: 1, cellClass: params => params.data ? eventPresentation(params.data).className : '' },
    { headerName: 'Event', valueGetter: params => params.data ? eventNames[params.data.kind] ?? params.data.kind.replaceAll('_', ' ') : '', minWidth: 165, flex: 1 },
    { headerName: 'Detail', field: 'details', flex: 2, minWidth: 230, wrapText: true, autoHeight: true },
    { headerName: 'When', valueGetter: params => params.data ? new Date(params.data.occurred_at).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' }) : '', minWidth: 150, flex: 1 },
  ]
  return <div className="panel audit-panel"><span className="eyebrow">IMMUTABLE EVENT HISTORY</span><h2>Audit trail</h2><p>PayPal responses, injected faults, conclusions, and operator actions are labeled by source.</p>
    {events.length ? <div className="audit-grid" role="region" aria-label="Event history"><AgGridReact<Event> rowData={events} columnDefs={columns} theme={themeQuartz} domLayout="autoHeight" defaultColDef={{ sortable: true, resizable: true }} getRowId={params => String(params.data.id)} rowHeight={52} /></div> : <p className="empty-history">Events will appear as the payout journey progresses.</p>}
  </div>
}

export function InvestigationPanel({ incident, result, loading, error, onInvestigate, approving, approvalError, onApprove }: { incident: Incident; result: Investigation | null; loading: boolean; error: string; onInvestigate: () => void; approving?: boolean; approvalError?: string; onApprove?: () => void }) {
  const labels = { provider: 'PayPal provider', internal: 'Internal record', injected: 'Injected event', policy: 'Recovery policy' }
  return <div className="panel investigation-panel">
    <span className="eyebrow">INCIDENT INVESTIGATION · {incident.id.slice(0, 8)}</span>
    <h2>Investigate the discrepancy</h2>
    <p>{incident.status === 'resolved' ? 'Record correction approved. The diagnosis and cited evidence remain available below.' : 'The read-only investigation uses the incident, payout record, current PayPal item, event history, and recovery policy.'}</p>
    {incident.status === 'open' && <button className="primary" onClick={onInvestigate} disabled={loading}>{loading ? 'Investigating…' : result ? 'Run investigation again' : 'Start AI investigation ↗'}</button>}
    {loading && <p role="status">Reading evidence and preparing a diagnosis…</p>}
    {error && <p className="error" role="alert">{error}</p>}
    {result && <div className="investigation-result">
      <span className={result.origin === 'ai' ? 'status diagnosis-ai' : 'status diagnosis-fallback'}>{result.origin === 'ai' ? 'AI diagnosis' : 'Deterministic fallback diagnosis'}</span>
      {result.origin === 'deterministic_fallback' && <p>AI analysis was unavailable or invalid. This diagnosis uses verified evidence and fixed recovery rules.</p>}
      <State label="Incident" value={result.incident_id} />
      <State label="PayPal item" value={`${result.item_id} · ${result.item_status}`} />
      <h3>Diagnosis</h3><p>{result.diagnosis}</p>
      <h3>Proposed action</h3><p>{result.proposed_action === 'correct_payout_record' ? 'Correct the internal payout record after operator approval and fresh PayPal verification.' : 'Leave the payout record unchanged.'}</p>
      {incident.status === 'open' && result.proposed_action === 'correct_payout_record' && onApprove && <button className="secondary" onClick={onApprove} disabled={approving}>{approving ? 'Checking PayPal item…' : 'Approve record correction'}</button>}
      {approvalError && <p className="error" role="alert">{approvalError}</p>}
      <h3>Cited evidence</h3>
      <ul className="investigation-evidence">{result.evidence.map((evidence, index) => <li key={`${evidence.source}-${evidence.reference}-${index}`}><span className={`event-source ${evidence.source === 'provider' ? 'paypal' : evidence.source}`}>{labels[evidence.source]}</span><code>{evidence.reference}</code><p>{evidence.finding}</p></li>)}</ul>
    </div>}
  </div>
}

export function PayoutPanel({ payout, creating, checking, pollingPaused, stateUnverified, dailyLimitReached, now, switchingBackup, error, onCreate, onVerify, onCheck, onBackup }: {
  payout?: Payout
  creating: boolean
  checking: boolean
  pollingPaused: boolean
  stateUnverified: boolean
  dailyLimitReached: boolean
  now: number
  switchingBackup: boolean
  error: string
  onCreate: () => void
  onVerify: () => void
  onCheck: () => void
  onBackup: () => void
}) {
  const pending = payout?.status === 'pending' && (!payout.provider_status || payout.provider_status === 'PENDING')
  const backupAvailable = dailyLimitReached || (!!payout && payout.origin === 'live' && payout.provider_status !== 'SUCCESS' && now - new Date(payout.created_at).getTime() >= 30_000)
  return <div className="panel payout-panel">
    <span className="eyebrow">PAYPAL SANDBOX</span>
    <h2>Observe a payout item</h2>
    {!payout ? <>
      <p>Start one genuine Sandbox payout for this session. RecoveryOS will follow its specific item through PayPal.</p>
      {stateUnverified ? <button className="primary" onClick={onVerify}>Verify session state</button>
        : <button className="primary" onClick={onCreate} disabled={creating || dailyLimitReached}>
          {creating ? 'Creating payout and checking PayPal…' : dailyLimitReached ? 'Daily Sandbox limit reached' : 'Create Sandbox payout ↗'}
        </button>}
      {creating && <p role="status">Waiting for PayPal item evidence, up to 25 seconds…</p>}
    </> : <>
      {payout.origin === 'precreated' && <p className="injected-note">Pre-created Sandbox payout. This session has its own payout record and incident; the PayPal item was created earlier.</p>}
      <p>{payout.status === 'provider_unknown'
        ? 'PayPal creation could not be confirmed. Keep this creation key for investigation; this session will not create another payout.'
        : payout.status === 'creating' ? 'The payout request is in progress. Waiting for its batch reference.'
          : pending ? 'Waiting for the specific PayPal payout item. This may take a few moments.'
            : 'The provider item result is shown below.'}</p>
      <div className="payout-details">
        <State label={payout.origin === 'precreated' ? 'Original creation key' : 'Creation key'} value={payout.creation_key} />
        <State label="Batch ID" value={payout.batch_id ?? 'Awaiting batch'} />
        <State label="Payout item ID" value={payout.item_id ?? 'Awaiting item'} />
        <State label="Payout item status" value={payout.provider_status ?? 'Awaiting item'} />
        <State label="Internal record" value={payout.status} />
      </div>
      {checking && <p role="status">Checking the payout item…</p>}
      {pollingPaused && <button className="secondary" onClick={onCheck} disabled={checking}>Check payout item again</button>}
    </>}
    {backupAvailable && <div className="backup-option">
      <p>{dailyLimitReached ? 'The public daily creation limit has been reached.' : 'This live payout has not reached item-level SUCCESS within 30 seconds.'} Use a previously completed Sandbox payout to continue the incident journey. This opens a new demo session; your current payout record stays with this session.</p>
      <button className="secondary" onClick={onBackup} disabled={switchingBackup}>{switchingBackup ? 'Opening backup…' : 'Use pre-created Sandbox payout'}</button>
    </div>}
    {error && <p className="error" role="alert">{error}</p>}
  </div>
}
