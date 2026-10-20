# RecoveryOS MVP specification

**Status:** Agreed for publication. This specification describes the agreed P0 hackathon application. Implementation proceeds sequentially when requested.

## Problem Statement

Payment operators can see a PayPal payout succeed while their internal payout record remains stale. Investigating the discrepancy requires gathering provider and internal evidence, deciding whether a correction is safe, and documenting the decision. A manual process is slow, and an unsafe retry could pay the recipient twice. Hackathon judges need to experience this problem and its resolution in a hosted, self-service demo without credentials, in about two minutes.

## Solution

RecoveryOS gives each visitor an isolated demo session. It creates a genuine PayPal Sandbox payout, deliberately skips one internal payout record update, and automatically reconciles the provider's payout item state with the record. An investigation agent retrieves read-only evidence and produces a structured diagnosis and proposed record correction. A visitor acting as the operator reviews and approves the proposal. The Go backend then fetches the specific payout item again, applies deterministic recovery policy, and corrects only the payout record while preserving an immutable event history. No investigation or approval path initiates another payout.

If a new payout item does not reach `SUCCESS` within 25–30 seconds, or the daily creation limit is reached, the visitor can use an isolated session backed by a pre-created genuine Sandbox payout. If the LLM is unavailable, a deterministic diagnosis keeps the recovery workflow available. Both fallbacks and the injected fault are labeled clearly and never presented as live PayPal events or AI output.

## User Stories

1. As a judge, I want to open the hosted demo without registering, so that I can evaluate it immediately.
2. As a judge, I want to start a fresh demo session, so that another visitor's actions do not alter my investigation.
3. As a judge, I want to see the steps of the incident journey, so that I can finish the main workflow in about two minutes.
4. As a judge, I want a real Sandbox payout to be created for my session when available, so that I can see genuine PayPal evidence.
5. As a judge, I want to see the specific payout item identifier and status, so that I can distinguish an item result from a batch result.
6. As a judge, I want the interface to wait only briefly for a live payout item to succeed, so that a slow Sandbox response does not block the demo.
7. As a judge, I want an explicitly labeled pre-created payout fallback, so that I can continue when live creation or completion is unavailable.
8. As a judge, I want the fallback session to have its own payout record and incident, so that my approval cannot change another visitor's state.
9. As a judge, I want to see which internal update was deliberately skipped, so that I understand the cause of the discrepancy.
10. As a judge, I want injected faults to be visibly distinct from PayPal responses, so that I do not mistake a simulation for provider evidence.
11. As a judge, I want reconciliation to run automatically after the injected fault, so that I do not need to discover a hidden control.
12. As a judge, I want to see the stale payout record beside the verified provider item state, so that I can understand the discrepancy.
13. As a judge, I want a concise Overview of payout and incident status, so that I can find the active case quickly.
14. As a judge, I want the Overview to lead directly to Incident Lab, so that I can continue the investigation without searching.
15. As an operator, I want to start an AI investigation of an incident, so that I receive a proposed explanation supported by evidence.
16. As an operator, I want the investigation to use incident, payout record, provider item, event history, and recovery policy evidence, so that the diagnosis is grounded in relevant facts.
17. As an operator, I want the diagnosis to identify its supporting evidence and recommended action, so that I can review its reasoning before approval.
18. As an operator, I want to know when evidence is insufficient or a payout item is not `SUCCESS`, so that I do not approve an unsupported correction.
19. As an operator, I want the interface to identify deterministic diagnosis as a fallback, so that I know when the AI agent was unavailable.
20. As an operator, I want to see the proposed record correction before approving it, so that I know exactly what will change.
21. As an operator, I want approval to affect only my demo session, so that I cannot modify another visitor's record.
22. As an operator, I want the backend to recheck the exact payout item at approval time, so that an earlier AI observation cannot authorize a stale correction.
23. As an operator, I want correction to be rejected when the provider is unavailable or the item is not `SUCCESS`, so that the incident remains safe and open.
24. As an operator, I want correction to be rejected when the provider item identity does not match the payout record, so that the wrong record cannot be updated.
25. As an operator, I want an unsupported AI recommendation to be rejected with a visible reason, so that I understand why approval did not proceed.
26. As an operator, I want a repeated approval to have no additional effect, so that retries cannot create duplicate corrections.
27. As an operator, I want the reconciled payout record and resolved incident to appear after approval, so that I can verify the outcome.
28. As an operator, I want an audit history of provider evidence, investigation, approval, rejection, and correction, so that the decision can be reconstructed.
29. As an operator, I want record changes preserved in an immutable event history, so that later investigation does not erase the earlier state.
30. As a visitor, I want the public demo to limit fresh payouts per session and per day, so that the service remains usable for other judges.
31. As a visitor, I want the demo to explain when a limit triggers the pre-created payout path, so that its behavior is transparent.
32. As a maintainer, I want PayPal credentials and the OpenAI API key kept outside public client code, so that the hosted demo does not expose them.
33. As a maintainer, I want the model ID and daily payout limit configurable, so that I can operate the demo without changing its core logic.
34. As a maintainer, I want an ambiguous payout creation result to retain its original creation key, so that retries cannot silently create a second payout.
35. As a maintainer, I want to rehearse the public workflow against real Sandbox evidence, so that the submitted demo reflects actual integration behavior.
36. As a judge, I want a public repository with a license and runnable instructions, so that I can inspect or reproduce the project.
37. As a judge, I want a public video shorter than three minutes, so that I can evaluate the complete workflow even if I do not run the demo.
38. As a judge, I want the hosted demo to remain accessible during judging, so that I can revisit the application after submission.

## Implementation Decisions

