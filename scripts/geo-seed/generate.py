#!/usr/bin/env python3
"""Generates backend/migrations/000035_geo_territory_rates.up.sql (TEC-84).

Inputs (read only):
  /usr/share/iso-codes/json/iso_3166-1.json, iso_3166-2.json  (Debian iso-codes)
  names_tr.json      ISO alpha-2 -> Turkish name (CLDR via golang.org/x/text/language/display)
  il-ilce.json       olexfilms database/data/il-ilce.json (81 il, 973 ilçe)
  head.sql, tail.sql static schema parts next to this script

Usage:
  python3 scripts/geo-seed/generate.py names_tr.json il-ilce.json > backend/migrations/000035_geo_territory_rates.up.sql
"""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ISO1 = "/usr/share/iso-codes/json/iso_3166-1.json"
ISO2 = "/usr/share/iso-codes/json/iso_3166-2.json"

# Countries with full metadata (organizations, phone default, plate).
META = {
    "TR": ("+90", "TRY", "tr", "Europe/Istanbul"),
    "DE": ("+49", "EUR", "de", "Europe/Berlin"),
    "NL": ("+31", "EUR", "en", "Europe/Amsterdam"),
    "GR": ("+30", "EUR", "el", "Europe/Athens"),
    "US": ("+1", "USD", "en", "America/New_York"),
    "UA": ("+380", "UAH", "uk", "Europe/Kyiv"),
    "BG": ("+359", "BGN", "bg", "Europe/Sofia"),
    "AZ": ("+994", "AZN", "az", "Asia/Baku"),
    "CN": ("+86", "CNY", "zh-CN", "Asia/Shanghai"),
    "AE": ("+971", "AED", "ar", "Asia/Dubai"),
    "GB": ("+44", "GBP", "en", "Europe/London"),
    "FR": ("+33", "EUR", "fr", "Europe/Paris"),
    "ES": ("+34", "EUR", "es", "Europe/Madrid"),
    "IT": ("+39", "EUR", "it", "Europe/Rome"),
    "RU": ("+7", "RUB", "ru", "Europe/Moscow"),
}

# ISO 3166-2 province level for the target countries (subdivision types kept).
PROVINCE_TYPES = {
    "DE": {"Land"},
    "NL": {"Province"},
    "GR": {"Administrative region"},
    "US": {"State", "District"},
    "UA": {"Region", "City", "Republic"},
    "BG": {"District"},
    "AZ": None,  # every top-level subdivision (no parent)
    "CN": None,
    "AE": None,
}

# NL gemeenten (district level): all of Noord-Holland + the province capitals
# and largest cities. The rest is added by the admin (platform.geo.write).
NL_DISTRICTS = {
    "NL-NH": [
        "Aalsmeer", "Alkmaar", "Amstelveen", "Amsterdam", "Bergen (NH)", "Beverwijk",
        "Blaricum", "Bloemendaal", "Castricum", "Den Helder", "Diemen", "Dijk en Waard",
        "Drechterland", "Edam-Volendam", "Enkhuizen", "Gooise Meren", "Haarlem",
        "Haarlemmermeer", "Heemskerk", "Heemstede", "Heiloo", "Hilversum", "Hollands Kroon",
        "Hoorn", "Huizen", "Koggenland", "Landsmeer", "Laren", "Medemblik", "Oostzaan",
        "Opmeer", "Ouder-Amstel", "Purmerend", "Schagen", "Stede Broec", "Texel",
        "Uitgeest", "Uithoorn", "Velsen", "Waterland", "Wijdemeren", "Wormerland",
        "Zaanstad", "Zandvoort",
    ],
    "NL-ZH": ["Rotterdam", "'s-Gravenhage", "Leiden", "Dordrecht"],
    "NL-UT": ["Utrecht", "Amersfoort"],
    "NL-NB": ["'s-Hertogenbosch", "Eindhoven", "Tilburg", "Breda"],
    "NL-GE": ["Arnhem", "Nijmegen"],
    "NL-GR": ["Groningen"],
    "NL-FR": ["Leeuwarden"],
    "NL-DR": ["Assen"],
    "NL-OV": ["Zwolle", "Enschede"],
    "NL-FL": ["Lelystad", "Almere"],
    "NL-ZE": ["Middelburg"],
    "NL-LI": ["Maastricht"],
}


