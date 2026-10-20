# RecoveryOS API — first vertical slice

This Go API creates an anonymous demo session, sends at most one new PayPal
Sandbox payout for that session, reads the specific payout item's status, and
opens an incident when the item reaches `SUCCESS` while the internal payout
record is deliberately left stale. The skipped update is stored as an event
with source `injected`; PayPal observations use source `paypal`.

## Run locally

Requirements: Go 1.26.1, Task, Docker with PostgreSQL, and PayPal Sandbox
credentials that can create Payouts. The receiver must be a Sandbox account.

From the repository root:

```sh
cp api/.env.example api/.env
# Fill PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET and PAYPAL_SANDBOX_RECEIVER.
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
curl -i -c cookies.txt -X POST http://127.0.0.1:8080/api/demo/sessions
curl -i -b cookies.txt -X POST http://127.0.0.1:8080/api/demo/start
curl -i -b cookies.txt http://127.0.0.1:8080/api/demo
curl -i -b cookies.txt -X POST http://127.0.0.1:8080/api/demo/reconcile
```

`POST /api/demo/start` checks the payout item for up to 25 seconds. If the item
is still `PENDING`, later `POST /api/demo/reconcile` requests can check it
again. The session cookie is the only way to access that visitor's record.

The API reserves a stable `sender_batch_id` before making the provider call.
If creation times out or its result is ambiguous, the record becomes
`provider_unknown` and the API does not issue a second create request. The
configurable `DAILY_PAYOUT_LIMIT` also caps fresh payouts across sessions.

This slice does not yet include the pre-created payout fallback, AI
investigation, operator approval, or record correction. Those belong to the
next roadmap stages.

## Verification

```sh
cd api
GOCACHE="$PWD/.cache/go-build" go test ./...
GOCACHE="$PWD/.cache/go-build" go vet ./...
```

The default tests exercise the HTTP workflow with an in-memory store and a
scripted provider, plus the PayPal adapter with an in-process HTTP transport.
A real PayPal Sandbox smoke test requires Sandbox credentials and is a separate
manual step; do not point the public demo at a production account.

To exercise the SQL migration and PostgreSQL workflow, set `TEST_DB_URL` to a
disposable database whose name ends in `_test`, then run
`go test ./tests/api -run TestPostgresDemoWorkflow -v`. This test clears its
tables before running. It does not call PayPal; the live Sandbox smoke test is
a separate manual step with your credentials.
