"""Opt-in check of the actual bundled PostgreSQL Compose service and its volume."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import uuid

from deploy_release import Release


@unittest.skipUnless(os.environ.get("RUN_RELEASE_INTEGRATION") == "1", "set RUN_RELEASE_INTEGRATION=1 for Docker")
class BundledDatabaseIntegration(unittest.TestCase):
    def test_first_start_backup_and_container_recreation_preserve_data(self):
        repository = Path(__file__).resolve().parents[1]
        project = "release-db-" + uuid.uuid4().hex[:12]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # Synthetic settings only; never read the developer's .env.
            env_file = root / ".env"
            env_file.write_text("IMAGE_BASE=example/test\nIMAGE_TAG=" + "a" * 40 + "\n"
                                "POSTGRES_USER=todo\nPOSTGRES_DB=todo\nPOSTGRES_PASSWORD=" + "p" * 32 + "\n"
                                "APP_SECRET=test\nWEBHOOK_SECRET=test\nSMTP_USERNAME=test\nSMTP_PASSWORD=test\nSMTP_FROM=test@example.com\n",
                                encoding="utf-8")
            environment = {key: value for key, value in os.environ.items()
                           if key not in ("DATABASE_URL", "POSTGRES_USER", "POSTGRES_DB", "POSTGRES_PASSWORD", "IMAGE_BASE", "IMAGE_TAG", "COMPOSE_PROJECT_NAME")}
            model = subprocess.run(["docker", "compose", "--project-name", project,
                                    "--project-directory", str(root), "--env-file", str(env_file),
                                    "-f", str(repository / "docker-compose.yml"), "--profile", "database", "--profile", "tools",
                                    "config", "--format", "json"],
                                   check=True, capture_output=True, text=True, env=environment)
            actual = json.loads(model.stdout)
            # Exercise actual service definitions without starting app or Traefik.
            fixture = {"name": project, "services": {key: actual["services"][key] for key in ("postgres", "db-tools")},
                       "volumes": {"postgres_data": {"name": project + "-data"}},
                       "networks": {"default": {"name": project + "-net"}}}
            (root / "docker-compose.yml").write_text(json.dumps(fixture), encoding="utf-8")

            def runner(args, **kwargs):
                kwargs.setdefault("env", environment)
                return subprocess.run(args, **kwargs)

            release = Release(root, root / "backups", runner)
            release.managed_postgres = True

            def sql(statement):
                # Statement is a fixed test fixture, never user input.
                return release.compose_call(["run", "--rm", "-T", "--no-deps", "db-tools", "sh", "-ec",
                    'exec psql --dbname="$DATABASE_URL" --no-psqlrc -v ON_ERROR_STOP=1 -tAc "$1"', "sh", statement],
                    capture_output=True, text=True).stdout.strip()

            try:
                release.prepare_database()
                sql("CREATE TABLE persistence_check(id int PRIMARY KEY); INSERT INTO persistence_check VALUES (42)")
                release.save_backup()
                self.assertTrue(release.backup.with_suffix(".dump.sha256").is_file())
                release.compose_call(["rm", "--force", "--stop", "postgres"], capture_output=True)
                release.prepare_database()
                self.assertEqual(sql("SELECT id FROM persistence_check"), "42")
            finally:
                # Only the unique, disposable test project and its named volume.
                release.compose_call(["down", "--volumes", "--remove-orphans"], capture_output=True)


if __name__ == "__main__":
    unittest.main()
