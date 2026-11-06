"""Generate Material 3 color roles for Nacre's palettes from HCT tonal palettes, and check contrast."""
import json
from materialyoucolor.palettes.tonal_palette import TonalPalette

PALETTES = {
    # name: (primary, secondary, tertiary, neutral, neutral variant) as (hue, chroma)
    "ocean":  ((268, 56), (250, 24), (236, 44), (258, 6), (255, 12)),   # lots of blues
    "sage":   ((140, 36), (125, 16), (190, 26), (130, 4), (130, 8)),
    "ember":  ((42, 60),  (48, 20),  (80, 38),  (45, 6),  (45, 10)),
    "orchid": ((330, 44), (318, 18), (15, 34),  (320, 5), (320, 10)),
}

def hx(argb):
    return "#%06X" % (argb & 0xFFFFFF)

def lum(h):
    r, g, b = (int(h[i:i + 2], 16) / 255 for i in (1, 3, 5))
    f = lambda c: c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)

def cr(a, b):
    la, lb = sorted((lum(a), lum(b)), reverse=True)
    return (la + 0.05) / (lb + 0.05)

def roles(p, s, t, n, nv, dark):
    P, S, T, N, NV = (TonalPalette.from_hue_and_chroma(h, c) for h, c in (p, s, t, n, nv))
    g = lambda pal, tone: hx(pal.tone(tone))
    if not dark:
        return {
            "primary": g(P, 40), "on-primary": g(P, 100), "primary-container": g(P, 90), "on-primary-container": g(P, 30),
            "secondary": g(S, 40), "on-secondary": g(S, 100), "secondary-container": g(S, 90), "on-secondary-container": g(S, 30),
            "tertiary": g(T, 40), "on-tertiary": g(T, 100), "tertiary-container": g(T, 90), "on-tertiary-container": g(T, 30),
            "surface": g(N, 98), "on-surface": g(N, 10), "on-surface-variant": g(NV, 30),
            "sc-lowest": g(N, 100), "sc-low": g(N, 96), "sc": g(N, 94), "sc-high": g(N, 92), "sc-highest": g(N, 90),
            "outline": g(NV, 50), "outline-variant": g(NV, 80),
            "inverse-surface": g(N, 20), "inverse-on-surface": g(N, 95), "inverse-primary": g(P, 80), "on-inverse-primary": g(P, 20),
        }
    return {
        "primary": g(P, 80), "on-primary": g(P, 20), "primary-container": g(P, 30), "on-primary-container": g(P, 90),
        "secondary": g(S, 80), "on-secondary": g(S, 20), "secondary-container": g(S, 30), "on-secondary-container": g(S, 90),
        "tertiary": g(T, 80), "on-tertiary": g(T, 20), "tertiary-container": g(T, 30), "on-tertiary-container": g(T, 90),
        "surface": g(N, 6), "on-surface": g(N, 90), "on-surface-variant": g(NV, 80),
        "sc-lowest": g(N, 4), "sc-low": g(N, 10), "sc": g(N, 12), "sc-high": g(N, 17), "sc-highest": g(N, 22),
        "outline": g(NV, 60), "outline-variant": g(NV, 30),
        "inverse-surface": g(N, 90), "inverse-on-surface": g(N, 20), "inverse-primary": g(P, 40), "on-inverse-primary": g(P, 100),
    }

CHECKS = [
    ("on-surface-variant", "sc-highest", 4.5), ("on-surface", "surface", 4.5), ("on-primary", "primary", 4.5),
    ("on-primary-container", "primary-container", 4.5), ("on-secondary-container", "secondary-container", 4.5),
    ("on-tertiary-container", "tertiary-container", 4.5), ("on-secondary", "secondary", 4.5),
    ("secondary", "sc", 4.5), ("outline", "sc-high", 3.0), ("primary", "surface", 3.0),
    ("on-inverse-primary", "inverse-primary", 4.5), ("on-surface", "sc-highest", 4.5),
]

css, swatches, report = [], {}, []
for name, spec in PALETTES.items():
    for dark in (False, True):
        r = roles(*spec, dark)
        sel = (".pal-%s.dark, .pal-%s .m3.dark, .m3.dark.pal-%s" % (name, name, name)) if dark else (".pal-%s, .pal-%s .m3" % (name, name))
        body = " ".join("--%s: %s;" % (k, v) for k, v in r.items())
        css.append("%s { %s }" % (sel, body))
        for a, b, need in CHECKS:
            c = cr(r[a], r[b])
            if c < need:
                report.append("FAIL %s %s %s on %s %.2f < %.1f" % (name, "dark" if dark else "light", a, b, c, need))
        if not dark:
            P, S, T = (TonalPalette.from_hue_and_chroma(h, c) for h, c in spec[:3])
            swatches[name] = [hx(P.tone(40)), hx(S.tone(70)), hx(T.tone(60)), r["primary-container"]]
open("palettes.css", "w").write("\n".join(css) + "\n")
json.dump(swatches, open("swatches.json", "w"), indent=1)
print("\n".join(report) or "all contrast checks pass")
print(json.dumps(swatches))
