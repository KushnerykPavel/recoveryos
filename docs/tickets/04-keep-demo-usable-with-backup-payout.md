# 04: Keep Incident Lab usable with a backup payout

**What to build:** A visitor can continue the same incident journey using a pre-created genuine Sandbox payout when a new payout item is slow or the public daily creation limit is reached. The visitor's payout record and incident remain isolated.

**Blocked by:** 03: Detect an injected payout discrepancy.

**Status:** ready-for-agent

- [ ] A live item that has not reached `SUCCESS` within 25–30 seconds offers the clearly labeled backup path.
- [ ] A configurable global daily creation limit prevents new payouts and offers the backup path without blocking the visitor.
- [ ] Each backup session has its own payout record and incident even when provider evidence is reused.
- [ ] Switching to backup never initiates another payout or presents a pre-created item as newly created.
