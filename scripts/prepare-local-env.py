#!/usr/bin/env python3
"""Create the single private local configuration without exposing secrets."""
import os
from pathlib import Path
import secrets

root = Path(__file__).resolve().parent.parent
path = root / ".env.local"
if path.exists():
    print(".env.local already exists; leaving it unchanged.")
    raise SystemExit(0)

content = (root / ".env.example").read_text().replace(
    "SPV_TOKEN_ENCRYPTION_KEY=\n", "SPV_TOKEN_ENCRYPTION_KEY=" + secrets.token_hex(32) + "\n"
)
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w") as file:
    file.write(content)
print("Created .env.local for the canonical local environment.")
