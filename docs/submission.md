# Demo and submission notes

## Suggested video, under three minutes

Record the actual hosted browser workflow at
`https://recoveryos.valkirie-shield.xyz/` in a clean browser session. Keep the
cursor and UI text visible; capture the real PayPal item ID and status. Use
the following outline as narration, adapting the timing to the live result.

| Time | Show | Say |
| --- | --- | --- |
| 0:00–0:20 | Overview and Start demo session | “RecoveryOS helps operators investigate a payout that succeeded at PayPal but remains pending internally.” |
| 0:20–0:50 | Incident Lab, Create Sandbox payout, item ID and `SUCCESS` | “This is a real PayPal Sandbox payout. We check the item itself, not just the batch.” |
| 0:50–1:15 | Pending internal record, labeled injected fault, incident | “For the demo, we deliberately skip one internal update. Reconciliation detects the mismatch automatically.” |
| 1:15–1:50 | Investigation and cited evidence | “The AI reads the incident, payout record, PayPal item, event history, and policy, then proposes a record correction. It cannot send or retry a payout.” |
| 1:50–2:20 | Approve and show fresh provider verification | “Approval triggers a new PayPal item check and backend policy evaluation.” |
| 2:20–2:45 | Resolved incident, corrected record, audit history | “Only the internal record changes. The event history preserves the evidence and decision.” |

If the daily limit or a slow payout leads to the pre-created item, show its
**backup** label and say that its provider evidence is genuine but was created
earlier. If the investigation uses the deterministic fallback, show that label
and describe it accurately instead of calling it an AI result. Avoid visible
credentials, private browser tabs, copyrighted music, and claims that the
injected event came from PayPal. Check the final file's duration before upload.

## Submission description draft

RecoveryOS turns a payout discrepancy into a safe, inspectable recovery
workflow. It creates and reads a genuine PayPal Sandbox payout item, then
deliberately skips an internal operational record update. Automatic
reconciliation opens an incident when the item is `SUCCESS` but the record is
still pending. An AI investigation using OpenAI `gpt-6-luna` gathers five
read-only evidence sources and proposes a correction with citations. A visitor
reviews and approves the proposal. The Go backend independently rechecks the
exact PayPal item, enforces recovery policy, and atomically corrects only the
internal record with an immutable audit event. It never retries the payout.
The hosted, account-free Incident Lab distinguishes genuine provider evidence,
the injected fault, AI conclusions, and the labeled deterministic and
pre-created-payout fallback paths.

## Final publication checks

The [hackathon submission requirements](https://paypalaihackathon.devpost.com/)
call for a project description, a functional demo or complete setup
instructions, identification of the AI tools used, a **public open-source
repository with a visible top-level license**, and a **public YouTube video
under three minutes** showing the working project. Before the planned
11 November 2026 submission:

- [ ] Push the committed README and license; open the public repository in a
  signed-out browser and verify the license appears in GitHub's About section.
- [ ] Open the hosted demo in a clean browser, finish the journey, and check
  `https://recoveryos.valkirie-shield.xyz/readyz` returns HTTP 204.
- [ ] Record the working browser demo, confirm duration below 3:00, publish it
  publicly on YouTube, and test its link while signed out.
- [ ] Add the public repository, hosted demo, and YouTube URLs to the Devpost
  submission. Use the description above and name PayPal Sandbox Payouts API,
  OpenAI Responses API / `gpt-6-luna`, and AG Grid in the tools section.
- [ ] Preview the submitted page while signed out and verify every link. Keep
  the service and `/readyz` available through 15 December 2026.

The target submission is 11 November 2026, ahead of the official
12 November 2026 deadline (21:00 Warsaw time).
