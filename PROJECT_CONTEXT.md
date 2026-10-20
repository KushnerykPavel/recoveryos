# RecoveryOS — Project Context

## 1. Project objective

**RecoveryOS — AI Incident Response for Payments**

Build an AI-powered payment operations application that detects, investigates, and safely resolves inconsistencies between PayPal payouts and internal payout records.

**Hackathon deadline:** November 12, 2026.

**Primary prize targets:** Overall Placement, Best Use of PayPal + AI, Best Use of Agentic Commerce, and AG Grid sponsor award.

## 2. Problem

Marketplaces experience payment incidents involving failed payouts, missed webhooks, duplicate events, uncertain API outcomes, and inconsistent transaction records.

RecoveryOS reduces manual investigation by combining provider evidence, internal transaction records, and AI-assisted root-cause analysis.

## 3. MVP user journey

The application will provide a **hosted self-service demo**, requiring no registration or PayPal API credentials from judges.

The main workflow:

1. Open the RecoveryOS dashboard.
2. Select a payment incident in **Incident Lab**.
3. Create a genuine PayPal Sandbox payout and deliberately skip its internal ledger update. The injected fault is shown explicitly in the demo.
4. Detect the inconsistency between PayPal and the internal payout record.
5. Start an AI investigation.
6. AI retrieves evidence, determines the probable root cause, and proposes a recovery action.
7. Operator approves the proposed action.
8. Go backend fetches the specific payout item again at approval time, verifies `SUCCESS`, and corrects only the session's internal payout record. If verification fails or the item is no longer `SUCCESS`, the action is rejected and the incident remains open.
9. Dashboard displays the reconciled state and audit trail.

**Critical requirement:** No duplicate payment can result from an AI recommendation. The payout record is an operational state record with an immutable event history, not a double-entry accounting ledger.

## 4. Technology stack

- **Backend:** Go
- **Database:** PostgreSQL
- **Frontend:** React + TypeScript
- **Dashboard:** AG Studio
- **AI:** One investigation agent using an LLM with function calling and structured outputs
- **Payments:** PayPal Sandbox, primarily Payouts API
- **Infrastructure:** Hosted application on my-vps with HTTPS
- **Architecture:** Modular monolith

The initial LLM is OpenAI GPT-6 Luna via the Responses API. Keep the model configurable. No multi-agent framework or vector database is required for the MVP.

If the LLM is unavailable, the application may use a deterministic diagnosis that is clearly labeled as a fallback. The backend rejects recommendations unsupported by provider evidence; the MVP has no policy override.

Each judge uses an isolated demo session with no registration or PayPal credentials. The primary path creates one new Sandbox payout per session. If its item has not reached `SUCCESS` within 25–30 seconds, or a global daily creation limit has been reached, the interface offers a clearly labeled fallback based on a pre-created genuine Sandbox payout. A Sandbox Business account, API credentials, and create/read access to payout items are already available.

## 5. MVP functionality

**P0 — Mandatory**

- Real PayPal Sandbox integration
- Internal payout records and immutable event history
- Reconciliation engine
- Incident detection
- AI investigation agent
- Structured diagnosis and evidence
- Deterministic recovery policy engine
- Human approval workflow
- Audit logging
- AG Studio dashboard
- Hosted interactive demo

AG Studio is the preferred dashboard implementation. Request a trial license early; use AG Grid tables in the hosted demo if the AG Studio license is unavailable by the integration deadline.

**P1 — If time permits**

- Unknown payment outcome investigation
- Duplicate webhook handling
- Unclaimed payout investigation
- AG Studio agent-driven analytics
- Webhook signature verification and event processing (first P1 priority)

## 6. AI responsibilities

The agent investigates incidents using read-only tools:

- `get_incident`
- `get_internal_payment`
- `get_paypal_payout`
- `get_payment_events`
- `get_recovery_policy`

It produces a structured diagnosis, supporting evidence, and recommended action.

**AI cannot independently send money, modify balances, or bypass financial safety policies.**

## 7. Testing strategy

Three testing layers:

**Real PayPal Sandbox:** Execute genuine API operations and retrieve provider transaction states.

**Fault injection:** Introduce controlled missing events, API timeouts, duplicate notifications, and stale ledger records.

**Automated verification:** Test financial invariants, state transitions, idempotency, evidence accuracy, and AI recommendations.

The primary MVP scenario is **a successful PayPal payout item with a stale internal payout record caused by a deliberately skipped internal update**. The record correction requires an independently verified item-level `SUCCESS`; `PENDING`, `FAILED`, `UNCLAIMED`, and unknown states leave the incident open.

Injected incidents must be clearly distinguished from genuine provider responses.

## 8. Product interface

The application has two principal areas:

**Overview:** A concise summary of payout and reconciliation status, active incidents, and a route into Incident Lab. Full investigation, recovery, and audit details live in Incident Lab for the MVP.

**Incident Lab:** Interactive testing environment where judges can reproduce payment incidents and observe how RecoveryOS resolves them.

After the injected internal update loss, reconciliation runs automatically and surfaces the incident. A manual recheck control may be offered but is not required to complete the demo.

The judge should complete the main workflow in approximately two minutes.

Public demo controls: at most one newly created payout per session and a global daily creation limit. When a limit is reached, new sessions use the pre-created genuine payout fallback while keeping their payout records and incidents isolated.

## 9. Explicitly out of scope

- Production money movement
- Full merchant onboarding
- Multi-tenant SaaS infrastructure
- Multiple payment providers
- Accounting and ERP integrations
- Mobile applications
- Multi-agent orchestration
- Automated transfers without human approval

## 10. Definition of success

RecoveryOS must demonstrate that an AI agent can investigate a genuine PayPal Sandbox transaction discrepancy, recommend a justified recovery action, and resolve the incident through a deterministic, auditable workflow without creating another payout.

The completed submission needs a public open-source repository, runnable or hosted demo, documentation, and a YouTube video shorter than three minutes.
