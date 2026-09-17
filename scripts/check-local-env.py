#!/usr/bin/env python3
"""Validate the single local configuration without printing secret values."""

from __future__ import annotations

import argparse
from pathlib import Path
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parent.parent
ENV_FILE = ROOT / ".env.local"


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw_line in path.read_text().splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def problems(values: dict[str, str]) -> list[str]:
    errors: list[str] = []
    for name in ("DATABASE_URL", "REDIS_URL"):
        if not values.get(name):
            errors.append(f"{name} is empty or missing")
    if values.get("APP_ENV") != "development":
        errors.append("APP_ENV must be 'development' for the canonical local environment")
    if values.get("PIPELINE_DISPATCH_ENABLED") != "false":
        errors.append("PIPELINE_DISPATCH_ENABLED must be 'false' because the worker is a separate service")

    if values.get("SPV_ENABLED", "false").lower() == "true":
        for name in ("SPV_OAUTH_CLIENT_ID", "SPV_OAUTH_CLIENT_SECRET", "SPV_TOKEN_ENCRYPTION_KEY"):
            if not values.get(name):
                errors.append(f"{name} is required when SPV_ENABLED=true")
        key = values.get("SPV_TOKEN_ENCRYPTION_KEY", "")
        if key:
            try:
                if len(bytes.fromhex(key)) != 32:
                    errors.append("SPV_TOKEN_ENCRYPTION_KEY must encode exactly 32 bytes")
            except ValueError:
                errors.append("SPV_TOKEN_ENCRYPTION_KEY must contain hexadecimal characters only")
        for name in ("SPV_OAUTH_REDIRECT_URI", "FRONTEND_BASE_URL"):
            parsed = urlparse(values.get(name, ""))
            if not parsed.scheme or not parsed.netloc:
                errors.append(f"{name} must be a complete URL")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", action="store_true", help="report problems without failing")
    args = parser.parse_args()
    if not ENV_FILE.exists():
        print("ERROR: .env.local is missing. Run: make setup")
        return 0 if args.report else 1
    errors = problems(load_env(ENV_FILE))
    if errors:
        print("Local configuration is incomplete:")
        for error in errors:
            print(f"  - {error}")
        print("Edit .env.local privately; values were not displayed.")
        return 0 if args.report else 1
    print("Local configuration preflight: PASS (secret values not displayed).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
