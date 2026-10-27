"""Build the five production holograms: transparent, line-only, 16 s orbit.

Uses the standard-library mesh/projector from the approved design studies.
Run from any directory. The generated SVGs are committed and embedded by Go;
Python is not required to build or run the game.
"""

import importlib.util
import math
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "ship_geometry", ROOT / "docs/design/ship-holograms/generate.py"
)
geometry = importlib.util.module_from_spec(spec)
spec.loader.exec_module(geometry)
Mesh = geometry.Mesh


def engine(mesh, x, front, rear, width=.35, height=.38, z=0):
    mesh.loft([(x, front, z, width, height), (x, rear, z, width, height)], part="pods")
    mesh.loft([(x, rear + .03, z, width * .76, height * .76),
               (x, rear + .42, z, width * .76, height * .76)], part="engine")


def wanderer():
    m = Mesh()
    m.loft([(0, -4.8, 0, .08, .12), (0, -2.7, .05, .40, .30),
            (0, -.6, .08, .65, .42), (0, 2.7, .08, .53, .36),
            (0, 3.4, .08, .35, .28)])
    m.loft([(0, -2.6, .43, .20, .12), (0, -.9, .55, .30, .14)], sides=4, part="canopy")
    for sign in (-1, 1):
        m.prism([(sign * .45, -.9), (sign * 2.15, 1.6),
                 (sign * 1.65, 2.2), (sign * .45, .9)], -.1, .08)
        engine(m, sign * 1.28, 1.15, 3.5, .25, .29)
        m.loft([(sign * .44, -3.2, .1, .04, .05),
                (sign * .44, -1.5, .1, .04, .05)], sides=4, part="sensor")
    return m


def forge():
    m = Mesh()
    m.loft([(0, -2.9, .1, .55, .50), (0, -1.3, .1, 1.15, .75),
            (0, 1.8, .1, 1.15, .75), (0, 3, .1, .80, .60)], sides=8)
    m.loft([(0, -1.8, .85, .3, .18), (0, -.6, .93, .48, .20)], sides=4, part="canopy")
    for sign in (-1, 1):
        m.prism([(sign * .8, -1.7), (sign * 2.0, -2.9),
                 (sign * 2.15, 1.5), (sign * .8, 1.8)], -.3, .05)
        m.loft([(sign * 1.75, -3.4, -.15, .24, .28),
                (sign * 1.75, -.1, -.15, .42, .42)], sides=4, part="tool")
        m.loft([(sign * 1.75, -4.1, -.15, .10, .12),
                (sign * 1.75, -3.5, -.15, .44, .44)], sides=8, part="tool")
        engine(m, sign * 1.85, .5, 3.25, .45, .47)
    return m


def ark():
    m = Mesh()
    m.loft([(0, -4.2, .2, .32, .40), (0, -2.9, .2, .78, .70),
            (0, 3.8, .2, .78, .70), (0, 4.5, .2, .55, .58)], sides=4)
    m.loft([(0, -3.5, .85, .32, .13), (0, -2.4, .95, .45, .15)], sides=4, part="canopy")
    for sign in (-1, 1):
        m.prism([(sign * .5, -2.7), (sign * 2.3, -2.3),
                 (sign * 2.3, 3.2), (sign * .5, 3.4)], -.40, -.10)
        for y in (-2.3, -.5, 1.3):
            m.loft([(sign * 1.4, y, .25, .76, .70),
                    (sign * 1.4, y + 1.5, .25, .76, .70)], sides=4, part="cargo")
        engine(m, sign * 1.3, 3.15, 4.5, .46, .48)
    return m


def hermes():
    m = Mesh()
    m.loft([(0, -4.7, .05, .09, .12), (0, -2.7, .05, .48, .35),
            (0, -.2, .05, .85, .50), (0, 1.8, .05, .73, .46),
            (0, 3.4, .05, .45, .34)], sides=8)
    m.loft([(0, -2.7, .48, .23, .12), (0, -1.0, .65, .40, .17)], sides=4, part="canopy")
    for sign in (-1, 1):
        m.prism([(sign * .4, -1.6), (sign * 2.45, 1.7),
                 (sign * 2.1, 2.9), (sign * .5, 1.3)], -.14, .12)
        engine(m, sign * 1.55, .7, 3.6, .32, .34)
        m.prism([(sign * .35, 1.2), (sign * .95, 3.0),
                 (sign * .55, 3.5)], .25, .48, part="fin")
    engine(m, 0, 2.7, 4.2, .33, .36, -.12)
    return m


