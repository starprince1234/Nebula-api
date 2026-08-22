#!/usr/bin/env python3
from pathlib import Path
import yaml


CONFIG_PATH = Path("/opt/mihomo/config.yaml")
REQUIRED_TUN_CONFIGURATION = {
    "enable": True,
    "stack": "mixed",
    "device": "mihomo",
    "auto-route": True,
    "auto-redirect": True,
    "auto-detect-interface": True,
}
REQUIRED_LISTENER_CONFIGURATION = {
    "allow-lan": True,
    "bind-address": "*",
    "external-controller": "0.0.0.0:9090",
}
PROXY_HEALTH_CHECK_URL = "https://www.gstatic.com/generate_204"


def main() -> None:
    if not CONFIG_PATH.is_file():
        raise SystemExit(f"Mihomo configuration is missing: {CONFIG_PATH}")

    with CONFIG_PATH.open("r", encoding="utf-8") as config_file:
        configuration = yaml.safe_load(config_file)
    if not isinstance(configuration, dict):
        raise SystemExit("Mihomo configuration root must be a mapping")

    configuration.update(REQUIRED_LISTENER_CONFIGURATION)

    tun_configuration = configuration.get("tun")
    if tun_configuration is None:
        tun_configuration = {}
        configuration["tun"] = tun_configuration
    if not isinstance(tun_configuration, dict):
        raise SystemExit("Mihomo tun configuration must be a mapping")
    tun_configuration.update(REQUIRED_TUN_CONFIGURATION)

    proxy_groups = configuration.get("proxy-groups", [])
    if not isinstance(proxy_groups, list):
        raise SystemExit("Mihomo proxy-groups configuration must be a list")
    for group in proxy_groups:
        if not isinstance(group, dict):
            raise SystemExit("Each Mihomo proxy group must be a mapping")
        if group.get("type") in {"fallback", "url-test"}:
            group["url"] = PROXY_HEALTH_CHECK_URL
            group["interval"] = 300

    temporary_path = CONFIG_PATH.with_suffix(".yaml.tmp")
    with temporary_path.open("w", encoding="utf-8", newline="\n") as config_file:
        yaml.safe_dump(
            configuration,
            config_file,
            allow_unicode=True,
            sort_keys=False,
            width=120,
        )
    temporary_path.chmod(0o600)
    temporary_path.replace(CONFIG_PATH)


if __name__ == "__main__":
    main()
