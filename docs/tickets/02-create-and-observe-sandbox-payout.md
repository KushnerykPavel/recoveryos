# 02: Create and observe a genuine Sandbox payout

**What to build:** A visitor starts one real PayPal Sandbox payout in their session and sees the exact payout item identifier and status in Incident Lab. This slice establishes genuine provider evidence before fault injection.

**Blocked by:** 01: Start an isolated demo session.

**Status:** done.

- [x] A session creates at most one new payout using a stable creation key.
- [x] The UI shows the payout item status and identifier rather than inferring item success from batch success.
- [x] An ambiguous creation result keeps the same key for investigation and never creates a second payout under a new key.
- [x] A real Sandbox smoke test creates and reads a payout item; scripted integration tests cover success and non-success responses.
- [x] The Sandbox smoke test records how long item-level `SUCCESS` takes, to validate the 25–30-second live-demo threshold.

Completed 21 October 2026: item `KDWNH6JFYC2W2` reached `SUCCESS` in 18.355 seconds. See [the smoke test report](../sandbox-smoke-2026-10-21.md).

Incident Lab now starts the genuine payout with the `missed-update` scenario, displays the stable creation key and separate batch and item evidence, and continues checking a pending item. The injected incident that appears at item-level `SUCCESS` is labeled; its detailed discrepancy view belongs to ticket 03. An uncertain creation remains visible with its original key and no new-create action. Scripted HTTP tests cover pending, successful, non-success, mismatched, and uncertain outcomes without creating another live Sandbox payout.
