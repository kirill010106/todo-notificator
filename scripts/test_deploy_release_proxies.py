import copy
import json
import subprocess
import tempfile
from pathlib import Path
import unittest

from deploy_release import Release
from test_deploy_release import FakeDocker
from test_release_config import valid_config
from trusted_proxies import discover


PROXY_ID = "b" * 64
APP_ID = "c" * 64
NETWORK_ID = "d" * 64


def auto_config():
    config = valid_config()
    config["services"]["backend"].update(
        labels={"traefik.docker.network": "traefik"}, networks={"proxy": {}, "default": {}})
    config["services"]["backend"]["environment"]["TRUSTED_PROXIES"] = "auto"
    config["networks"] = {"proxy": {"name": "traefik", "external": True}, "default": {"name": "app_default"}}
    return config


def network():
    return {"Id": NETWORK_ID, "Driver": "bridge", "Scope": "local", "Containers": {
        PROXY_ID: {"IPv4Address": "172.20.0.2/16", "IPv6Address": "fd00::2/64"},
        APP_ID: {"IPv4Address": "172.20.0.3/16", "IPv6Address": ""}}}


def containers():
    return [{"Id": PROXY_ID, "Config": {"Image": "docker.io/library/traefik:v3"}, "State": {"Running": True},
             "NetworkSettings": {"Networks": {"traefik": {"NetworkID": NETWORK_ID,
                 "IPAddress": "172.20.0.2", "GlobalIPv6Address": "fd00::2"}}}},
            {"Id": APP_ID, "Config": {"Image": "app-backend:latest"}, "State": {"Running": True}}]


class ProxyDocker(FakeDocker):
    def __init__(self):
        super().__init__()
        self.config = auto_config()
        self.network = network()
        self.containers = containers()
        self.compose_environments = []

    def __call__(self, args, **kwargs):
        if args[:3] == ["docker", "network", "inspect"]:
            self.calls.append(args)
            return subprocess.CompletedProcess(args, 0, stdout=json.dumps([self.network]))
        if args[:3] == ["docker", "container", "inspect"]:
            self.calls.append(args)
            return subprocess.CompletedProcess(args, 0, stdout=json.dumps(self.containers))
        result = super().__call__(args, **kwargs)
        if args[:2] == ["docker", "compose"]:
            self.compose_environments.append((args, kwargs.get("env")))
        if "config" in args:
            config = copy.deepcopy(self.config)
            if kwargs.get("env", {}).get("TRUSTED_PROXIES"):
                config["services"]["backend"]["environment"]["TRUSTED_PROXIES"] = kwargs["env"]["TRUSTED_PROXIES"]
            result.stdout = json.dumps(config)
        return result


class ProxyTests(unittest.TestCase):
    def release(self, fake):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        return Release(Path(temp.name) / "app", Path(temp.name) / "backups", fake)

    def test_discovery_and_every_release_command_use_exact_addresses(self):
        fake = ProxyDocker()
        release = self.release(fake)
        release.deploy()
        first_stop = next(i for i, args in enumerate(fake.calls) if "stop" in args)
        discovery = next(i for i, args in enumerate(fake.calls) if args[:3] == ["docker", "container", "inspect"])
        self.assertLess(discovery, first_stop)
        for args, env in fake.compose_environments[1:]:
            self.assertEqual(env["TRUSTED_PROXIES"], "172.20.0.2/32,fd00::2/128")
        self.assertFalse((release.root / ".env").exists())

    def test_static_check_does_not_need_docker_daemon(self):
        fake = ProxyDocker()
        self.release(fake).preflight()
        self.assertEqual(len(fake.calls), 1)

    def test_manual_override_skips_discovery(self):
        fake = ProxyDocker()
        fake.config["services"]["backend"]["environment"]["TRUSTED_PROXIES"] = "172.20.0.2/32"
        self.release(fake).deploy()
        self.assertFalse(any(args[:3] == ["docker", "network", "inspect"] for args in fake.calls))

    def test_unavailable_or_ambiguous_proxy_never_stops_services(self):
        for mode in ("missing", "stopped", "multiple", "changed_address", "changed_network", "missing_network", "overlay"):
            with self.subTest(mode=mode):
                fake = ProxyDocker()
                if mode == "missing":
                    fake.containers = []
                elif mode == "stopped":
                    fake.containers[0]["State"]["Running"] = False
                elif mode == "multiple":
                    second = copy.deepcopy(fake.containers[0])
                    second["Id"] = "e" * 64
                    second["NetworkSettings"]["Networks"]["traefik"]["IPAddress"] = "172.20.0.4"
                    second["NetworkSettings"]["Networks"]["traefik"]["GlobalIPv6Address"] = ""
                    fake.network["Containers"][second["Id"]] = {"IPv4Address": "172.20.0.4/16"}
                    fake.containers.append(second)
                elif mode == "changed_address":
                    fake.containers[0]["NetworkSettings"]["Networks"]["traefik"]["IPAddress"] = "172.20.0.99"
                elif mode == "changed_network":
                    fake.containers[0]["NetworkSettings"]["Networks"]["traefik"]["NetworkID"] = "other"
                elif mode == "missing_network":
                    del fake.config["networks"]
                elif mode == "overlay":
                    fake.network["Driver"] = "overlay"
                with self.assertRaises(RuntimeError):
                    self.release(fake).deploy()
                self.assertFalse(any("stop" in args or "pull" in args for args in fake.calls))

    def test_docker_errors_do_not_expose_inspection_contents(self):
        def fail(args, **kwargs):
            raise subprocess.CalledProcessError(1, args, output="PRIVATE", stderr="PASSWORD")
        with self.assertRaises(RuntimeError) as result:
            discover(auto_config(), fail)
        self.assertNotIn("PRIVATE", str(result.exception))
        self.assertNotIn("PASSWORD", str(result.exception))


if __name__ == "__main__":
    unittest.main()
