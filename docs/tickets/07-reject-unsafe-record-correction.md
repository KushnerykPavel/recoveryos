# 07: Reject unsafe record correction

**What to build:** When an operator attempts to approve a proposed record correction, the backend re-reads the exact PayPal payout item and rejects the action with a visible reason if evidence or policy does not support it. The incident stays open and the payout record stays unchanged.

**Blocked by:** 05: Investigate an incident with GPT-6 Luna.

**Status:** complete

- [x] Approval performs a fresh item-level PayPal read rather than trusting the investigation snapshot or batch status.
- [x] `PENDING`, `FAILED`, `UNCLAIMED`, unknown, mismatched, and unavailable item results all reject correction.
- [x] An unsupported or malformed recommendation, wrong session, or no-longer-stale payout record also rejects correction.
- [x] The operator sees the rejection reason; the event history records the attempt without altering the payout record.
- [x] No approval path invokes payout creation or retry.

Valid proposals receive a not-yet-available response until ticket 08 adds the atomic record correction.
