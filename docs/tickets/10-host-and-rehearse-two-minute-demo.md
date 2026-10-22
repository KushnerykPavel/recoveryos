# 10: Host and rehearse the two-minute demo

**What to build:** Judges can use the complete RecoveryOS workflow on my-vps over HTTPS without registration. Both live and backup payout journeys work for separate visitors and complete in about two minutes.

**Blocked by:** 04: Keep Incident Lab usable with a backup payout; 06: Label deterministic diagnosis when AI is unavailable; 09: Complete the Incident Lab and concise Overview.

**Status:** ready-for-human

- [x] The hosted frontend, Go backend, and PostgreSQL run over HTTPS with PayPal and OpenAI secrets kept server-side.
- [x] Two clean visitor sessions cannot inspect or change each other's payout records, incidents, approvals, or audit views.
- [x] The one-new-payout-per-session rule and global daily limit work on the hosted site.
- [ ] The 30-second backup offer is observed on the hosted site after a slow live item.
- [x] The deterministic diagnosis fallback works on the hosted site.
- [x] Rehearsals of live and backup paths show the full incident, investigation, approval, and resolution journey in about two minutes.
- [x] The demo has a restart and availability check suitable for keeping it accessible through the judging period.

The public demo is at https://recoveryos.valkirie-shield.xyz/. The hosted
frontend, API, and PostgreSQL are managed by Compose behind the existing HTTPS
proxy. Configuration and restart steps are in `deploy/README.md`. The API reads
PayPal credentials from a private server-side environment file. The OpenAI key
was absent during the first 22 October rehearsal, which verified the visibly
labeled deterministic fallback; it was configured later that day and AI mode
was rehearsed successfully.

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
browser profile. Those two hosted UI checks remained open at that point. The
OpenAI key was absent then, so only the visibly labeled fallback was rehearsed.

On 22 October, the demo moved to the dedicated subdomain. A separate valid
certificate covers `recoveryos.valkirie-shield.xyz`; the former apex path
redirects there. The root page, asset, and `/readyz` checks passed, a clean
browser started a session at the new address, the backup workflow resolved in
3.0 seconds, and the two-visitor isolation check passed again.

Later on 22 October, a new clean-browser live rehearsal reached item-level
`SUCCESS` for PayPal Sandbox item `KJ2732TMLU53Y`, opened the injected incident,
and produced an `ai` diagnosis with cited internal, PayPal, injected-event, and
policy evidence. The hosted API log confirmed a completed OpenAI response and
saved `origin=ai`; `/readyz` returned 204, and the API and database containers
were healthy. Reloading the browser preserved the session and diagnosis without
offering another payout creation action. After explicit user approval, the
browser approval performed a fresh item-level `SUCCESS` check, corrected the
record, resolved the incident, and showed distinct provider-verification,
operator-approval, and record-correction audit events. A safety-review pause
interrupted this rehearsal, so its elapsed end-to-end time was not measured.
Local Go and client tests and the production client build passed; PostgreSQL
integration tests were skipped because no disposable `TEST_DB_URL` was
configured locally.

A further 22 October hosted check ran the full live journey in 25.3 seconds
with an `ai` diagnosis and the full backup journey in 6.9 seconds with an `ai`
diagnosis. A two-visitor check blocked cross-session reads and approval and
kept the audit trails separate. A fresh session sent the payout-create request
twice: both responses had the same payout ID and creation key, and the global
daily count increased from five to six. The public root returned 200 with valid
TLS and `/readyz` returned 204. Local and VPS hashes of the client source and
rehearsal scripts matched; local Go and client tests passed. The 30-second
slow-item offer still needs a hosted observation because actual PayPal Sandbox
items reached `SUCCESS` before that threshold. The client test verifies the
button appears at 30 seconds and not at 29.999 seconds.

Another 22 October check reached `/readyz` over verified HTTPS (204). A fresh
backup journey resolved in 9.7 seconds with an `ai` diagnosis; a separate
two-visitor check again blocked cross-session reads and approval and kept the
audit trails isolated. One more genuine live Sandbox payout reached its
incident after 17.9 seconds and completed the full AI investigation, approval,
and resolution journey in 25.7 seconds. That item also reached `SUCCESS`
before the 30-second threshold, so the hosted slow-item offer remains
unobserved. The local Python installation needed `SSL_CERT_FILE` set to the
system CA bundle for these HTTPS scripts; certificate verification remained
enabled. The client tests, production build, and Go tests passed; the Go test
command's final cache-trim step could not write to the default cache directory
inside this workspace sandbox. Re-running Go tests and vet with a workspace-local
cache passed.
