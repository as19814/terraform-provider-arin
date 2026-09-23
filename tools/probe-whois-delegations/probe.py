#!/usr/bin/env python3
"""Probe public delegation lookup/search behavior without credentials or redirects."""
import concurrent.futures
import datetime
import json
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        return None


NAME = "120.189.23.in-addr.arpa"
ORIGINS = ("https://whois.ote.arin.net", "https://whois.arin.net")
PATHS = (
    f"/rest/rdns/{NAME}",
    f"/rest/rdns/{NAME}.",
    f"/rest/rdns/{NAME}*",
    f"/rest/rdns;name={NAME}",
    f"/rest/rdns;dname={NAME}",
    f"/rest/rdns;delegationName={NAME}",
    f"/rest/rdns;delegation%20name={NAME}",
    f"/rest/rdns?name={NAME}",
    # A contradictory predicate detects a silently ignored matrix key.
    f"/rest/rdns/{NAME};name=does-not-exist.invalid",
    "/rest/net/NET-23-189-120-0-1/rdns",
)


def probe(pair):
    origin, path = pair
    result = {"origin": origin, "path": path}
    request = urllib.request.Request(origin + path, headers={"Accept": "application/xml"})
    try:
        try:
            response = urllib.request.build_opener(NoRedirect).open(request, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            result["status"] = response.status
            body = response.read(65537)
            result["over_limit"] = len(body) > 65536
            if response.status == 200 and not result["over_limit"]:
                root = ET.fromstring(body)
                result["root"] = root.tag
                # Report only whether the expected identity was returned.
                result["expected_delegation"] = any(
                    child.tag.rsplit("}", 1)[-1] == "name"
                    and (child.text or "").rstrip(".") == NAME
                    for child in root
                )
    except (OSError, ValueError, ET.ParseError) as error:
        result["error_type"] = type(error).__name__
    return result


if __name__ == "__main__":
    print(json.dumps({"checked_at": datetime.datetime.now(datetime.timezone.utc).isoformat()}))
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        for result in pool.map(probe, [(origin, path) for origin in ORIGINS for path in PATHS]):
            print(json.dumps(result, sort_keys=True))
