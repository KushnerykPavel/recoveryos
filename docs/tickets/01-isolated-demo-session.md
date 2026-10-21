# 01: Start an isolated demo session

**What to build:** A visitor can open RecoveryOS without registration, start a demo session, and see a session-specific Incident Lab shell. This establishes one visible path through the React interface, Go API, and PostgreSQL before payment behavior is added.

**Blocked by:** None (can start immediately).

**Status:** done.

- [x] A clean browser can start a session without credentials and reach Incident Lab.
- [x] Two browser sessions receive different identities and cannot read each other's session state.
- [x] The first application-level test exercises the HTTP workflow against PostgreSQL with outbound services replaceable by scripted responses.
- [x] The app can be started locally using documented commands without putting secrets in client code.
- [x] my-vps access and the AG Studio trial-license request are checked early enough to inform the hosted dashboard choice.

my-vps read-only SSH access was confirmed on 21 October 2026. Docker is present,
and HTTPS listeners are active. The [AG Studio trial form](https://www.ag-grid.com/studio/license-pricing/?tab=trial)
was checked; it lists a 45-day trial. The owner chose to start with the React
shell and defer AG Studio and its trial request until the later dashboard
integration stage to preserve licence coverage near the deadline.
