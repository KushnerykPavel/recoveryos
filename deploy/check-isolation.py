#!/usr/bin/env python3
"""Check that two anonymous HTTPS visitors cannot cross session boundaries."""

import http.cookiejar
import json
import urllib.error
import urllib.request

ROOT = "https://recoveryos.valkirie-shield.xyz/"


def visitor():
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def call(path, method="GET", payload=None):
        data = None if method == "GET" else (json.dumps(payload).encode() if payload is not None else b"")
        headers = {"Content-Type": "application/json"} if payload is not None else {}
        req = urllib.request.Request(ROOT + path, data=data, method=method, headers=headers)
        try:
            response = client.open(req, timeout=50)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read()
            return response.status, json.loads(body) if body else None

    return call


def main():
    first = visitor()
    second = visitor()
    status, first_session = first("api/sessions", "POST")
    assert status == 201
    status, second_session = second("api/sessions", "POST")
    assert status == 201 and first_session["session"]["id"] != second_session["session"]["id"]
    status, first_case = first("api/sessions/backup", "POST")
    assert status == 201 and first_case.get("incident")
    status, _ = first("api/incidents/investigate", "POST")
    assert status == 200
    first_id = first_case["session"]["id"]
    incident_id = first_case["incident"]["id"]
    status, hidden = second("api/sessions/current?session_id=" + first_id)
    assert status == 200 and hidden["session"]["id"] == second_session["session"]["id"]
    assert hidden.get("payout") is None and hidden.get("incident") is None and not hidden["events"]
    status, _ = second("api/incidents/approve", "POST", {"incident_id": incident_id})
    assert status == 409
    status, first_before = first("api/sessions/current")
    assert status == 200 and first_before["incident"]["status"] == "open"
    assert first_before["payout"]["status"] == "pending"
    assert not any(event["kind"] == "record_correction_rejected" for event in first_before["events"])
    status, second_after = second("api/sessions/current")
    assert status == 200 and second_after.get("payout") is None and second_after.get("incident") is None
    assert len(second_after["events"]) == 1 and second_after["events"][0]["kind"] == "record_correction_rejected"
    status, _ = first("api/incidents/approve", "POST", {"incident_id": incident_id})
    assert status == 204
    status, first_after = first("api/sessions/current")
    assert status == 200 and first_after["incident"]["status"] == "resolved"
    assert first_after["payout"]["status"] == "corrected"
    status, second_final = second("api/sessions/current?session_id=" + first_id)
    assert status == 200 and second_final["session"]["id"] == second_session["session"]["id"]
    assert second_final.get("payout") is None and second_final.get("incident") is None
    print(json.dumps({"first_session": first_id, "second_session": second_session["session"]["id"],
                      "cross_read": "blocked", "cross_approval": "blocked", "audit": "isolated",
                      "first_case": "resolved"}))


if __name__ == "__main__":
    main()
