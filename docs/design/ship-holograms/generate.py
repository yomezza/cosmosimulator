"""Generate standalone line-only SVG projections from small 3D meshes.

Python standard library only. Run from any directory; output is next to this file.
Models have transparent backgrounds, no text, scripts or external dependencies.
"""

from collections import defaultdict
from html import escape
from math import cos, pi, radians, sin
from pathlib import Path

ROOT = Path(__file__).resolve().parent
WIDTH, HEIGHT = 900, 560


class Mesh:
    def __init__(self):
        self.points = []
        self.faces = []

    def loft(self, sections, sides=6, part="hull"):
        """Closed polygonal sections: x centre, fore/aft y, z centre, radii."""
        start = len(self.points)
        for x, y, z, width, height in sections:
            for i in range(sides):
                angle = 2 * pi * i / sides + pi / sides
                self.points.append((x + width * cos(angle), y, z + height * sin(angle)))
        faces = [list(range(start, start + sides))]
        for j in range(len(sections) - 1):
            for i in range(sides):
                a = start + j * sides + i
                b = start + j * sides + (i + 1) % sides
                faces.append([a, b, b + sides, a + sides])
        last = start + (len(sections) - 1) * sides
        faces.append(list(range(last, last + sides)))
        self.add_faces(faces, start, part)

    def prism(self, polygon, bottom, top, part="wing"):
        start, count = len(self.points), len(polygon)
        self.points.extend((x, y, bottom) for x, y in polygon)
        self.points.extend((x, y, top) for x, y in polygon)
        faces = [list(range(start, start + count)), list(range(start + count, start + count * 2))]
        for i in range(count):
            a, b = start + i, start + (i + 1) % count
            faces.append([a, b, b + count, a + count])
        self.add_faces(faces, start, part)

    def add_faces(self, faces, start, part):
        centre = tuple(sum(p[i] for p in self.points[start:]) / (len(self.points) - start) for i in range(3))
        for face in faces:
            a, b, c = (self.points[index] for index in face[:3])
            u, v = [b[i] - a[i] for i in range(3)], [c[i] - a[i] for i in range(3)]
            normal = (u[1] * v[2] - u[2] * v[1], u[2] * v[0] - u[0] * v[2], u[0] * v[1] - u[1] * v[0])
            fc = tuple(sum(self.points[index][i] for index in face) / len(face) for i in range(3))
            if sum(normal[i] * (fc[i] - centre[i]) for i in range(3)) < 0:
                normal = tuple(-n for n in normal)
            self.faces.append((face, normal, part))

    def edges(self):
        edges = defaultdict(list)
        for face, normal, part in self.faces:
            for i, a in enumerate(face):
                b = face[(i + 1) % len(face)]
                edges[tuple(sorted((a, b)))].append((normal, part))
        return edges


def pilgrim(dense=False):
    mesh = Mesh()
    sections = [(0, -4.1, .12, .12, .16), (0, -2.6, .12, .65, .43), (0, -1, .12, 1, .55), (0, 1.6, .12, 1.03, .52), (0, 3.1, .12, .73, .40)]
    if dense:
        sections = [sections[0], sections[1], (0, -1.8, .12, .82, .49), sections[2], (0, .1, .12, 1.01, .54), (0, .9, .12, 1.02, .53), sections[3], (0, 2.4, .12, .87, .46), sections[4]]
    mesh.loft(sections, sides=8 if dense else 6)
    for sign in (-1, 1):
        mesh.prism([(sign * .8, -.8), (sign * 2.25, .5), (sign * 2.25, 2.3), (sign * .8, 1.8)], -.12, .12)
        pods = [(sign * 1.9, -.35, 0, .40, .42), (sign * 1.9, 2.8, 0, .40, .42)]
        if dense:
            pods.insert(1, (sign * 1.9, 1.0, 0, .40, .42))
        mesh.loft(pods, sides=8 if dense else 6, part="pods")
        mesh.loft([(sign * 1.9, 2.83, 0, .31, .32), (sign * 1.9, 3.25, 0, .31, .32)], sides=6, part="engine")
    mesh.loft([(0, -2.6, .60, .25, .15), (0, -1.4, .73, .40, .18)], sides=4, part="canopy")
    return mesh


def modular():
    mesh = Mesh()
    mesh.loft([(0, -3.5, .1, .25, .28), (0, -2.4, .1, .72, .62), (0, -.8, .1, .72, .62), (0, 2.9, .1, .62, .52)], sides=4)
    mesh.loft([(0, -2.6, .70, .32, .12), (0, -1.2, .70, .4, .12)], sides=4, part="canopy")
    for sign in (-1, 1):
        mesh.prism([(sign * .4, -.2), (sign * 2.1, -.2), (sign * 2.1, 2.1), (sign * .4, 2.1)], -.22, .1)
        for y in (-.55, .45, 1.45):
            mesh.loft([(sign * 1.25, y, .2, .42, .45), (sign * 1.25, y + .8, .2, .42, .45)], sides=4, part="cargo")
        mesh.loft([(sign * 2.05, -.2, -.06, .30, .36), (sign * 2.05, 3.0, -.06, .30, .36)], sides=6, part="pods")
        mesh.loft([(sign * 2.05, 3.02, -.06, .22, .28), (sign * 2.05, 3.42, -.06, .22, .28)], sides=6, part="engine")
    return mesh


