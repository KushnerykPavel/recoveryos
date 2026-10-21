# RecoveryOS API — first vertical slice

This Go API creates an anonymous session, sends at most one new PayPal Sandbox
payout for that session, and reads the specific payout item's status. Normally,
item-level `SUCCESS` completes the internal payout record. An optional fault
scenario deliberately leaves the record `pending` and opens an incident. The
skipped update is stored as an event with source `injected`; PayPal observations
use source `paypal`.

## Run locally

Requirements: Go 1.26.1, Task, Docker with PostgreSQL, and PayPal Sandbox
credentials that can create Payouts. The receiver must be a Sandbox account.

For the complete local stack, copy the example files if the local files do not
already exist, fill the PayPal Sandbox credentials, and start Compose:

```sh
test -f .env || cp .env.example .env
test -f api/.env || cp api/.env.example api/.env
# Set PostgreSQL credentials and ports in .env.
# Keep api/.env DB_URL credentials in sync, and fill its PayPal values.
task app:up
curl -i http://127.0.0.1:8080/readyz
```

The React client is at `http://127.0.0.1:8080/` by default. Nginx forwards
`/api/` and health endpoints to the Go API. The `migrate` container applies
the schema after PostgreSQL becomes healthy and before the API starts. A clean
browser can start a demo session without signing in and enter Incident Lab.
The session token stays in an HTTP-only cookie; the client receives only that
session's snapshot.

In Incident Lab, **Create Sandbox payout** starts one genuine payout using the
`missed-update` scenario so the same session can continue into incident
detection. The page shows the stable creation key, batch ID, payout item ID,
and item status separately. It keeps checking while the item is pending and
restores the session's payout state after a reload. An uncertain creation keeps
its original key visible and offers no second create action. A daily creation
limit is shown in the page. If the item has not reached item-level `SUCCESS`
after 30 seconds, or the global daily creation limit is reached, the visitor
can switch to a pre-created Sandbox payout. Switching opens a new isolated demo
session with its own payout record, event history, and incident. The original
live session remains separate; switching never creates another PayPal payout.
The page labels the backup as pre-created.

Set `OPENAI_API_KEY` in `api/.env` to enable Incident Lab investigations. The
model defaults to `gpt-6-luna`; `OPENAI_MODEL` overrides it. The key stays on
the API server. After an incident opens, **Start AI investigation** reads five
evidence sources and returns a validated diagnosis and proposed record action.
The latest diagnosis is saved with the visitor's session and appears after a
reload. This step does not approve or apply the proposal.

For client development, run `npm ci` and `npm run dev` from `client/` while the
Go API is listening on `127.0.0.1:8080`. Vite proxies `/api/` to the API.
For a production client build, run `npm run build` in `client/`.

The root `.env` sets `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, and the
host ports `POSTGRES_PORT`, `API_PORT`, `CLIENT_PORT`, and `NGINX_PORT` for
Compose. The API and migration containers read `DB_URL` from `api/.env`; in
Compose its host must be `db` and its credentials must match the root `.env`.
Both local env files are ignored by Git.

For a Go process running directly on the host, set the host in `api/.env`'s
`DB_URL` to `127.0.0.1` and load that file:

```sh
set -a
. api/.env
set +a
task db:up
task api:migrate
task api:run
```

Use `COOKIE_SECURE=true` when serving through HTTPS. The API only targets
`https://api-m.sandbox.paypal.com`; it does not accept a production PayPal
base URL from the environment.

## HTTP flow

```sh
curl -i -c cookies.txt -X POST http://127.0.0.1:8080/api/sessions
curl -i -b cookies.txt -X POST http://127.0.0.1:8080/api/payouts
curl -i -b cookies.txt http://127.0.0.1:8080/api/sessions/current
curl -i -b cookies.txt -X POST http://127.0.0.1:8080/api/payouts/reconcile
curl -i -b cookies.txt -X POST http://127.0.0.1:8080/api/incidents/investigate
```

To run the missed-update scenario, send a header on the **first** payout
request in a fresh session:

```sh
curl -i -b cookies.txt -H 'X-RecoveryOS-Scenario: missed-update' \
  -X POST http://127.0.0.1:8080/api/payouts
```

Without this header the payout record is updated normally after PayPal item
`SUCCESS`. The selected scenario is stored on the payout and returned as
`payout.scenario`. Reconciliation and repeat requests use that stored value;
they cannot switch an existing payout to a different scenario. An explicit
conflicting scenario returns HTTP 409. Unsupported header values return 400.
The scenario changes only the internal record update: the Sandbox payout and
provider read are genuine in both cases.

`POST /api/payouts` checks the payout item for up to 25 seconds. If the item
is still `PENDING`, later `POST /api/payouts/reconcile` requests can check it
again. The session cookie is the only way to access that visitor's record.

The API reserves a stable `sender_batch_id` before making the provider call.
If creation times out or its result is ambiguous, the record becomes
`provider_unknown` and the API does not issue a second create request. The
configurable `DAILY_PAYOUT_LIMIT` also caps fresh payouts across sessions.
Backup sessions do not consume that limit. The default backup IDs come from the
successful Sandbox smoke test below. Set `BACKUP_PAYOUT_CREATION_KEY`,
`BACKUP_PAYOUT_SENDER_ITEM_ID`, `BACKUP_PAYOUT_BATCH_ID`, and
`BACKUP_PAYOUT_ITEM_ID` to use a different pre-created item. The API verifies
the configured batch, item, sender item ID, and `SUCCESS` status with PayPal
before opening a backup session. Use a payout from the Sandbox account whose
credentials the API is running with.

`POST /api/incidents/approve` rechecks the exact PayPal payout item and
records rejected correction attempts in the session event history. A verified
proposal does not yet change the payout record. Atomic record correction
belongs to a later roadmap stage.

## Verification

```sh
cd api
GOCACHE="$PWD/.cache/go-build" go test ./...
GOCACHE="$PWD/.cache/go-build" go vet ./...
```

The default tests exercise the HTTP workflow with an in-memory store and a
scripted provider, plus the PayPal adapter with an in-process HTTP transport.
A real PayPal Sandbox smoke test requires Sandbox credentials and is a separate
manual step; this API is restricted to Sandbox.

Run the repeatable Sandbox check from the repository root after filling
`api/.env`:

```sh
task paypal:smoke
```

The command creates one payout, polls the specific payout item for up to two
minutes, and records its ID, status, and time to `SUCCESS` in
`api/.local/paypal-smoke.json`. This private file is ignored by Git and can
serve as the source of a pre-created Sandbox item for the backup flow.
Running the command again with the same file only resumes observation or
reports the existing result. If creation was interrupted before its batch ID
was saved, the command stops rather than risking another payout. Inspect the
PayPal Sandbox account before starting a separate run with a new evidence file.

The first live result and the decision it supports are in
[`docs/sandbox-smoke-2026-10-21.md`](../docs/sandbox-smoke-2026-10-21.md).

To exercise the SQL migration and PostgreSQL workflow, set `TEST_DB_URL` to a
disposable database whose name ends in `_test`, then run
`go test ./tests/api -run TestPostgresRecoveryWorkflow -v`. This test clears its
tables before running. It creates two anonymous sessions over HTTP, checks that
their identities and state are isolated, and substitutes scripted responses at
the outbound PayPal boundary. It does not call PayPal; the live Sandbox smoke
test is a separate manual step with your credentials.
