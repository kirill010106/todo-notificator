import subprocess
import tempfile
from pathlib import Path
import unittest
from deploy_release import Release


class FakeDocker:
    def __init__(self, fail=None, invalid_dump=False):
        self.calls = []
        self.fail = fail
        self.invalid_dump = invalid_dump

    def __call__(self, args, **kwargs):
        self.calls.append(args)
        if self.fail and self.fail(args):
            raise subprocess.CalledProcessError(1, args)
        if "pg_dump" in " ".join(args):
            kwargs["stdout"].write(b"bad" if self.invalid_dump else b"PGDMParchive")
        if "pg_restore" in args:
            assert kwargs["stdin"].read(5) == b"PGDMP"
        return subprocess.CompletedProcess(args, 0, stdout="a" * 64 if "ps" in args else "")


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

    def test_sender_readiness_failure_stops_partial_release(self):
        fake = FakeDocker(fail=lambda a: "up" in a and a[-1] == "email-notifier")
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_release(fake).deploy()
        self.assertIn("stop", fake.calls[-1])


if __name__ == "__main__":
    unittest.main()
