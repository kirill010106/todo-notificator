import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from release_config import validate_config


def valid_config():
    common = {"DATABASE_URL": "postgres://todo:secret@db.internal:5432/todo",
              "WEBHOOK_SECRET": "w" * 32}
    backend = dict(common, APP_SECRET="s" * 32, ENV="prod", AUTO_MIGRATE="false",
                   TRUSTED_PROXIES="172.20.0.2/32", CLIENT_URL="https://todo.chkpnk.ru",
                   WEBHOOK_URL="http://email-notifier:8084", ACTIVITY_LOGGER_ADDR="activity-logger:50051")
    email = dict(common, APP_URL="https://todo.chkpnk.ru", SMTP_HOST="smtp-relay.brevo.com",
                 SMTP_PORT="587", SMTP_USERNAME="smtp-login", SMTP_PASSWORD="smtp-secret",
                 SMTP_FROM="noreply@chkpnk.ru")
    services = {name: {"image": f"ghcr.io/owner/todo-{suffix}:" + "a" * 40}
                for name, suffix in (("backend", "backend"), ("frontend", "frontend"),
                                     ("email-notifier", "email"), ("activity-logger", "activity-logger"))}
    services["backend"]["environment"] = backend
    services["email-notifier"]["environment"] = email
    services["activity-logger"]["environment"] = {"MONGO_URL": "mongodb://mongo:27017"}
    return {"services": services}


