#!/usr/bin/env python3
"""Regenerate the static app fonts in mobile/assets/fonts/ from the web fonts.

React Native (iOS in particular) cannot load woff2 or pick a weight from a
variable font, so each weight the app uses is cut into its own static TTF.
The source is the web's self-hosted variable woff2 files
(frontend/src/app/fonts, built by scripts/build-fonts.py), so both apps draw
the same glyphs and cover the same scripts (latin + latin-ext; other scripts
fall back to the system font).

Usage (needs fonttools + brotli, no network):
    python3 -m venv /tmp/fontvenv && /tmp/fontvenv/bin/pip install fonttools brotli
    /tmp/fontvenv/bin/python mobile/scripts/build-fonts.py
"""

from __future__ import annotations

import pathlib
import shutil

from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
SRC = ROOT / "frontend/src/app/fonts"
OUT = ROOT / "mobile/assets/fonts"

# (web woff2, file prefix = font family name in src/ui/fonts.ts, {wght: style})
FONTS = [
    (
        "plus-jakarta-sans-wght.woff2",
        "PlusJakartaSans",
        {400: "Regular", 500: "Medium", 600: "SemiBold", 700: "Bold"},
    ),
    ("outfit-wght.woff2", "Outfit", {600: "SemiBold", 700: "Bold"}),
    ("jetbrains-mono-wght.woff2", "JetBrainsMono", {400: "Regular", 500: "Medium"}),
]


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for woff2, family, weights in FONTS:
        for wght, style in weights.items():
            font = TTFont(SRC / woff2)
            static = instancer.instantiateVariableFont(
                font, {"wght": wght}, updateFontNames=True
            )
            static.flavor = None
            path = OUT / f"{family}-{style}.ttf"
            static.save(path)
            print(f"{path.relative_to(ROOT)}  wght={wght}")
    for license_file in SRC.glob("*-OFL.txt"):
        shutil.copy(license_file, OUT / license_file.name)


if __name__ == "__main__":
    main()