def delta():
    mesh = Mesh()
    mesh.loft([(0, -4.2, 0, .08, .09), (0, -1.3, 0, .62, .36), (0, 2.55, 0, .85, .42)], sides=6)
    for sign in (-1, 1):
        mesh.prism([(sign * .2, -2.0), (sign * 2.7, 2.4), (sign * 1.7, 2.9), (sign * .5, 1.6)], -.17, .07)
        mesh.loft([(sign * 1.55, .1, -.04, .35, .35), (sign * 1.55, 3.05, -.04, .35, .35)], sides=6, part="pods")
        mesh.loft([(sign * 1.55, 3.06, -.04, .25, .25), (sign * 1.55, 3.4, -.04, .25, .25)], sides=6, part="engine")
    mesh.loft([(0, -1.9, .40, .15, .1), (0, -.6, .49, .32, .15)], sides=4, part="canopy")
    return mesh


def raw_projection(points, yaw, elevation):
    yaw, elevation = radians(yaw), radians(elevation)
    xy = [(x * cos(yaw) - y * sin(yaw), (x * sin(yaw) + y * cos(yaw)) * sin(elevation) - z * cos(elevation)) for x, y, z in points]
    view = (sin(yaw) * cos(elevation), cos(yaw) * cos(elevation), sin(elevation))
    return xy, view


def project(points, yaw, elevation, fixed_scale=None):
    xy, view = raw_projection(points, yaw, elevation)
    xs, ys = [p[0] for p in xy], [p[1] for p in xy]
    scale = fixed_scale or min(700 / (max(xs) - min(xs)), 380 / (max(ys) - min(ys)))
    cx, cy = (min(xs) + max(xs)) / 2, (min(ys) + max(ys)) / 2
    return [(WIDTH / 2 + (x - cx) * scale, HEIGHT / 2 + (y - cy) * scale) for x, y in xy], view


def path_for(edges, xy):
    return "".join(f"M{xy[a][0]:.1f} {xy[a][1]:.1f}L{xy[b][0]:.1f} {xy[b][1]:.1f}" for a, b in edges)


def static_groups(mesh, yaw, elevation, color, accent, hidden=True, glow=False, scan=False):
    xy, view = project(mesh.points, yaw, elevation)
    buckets = defaultdict(list)
    for edge, neighbours in mesh.edges().items():
        front = [sum(n[i] * view[i] for i in range(3)) > 0 for n, _ in neighbours]
        part = neighbours[0][1]
        category = "accent" if part in ("canopy", "engine") else "rim" if any(front) and not all(front) else "wire" if any(front) else "hidden"
        buckets[category].append(edge)
    groups = []
    styles = {
        "hidden": (color, .19, .8, ' stroke-dasharray="3 5"'),
        "wire": (color, .60, 1.05, ""),
        "rim": (color, .95, 1.65, ""),
        "accent": (accent, 1, 1.6, ""),
    }
    if glow:
        d = path_for([edge for category in ("wire", "rim", "accent") for edge in buckets[category]], xy)
        groups.append(f'<path d="{d}" stroke="{color}" stroke-width="3" opacity=".28" filter="url(#glow)"/>')
    for category, (stroke, opacity, width, extra) in styles.items():
        if category == "hidden" and not hidden:
            continue
        d = path_for(buckets[category], xy)
        groups.append(f'<path d="{d}" stroke="{stroke}" opacity="{opacity}" stroke-width="{width}"{extra}/>')
    if scan:
        d = path_for(buckets["rim"], xy)
        groups.append(f'<path d="{d}" stroke="#c6fff8" stroke-width="2.2" stroke-dasharray="18 180" opacity=".8"><animate attributeName="stroke-dashoffset" from="0" to="-198" dur="5s" repeatCount="indefinite"/></path>')
    return "\n".join(groups)


def orbit_groups(mesh):
    edges = mesh.edges()
    main = [edge for edge, neighbours in edges.items() if neighbours[0][1] not in ("canopy", "engine")]
    accents = [edge for edge, neighbours in edges.items() if neighbours[0][1] in ("canopy", "engine")]
    # Coordinates are projected from rotated 3D vertices, rather than rotating a flat image.
    frames = [project(mesh.points, -35 + angle * 360 / 48, 28, fixed_scale=96)[0] for angle in range(49)]
    parts = []
    for lines, color, width, opacity in [(main, "#6edce7", 1.15, .68), (accents, "#c2fff7", 1.65, 1)]:
        values = [path_for(lines, xy) for xy in frames]
        parts.append(f'<path d="{values[0]}" stroke="{color}" stroke-width="{width}" opacity="{opacity}"><animate attributeName="d" values="{";".join(values)}" dur="16s" repeatCount="indefinite" calcMode="linear"/></path>')
    return "\n".join(parts)


