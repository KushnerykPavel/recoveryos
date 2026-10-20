# RecoveryOS hackathon roadmap

**Status:** Agreed; implementation will proceed sequentially when requested. **Owner:** one developer. **Capacity:** 15–20 hours per week, approximately 50–70 hours before submission. **Submission deadline:** 12 November 2026, 21:00 Warsaw time. Target submission: 11 November, leaving a one-day buffer.

## Outcome to deliver

A judge opens a hosted HTTPS demo without an account, starts an isolated Incident Lab session, sees a genuine PayPal Sandbox payout item reach `SUCCESS`, and observes an explicitly injected missed internal update. Automatic reconciliation finds the discrepancy. GPT-6 Luna investigates through read-only tools and produces structured evidence and a proposed record correction. After the judge approves, the Go backend fetches the payout item again, applies its own policy, updates only the operational payout record, and writes an immutable audit event. The journey should fit in about two minutes.

The backup journey uses a pre-created genuine Sandbox payout when a new item has not reached `SUCCESS` within 25–30 seconds or the daily creation cap is reached. The UI labels both the injected fault and any backup or deterministic diagnosis. It never describes a simulated event as a PayPal response or a fallback as an AI result.

## Schedule and gates

| Dates | Budget | Deliverable | Exit gate |
|---|---:|---|---|
| **19–21 Oct** | 6 h | Confirm a fresh Sandbox payout item can be created and read; measure time to `SUCCESS`; prepare a successful backup item. Test a minimal GPT-6 Luna function call and structured diagnosis. Request AG Studio trial license. Confirm my-vps deployment path, HTTPS, PostgreSQL, and secret storage. | Real provider item evidence and an explicit decision on the live versus backup demo path. |
| **22–26 Oct** | 12 h | Implement the Go and PostgreSQL vertical slice: isolated demo session, payout creation with a stable idempotency key, payout record, immutable event history, fault injection, provider item read, and automatic reconciliation. | A real `SUCCESS` item and deliberately stale record produce an incident without AI or UI shortcuts. |
| **27 Oct–1 Nov** | 14 h | Add five read-only investigation tools, GPT-6 Luna diagnosis, structured evidence, clearly labeled deterministic fallback, policy evaluation, approval, fresh item verification, atomic record update, and audit events. | Tests prove that unsupported recommendations, non-`SUCCESS` or unavailable provider states, repeated approvals, and concurrent approvals cannot produce a correction or another payout. |
| **2–6 Nov** | 12 h | Build the concise Overview and complete Incident Lab in React and TypeScript; use AG Studio if a production-ready license is available, otherwise AG Grid. Deploy to my-vps with HTTPS, session isolation, one fresh payout per session, a configurable daily cap, and the backup path. | An unauthenticated visitor completes the hosted journey in about two minutes; the UI distinguishes real provider evidence, injected faults, and fallbacks. |
| **7–9 Nov** | 9 h | Run end-to-end rehearsals, fix blockers, publish English README and setup instructions, add an open-source license, and produce a public YouTube video under three minutes. If and only if P0 passes its gate, use remaining time for a real webhook with signature verification. | Hosted demo, repository, documentation, and video are ready for submission. |
| **10–11 Nov** | 6 h | Test the public link and clean-room setup instructions; verify repository visibility, license, video, and submission fields; submit by 11 Nov. | Submission complete with one day of schedule buffer. |

**Total planned effort:** 59 hours. Reallocate time from P1 to any incomplete P0 gate. Keep the hosted demo accessible through the judging period ending 15 December 2026.

## P0 implementation boundaries

- **Financial safety:** The AI has no payout creation, retry, or record mutation tool. Recovery only corrects an internal payout record. The backend requires item-level `SUCCESS`, a fresh PayPal read at approval, matching payout identity, a still-stale record, session ownership, and a single-use approval. The correction and audit event commit together in one database transaction.
- **Idempotency:** Each live demo run has one stable payout creation key. Ambiguous create outcomes are investigated using that key and provider reads; they do not trigger a new payout request with a new key. The application enforces its own one-payout-per-session rule in addition to PayPal's finite idempotency window.
- **Evidence:** Store provider response references and timestamps separately from injected events and AI conclusions. Show the exact item status used for diagnosis and the later status used for approval.
- **Public demo:** No registration or judge credentials. Each session has its own payout record, incident, approvals, and audit view. A configurable global daily cap switches new sessions to the labeled pre-created payout path.
- **Overview:** Only the summary and route into Incident Lab are required for P0. Full investigation and recovery details live in Incident Lab.
- **Scope cut:** Unknown outcomes, duplicate webhook handling, unclaimed payouts, agent-driven analytics, and accounting postings do not block submission. A genuine webhook with signature verification is the first P1 item.

## Verification checklist

1. Genuine Sandbox payout item `SUCCESS` is recorded with its item ID; batch-level success alone is never treated as payout success.
2. The deliberately skipped internal update is visible and labeled as injected. Reconciliation creates the incident automatically.
3. The AI cites evidence from read-only tools and returns a schema-valid diagnosis; fallback diagnosis is labeled.
4. Approval performs a new provider item read. Provider error, changed status, wrong item, stale approval, or unsupported AI recommendation leaves the record unchanged and the incident open.
5. Duplicate and concurrent approval requests result in at most one record correction and one corresponding audit event. No approval code path sends a payout.
6. Two anonymous sessions cannot read or mutate each other's records. Daily payout cap and one-new-payout-per-session rules work.
7. Fresh and backup paths both finish the judge workflow in about two minutes on the hosted site. Rehearse with a clean browser session.
8. Public repository, visible license, English documentation, hosted demo, and public sub-three-minute video are ready before submission.

## External dependencies to settle early

- **PayPal timing:** Create/read access is available, but item completion latency must be measured. A batch `SUCCESS` does not guarantee item `SUCCESS`.
- **AG Studio:** Confirm the trial license permits the hosted demo for the judging period. Use AG Grid for hosted tables if it cannot be secured by 2 November.
- **my-vps:** Confirm deployment access, HTTPS, PostgreSQL backups, and a process restart path before UI integration.
- **LLM:** Use `gpt-6-luna` through the Responses API with function calling and structured outputs. Keep its model ID configurable; test it against representative incident evidence before locking the prompt and schema.

## Source notes

- [Hackathon rules and submission deadline](https://paypalaihackathon.devpost.com/rules)
- [PayPal payout batch status](https://developer.paypal.com/api/payments.payouts-batch/v1/definitions/payout_batch_header/) and [item status](https://developer.paypal.com/api/payments.payouts-batch/v1/definitions/transaction_enum/)
- [PayPal payout idempotency guidance](https://developer.paypal.com/docs/payouts/standard/integrate-api/customize/)
- [OpenAI Docs: GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna)
- [AG Studio React installation and license note](https://www.ag-grid.com/studio/react/installation/)
