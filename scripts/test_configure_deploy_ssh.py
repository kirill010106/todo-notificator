import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from configure_deploy_ssh import configure_known_hosts


class DeploymentHostPinTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.keys = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.keys.cleanup)
        key = Path(cls.keys.name) / "server"
        subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(key)], check=True)
        cls.public_key = key.with_suffix(".pub").read_text().strip()

    def test_verified_key_is_bound_to_exact_host(self):
        for host in ["192.0.2.10", "2001:db8::10", "deploy.example.test"]:
            with self.subTest(host=host), tempfile.TemporaryDirectory() as directory:
                target = Path(directory) / "known_hosts"
                configure_known_hosts(host, self.public_key, target)
                expected = f"{host} {' '.join(self.public_key.split()[:2])}\n"
                self.assertEqual(target.read_text(), expected)
                result = subprocess.run(["ssh-keygen", "-F", host, "-f", str(target)], capture_output=True)
                self.assertEqual(result.returncode, 0)
                if os.name != "nt":
                    self.assertEqual(target.stat().st_mode & 0o777, 0o600)

    def test_invalid_configuration_preserves_existing_pin(self):
        cases = [(host, self.public_key) for host in [
            "", " host", "host\nother", "*.example.test", "host,other", "-oProxyCommand=bad",
            "host:22", "fe80::1%zone", "example..test",
        ]]
        cases += [("deploy.example.test", key) for key in [
            "", self.public_key + "\n" + self.public_key, "ssh-ed25519 !!!",
            "ssh-ed25519 bm90LWEtc3NoLWtleQ==", "ssh-dss AAAA",
        ]]
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "known_hosts"
            target.write_text("existing verified pin\n")
            for host, key in cases:
                with self.subTest(host=host, key=key[:12]):
                    with self.assertRaises(ValueError):
                        configure_known_hosts(host, key, target)
                    self.assertEqual(target.read_text(), "existing verified pin\n")
                    self.assertEqual(list(Path(directory).iterdir()), [target])

    def test_missing_secret_fails_before_creating_known_hosts(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "known_hosts"
            env = dict(os.environ, SERVER_IP="deploy.example.test", SSH_HOST_PUBLIC_KEY="")
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("configure_deploy_ssh.py")),
                                     "--known-hosts", str(target)], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(target.exists())
            self.assertEqual(result.stdout, "")
            self.assertIn("missing or invalid", result.stderr)

    def test_cd_uses_only_the_pinned_file_with_strict_verification(self):
        workflow = (Path(__file__).resolve().parents[1] / ".github/workflows/cd.yml").read_text()
        self.assertNotIn("ssh-keyscan", workflow)
        self.assertIn('ANSIBLE_HOST_KEY_CHECKING: "True"', workflow)
        for option in ["StrictHostKeyChecking=yes", "UserKnownHostsFile=${{ runner.temp }}/deploy_known_hosts",
                       "GlobalKnownHostsFile=/dev/null", "UpdateHostKeys=no", "VerifyHostKeyDNS=no"]:
            self.assertIn(option, workflow)
        self.assertIn("SSH_HOST_PUBLIC_KEY: ${{ secrets.SSH_HOST_PUBLIC_KEY }}", workflow)
        self.assertLess(workflow.index("Install verified deployment host key"), workflow.index("Setup SSH key"))
        self.assertLess(workflow.index("Install verified deployment host key"), workflow.index("Generate .env file"))


if __name__ == "__main__":
    unittest.main()