EXAMPLES = [
    ("01-contour", "Контур", "Лаконичная проекция простого корпуса с минимумом линий.", pilgrim(), -35, 32, "#71d8e8", "#c2f8ff", False, False, False),
    ("02-frame", "Каркас", "Больше поперечных рёбер; дальние грани тонкие и пунктирные.", pilgrim(True), -42, 30, "#63cddd", "#befff5", True, False, False),
    ("03-modular", "Модульный", "Утилитарный корпус с внешними контейнерами и парой двигателей.", modular(), -32, 27, "#73c8cc", "#e8ba75", True, False, False),
    ("04-hologram", "Голограмма", "Мягкое свечение и бегущие по контуру световые импульсы.", pilgrim(), -38, 28, "#62e4d1", "#cefff4", True, True, True),
    ("05-delta", "Дельта", "Угловатый компактный корпус с широкими консолями.", delta(), 36, 25, "#8ac5f4", "#efc68d", True, False, False),
    ("06-orbit", "Облёт", "Медленный полный оборот: корпус виден спереди, сбоку и сзади.", pilgrim(), -35, 28, "#6edce7", "#c2fff7", True, False, False),
]


def make_svg(example, preview=False):
    slug, title, description, mesh, yaw, elevation, color, accent, hidden, glow, scan = example
    definitions = '<defs><filter id="glow" x="-25%" y="-25%" width="150%" height="150%"><feGaussianBlur stdDeviation="1.2"/></filter></defs>' if glow else ""
    body = orbit_groups(mesh) if slug == "06-orbit" else static_groups(mesh, yaw, elevation, color, accent, hidden, glow, scan)
    background = '<rect width="900" height="560" fill="#0c151f"/>\n' if preview else ""
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="900" height="560" viewBox="0 0 900 560" role="img" aria-labelledby="{slug}-title {slug}-description">
<title id="{slug}-title">{escape(title)}</title><desc id="{slug}-description">{escape(description)}</desc>
{definitions}
{background}<g fill="none" stroke-linejoin="round" stroke-linecap="round">{body}</g>
</svg>
'''


def make_gallery():
    cards = []
    for slug, title, description, *_ in EXAMPLES:
        cards.append(f'<article><header><span>{slug[:2]}</span><h2>{title}</h2><a href="{slug}.svg" download>SVG ↓</a></header><div class="model">{make_svg(next(x for x in EXAMPLES if x[0] == slug))}</div><p>{description}</p></article>')
    return '''<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Cosmosimulator · Голографические корабли</title>
<style>*{box-sizing:border-box}body{margin:0;padding:40px;background:#080f17;color:#c6d8e4;font:14px/1.6 system-ui,sans-serif}main{max-width:1400px;margin:auto}h1{font-weight:500;font-size:24px;margin:4px 0}main>p{margin:4px 0 28px;color:#8c9fae}.eyebrow{color:#d9b579;font:11px monospace;letter-spacing:2px}.gallery{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}article{background:#0c151f;border:1px solid #273d4e;overflow:hidden}article header{display:flex;align-items:center;gap:12px;padding:12px 18px;border-bottom:1px solid #213341}header span{color:#ddb77a;font:12px monospace}h2{margin:0;font-weight:500;font-size:15px}a{margin-left:auto;color:#8ac9d7;text-decoration:none;font-size:12px}.model{aspect-ratio:900/560}.model svg{display:block;width:100%;height:100%}article p{margin:0;padding:10px 18px 16px;color:#8c9fae;font-size:12px;min-height:62px}@media(max-width:720px){body{padding:18px}.gallery{grid-template-columns:1fr}}</style>
<main><div class="eyebrow">COSMOSIMULATOR / DESIGN STUDIES</div><h1>Голографические проекции корабля</h1><p>Шесть вариантов для выбора. В моделях — только линии и прозрачный фон. 04 и 06 анимированы.</p><div class="gallery">''' + "".join(cards) + "</div></main></html>\n"


def main():
    (ROOT / "previews").mkdir(exist_ok=True)
    for example in EXAMPLES:
        (ROOT / (example[0] + ".svg")).write_text(make_svg(example), encoding="utf-8")
    for slug in ("04-hologram", "06-orbit"):
        example = next(item for item in EXAMPLES if item[0] == slug)
        (ROOT / "previews" / (slug + ".svg")).write_text(make_svg(example, preview=True), encoding="utf-8")
    (ROOT / "index.html").write_text(make_gallery(), encoding="utf-8")


if __name__ == "__main__":
    main()
