# Hosted demo on my-vps

The public URL is `https://recoveryos.valkirie-shield.xyz/`. The existing
shared HTTPS proxy handles TLS and forwards this hostname to the RecoveryOS
Compose stack. PostgreSQL, the Go API, and the static client remain inside
Docker. PayPal and OpenAI credentials are read only by the Go API and migration
containers from `api/.env`; never place them in client build arguments.

## Deploy

1. Copy the repository to `/home/student/recoveryos` on `my-vps`, excluding
   local `.env` files, build output, and `.git`. Copy the root `.env` and
   `api/.env` separately with mode `0600`. Match the database credentials in
   both files, set `DB_URL` host to `db`, and set `COOKIE_SECURE=true` in
   `api/.env`. Set `OPENAI_API_KEY` there to enable AI investigation; when it
   is absent the diagnosis is visibly labeled as a deterministic fallback.
2. From that directory run
   `sudo docker compose -f docker-compose.yaml -f deploy/compose.vps.yaml up --build -d --wait`.
   Compose applies migrations before starting the API. It also gives the
   internal Nginx service the `recoveryos-nginx` name on the existing `proxy`
   Docker network.
3. Point the subdomain's A record at the VPS and issue a dedicated Let's
   Encrypt certificate with the shared proxy's certbot service. Copy
   `deploy/recoveryos.vhost.conf` to
   `/opt/vps-nginx/nginx/conf.d/recoveryos.valkirie-shield.xyz.conf`.
   Keep the legacy redirect locations from `deploy/recoveryos.location.conf`
   inside the apex `valkirie-shield.xyz` HTTPS server. Check with
   `sudo docker exec nginx-proxy nginx -t`, then reload with
   `sudo docker exec nginx-proxy nginx -s reload`.
4. Check `https://recoveryos.valkirie-shield.xyz/` and
   `https://recoveryos.valkirie-shield.xyz/readyz`. A ready API returns 204.
   The former `https://valkirie-shield.xyz/recoveryos/` route redirects to the
   dedicated hostname.

The application services use `restart: unless-stopped`; the database has a
persistent named volume. To restart after maintenance, run the same Compose
command without `--build` and check `/readyz`. To update code, copy the new
files and repeat step 2. Inspect `sudo docker compose -f docker-compose.yaml
-f deploy/compose.vps.yaml ps` and API logs if readiness fails. Existing
`/healthz` reports process availability; `/readyz` also checks PostgreSQL.

## Rehearse

Use a fresh browser profile for each visitor. The primary path is: open
Overview, start a demo session, enter Incident Lab, create a Sandbox payout,
wait for the item to reach `SUCCESS`, investigate, approve, and check the
resolved incident and audit history. For the backup path, open a second fresh
profile, start a session, and use the pre-created payout when the daily limit
or 30-second wait offers it. Record the elapsed time for each path. A backup
profile must never show the first profile's payout record or audit events.

The public daily payout creation cap is configured by `DAILY_PAYOUT_LIMIT`.
Keep it low enough to limit Sandbox use but high enough to permit planned live
rehearsals. Each live rehearsal creates one genuine Sandbox payout; repeat
visits to the same session do not create another payout. The backup journey
reuses verified provider evidence but creates a separate payout record and
incident for each visitor.

The repeatable public checks are `python3 deploy/rehearse.py live`,
`python3 deploy/rehearse.py backup`, and
`python3 deploy/check-isolation.py`. Use `--expect-daily-limit` on the backup
check only when the cap has already been reached. Each script starts with fresh
HTTP cookies. The live check creates one genuine Sandbox payout, so run it
deliberately; the backup and isolation checks only read pre-created provider
evidence. The 22 October 2026 run and its limits are recorded in ticket 10.

## Diagnose AI investigations

Follow the API logs while clicking **Run investigation** or **Run investigation
again** in Incident Lab:

```sh
sudo docker compose -f docker-compose.yaml -f deploy/compose.vps.yaml logs -f --tail=100 api
```

Each investigation logs its incident ID. `AI model call started` marks an
attempt; `OpenAI response received` reports the response status and output-item
count for that round. `investigation saved` with `origin=ai` confirms a valid
AI diagnosis was stored. If the model call fails or the response cannot be
validated, `AI investigation failed; using deterministic fallback` includes
the error, followed by `investigation saved` with
`origin=deterministic_fallback`. The logs omit API keys, prompts, provider
evidence, and response text.
