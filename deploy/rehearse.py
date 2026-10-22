#!/usr/bin/env python3
"""Run one public RecoveryOS journey with a fresh HTTP cookie jar."""

import argparse
import http.cookiejar
import json
import time
import urllib.error
import urllib.request


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("live", "backup"))
    parser.add_argument("--base-url", default="https://valkirie-shield.xyz/recoveryos/")
    parser.add_argument("--expect-daily-limit", action="store_true")
    parser.add_argument("--other-session-id")
    args = parser.parse_args()
    root = args.base_url.rstrip("/") + "/"
    assert root.startswith("https://"), "rehearsal requires HTTPS"
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    started = time.monotonic()

    def request(path: str, method: str = "GET", payload=None, headers=None):
        data = None if method == "GET" else (json.dumps(payload).encode() if payload is not None else b"")
        request_headers = headers or {}
        if payload is not None:
            request_headers["Content-Type"] = "application/json"
        req = urllib.request.Request(root + path, data=data, method=method, headers=request_headers)
        try:
            with client.open(req, timeout=50) as response:
                body = response.read()
                return response.status, json.loads(body) if body else None, response.headers
        except urllib.error.HTTPError as error:
            body = error.read()
            return error.code, json.loads(body) if body else None, error.headers

    status, snapshot, headers = request("api/sessions", "POST")
    assert status == 201, (status, snapshot)
    cookie = headers.get("Set-Cookie", "")
    assert all(flag in cookie for flag in ("Secure", "HttpOnly", "SameSite=Lax")), cookie
    session_id = snapshot["session"]["id"]
    if args.other_session_id:
        status, observed, _ = request("api/sessions/current?session_id=" + args.other_session_id)
        assert status == 200 and observed["session"]["id"] == session_id
        assert observed.get("payout") is None and observed.get("incident") is None and not observed["events"]

    if args.mode == "live":
        status, snapshot, _ = request("api/payouts", "POST", headers={"X-RecoveryOS-Scenario": "missed-update"})
        assert status == 200, (status, snapshot)
        assert snapshot["payout"]["origin"] == "live"
        print("live payout observed after", round(time.monotonic() - started, 1), "seconds")
        deadline = time.monotonic() + 50
        while snapshot.get("incident") is None and time.monotonic() < deadline:
            time.sleep(3)
            status, snapshot, _ = request("api/payouts/reconcile", "POST")
            assert status == 200, (status, snapshot)
        assert snapshot.get("incident") and snapshot["payout"]["provider_status"] == "SUCCESS", snapshot
    else:
        if args.expect_daily_limit:
            status, response, _ = request("api/payouts", "POST", headers={"X-RecoveryOS-Scenario": "missed-update"})
            assert status == 429, (status, response)
        status, snapshot, _ = request("api/sessions/backup", "POST")
        assert status == 201, (status, snapshot)
        assert snapshot["session"]["id"] != session_id
        assert snapshot["payout"]["origin"] == "precreated"
        assert snapshot["payout"]["provider_status"] == "SUCCESS"
        assert snapshot.get("incident"), snapshot
        session_id = snapshot["session"]["id"]
        print("backup incident ready after", round(time.monotonic() - started, 1), "seconds")

    incident_id = snapshot["incident"]["id"]
    payout_id = snapshot["payout"]["id"]
    status, investigation, _ = request("api/incidents/investigate", "POST")
    assert status == 200 and investigation["proposed_action"] == "correct_payout_record", (status, investigation)
    assert investigation["origin"] in ("ai", "deterministic_fallback"), investigation
    status, _, _ = request("api/incidents/approve", "POST", {"incident_id": incident_id})
    assert status == 204, status
    status, resolved, _ = request("api/sessions/current")
    assert status == 200 and resolved["session"]["id"] == session_id
    assert resolved["payout"]["id"] == payout_id and resolved["payout"]["status"] == "corrected"
    assert resolved["incident"]["id"] == incident_id and resolved["incident"]["status"] == "resolved"
    kinds = {event["kind"] for event in resolved["events"]}
    assert {"internal_update_skipped", "investigation_completed", "operator_approval", "record_corrected"} <= kinds
    print(json.dumps({"mode": args.mode, "session_id": session_id, "payout_id": payout_id,
                      "incident_id": incident_id, "diagnosis": investigation["origin"],
                      "seconds": round(time.monotonic() - started, 1), "result": "resolved"}))


if __name__ == "__main__":
    main()
