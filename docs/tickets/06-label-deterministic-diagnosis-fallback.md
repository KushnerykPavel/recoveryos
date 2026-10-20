# 06: Label deterministic diagnosis when AI is unavailable

**What to build:** If the LLM fails, the operator can still inspect a deterministic diagnosis and continue the recovery workflow. Incident Lab visibly identifies this as a fallback rather than an AI result.

**Blocked by:** 05: Investigate an incident with GPT-6 Luna.

**Status:** ready-for-agent

- [ ] An unavailable or invalid LLM response activates a clearly labeled deterministic diagnosis.
- [ ] The fallback presents the same evidence and proposed-action shape needed for review and policy evaluation.
- [ ] The UI and audit history record which diagnosis mode was used.
- [ ] The fallback cannot grant broader recovery permission than a valid AI diagnosis.
