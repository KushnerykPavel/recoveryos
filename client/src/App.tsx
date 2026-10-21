import { useEffect, useState } from 'react'

type Session = { id: string; created_at: string; expires_at: string }
type Payout = { id: string; creation_key: string; batch_id?: string; item_id?: string; status: string; provider_status?: string }
type Incident = { id: string; status: string }
type Event = { id: number; source: string; kind: string }
type Snapshot = { session: Session; payout?: Payout; incident?: Incident; events: Event[] }
type View = 'overview' | 'lab'

async function getSnapshot(path: string, method: 'GET' | 'POST' = 'GET', headers?: HeadersInit): Promise<Snapshot | null> {
  const response = await fetch(path, { method, headers, credentials: 'same-origin', cache: 'no-store' })
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
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    getSnapshot('/api/sessions/current')
      .then(value => { if (active) setSnapshot(value) })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : 'Could not reach the API') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const payout = snapshot?.payout
  const awaitingBatch = payout?.status === 'creating'
  const awaitingItem = payout?.status === 'pending' && !!payout.batch_id && (!payout.provider_status || payout.provider_status === 'PENDING')

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

  return <div className="app-shell">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">R</span>RecoveryOS</div>
      <div className="topbar-right"><span className="environment"><span className="pulse" /> Sandbox environment</span>{snapshot && <span className="session-pill">Session {snapshot.session.id.slice(0, 8)}</span>}</div>
    </header>
    <div className="workspace">
      <aside className="sidebar" aria-label="Main navigation">
        <span className="sidebar-label">WORKSPACE</span>
        <button className={view === 'overview' ? 'nav active' : 'nav'} onClick={() => setView('overview')}>▦ <span>Overview</span></button>
        <button className={view === 'lab' ? 'nav active' : 'nav'} onClick={() => setView('lab')}>◇ <span>Incident Lab</span></button>
        <div className="sidebar-bottom">● &nbsp;Demo workspace</div>
      </aside>
      <main className="content">
        {loading ? <p className="loading" role="status">Loading your workspace…</p> : !snapshot ?
          <section className="welcome"><div className="eyebrow">WELCOME TO RECOVERYOS</div><h1>See recovery in action.</h1><p>Start a private demo workspace to explore how an incident moves from detection to a safe resolution.</p><div className="welcome-card"><span className="welcome-icon">◇</span><div><strong>Your own Incident Lab</strong><small>No account or credentials needed. Your session and activity are separate from other visitors.</small></div></div><button className="primary" onClick={startSession} disabled={starting}>{starting ? 'Starting…' : 'Start demo session'} ↗</button>{error && <p className="error" role="alert">{error}</p>}</section>
          : view === 'overview' ? <section className="page"><div className="eyebrow">YOUR WORKSPACE</div><h1>Overview</h1><p className="intro">A quick view of your private demo session.</p><div className="metrics"><Metric label="Demo session" value="Active" detail={`Expires ${new Date(snapshot.session.expires_at).toLocaleString()}`} /><Metric label="Payout record" value={snapshot.payout?.status ?? 'Not started'} detail="Your session only" /><Metric label="Open incident" value={snapshot.incident?.status === 'open' ? '1' : '0'} detail="In this workspace" /></div><div className="feature-card"><div><div className="eyebrow">NEXT STEP</div><h2>Explore the Incident Lab</h2><p>Follow the demo journey and inspect the state attached to this session.</p></div><button className="primary" onClick={() => setView('lab')}>Open Incident Lab ↗</button></div></section>
          : <section className="page"><div className="eyebrow">DEMO WORKSPACE / INCIDENT LAB</div><h1>Incident Lab</h1><p className="intro">Your isolated space for a payout recovery walkthrough.</p><div className="lab-layout"><div className="lab-main"><div className="panel"><div className="panel-header"><div><span className="eyebrow">CURRENT SESSION</span><h2>Ready to investigate</h2></div><span className="status">Active</span></div><p>This workspace is linked to session <code>{snapshot.session.id}</code>. Its payout and incident data belong only to this browser session.</p><div className="session-meta"><div><small>Started</small><strong>{new Date(snapshot.session.created_at).toLocaleString()}</strong></div><div><small>Expires</small><strong>{new Date(snapshot.session.expires_at).toLocaleString()}</strong></div></div></div><PayoutPanel payout={payout} incident={snapshot.incident} creating={creatingPayout} checking={checkingPayout} pollingPaused={pollingPaused} stateUnverified={payoutStateUnverified} dailyLimitReached={dailyLimitReached} error={error} onCreate={startPayout} onVerify={verifyPayoutState} onCheck={checkPayout} /><div className="panel"><span className="eyebrow">JOURNEY</span><h2>Recovery workflow</h2><Step number="01" title="Start a private session" detail="Your workspace is ready." done /><Step number="02" title="Observe a Sandbox payout" detail={payout ? `PayPal item: ${payout.provider_status ?? 'awaiting status'}` : 'Create a payout to observe its PayPal item.'} done={!!payout?.item_id && payout.provider_status === 'SUCCESS'} /><Step number="03" title="Detect a discrepancy" detail={snapshot.incident ? `Injected fault opened incident ${snapshot.incident.id.slice(0, 8)}.` : 'Reconciliation and evidence appear here when an incident is detected.'} done={!!snapshot.incident} /><Step number="04" title="Review and resolve" detail="Investigation and approval will be added in a later stage." done={snapshot.incident?.status === 'resolved'} /></div></div><aside className="lab-aside"><div className="panel"><span className="eyebrow">SESSION STATE</span><h2>At a glance</h2><State label="Payout record" value={payout?.status ?? 'None yet'} /><State label="Provider item" value={snapshot.payout?.provider_status ?? '—'} /><State label="Incident" value={snapshot.incident?.status ?? 'None'} /><State label="Events" value={String(snapshot.events.length)} /></div><div className="aside-note">ⓘ &nbsp;PayPal credentials stay on the server. This browser receives only its own session state.</div></aside></div></section>}
      </main>
    </div>
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

