# 12: Process a signed PayPal webhook (P1)

**What to build:** Once the complete hosted P0 demo passes its gate, a genuine signed PayPal webhook becomes visible in the payout event history and investigation evidence. This is the first optional extension and does not replace the injected-fault demo.

**Blocked by:** 10: Host and rehearse the two-minute demo.

**Status:** ready-for-agent

- [ ] A valid signed webhook is accepted and linked to the corresponding payout evidence.
- [ ] An invalid signature or unrecognized payout reference is rejected without changing a payout record.
- [ ] Incident Lab distinguishes a genuine webhook from an injected internal fault.
- [ ] Webhook work begins only if P0, documentation, and submission readiness retain sufficient time before the deadline.

Backend implementation is deployed: the optional endpoint verifies signed
item-success events through PayPal's Sandbox verification API, matches only
live payout records, and stores deduplicated immutable evidence without
changing payout or incident state. Investigation may cite that event in
addition to the current PayPal item read. The React UI distinction remains open.

On 23 October 2026, Sandbox webhook `9PS687238N7404452` was registered on the
same REST app as the payout credentials and deployed. One genuine live payout
produced item `NZ8CX9HMLQQGG` and PayPal event
`WH-6AK804027J654324Y-7FL78833MT976042V`. PayPal's event API confirmed the
event type and item ID. The database contained one `paypal_webhook` event for
the live payout, with the payout record still `pending` and its incident still
`open`. PayPal accepted a resend of that same event, after which the stored
event count remained one. The PostgreSQL integration test still needs a
disposable `TEST_DB_URL` to run locally.
