# 03: Detect an injected payout discrepancy

**What to build:** After a genuine payout item reaches `SUCCESS`, a visitor sees RecoveryOS deliberately skip the internal payout record update. Reconciliation runs automatically, finds the discrepancy, and opens an incident showing provider evidence beside the stale payout record.

**Blocked by:** 02: Create and observe a genuine Sandbox payout.

**Status:** ready-for-agent

- [ ] The payout record and immutable event history distinguish the real provider result from the injected fault.
- [ ] Reconciliation opens an incident automatically when the item is `SUCCESS` and the payout record is stale.
- [ ] The UI labels the skipped update as injected and does not describe it as a missing PayPal webhook.
- [ ] A non-`SUCCESS` item does not produce the completed-payout discrepancy.
