# 05: Investigate an incident with GPT-6 Luna

**What to build:** An operator starts an AI investigation in Incident Lab and receives a structured diagnosis, cited evidence, and a proposed record correction. The agent can only read the incident, payout record, PayPal payout, payment events, and recovery policy.

**Blocked by:** 03: Detect an injected payout discrepancy.

**Status:** complete

- [x] The investigation uses OpenAI `gpt-6-luna` through the Responses API with a configurable model ID.
- [x] The result identifies the incident, evidence sources, item status, diagnosis, and proposed action in a validated structure.
- [x] Investigation tools cannot create or retry payouts, approve actions, or mutate payout records.
- [x] Evidence shown in the UI distinguishes provider facts, internal records, and injected events.
- [x] Scripted external responses verify the visible diagnosis without asserting private prompt wording.

The read-only investigation result is saved for the visitor's session. A proposed correction remains a proposal; approval and record mutation are separate tickets.
