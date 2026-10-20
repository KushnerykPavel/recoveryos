# 08: Correct the payout record once after approval

**What to build:** For a supported proposal and freshly verified item-level `SUCCESS`, an operator approves one record correction. Incident Lab then shows the reconciled payout record, resolved incident, and audit trail; repeat or concurrent approvals have no additional effect.

**Blocked by:** 07: Reject unsafe record correction.

**Status:** ready-for-agent

- [ ] The backend checks item identity, session ownership, current record state, and single-use approval before committing.
- [ ] The payout record correction and its audit event commit atomically; the incident appears resolved afterward.
- [ ] Repeated and concurrent approvals produce at most one successful correction and one correction audit event.
- [ ] The audit view distinguishes provider verification, operator approval, and record correction.
- [ ] Approval never creates or retries a payout, including when the request is repeated.