- Build a modular monolith with a Go backend, PostgreSQL, and a React/TypeScript frontend hosted on my-vps with HTTPS.
- Use a payout record as the internal operational state of a payout and preserve changes in an immutable event history. Do not model accounting balances or double-entry postings.
- Keep demo sessions isolated. Each session owns its payout record, incident, investigation, approval, and audit view. The no-registration demo grants operator actions only within the visitor's session.
- Permit at most one newly created Sandbox payout per session and enforce a configurable global daily creation limit. New sessions can use a pre-created genuine payout after the limit is reached.
- Give each live payout creation a stable application key. Ambiguous outcomes are investigated with that key and provider reads; never issue a new payout request under a new key to resolve uncertainty.
- Use the status of the specific PayPal payout item, not only the payout batch. The primary correction is eligible only when the item is independently confirmed as `SUCCESS`.
- Inject the primary fault by deliberately skipping an internal payout record update after a genuine successful Sandbox payout. Automatically reconcile the item state against the stale record and open an incident. Label the injected fault in the UI and evidence history.
- If a new item does not reach `SUCCESS` within 25–30 seconds, offer a clearly labeled pre-created genuine payout path while preserving isolation of the visitor's payout record and incident.
- Expose five read-only investigation capabilities corresponding to incident, internal payout record, PayPal payout, payment events, and recovery policy. The agent cannot call payout creation, retry, approval, or record mutation operations.
- Use OpenAI `gpt-6-luna` through the Responses API with function calling and structured outputs. Keep the model identifier configurable. A deterministic diagnosis is allowed only as a visibly labeled fallback when the LLM is unavailable.
- Make the Go backend the authority for recovery policy. Approval triggers a fresh read of the exact provider item and checks item identity, `SUCCESS`, a still-stale payout record, session ownership, and single-use approval. There is no manual override in P0.
- Apply the record correction and its audit event atomically. Competing or repeated approvals produce at most one correction. Approval and AI recommendations never send another payout.
- Keep genuine provider evidence, injected events, AI conclusions, and deterministic fallback conclusions distinguishable in persisted data and on screen.
- Build a concise Overview with status summary, active incidents, and navigation into Incident Lab. Put detailed evidence, investigation, approval, resolution, and audit history in Incident Lab.
- Prefer AG Studio for the dashboard if a trial license permits the hosted judging period. If that dependency is unavailable by the integration gate, use AG Grid tables in the hosted demo.
- Host the demo through the judging period, publish an open-source repository with an appropriate license and English documentation, and provide a public YouTube video under three minutes.

## Testing Decisions

- Test externally observable behavior: HTTP responses, payout record state, incident state, audit events, and absence of new payout calls. Avoid tests tied to private functions, prompt wording, or storage implementation details.
- Use one primary application-level test seam, confirmed by the project owner: run the real HTTP workflow against PostgreSQL while substituting scripted responses for outbound PayPal and OpenAI calls at the external-service boundary. The repository has no existing application or test seams to reuse.
- Test the genuine PayPal Sandbox separately as an integration smoke test: create a payout with a stable key, retrieve its item state, and retain evidence of the item identifier and eventual status. Do not treat batch `SUCCESS` as item `SUCCESS`.
- Verify the happy path from session creation through injected fault, automatic incident detection, investigation, approval, record correction, and audit history.
- Verify `PENDING`, `FAILED`, `UNCLAIMED`, unknown, mismatched, and unavailable provider item responses all leave the payout record unchanged and the incident open.
- Verify a provider state change between investigation and approval is caught by the fresh approval-time read.
- Verify unsupported or malformed AI output cannot bypass backend recovery policy; verify the deterministic fallback is labeled and still subject to the same policy.
- Verify repeated and concurrent approvals yield exactly one successful correction and no payout creation on the approval path.
- Verify ambiguous payout creation retries retain the same key and do not create a second payout; verify one-new-payout-per-session and daily limit behavior.
- Verify two anonymous sessions cannot inspect or modify each other's payout records, incidents, approvals, or audit views.
- Rehearse both the live and pre-created payout journeys on the hosted site using a clean browser session and measure completion against the two-minute target.
- No prior test pattern exists in the repository; the first implementation stage establishes the application-level harness described above.

## Out of Scope

- Production money movement or production PayPal credentials.
- Automatic payout retries, a human override of recovery policy, or any recovery action that sends money.
- A double-entry accounting ledger, financial balances, or ERP integration.
- Merchant onboarding, multi-tenant SaaS operation, mobile apps, and additional payment providers.
- Multi-agent orchestration and vector search.
- Unknown payment outcome investigation, duplicate webhook handling, unclaimed payout investigation, and agent-driven analytics for P0.
- Live webhook receipt and signature verification for P0; this is the first P1 candidate only after the complete hosted P0 workflow passes its gate.

## Further Notes

- The agreed roadmap allocates about 59 hours and targets submission on 11 November 2026, ahead of the official deadline on 12 November at 21:00 Warsaw time. P1 work does not displace P0 completion, documentation, or the video.
- The available PayPal Sandbox Business account and credentials can already create and read payout items. The early experiment measures how quickly items reach `SUCCESS` and prepares the backup path.
- AG Studio licensing, my-vps deployment, PayPal item timing, and the quality of GPT-6 Luna's structured diagnosis are early integration gates.
- The implementation should follow the project's glossary and recorded decisions concerning operational payout records, item-level verification, injected faults, and labeled fallback diagnosis.
- Relevant primary sources: [hackathon rules](https://paypalaihackathon.devpost.com/rules), [PayPal item states](https://developer.paypal.com/api/payments.payouts-batch/v1/definitions/transaction_enum/), [PayPal idempotency](https://developer.paypal.com/docs/payouts/standard/integrate-api/customize/), [OpenAI Docs: GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna), and [AG Studio licensing](https://www.ag-grid.com/studio/react/installation/).
