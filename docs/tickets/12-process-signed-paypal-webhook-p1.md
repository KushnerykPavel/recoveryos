# 12: Process a signed PayPal webhook (P1)

**What to build:** Once the complete hosted P0 demo passes its gate, a genuine signed PayPal webhook becomes visible in the payout event history and investigation evidence. This is the first optional extension and does not replace the injected-fault demo.

**Blocked by:** 10: Host and rehearse the two-minute demo.

**Status:** ready-for-agent

- [ ] A valid signed webhook is accepted and linked to the corresponding payout evidence.
- [ ] An invalid signature or unrecognized payout reference is rejected without changing a payout record.
- [ ] Incident Lab distinguishes a genuine webhook from an injected internal fault.
- [ ] Webhook work begins only if P0, documentation, and submission readiness retain sufficient time before the deadline.

Backend implementation is prepared: the optional endpoint verifies signed
item-success events through PayPal's Sandbox verification API, matches only
live payout records, and stores deduplicated immutable evidence without
changing payout or incident state. Investigation may cite that event in
addition to the current PayPal item read. The UI distinction, a configured
Sandbox Webhook ID, deployment, and genuine hosted delivery check remain open. The
PostgreSQL integration test needs a disposable `TEST_DB_URL` to run.
