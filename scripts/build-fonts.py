#!/usr/bin/env python3
"""Regenerate the self-hosted web fonts in frontend/src/app/fonts/.

The build must never fetch fonts from fonts.googleapis.com (TEC-141), so the
woff2 files are committed and loaded with next/font/local. This script is the
reproducible recipe for those files; the build itself never runs it.

Source: the variable TTFs in github.com/google/fonts at a pinned commit (the
same files the Google Fonts API serves), checked against a sha256. Each font
is cut to the weight range the app uses and to the unicode ranges of the
Google Fonts subsets below, then written as a single woff2 per family.
One file per family is required because next/font/local cannot emit a
per-file unicode-range. Scripts a family does not draw (Arabic everywhere,
Cyrillic/Greek in the sans fonts) fall through to the CSS fallback stack in
src/styles/theme.css.

Usage (needs fonttools + brotli; network access only here):
    python3 -m venv /tmp/fontvenv && /tmp/fontvenv/bin/pip install fonttools brotli
    /tmp/fontvenv/bin/python scripts/build-fonts.py
"""

from __future__ import annotations

import hashlib
import io
import pathlib
import urllib.request

from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

GOOGLE_FONTS_REF = "9710da1eacb3be272583c3224dcb70f9da6eadbb"
RAW = f"https://raw.githubusercontent.com/google/fonts/{GOOGLE_FONTS_REF}/ofl"
OUT = pathlib.Path(__file__).resolve().parent.parent / "frontend/src/app/fonts"

# Google Fonts subset unicode ranges (as published by the CSS2 API).
SUBSETS = {
    "latin": "U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,"
    "U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,"
    "U+2215,U+FEFF,U+FFFD",
    "latin-ext": "U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,"
    "U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,"
    "U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF",
    "cyrillic": "U+0301,U+0400-045F,U+0490-0491,U+04B0-04B1,U+2116",
    "cyrillic-ext": "U+0460-052F,U+1C80-1C8A,U+20B4,U+2DE0-2DFF,U+A640-A69F,"
    "U+FE2E-FE2F",
    "greek": "U+0370-0377,U+037A-037F,U+0384-038A,U+038C,U+038E-03A1,U+03A3-03FF",
}

# (output name, google/fonts dir, TTF name, sha256, wght min, wght max, subsets)
FONTS = [
    (
        "plus-jakarta-sans",
        "plusjakartasans",
        "PlusJakartaSans[wght].ttf",
        "89b3fb38aa0d275d7a731d0d817a4f1622b316b4d7fbdedcf02ee9099ff68bc8",
        400,
        700,
        ["latin", "latin-ext"],
    ),
    (
        "outfit",
        "outfit",
        "Outfit[wght].ttf",
        "fc7287273e66929776e2ba54f144fe699080bec29f61bf649d70d871468aeade",
        500,
        700,
        ["latin", "latin-ext"],
    ),
    (
        "jetbrains-mono",
        "jetbrainsmono",
        "JetBrainsMono[wght].ttf",
        "48715a42ec242c21e9f02692891e147d022299a52e48d5e413e1a942193ffeda",
        400,
        500,
        ["latin", "latin-ext", "cyrillic", "cyrillic-ext", "greek"],
    ),
]


def fetch(url: str) -> bytes:
    with urllib.request.urlopen(url, timeout=60) as resp:
        return resp.read()


def unicodes(names: list[str]) -> list[int]:
    points: set[int] = set()
    for name in names:
        for part in SUBSETS[name].split(","):
            lo, _, hi = part.removeprefix("U+").partition("-")
            points.update(range(int(lo, 16), int(hi or lo, 16) + 1))
    return sorted(points)


def build(name, family_dir, ttf, sha256, wmin, wmax, subsets) -> None:
    quoted = ttf.replace("[", "%5B").replace("]", "%5D")
    data = fetch(f"{RAW}/{family_dir}/{quoted}")
    digest = hashlib.sha256(data).hexdigest()
    if digest != sha256:
        raise SystemExit(f"{ttf}: sha256 {digest} != pinned {sha256}")

    # Limit the weight axis, then reload from bytes: subsetting a freshly
    # instanced in-memory font trips over lazily loaded gvar data.
    font = instancer.instantiateVariableFont(
        TTFont(io.BytesIO(data)), {"wght": (wmin, wmax)}
    )
    buf = io.BytesIO()
    font.save(buf)
    font = TTFont(io.BytesIO(buf.getvalue()))

    opts = subset.Options()
    opts.flavor = "woff2"
    opts.layout_features = ["*"]
    opts.name_IDs = ["*"]
    opts.name_languages = ["*"]
    sub = subset.Subsetter(options=opts)
    sub.populate(unicodes=unicodes(subsets))
    sub.subset(font)

    target = OUT / f"{name}-wght.woff2"
    font.flavor = "woff2"
    font.save(target)
    (OUT / f"{name}-OFL.txt").write_bytes(fetch(f"{RAW}/{family_dir}/OFL.txt"))
    print(f"{target.relative_to(OUT.parent.parent.parent)}: {target.stat().st_size} bytes")


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for spec in FONTS:
        build(*spec)


if __name__ == "__main__":
    main()
