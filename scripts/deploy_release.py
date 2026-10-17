"""Quiesced Compose release. Run on the Linux deployment host, never on import."""
import argparse
import contextlib
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
from datetime import datetime, timezone


class Release:
    services = ("frontend", "backend", "email-notifier")

    def __init__(self, root, backups, runner=subprocess.run):
        self.root, self.backups, self.runner = Path(root), Path(backups), runner
        self.compose = ["docker", "compose", "--project-directory", str(self.root),
                        "--env-file", str(self.root / ".env"), "-f", str(self.root / "docker-compose.yml"), "--profile", "tools"]
        self.backup = None

    def call(self, args, **kwargs):
        # Never print resolved Compose configuration or database credentials.
        return self.runner(args, check=True, **kwargs)

    def compose_call(self, args, **kwargs):
        return self.call(self.compose + list(args), **kwargs)

    def disable_restarts(self):
        ids = self.compose_call(["ps", "--all", "--quiet", *self.services], capture_output=True, text=True).stdout.split()
        if any(not re.fullmatch(r"[0-9a-f]{12,64}", value) for value in ids):
            raise RuntimeError("Invalid container IDs; release refused")
        if ids:
            self.call(["docker", "update", "--restart=no", *ids], stdout=subprocess.DEVNULL)

    def stop(self):
        self.compose_call(["stop", "--timeout", "60", *self.services])

    def save_backup(self):
        self.backups.mkdir(parents=True, exist_ok=True, mode=0o700)
        name = datetime.now(timezone.utc).strftime("postgres-%Y%m%dT%H%M%S.%fZ.dump")
        self.backup = self.backups / name
        # Exclusive creation prevents overwriting any existing backup.
        fd = os.open(self.backup, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, "wb") as output:
            self.compose_call(["run", "--rm", "-T", "--no-deps", "db-tools", "sh", "-ec",
                               'exec pg_dump --dbname="$DATABASE_URL" --format=custom'], stdout=output)
            output.flush()
            os.fsync(output.fileno())
        with self.backup.open("rb", buffering=0) as source:
            if source.read(5) != b"PGDMP":
                raise RuntimeError("Backup is not a PostgreSQL custom archive; migration refused")
            source.seek(0)
            self.compose_call(["run", "--rm", "-T", "--no-deps", "db-tools", "pg_restore", "--list"],
                              stdin=source, stdout=subprocess.DEVNULL)
            source.seek(0)
            # Read/decompress all data sections as well as the table of contents.
            self.compose_call(["run", "--rm", "-T", "--no-deps", "db-tools", "pg_restore", "--file=/dev/null"],
                              stdin=source, stdout=subprocess.DEVNULL)
        digest = hashlib.sha256()
        with self.backup.open("rb") as source:
            for block in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(block)
        checksum = self.backup.with_suffix(".dump.sha256")
        with os.fdopen(os.open(checksum, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as f:
            f.write(digest.hexdigest() + "  " + self.backup.name + "\n")
        print("Verified archive and checksum:", self.backup, flush=True)

    def deploy(self):
        self.compose_call(["config", "--quiet"])
        self.compose_call(["pull", "backend", "frontend", "email-notifier", "activity-logger", "mongo", "db-tools"])
        quiescing = False
        try:
            quiescing = True
            # A daemon restart must not revive the old sender after the cutover.
            self.disable_restarts()
            self.stop()
            self.save_backup()
            print("Applying migrations with application writers stopped", flush=True)
            self.compose_call(["run", "--rm", "-T", "--no-deps", "backend", "backend", "migrate"])
            self.compose_call(["up", "-d", "mongo", "activity-logger"])
            self.compose_call(["up", "-d", "--no-deps", "--force-recreate", "--wait", "--wait-timeout", "120", "backend", "frontend"])
            # Start delivery only after backend schema gate and HTTP readiness.
            self.compose_call(["up", "-d", "--no-deps", "--force-recreate", "--wait", "--wait-timeout", "120", "email-notifier"])
            print("Release ready", flush=True)
        except BaseException:
            if quiescing:
                try:
                    # Leave no partially updated application serving or delivering.
                    self.disable_restarts()
                    self.stop()
                except Exception:
                    print("Could not stop services; operator must check all writers", file=sys.stderr)
                print("Release failed. Application services remain stopped; no automatic rollback. Backup:",
                      self.backup or "not completed", file=sys.stderr)
            raise


@contextlib.contextmanager
def release_lock(directory):
    import fcntl  # Linux deployment host; unit tests do not need platform locks.
    Path(directory).mkdir(parents=True, exist_ok=True, mode=0o700)
    with open(Path(directory) / "release.lock", "a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("Another release is already running") from None
        yield


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--backups", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", os.environ.get("IMAGE_TAG", "")):
        parser.error("IMAGE_TAG must be the immutable 40-character release commit SHA")
    with release_lock(args.backups):
        Release(args.root.resolve(), args.backups.resolve()).deploy()


if __name__ == "__main__":
    main()
