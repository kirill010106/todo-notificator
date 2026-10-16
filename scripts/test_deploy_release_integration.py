"""Opt-in archive/restore check with a disposable PostgreSQL container."""
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
import uuid
from deploy_release import Release


@unittest.skipUnless(os.environ.get("RUN_RELEASE_INTEGRATION") == "1", "set RUN_RELEASE_INTEGRATION=1 for Docker")
class ArchiveIntegration(unittest.TestCase):
    def test_archive_is_restorable_with_rows(self):
        name = "release-test-" + uuid.uuid4().hex[:12]
        network = name + "-net"
        def docker(*args, **kwargs):
            return subprocess.run(["docker", *args], check=True, capture_output=True, **kwargs)
        docker("network", "create", network)
        try:
            docker("run", "-d", "--name", name, "--network", network,
                   "-e", "POSTGRES_PASSWORD=test-only", "-e", "POSTGRES_DB=release", "postgres:16-alpine")
            try:
                for _ in range(60):
                    ready = subprocess.run(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "release"], capture_output=True)
                    if ready.returncode == 0:
                        break
                    time.sleep(.5)
                else:
                    self.fail("PostgreSQL did not become ready")
                docker("exec", name, "psql", "-U", "postgres", "-d", "release", "-v", "ON_ERROR_STOP=1", "-c",
                       "CREATE TABLE tasks(id int PRIMARY KEY,title text); INSERT INTO tasks VALUES (1,'first'),(2,'second');")
                with tempfile.TemporaryDirectory() as temp:
                    root = Path(temp)
                    (root / ".env").write_text("", encoding="utf-8")
                    (root / "docker-compose.yml").write_text(
                        "services:\n  db-tools:\n    image: postgres:16-alpine\n    environment:\n"
                        "      DATABASE_URL: postgres://postgres:test-only@" + name + ":5432/release?sslmode=disable\n"
                        "    networks: [test]\nnetworks:\n  test:\n    external: true\n    name: " + network + "\n", encoding="utf-8")
                    release = Release(root, root / "backups")
                    release.save_backup()
                    self.assertTrue(release.backup.with_suffix(".dump.sha256").is_file())
                    docker("exec", name, "createdb", "-U", "postgres", "restored")
                    with release.backup.open("rb") as source:
                        docker("exec", "-i", name, "pg_restore", "-U", "postgres", "-d", "restored", stdin=source)
                    restored = docker("exec", name, "psql", "-U", "postgres", "-d", "restored", "-Atc", "SELECT COUNT(*) FROM tasks")
                    self.assertEqual(b"2", restored.stdout.strip())
            finally:
                docker("rm", "-f", name)
        finally:
            docker("network", "rm", network)


if __name__ == "__main__":
    unittest.main()
