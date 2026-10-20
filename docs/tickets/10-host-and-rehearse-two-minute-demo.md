# 10: Host and rehearse the two-minute demo

**What to build:** Judges can use the complete RecoveryOS workflow on my-vps over HTTPS without registration. Both live and backup payout journeys work for separate visitors and complete in about two minutes.

**Blocked by:** 04: Keep Incident Lab usable with a backup payout; 06: Label deterministic diagnosis when AI is unavailable; 09: Complete the Incident Lab and concise Overview.

**Status:** ready-for-agent

- [ ] The hosted frontend, Go backend, and PostgreSQL run over HTTPS with PayPal and OpenAI secrets kept server-side.
- [ ] Two clean browser sessions cannot inspect or change each other's payout records, incidents, approvals, or audit views.
- [ ] The one-new-payout-per-session rule, global daily limit, 25–30-second backup path, and deterministic diagnosis fallback work on the hosted site.
- [ ] Rehearsals of live and backup paths show the full incident, investigation, approval, and resolution journey in about two minutes.
- [ ] The demo has a restart and availability check suitable for keeping it accessible through the judging period.