def q(s):
    return "'" + s.replace("'", "''") + "'"


def tr_title(word):
    """Turkish title case for one upper-case word (ALADAĞ -> Aladağ)."""
    lower = word.replace("I", "ı").replace("İ", "i").lower()
    if not lower:
        return lower
    first = lower[0]
    first = {"i": "İ", "ı": "I"}.get(first, first.upper())
    return first + lower[1:]


def tr_name(s):
    return " ".join(tr_title(w) for w in s.strip().split())


def nullable(v):
    return "NULL" if v is None else q(v)


def main():
    names_tr = json.load(open(sys.argv[1], encoding="utf-8"))
    il = json.load(open(sys.argv[2], encoding="utf-8"))["data"]
    iso1 = json.load(open(ISO1, encoding="utf-8"))["3166-1"]
    iso2 = json.load(open(ISO2, encoding="utf-8"))["3166-2"]

    out = [open(os.path.join(HERE, "head.sql"), encoding="utf-8").read()]

    rows = []
    for c in sorted(iso1, key=lambda x: x["alpha_2"]):
        a2 = c["alpha_2"]
        en = c.get("common_name") or c["name"]
        trn = names_tr.get(a2) or en
        phone, cur, loc, tz = META.get(a2, (None, None, None, None))
        rows.append(
            f"    ({q(a2)}, {q(c['alpha_3'])}, {q(c['numeric'])}, {q(en)}, {q(trn)}, "
            f"{nullable(phone)}, {nullable(cur)}, {nullable(loc)}, {nullable(tz)})"
        )
    out.append(
        "INSERT INTO countries (iso2, iso3, numeric_code, name_en, name_tr, phone_code, "
        "default_currency, default_locale, timezone) VALUES\n" + ",\n".join(rows) + ";\n\n"
    )

    # Provinces
    prov = []
    for p in il:
        prov.append(("TR", p["plaka_kodu"].strip().zfill(2), p["il_adi"].strip()))
    for cc, types in PROVINCE_TYPES.items():
        for s in sorted((x for x in iso2 if x["code"].startswith(cc + "-")), key=lambda x: x["code"]):
            if "parent" in s:
                continue
            if types is not None and s["type"] not in types:
                continue
            prov.append((cc, s["code"], s["name"]))
    rows = [f"    ({q(cc)}, {q(code)}, {q(name)})" for cc, code, name in prov]
    out.append(
        "INSERT INTO provinces (country_id, code, name)\nSELECT c.id, v.code, v.name\nFROM (VALUES\n"
        + ",\n".join(rows)
        + "\n) AS v (iso2, code, name)\nJOIN countries c ON c.iso2 = v.iso2;\n\n"
    )

    # Districts
    dist = []
    for p in il:
        pcode = p["plaka_kodu"].strip().zfill(2)
        seen = set()
        for d in p["ilceler"]:
            name = tr_name(d["ilce_adi"])
            if name in seen:
                continue
            seen.add(name)
            dist.append(("TR", pcode, d.get("ilce_kodu"), name))
    for pcode, names in NL_DISTRICTS.items():
        for n in names:
            dist.append(("NL", pcode, None, n))
    rows = [f"    ({q(cc)}, {q(pc)}, {nullable(code)}, {q(n)})" for cc, pc, code, n in dist]
    out.append(
        "INSERT INTO districts (province_id, code, name)\nSELECT p.id, v.code, v.name\nFROM (VALUES\n"
        + ",\n".join(rows)
        + "\n) AS v (iso2, province_code, code, name)\n"
        "JOIN countries c ON c.iso2 = v.iso2\n"
        "JOIN provinces p ON p.country_id = c.id AND p.code = v.province_code;\n"
    )

    out.append(open(os.path.join(HERE, "tail.sql"), encoding="utf-8").read())
    sys.stdout.write("".join(out))
    sys.stderr.write(f"countries={len(iso1)} provinces={len(prov)} districts={len(dist)}\n")


if __name__ == "__main__":
    main()
