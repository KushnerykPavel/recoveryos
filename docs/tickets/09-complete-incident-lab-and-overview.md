# 09: Complete the Incident Lab and concise Overview

**What to build:** A judge can move from a concise Overview into a complete Incident Lab view of provider evidence, injected fault, investigation, approval, reconciliation result, and audit history. Use AG Studio when its hosted license is available, otherwise AG Grid tables.

**Blocked by:** 08: Correct the payout record once after approval.

**Status:** complete

- [x] Overview shows a concise payout and incident summary and a direct route into Incident Lab.
- [x] Incident Lab presents the complete journey without requiring a manual reconciliation step.
- [x] Genuine provider responses, injected faults, AI conclusions, and fallback conclusions are visually distinct.
- [x] The hosted-license decision for AG Studio is made by the integration gate; the AG Grid fallback preserves the same workflow.
- [x] A clean browser can follow the end-to-end journey without needing implementation knowledge.

AG Studio has no hosted trial license in this project as of 22 October 2026. The
dashboard uses AG Grid Community for the audit history, with the same evidence,
investigation, approval, and resolution flow. The Overview gives the next action
and direct case navigation; Incident Lab shows all five steps and automatically
checks the payout item. A clean browser completed the full UI path against a
local scripted API, including AG Grid evidence rows, investigation, approval,
resolved state, and audit history. Hosted live and backup rehearsals are tracked
in ticket 10.
