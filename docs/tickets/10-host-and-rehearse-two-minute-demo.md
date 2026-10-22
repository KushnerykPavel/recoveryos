# 10: Host and rehearse the two-minute demo

**What to build:** Judges can use the complete RecoveryOS workflow on my-vps over HTTPS without registration. Both live and backup payout journeys work for separate visitors and complete in about two minutes.

**Blocked by:** 04: Keep Incident Lab usable with a backup payout; 06: Label deterministic diagnosis when AI is unavailable; 09: Complete the Incident Lab and concise Overview.

**Status:** ready-for-human

- [x] The hosted frontend, Go backend, and PostgreSQL run over HTTPS with PayPal and OpenAI secrets kept server-side.
- [x] Two clean visitor sessions cannot inspect or change each other's payout records, incidents, approvals, or audit views.
- [ ] The one-new-payout-per-session rule, global daily limit, 25–30-second backup path, and deterministic diagnosis fallback work on the hosted site.
- [ ] Rehearsals of live and backup paths show the full incident, investigation, approval, and resolution journey in about two minutes.
- [x] The demo has a restart and availability check suitable for keeping it accessible through the judging period.

The public demo is at https://valkirie-shield.xyz/recoveryos/. The hosted
frontend, API, and PostgreSQL are managed by Compose behind the existing HTTPS
proxy. Configuration and restart steps are in `deploy/README.md`. The API reads
PayPal credentials from a private server-side environment file. No OpenAI key
was available in that file on 22 October 2026, so hosted investigations used
the visibly labeled deterministic fallback. AI mode still requires adding a
server-side `OPENAI_API_KEY` and rehearsing that mode.

On 22 October, a live HTTPS rehearsal created one genuine Sandbox payout item,
verified item-level `SUCCESS`, opened the injected discrepancy, investigated,
approved, and resolved the incident in 19.2 seconds. A clean browser also
completed the live journey and displayed provider evidence, injected fault,
fallback diagnosis, fresh approval-time verification, correction, and audit
history. A separate HTTPS visitor reached the temporary daily limit of one,
used the pre-created payout, and completed the same workflow in 2.1 seconds.
The normal hosted limit of ten was restored after that check. A two-visitor
HTTPS isolation check blocked cross-session reads and approval and kept audit
events separate. Public `/readyz` returned 204 before and after API restart.

The live provider item reached `SUCCESS` within the wait window, so the
30-second slow-item offer was verified in the client test and deployed build,
while the hosted backup journey was exercised through the daily-limit route.
The backup journey was rehearsed through a fresh HTTPS cookie jar, not a second
browser profile. Those two hosted UI checks remain open. An OpenAI key must be
added to the private server-side environment file before the AI mode can be
rehearsed; the fallback mode is working and visibly labeled.
