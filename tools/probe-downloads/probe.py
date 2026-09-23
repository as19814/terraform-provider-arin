#!/usr/bin/env python3
"""Probe OT&E download authorization without retrieving report content."""

import concurrent.futures
import datetime
import json
import os
import urllib.error
import urllib.parse
import urllib.request

ORIGIN = "https://accountws.ote.arin.net"
PATHS = (
    "/public/rest/downloads/bulkwhois",
    "/public/rest/downloads/bulkwhois/asns.xml",
    "/public/rest/downloads/nvpr",
    "/public/rest/downloads/terraform-provider-arin-nonexistent",
)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def probe(path, auth, key):
    url = ORIGIN + path
    headers = {}
    if auth == "header":
        headers["Authorization"] = "ApiKey " + key
    elif auth == "query":
        url += "?" + urllib.parse.urlencode({"apikey": key})
    result = {"path": path, "auth": auth}
    request = urllib.request.Request(url, headers=headers, method="HEAD")
    opener = urllib.request.build_opener(NoRedirect())
    try:
        with opener.open(request, timeout=15) as response:
            result["status"] = response.status
    except urllib.error.HTTPError as error:
        result["status"] = error.code
        error.close()
    except Exception as error:
        # Exception messages can contain the authenticated URL. Never print them.
        result["error_type"] = type(error).__name__
    return result


def main():
    key = os.environ.get("ARIN_OTE_API_KEY", "")
    if not key:
        raise SystemExit("Set ARIN_OTE_API_KEY; production-key fallback is disabled")
    print(json.dumps({"origin": ORIGIN, "method": "HEAD", "time": datetime.datetime.now(datetime.timezone.utc).isoformat()}))
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        futures = [pool.submit(probe, path, auth, key) for path in PATHS for auth in ("none", "header", "query")]
        results = [future.result() for future in futures]
    for result in results:
        print(json.dumps(result))
    # HTTP denials are evidence, not proof of successful download support.
    return int(any("error_type" in result for result in results))


if __name__ == "__main__":
    raise SystemExit(main())