function PayoutPanel({ payout, incident, creating, checking, pollingPaused, stateUnverified, dailyLimitReached, error, onCreate, onVerify, onCheck }: {
  payout?: Payout
  incident?: Incident
  creating: boolean
  checking: boolean
  pollingPaused: boolean
  stateUnverified: boolean
  dailyLimitReached: boolean
  error: string
  onCreate: () => void
  onVerify: () => void
  onCheck: () => void
}) {
  const pending = payout?.status === 'pending' && (!payout.provider_status || payout.provider_status === 'PENDING')
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
      <p>{payout.status === 'provider_unknown'
        ? 'PayPal creation could not be confirmed. Keep this creation key for investigation; this session will not create another payout.'
        : payout.status === 'creating' ? 'The payout request is in progress. Waiting for its batch reference.'
          : pending ? 'Waiting for the specific PayPal payout item. This may take a few moments.'
            : 'The provider item result is shown below.'}</p>
      <div className="payout-details">
        <State label="Creation key" value={payout.creation_key} />
        <State label="Batch ID" value={payout.batch_id ?? 'Awaiting batch'} />
        <State label="Payout item ID" value={payout.item_id ?? 'Awaiting item'} />
        <State label="Payout item status" value={payout.provider_status ?? 'Awaiting item'} />
        <State label="Internal record" value={payout.status} />
      </div>
      {checking && <p role="status">Checking the payout item…</p>}
      {pollingPaused && <button className="secondary" onClick={onCheck} disabled={checking}>Check payout item again</button>}
      {incident && <p className="injected-note">Injected fault: RecoveryOS deliberately skipped the internal record update after PayPal reported item SUCCESS.</p>}
    </>}
    {error && <p className="error" role="alert">{error}</p>}
  </div>
}
