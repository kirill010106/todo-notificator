"""Discover the single running Traefik on the backend's explicit Docker network."""
import ipaddress
import json
import re
import subprocess


def discover(config, call):
    backend = config["services"]["backend"]
    network_name = backend.get("labels", {}).get("traefik.docker.network")
    attached = backend.get("networks", {})
    matches = [item for key, item in config.get("networks", {}).items()
               if key in attached and item.get("name") == network_name and item.get("external")]
    if not network_name or len(matches) != 1:
        raise RuntimeError("Cannot discover Traefik: backend must select one external Docker network")

    def inspect(args):
        try:
            result = call(["docker", *args], capture_output=True, text=True, timeout=20)
            return json.loads(result.stdout)
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError, json.JSONDecodeError):
            # Container inspection may include credentials. Never echo Docker output.
            raise RuntimeError("Cannot discover Traefik: Docker inspection failed; running services unchanged") from None

    networks = inspect(["network", "inspect", network_name])
    if not isinstance(networks, list) or len(networks) != 1:
        raise RuntimeError("Cannot discover Traefik: unexpected Docker network response")
    network = networks[0]
    if not re.fullmatch(r"[0-9a-f]{64}", network.get("Id", "")):
        raise RuntimeError("Cannot discover Traefik: invalid network identity")
    if network.get("Driver") != "bridge" or network.get("Scope") != "local":
        raise RuntimeError("Automatic proxy discovery requires a local Docker bridge network")
    endpoints = network.get("Containers") or {}
    if not endpoints or any(not re.fullmatch(r"[0-9a-f]{64}", key) for key in endpoints):
        raise RuntimeError("Cannot discover Traefik: proxy network has no valid container endpoints")
    containers = inspect(["container", "inspect", *sorted(endpoints)])
    if not isinstance(containers, list):
        raise RuntimeError("Cannot discover Traefik: unexpected container response")

    candidates = []
    for container in containers:
        image = container.get("Config", {}).get("Image", "").split("@", 1)[0]
        # Remove a tag only from the last path segment (registries can have ports).
        image = image.rsplit("/", 1)
        image[-1] = image[-1].split(":", 1)[0]
        image = "/".join(image)
        if image not in ("traefik", "library/traefik", "docker.io/traefik", "docker.io/library/traefik", "dhi.io/traefik"):
            continue
        if not container.get("State", {}).get("Running"):
            continue
        endpoint = container.get("NetworkSettings", {}).get("Networks", {}).get(network_name)
        identifier = container.get("Id")
        if not endpoint or identifier not in endpoints or endpoint.get("NetworkID") != network.get("Id"):
            raise RuntimeError("Traefik network changed during discovery; retry the release")
        addresses = []
        for field, listed_field in (("IPAddress", "IPv4Address"), ("GlobalIPv6Address", "IPv6Address")):
            raw = endpoint.get(field)
            if not raw:
                continue
            try:
                address = ipaddress.ip_address(raw)
                listed = ipaddress.ip_interface(endpoints[identifier].get(listed_field, "")).ip
                if address != listed or address.is_loopback or address.is_unspecified or address.is_multicast or address.is_link_local:
                    raise ValueError()
            except ValueError:
                raise RuntimeError("Cannot discover Traefik: invalid or changed proxy address") from None
            addresses.append(str(ipaddress.ip_network(address)))
        if not addresses:
            raise RuntimeError("Cannot discover Traefik: running proxy has no usable address")
        candidates.append(addresses)
    if len(candidates) != 1:
        raise RuntimeError("Expected exactly one running Traefik on the backend network; release refused before stopping services")
    return ",".join(candidates[0])