class ConfigTests(unittest.TestCase):
    def test_valid_configuration(self):
        self.assertEqual(validate_config(valid_config()), [])

    def test_auto_is_allowed_only_before_host_resolution(self):
        config = valid_config()
        config["services"]["backend"]["environment"]["TRUSTED_PROXIES"] = "auto"
        self.assertEqual(validate_config(config, allow_auto_proxies=True), [])
        self.assertIn("TRUSTED_PROXIES", "\n".join(validate_config(config)))

    def test_local_or_incomplete_settings_are_rejected_without_values(self):
        cases = [("backend", "APP_SECRET", "change-this-secret", "APP_SECRET"),
                 ("backend", "DATABASE_URL", "postgres://u:PRIVATE@127.0.0.1/todo", "DATABASE_URL"),
                 ("backend", "TRUSTED_PROXIES", "0.0.0.0/0", "TRUSTED_PROXIES"),
                 ("backend", "TRUSTED_PROXIES", "127.0.0.1", "TRUSTED_PROXIES"),
                 ("backend", "CLIENT_URL", "http://localhost:3030", "CLIENT_URL"),
                 ("backend", "ENV", "local", "ENV"),
                 ("backend", "AUTO_MIGRATE", "true", "AUTO_MIGRATE"),
                 ("backend", "WEBHOOK_URL", "http://localhost:8084", "WEBHOOK_URL"),
                 ("email-notifier", "SMTP_PASSWORD", "", "SMTP_PASSWORD"),
                 ("email-notifier", "SMTP_PORT", "25", "SMTP_PORT"),
                 ("email-notifier", "SMTP_FROM", "invalid", "SMTP_FROM"),
                 ("backend", "SHOP_ID", "only-one", "SHOP_ID")]
        cases.append(("activity-logger", "MONGO_URL", "mongodb://127.0.0.1:27017", "MONGO_URL"))
        for service, key, value, expected in cases:
            with self.subTest(key=key, value=value):
                config = valid_config()
                config["services"][service]["environment"][key] = value
                errors = "\n".join(validate_config(config))
                self.assertIn(expected, errors)
                self.assertNotIn("PRIVATE", errors)

    def test_mixed_releases_are_rejected(self):
        config = valid_config()
        config["services"]["frontend"]["image"] = "ghcr.io/owner/todo-frontend:" + "b" * 40
        self.assertIn("same IMAGE_BASE and IMAGE_TAG", "\n".join(validate_config(config)))

    def test_bundled_database_requires_matching_credentials_and_private_connection(self):
        for mode in ("short_password", "mismatch", "tls", "missing_service"):
            with self.subTest(mode=mode):
                config = valid_config()
                config["services"]["postgres"] = {"environment": {
                    "POSTGRES_USER": "todo", "POSTGRES_DB": "todo", "POSTGRES_PASSWORD": "p" * 32}}
                url = "postgres://todo:" + "p" * 32 + "@postgres:5432/todo?sslmode=disable"
                if mode == "short_password":
                    config["services"]["postgres"]["environment"]["POSTGRES_PASSWORD"] = "short"
                elif mode == "mismatch":
                    url = url.replace("todo?", "other?")
                elif mode == "tls":
                    url = url.replace("disable", "require")
                else:
                    del config["services"]["postgres"]
                for service in ("backend", "email-notifier"):
                    config["services"][service]["environment"]["DATABASE_URL"] = url
                self.assertTrue(validate_config(config))

    @unittest.skipUnless(shutil.which("docker"), "Docker CLI is required for Compose config")
    def test_actual_compose_and_production_template(self):
        root = Path(__file__).resolve().parents[1]
        template = (root / ".env.production.example").read_text(encoding="utf-8")
        replacements = {"REPLACE_WITH_40_CHARACTER_COMMIT_SHA": "a" * 40,
                        "REPLACE_WITH_RANDOM_POSTGRES_PASSWORD_AT_LEAST_32_CHARACTERS": "p" * 32,
                        "REPLACE_WITH_RANDOM_SECRET_AT_LEAST_32_CHARACTERS": "s" * 32,
                        "REPLACE_WITH_ANOTHER_RANDOM_SECRET_AT_LEAST_32_CHARACTERS": "w" * 32,
                        "REPLACE_WITH_BREVO_SMTP_LOGIN": "smtp-login", "REPLACE_WITH_BREVO_SMTP_KEY": "smtp-key"}
        for before, after in replacements.items():
            template = template.replace(before, after)
        with tempfile.TemporaryDirectory() as temp:
            target = Path(temp)
            shutil.copyfile(root / "docker-compose.yml", target / "docker-compose.yml")
            (target / ".env").write_text(template, encoding="utf-8")
            # Host deployment variables must not override this isolated fixture.
            env = copy.copy(os.environ)
            env.pop("DATABASE_URL", None)
            for key in (line.split("=", 1)[0] for line in template.splitlines() if "=" in line and not line.startswith("#")):
                env.pop(key, None)
            result = subprocess.run(["docker", "compose", "--project-directory", temp,
                                     "--env-file", str(target / ".env"), "-f", str(target / "docker-compose.yml"),
                                     "--profile", "tools", "--profile", "database", "config", "--format", "json"],
                                    check=True, capture_output=True, text=True, env=env)
            config = json.loads(result.stdout)
            self.assertEqual(validate_config(config, allow_auto_proxies=True), [])
            backend = config["services"]["backend"]["environment"]
            email = config["services"]["email-notifier"]["environment"]
            self.assertEqual(backend["CONFIG_PATH"], "/app/config/prod.yaml")
            self.assertEqual(backend["TRUSTED_PROXIES"], "auto")
            self.assertEqual(email["EMAIL_CONFIG_PATH"], "/app/config/prod.yaml")
            self.assertNotIn("SMTP_PASSWORD", backend)
            self.assertNotIn("APP_SECRET", email)
            self.assertNotIn("SECRET_KEY", email)
            self.assertEqual(backend["DATABASE_URL"], "postgres://todo:" + "p" * 32 + "@postgres:5432/todo?sslmode=disable")
            self.assertEqual(email["DATABASE_URL"], backend["DATABASE_URL"])
            self.assertEqual(config["services"]["db-tools"]["environment"]["DATABASE_URL"], backend["DATABASE_URL"])
            self.assertFalse(config["services"]["postgres"].get("ports"))
            self.assertEqual(config["services"]["postgres"]["volumes"][0]["target"], "/var/lib/postgresql/data")
            for service in config["services"].values():
                self.assertNotIn("ENV_FILE", service.get("environment", {}))
            for service in ("grafana", "prometheus"):
                self.assertTrue(all(port["host_ip"] == "127.0.0.1" for port in config["services"][service]["ports"]))


if __name__ == "__main__":
    unittest.main()
