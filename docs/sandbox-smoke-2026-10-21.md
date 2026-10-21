# PayPal Sandbox payout smoke test — 21 October 2026

**Status: Done.**

The `task paypal:smoke` run created Sandbox batch `J2B2LJ6B7ECT8` and read
item `KDWNH6JFYC2W2` at item-level `SUCCESS`. The measured time from the
saved creation attempt to the successful item read was **18.355 seconds**,
inside the planned 25–30-second live-demo window. The private machine-readable
evidence is in `api/.local/paypal-smoke.json`.

**Demo-path decision:** Keep a newly created Sandbox payout as the primary
path. Retain this successful item as the pre-created backup candidate for
sessions where a new item is slow or the daily creation cap is reached. A
single timing sample does not establish reliability; rehearse the hosted path
before submission.

The smoke command checks item identity and `SUCCESS`; batch-level state alone
does not count. Re-running it with the same evidence file did not create a
second payout.
