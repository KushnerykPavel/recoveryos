# 05: Investigate an incident with GPT-6 Luna

**What to build:** An operator starts an AI investigation in Incident Lab and receives a structured diagnosis, cited evidence, and a proposed record correction. The agent can only read the incident, payout record, PayPal payout, payment events, and recovery policy.

**Blocked by:** 03: Detect an injected payout discrepancy.

**Status:** ready-for-agent

- [ ] The investigation uses OpenAI `gpt-6-luna` through the Responses API with a configurable model ID.
- [ ] The result identifies the incident, evidence sources, item status, diagnosis, and proposed action in a validated structure.
- [ ] Investigation tools cannot create or retry payouts, approve actions, or mutate payout records.
- [ ] Evidence shown in the UI distinguishes provider facts, internal records, and injected events.
- [ ] Scripted external responses verify the visible diagnosis without asserting private prompt wording.
