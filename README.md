# RecoveryOS

RecoveryOS is an interactive incident lab for a payout that succeeded at PayPal
but still appears pending in an internal operational record. A visitor can
follow the discrepancy from genuine PayPal Sandbox evidence through an AI
investigation and an operator-approved record correction.

**[Open the hosted demo](https://recoveryos.valkirie-shield.xyz/)** ·
[API details](api/README.md) · [Deployment guide](deploy/README.md) ·
[Demo and submission notes](docs/submission.md)

## Try the demo

No account is needed. Start a private session, open Incident Lab, and create a
Sandbox payout. RecoveryOS checks the status of the **specific payout item**.
When PayPal reports `SUCCESS`, the demo deliberately skips the corresponding
internal record update. This injected fault is labeled in the interface and
event history; it is not a missing PayPal webhook or a PayPal failure.
Reconciliation opens an incident automatically.

Run the investigation to see cited evidence from the incident, operational
payout record, PayPal item, event history, and recovery policy. The AI proposes
a correction but cannot perform it. If the AI service is unavailable, the page
labels a deterministic diagnosis as a fallback. Approve the proposal to make
the Go backend read the PayPal item again and apply its own policy. Only then
does it correct the internal record and append an immutable audit event.
Approval never sends another payout.

If a new item takes more than 30 seconds to reach `SUCCESS`, or the daily limit
for fresh payouts has been reached, the page offers a **pre-created Sandbox
payout**. Its PayPal evidence is genuine, while each visitor still receives a
separate internal record, incident, and audit history. The interface labels
this route as a backup.

## Run locally

You need Docker with Compose, a PayPal Developer Sandbox app with Payouts
access, and a Sandbox receiver account. The full stack runs in containers;
Go, Node, and Task are needed only for development commands.

1. Copy `.env.example` to `.env` and `api/.env.example` to `api/.env`.
2. Set `POSTGRES_PASSWORD` in `.env`. Set the same password in the `DB_URL` in
   `api/.env`; its hostname must remain `db` for Compose.
3. Set `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET`, and
   `PAYPAL_SANDBOX_RECEIVER` in `api/.env`. Set `OPENAI_API_KEY` there to use AI
   investigations; without it, the deterministic fallback remains available.
4. From the repository root, run `docker compose up --build -d --wait` and open
   `http://127.0.0.1:8080/`. Confirm readiness with
   `curl -i http://127.0.0.1:8080/readyz` (HTTP 204).

The first run applies the PostgreSQL migrations. The local `.env` files are
ignored by Git; do not put credentials in source, browser code, or a commit.
`DAILY_PAYOUT_LIMIT` defaults to 10. The example backup identifiers refer to
an item in the project's Sandbox account. To reproduce the backup route with
**your own** credentials, install Go and Task, run `task paypal:smoke` once,
and wait for item-level `SUCCESS`. This creates one genuine Sandbox payout and
saves its identifiers in the ignored `api/.local/paypal-smoke.json` file. Copy
its `creation_key`, `sender_item_id`, `batch_id`, and `item_id` into the four
matching `BACKUP_PAYOUT_*` settings in `api/.env`, then restart Compose. The
API verifies the item again before offering it as a backup. A repeat smoke
run with the same evidence file observes that item rather than creating one
more payout.

To stop the stack, run `docker compose down`. This preserves the named
PostgreSQL volume; `docker compose down --volumes` deletes its data.

## How it is built

- **PayPal Sandbox Payouts API:** creates at most one new payout per session
  with a stable creation key and reads the item status. An ambiguous creation
  result cannot cause a second create request under a new key.
- **OpenAI Responses API:** `gpt-6-luna` (configurable with `OPENAI_MODEL`)
  investigates through five read-only evidence tools and returns a validated
  diagnosis. It has no payout creation, approval, or record mutation tool.
- **Go and PostgreSQL:** enforce session isolation, the daily creation limit,
  fresh item verification at approval, single-use correction, and immutable
  audit history. Record correction and its audit event commit together.
- **React, TypeScript, and AG Grid:** present Overview and Incident Lab. The
  display distinguishes PayPal observations, injected events, AI conclusions,
  and deterministic fallback conclusions.

This is a Sandbox demonstration. The internal payout record is an operational
status, not an accounting ledger; the correction does not move money. See the
[specification](SPEC.md) and [architecture decisions](docs/adr/) for the
policy and scope.

## Verify

Run `npm ci`, `npm test`, and `npm run build` in `client/`. Run
`GOCACHE="$PWD/.cache/go-build" go test ./...` and
`GOCACHE="$PWD/.cache/go-build" go vet ./...` in `api/`.
The PostgreSQL integration test additionally needs a disposable `TEST_DB_URL`
whose database name ends in `_test`; it clears that database's tables. A live
PayPal check is available with `task paypal:smoke` after configuring
`api/.env`. That command creates a real Sandbox payout, so run it deliberately.

## License

MIT. See [LICENSE](LICENSE).
