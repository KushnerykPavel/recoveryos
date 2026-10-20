# 07: Reject unsafe record correction

**What to build:** When an operator attempts to approve a proposed record correction, the backend re-reads the exact PayPal payout item and rejects the action with a visible reason if evidence or policy does not support it. The incident stays open and the payout record stays unchanged.

**Blocked by:** 05: Investigate an incident with GPT-6 Luna.

**Status:** ready-for-agent

- [ ] Approval performs a fresh item-level PayPal read rather than trusting the investigation snapshot or batch status.
- [ ] `PENDING`, `FAILED`, `UNCLAIMED`, unknown, mismatched, and unavailable item results all reject correction.
- [ ] An unsupported or malformed recommendation, wrong session, or no-longer-stale payout record also rejects correction.
- [ ] The operator sees the rejection reason; the event history records the attempt without altering the payout record.
- [ ] No approval path invokes payout creation or retry.
