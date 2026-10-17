"""Validate resolved production Compose settings without logging secrets."""
import ipaddress
import re
from urllib.parse import urlsplit, unquote, parse_qs


def validate_config(config, allow_auto_proxies=False):
    errors = []
    services = config.get("services", {})
    backend = services.get("backend", {}).get("environment", {})
    email = services.get("email-notifier", {}).get("environment", {})

    def require(env, key, service, minimum=1):
        value = str(env.get(key) or "").strip()
        if len(value) < minimum or any(token in value.lower() for token in
                ("replace_with", "change-this", "your_", "your-")):
            errors.append(f"{service}: configure {key}")
        return value

    images = []
    for service, suffix in (("backend", "backend"), ("frontend", "frontend"),
                            ("email-notifier", "email"), ("activity-logger", "activity-logger")):
        image = services.get(service, {}).get("image", "")
        match = re.fullmatch(r"(.+)-" + suffix + r":([0-9a-f]{40})", image)
        if not match:
            errors.append(f"{service}: require IMAGE_BASE and a 40-character IMAGE_TAG")
        else:
            images.append(match.groups())
    if len(set(images)) > 1:
        errors.append("All application images must use the same IMAGE_BASE and IMAGE_TAG")

    for name, env in (("backend", backend), ("email-notifier", email)):
        raw = require(env, "DATABASE_URL", name)
        try:
            url = urlsplit(raw)
            host = url.hostname
            if url.scheme not in ("postgres", "postgresql") or not host or not url.path.strip("/"):
                raise ValueError()
            if host.lower() == "localhost" or ip_is_local(host):
                raise ValueError()
        except ValueError:
            errors.append(f"{name}: DATABASE_URL must point to PostgreSQL reachable from containers")
        require(env, "WEBHOOK_SECRET", name, 32)

    if backend.get("DATABASE_URL") != email.get("DATABASE_URL"):
        errors.append("Backend and email-notifier must use the same DATABASE_URL")
    try:
        database = urlsplit(backend.get("DATABASE_URL") or "")
        if database.hostname == "postgres":
            postgres = services.get("postgres", {}).get("environment", {})
            password = require(postgres, "POSTGRES_PASSWORD", "postgres", 32)
            user = require(postgres, "POSTGRES_USER", "postgres")
            name = require(postgres, "POSTGRES_DB", "postgres")
            if not re.fullmatch(r"[A-Za-z0-9_-]+", password):
                errors.append("postgres: use a URL-safe POSTGRES_PASSWORD (for example secrets.token_hex(32))")
            if any(not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]{0,62}", value) for value in (user, name)):
                errors.append("postgres: POSTGRES_USER and POSTGRES_DB must be simple SQL identifiers")
            if (unquote(database.username or "") != user or unquote(database.password or "") != password
                    or unquote(database.path.lstrip("/")) != name or database.port not in (None, 5432)):
                errors.append("DATABASE_URL must match the bundled PostgreSQL credentials, database and port")
            if parse_qs(database.query).get("sslmode") != ["disable"]:
                errors.append("Bundled PostgreSQL uses sslmode=disable on the private Docker network")
    except ValueError:
        errors.append("Invalid DATABASE_URL for bundled PostgreSQL")
    if backend.get("WEBHOOK_SECRET") != email.get("WEBHOOK_SECRET"):
        errors.append("Backend and email-notifier must use the same WEBHOOK_SECRET")
    require(backend, "APP_SECRET", "backend", 32)
    if backend.get("ENV") != "prod" or str(backend.get("AUTO_MIGRATE")).lower() != "false":
        errors.append("Backend must use ENV=prod and AUTO_MIGRATE=false")

    proxies = require(backend, "TRUSTED_PROXIES", "backend")
    if not (allow_auto_proxies and proxies == "auto"):
        try:
            for raw in proxies.split(","):
                network = ipaddress.ip_network(raw.strip(), strict=False)
                if network.prefixlen == 0 or network.is_loopback or network.is_unspecified:
                    raise ValueError()
        except ValueError:
            errors.append("TRUSTED_PROXIES must contain actual proxy IPs or restricted CIDRs")

    for env, key, service in ((backend, "CLIENT_URL", "backend"), (email, "APP_URL", "email-notifier")):
        raw = require(env, key, service)
        try:
            url = urlsplit(raw)
            if url.scheme != "https" or not url.hostname or url.username or url.password:
                raise ValueError()
            if url.hostname.lower() == "localhost" or ip_is_local(url.hostname):
                raise ValueError()
        except ValueError:
            errors.append(f"{service}: {key} must be a public HTTPS URL")

    # Explicit production endpoints avoid inheriting localhost from local .env files.
    if backend.get("WEBHOOK_URL") != "http://email-notifier:8084":
        errors.append("backend: set WEBHOOK_URL=http://email-notifier:8084")
    if backend.get("ACTIVITY_LOGGER_ADDR") != "activity-logger:50051":
        errors.append("backend: set ACTIVITY_LOGGER_ADDR=activity-logger:50051")
    for key in ("SMTP_HOST", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM"):
        require(email, key, "email-notifier")
    if str(email.get("SMTP_PORT")) not in ("465", "587", "2525"):
        errors.append("email-notifier: SMTP_PORT must be 465, 587 or 2525")
    if not re.fullmatch(r"[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+", str(email.get("SMTP_FROM") or "")):
        errors.append("email-notifier: SMTP_FROM must be a sender email address")
    if bool(backend.get("SHOP_ID")) != bool(backend.get("SECRET_KEY")):
        errors.append("Set both SHOP_ID and SECRET_KEY, or leave both empty")
    mongo = require(services.get("activity-logger", {}).get("environment", {}), "MONGO_URL", "activity-logger")
    try:
        url = urlsplit(mongo)
        if url.scheme not in ("mongodb", "mongodb+srv") or not url.hostname:
            raise ValueError()
        if url.hostname.lower() == "localhost" or ip_is_local(url.hostname):
            raise ValueError()
    except ValueError:
        errors.append("activity-logger: MONGO_URL must be reachable from containers")
    return errors


def ip_is_local(host):
    try:
        address = ipaddress.ip_address(host)
        return address.is_loopback or address.is_unspecified
    except ValueError:
        return False
