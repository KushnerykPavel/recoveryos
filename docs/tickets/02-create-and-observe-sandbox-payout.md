# 02: Create and observe a genuine Sandbox payout

**What to build:** A visitor starts one real PayPal Sandbox payout in their session and sees the exact payout item identifier and status in Incident Lab. This slice establishes genuine provider evidence before fault injection.

**Blocked by:** 01: Start an isolated demo session.

**Status:** ready-for-agent

- [ ] A session creates at most one new payout using a stable creation key.
- [ ] The UI shows the payout item status and identifier rather than inferring item success from batch success.
- [ ] An ambiguous creation result keeps the same key for investigation and never creates a second payout under a new key.
- [ ] A real Sandbox smoke test creates and reads a payout item; scripted integration tests cover success and non-success responses.
- [ ] The Sandbox smoke test records how long item-level `SUCCESS` takes, to validate the 25–30-second live-demo threshold.
