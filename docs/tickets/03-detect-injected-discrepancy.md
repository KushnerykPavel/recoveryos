# 03: Detect an injected payout discrepancy

**What to build:** After a genuine payout item reaches `SUCCESS`, a visitor sees RecoveryOS deliberately skip the internal payout record update. Reconciliation runs automatically, finds the discrepancy, and opens an incident showing provider evidence beside the stale payout record.

**Blocked by:** 02: Create and observe a genuine Sandbox payout.

**Status:** done.

- [x] The payout record and immutable event history distinguish the real provider result from the injected fault.
- [x] Reconciliation opens an incident automatically when the item is `SUCCESS` and the payout record is stale.
- [x] The UI labels the skipped update as injected and does not describe it as a missing PayPal webhook.
- [x] A non-`SUCCESS` item does not produce the completed-payout discrepancy.

Completed 21 October 2026: PayPal item `SUCCESS` is recorded separately from the internal record, which remains `pending` in the injected scenario. Reconciliation opens the incident in the same transaction that saves the provider observation and fault event. Incident Lab displays both states side by side and labels each event by source. Scripted HTTP and PostgreSQL integration tests cover successful, pending, and non-success items.
