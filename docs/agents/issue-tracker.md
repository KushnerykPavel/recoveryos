# Issue tracker: Local Markdown

Issues and specs for this repo live under `.scratch/`. Existing `docs/tickets/` files remain project roadmap specs.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`
- Feature spec: `.scratch/<feature-slug>/spec.md`
- Implementation issues: `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, numbered from `01`
- Triage state: a `Status:` line near the top of each issue
- Comments: append under `## Comments`

When a skill says to publish to the issue tracker, create a file under `.scratch/<feature-slug>/`. When it says to fetch a ticket, read the referenced local file.

## Wayfinding

Use `.scratch/<effort>/map.md` for the map and numbered files under `issues/` for child tickets. Record `Type:`, `Status:`, and `Blocked by:` in each child. Claim by setting `Status: claimed`; resolve by adding `## Answer` and setting `Status: resolved`.