MODELS = {
    "pilgrim": ("Pilgrim", geometry.pilgrim, "#71d8bb", "#c5ffe9", 28),
    "wanderer": ("Wanderer", wanderer, "#79decb", "#badfff", 26),
    "forge": ("Forge", forge, "#7bd9ad", "#e6c080", 30),
    "ark": ("Ark", ark, "#93d8ae", "#d9e9a8", 28),
    "hermes": ("Hermes", hermes, "#74e7d0", "#cefff6", 25),
}


def render(model_id, settings):
    name, build, color, accent, elevation = settings
    mesh = build()
    edges = mesh.edges()
    important = {"engine", "canopy", "tool", "sensor"}
    body = [edge for edge, faces in edges.items() if faces[0][1] not in important]
    details = [edge for edge, faces in edges.items() if faces[0][1] in important]
    raw_frames = [geometry.raw_projection(mesh.points, -35 + i * 360 / 48, elevation)[0] for i in range(49)]
    # Use one scale throughout the orbit, so the model does not grow/shrink as it turns.
    span_x = max(max(x for x, _ in frame) - min(x for x, _ in frame) for frame in raw_frames)
    span_y = max(max(y for _, y in frame) - min(y for _, y in frame) for frame in raw_frames)
    scale = min(730 / span_x, 405 / span_y)
    frames = [geometry.project(mesh.points, -35 + i * 360 / 48, elevation, fixed_scale=scale)[0] for i in range(49)]
    assert all(math.isfinite(x) and math.isfinite(y) and 20 < x < 880 and 20 < y < 540 for f in frames for x, y in f)
    definitions = []
    for part, lines in (("body", body), ("details", details)):
        paths = [geometry.path_for(lines, frame) for frame in frames]
        assert paths[0] == paths[-1]
        definitions.append(f'<path id="{model_id}-{part}" d="{paths[0]}"><animate attributeName="d" values="{";".join(paths)}" dur="16s" repeatCount="indefinite" calcMode="linear"/></path>')
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="900" height="560" viewBox="0 0 900 560" role="img" fill="none" aria-labelledby="{model_id}-title {model_id}-description">
<title id="{model_id}-title">{name} hologram</title>
<desc id="{model_id}-description">Transparent wireframe spacecraft. Full spatial orbit in 16 seconds, soft green glow and moving light pulses.</desc>
<defs><filter id="{model_id}-glow" x="-25%" y="-25%" width="150%" height="150%"><feGaussianBlur stdDeviation="1.2"/></filter>{"".join(definitions)}</defs>
<g fill="none" stroke-linecap="round" stroke-linejoin="round">
<use href="#{model_id}-body" stroke="{color}" stroke-width="3.4" opacity=".26" filter="url(#{model_id}-glow)"/>
<use href="#{model_id}-body" stroke="{color}" stroke-width="1.2" opacity=".72"/>
<use href="#{model_id}-details" stroke="{color}" stroke-width="3" opacity=".22" filter="url(#{model_id}-glow)"/>
<use href="#{model_id}-details" stroke="{accent}" stroke-width="1.6" opacity=".96"/>
<use href="#{model_id}-body" stroke="#cefff0" stroke-width="1.9" stroke-dasharray="16 190" opacity=".8"><animate attributeName="stroke-dashoffset" from="0" to="-206" dur="5s" repeatCount="indefinite"/></use>
</g></svg>
'''


if __name__ == "__main__":
    for model_id, settings in MODELS.items():
        destination = ROOT / "internal/server/web" / (model_id + ".svg")
        destination.write_text(render(model_id, settings), encoding="utf-8")
        print(destination.name, destination.stat().st_size, "bytes")
