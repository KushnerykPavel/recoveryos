# 12: Process a signed PayPal webhook (P1)

**What to build:** Once the complete hosted P0 demo passes its gate, a genuine signed PayPal webhook becomes visible in the payout event history and investigation evidence. This is the first optional extension and does not replace the injected-fault demo.

**Blocked by:** 10: Host and rehearse the two-minute demo.

**Status:** ready-for-agent

- [ ] A valid signed webhook is accepted and linked to the corresponding payout evidence.
- [ ] An invalid signature or unrecognized payout reference is rejected without changing a payout record.
- [ ] Incident Lab distinguishes a genuine webhook from an injected internal fault.
- [ ] Webhook work begins only if P0, documentation, and submission readiness retain sufficient time before the deadline.
