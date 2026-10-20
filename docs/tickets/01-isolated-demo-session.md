# 01: Start an isolated demo session

**What to build:** A visitor can open RecoveryOS without registration, start a demo session, and see a session-specific Incident Lab shell. This establishes one visible path through the React interface, Go API, and PostgreSQL before payment behavior is added.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] A clean browser can start a session without credentials and reach Incident Lab.
- [ ] Two browser sessions receive different identities and cannot read each other's session state.
- [ ] The first application-level test exercises the HTTP workflow against PostgreSQL with outbound services replaceable by scripted responses.
- [ ] The app can be started locally using documented commands without putting secrets in client code.
- [ ] my-vps access and the AG Studio trial-license request are checked early enough to inform the hosted dashboard choice.
