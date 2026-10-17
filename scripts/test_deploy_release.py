import subprocess
import json
import tempfile
from pathlib import Path
import unittest
from deploy_release import Release
from test_release_config import valid_config


class FakeDocker:
    def __init__(self, fail=None, invalid_dump=False):
        self.calls = []
        self.fail = fail
        self.invalid_dump = invalid_dump
        self.config = valid_config()

    def __call__(self, args, **kwargs):
        self.calls.append(args)
        if self.fail and self.fail(args):
            raise subprocess.CalledProcessError(1, args)
        if "pg_dump" in " ".join(args):
            kwargs["stdout"].write(b"bad" if self.invalid_dump else b"PGDMParchive")
        if "pg_restore" in args:
            assert kwargs["stdin"].read(5) == b"PGDMP"
        output = json.dumps(self.config) if "config" in args else "a" * 64 if "ps" in args else ""
        return subprocess.CompletedProcess(args, 0, stdout=output)


class ReleaseTests(unittest.TestCase):
    def run_release(self, fake):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        release = Release(Path(temp.name) / "app", Path(temp.name) / "backups", fake)
        return release

    def index(self, calls, predicate):
        return next(i for i, args in enumerate(calls) if predicate(args))

    def test_stop_backup_verify_migrate_ready_then_sender(self):
        fake = FakeDocker()
        release = self.run_release(fake)
        release.deploy()
        calls = fake.calls
        stop = self.index(calls, lambda a: "stop" in a)
        dump = self.index(calls, lambda a: "pg_dump" in " ".join(a))
        verify = self.index(calls, lambda a: "pg_restore" in a)
        migrate = self.index(calls, lambda a: "migrate" in a)
        ready = self.index(calls, lambda a: "up" in a and a[-1] == "frontend")
        sender = self.index(calls, lambda a: "up" in a and a[-1] == "email-notifier")
        self.assertLess(stop, dump)
        self.assertLess(dump, verify)
        self.assertLess(verify, migrate)
        self.assertLess(migrate, ready)
        self.assertLess(ready, sender)
        self.assertIn("--wait", calls[ready])
        self.assertIn("--wait", calls[sender])
        self.assertTrue(release.backup.with_suffix(".dump.sha256").exists())
        self.assertTrue(any("--restart=no" in a for a in calls[:stop]))

    def test_failures_never_restart_application_or_sender(self):
        stages = [lambda a: "pg_dump" in " ".join(a),
                  lambda a: "pg_restore" in a,
                  lambda a: "--file=/dev/null" in a,
                  lambda a: "migrate" in a,
                  lambda a: "up" in a and a[-1] == "frontend"]
        for stage in stages:
            with self.subTest(stage=stage):
                fake = FakeDocker(fail=stage)
                with self.assertRaises(subprocess.CalledProcessError):
                    self.run_release(fake).deploy()
                self.assertIn("stop", fake.calls[-1])
                self.assertFalse(any("up" in a and a[-1] == "email-notifier" for a in fake.calls))

    def test_invalid_archive_prevents_migration(self):
        fake = FakeDocker(invalid_dump=True)
        with self.assertRaises(RuntimeError):
            self.run_release(fake).deploy()
        self.assertFalse(any("migrate" in a for a in fake.calls))
        self.assertIn("stop", fake.calls[-1])

    def test_pull_failure_does_not_stop_running_services(self):
        fake = FakeDocker(fail=lambda a: "pull" in a)
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_release(fake).deploy()
        self.assertFalse(any("stop" in a for a in fake.calls))

    def managed_release(self, fake):
        fake.config["services"]["postgres"] = {"environment": {
            "POSTGRES_USER": "todo", "POSTGRES_DB": "todo", "POSTGRES_PASSWORD": "p" * 32}}
        for name in ("backend", "email-notifier"):
            fake.config["services"][name]["environment"]["DATABASE_URL"] = "postgres://todo:" + "p" * 32 + "@postgres:5432/todo?sslmode=disable"
        return self.run_release(fake)

    def test_managed_database_ready_and_authenticated_before_stop_and_backup(self):
        fake = FakeDocker()
        self.managed_release(fake).deploy()
        calls = fake.calls
        ready = self.index(calls, lambda a: "up" in a and a[-1] == "postgres")
        auth = self.index(calls, lambda a: "SELECT 1" in " ".join(a))
        stop = self.index(calls, lambda a: "stop" in a)
        backup = self.index(calls, lambda a: "pg_dump" in " ".join(a))
        self.assertLess(ready, auth)
        self.assertLess(auth, stop)
        self.assertLess(stop, backup)
        self.assertIn("--no-recreate", calls[ready])
        self.assertTrue(any("pull" in a and "postgres" in a for a in calls))

    def test_database_failure_does_not_stop_running_application(self):
        for fail in (lambda a: "up" in a and a[-1] == "postgres", lambda a: "SELECT 1" in " ".join(a)):
            fake = FakeDocker(fail=fail)
            with self.assertRaises((subprocess.CalledProcessError, RuntimeError)):
                self.managed_release(fake).deploy()
            self.assertFalse(any("stop" in a for a in fake.calls))

    def test_external_database_does_not_start_bundled_postgres(self):
        fake = FakeDocker()
        self.run_release(fake).deploy()
        self.assertFalse(any("postgres" in a for a in fake.calls))
        self.assertTrue(any("SELECT 1" in " ".join(a) for a in fake.calls))

    def test_check_only_reads_configuration(self):
        fake = FakeDocker()
        self.run_release(fake).preflight()
        self.assertEqual(len(fake.calls), 1)
        self.assertEqual(fake.calls[0][-3:], ["config", "--format", "json"])

    def test_config_failure_hides_values_and_never_stops_services(self):
        calls = []
        def fail(args, **kwargs):
            calls.append(args)
            raise subprocess.CalledProcessError(1, args, stderr="secret-value required variable IMAGE_TAG is missing")
        with self.assertRaises(RuntimeError) as result:
            self.run_release(fail).deploy()
        self.assertIn("IMAGE_TAG", str(result.exception))
        self.assertNotIn("secret-value", str(result.exception))
        self.assertEqual(len(calls), 1)

    def test_invalid_settings_never_pull_or_stop_services(self):
        fake = FakeDocker()
        def invalid_settings(args, **kwargs):
            result = fake(args, **kwargs)
            config = valid_config()
            config["services"]["backend"]["environment"]["APP_SECRET"] = "short"
            result.stdout = json.dumps(config)
            return result
        with self.assertRaisesRegex(RuntimeError, "APP_SECRET"):
            self.run_release(invalid_settings).deploy()
        self.assertEqual(len(fake.calls), 1)

    def test_sender_readiness_failure_stops_partial_release(self):
        fake = FakeDocker(fail=lambda a: "up" in a and a[-1] == "email-notifier")
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_release(fake).deploy()
        self.assertIn("stop", fake.calls[-1])


if __name__ == "__main__":
    unittest.main()
