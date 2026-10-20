"""Install an independently verified deployment host key; never fetch one online."""

import argparse
import base64
import ipaddress
import os
from pathlib import Path
import re
import subprocess
import tempfile


def configure_known_hosts(host: str, public_key: str, destination: Path) -> None:
    if not host or host != host.strip() or "%" in host or any(c.isspace() for c in host):
        raise ValueError("SERVER_IP must contain one host without surrounding whitespace")
    try:
        ipaddress.ip_address(host)
    except ValueError:
        labels = host.rstrip(".").split(".")
        if len(host) > 253 or any(
            re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?", label) is None
            for label in labels
        ):
            raise ValueError("SERVER_IP must be an IP address or DNS hostname") from None

    lines = public_key.strip().splitlines()
    if len(lines) != 1:
        raise ValueError("SSH_HOST_PUBLIC_KEY must contain one verified public key")
    fields = lines[0].split()
    if len(fields) < 2 or fields[0] not in {
        "ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521"
    }:
        raise ValueError("SSH_HOST_PUBLIC_KEY has an unsupported public key format")
    try:
        base64.b64decode(fields[1], validate=True)
    except ValueError:
        raise ValueError("SSH_HOST_PUBLIC_KEY contains invalid base64") from None

    destination.parent.mkdir(parents=True, exist_ok=True)
    # Validate before replacing the trusted file; mkstemp creates a private file
    # and os.replace replaces a symlink itself rather than following its target.
    fd, name = tempfile.mkstemp(prefix="deploy-host-", dir=destination.parent)
    temporary = Path(name)
    try:
        with os.fdopen(fd, "w", encoding="ascii", newline="\n") as stream:
            stream.write(f"{host} {fields[0]} {fields[1]}\n")
        result = subprocess.run(
            ["ssh-keygen", "-l", "-f", str(temporary)],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False,
        )
        if result.returncode:
            raise ValueError("SSH_HOST_PUBLIC_KEY is not a valid SSH public key")
        os.chmod(temporary, 0o600)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--known-hosts", type=Path, required=True)
    args = parser.parse_args()
    try:
        configure_known_hosts(os.environ.get("SERVER_IP", ""), os.environ.get("SSH_HOST_PUBLIC_KEY", ""), args.known_hosts)
    except (ValueError, OSError, subprocess.SubprocessError):
        # Do not echo environment values or parser/ssh-keygen diagnostics.
        parser.exit(1, "Deployment SSH pin is missing or invalid; configure a verified SERVER_IP and SSH_HOST_PUBLIC_KEY.\n")


if __name__ == "__main__":
    main()
