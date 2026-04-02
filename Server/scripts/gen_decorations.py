#!/usr/bin/env python3
"""Shim: runs the Go generator (no Python dependency for logic)."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

if __name__ == "__main__":
    repo = Path(__file__).resolve().parents[2]
    sys.exit(
        subprocess.call(
            ["go", "run", "./Server/cmd/gendecorationwids"],
            cwd=repo,
        )
    )
