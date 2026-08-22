#!/usr/bin/env python3
import json
from urllib import parse, request


CONTROLLER_URL = "http://127.0.0.1:9090"
PRIMARY_GROUP = "闪电猫"
PRODUCTION_SELECTION = "故障转移"


def main() -> None:
    group_name = parse.quote(PRIMARY_GROUP, safe="")
    selection_request = request.Request(
        f"{CONTROLLER_URL}/proxies/{group_name}",
        data=json.dumps({"name": PRODUCTION_SELECTION}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="PUT",
    )
    with request.urlopen(selection_request, timeout=5) as response:
        if response.status != 204:
            raise SystemExit(
                f"Unexpected Mihomo controller status: {response.status}"
            )


if __name__ == "__main__":
    main()
