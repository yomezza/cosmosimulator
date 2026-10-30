import { I18n } from "../../core/i18n.js";
const specs = {
  tree_old: [225, 0.5, 0.82],
  tree_young: [160, 0.5, 0.82],
  bush_sparse: [88, 0.5, 0.8],
  grass_tuft: [57, 0.5, 0.87],
  rock_flat: [98, 0.5, 0.7],
  rock_large_cracked: [168, 0.5, 0.78],
  grass_dry: [63, 0.5, 0.78],
  colonist: [104, 0.5, 0.86],
};
// This controller only interpolates server positions. Camera scale never changes
// physical speed, and pointer input sends a selected cell, never a new position.
export async function createController(canvas, scene, w, { current, target }) {
  const images = new Map();
  await Promise.all(
    [
      ...new Set(
        scene.objects.map((o) => o.asset.split("/").pop().replace(".png", "")),
      ),
      "colonist",
    ].map(async (name) => {
      if (!specs[name]) return;
      const img = new Image();
      img.src = "/planet-art/" + name + ".png";
      try {
        await img.decode();
        images.set(name, img);
      } catch {}
    }),
  );
  if (!current()) return null;
  const terrain = document.createElement("canvas");
  terrain.width = 1024;
  terrain.height = 1024;
  const ground = terrain.getContext("2d"),
    cells = Uint8Array.from(atob(scene.cells), (c) => c.charCodeAt(0));
  for (let y = 0; y < 256; y++)
    for (let x = 0; x < 256; x++) {
      const f = cells[y * 256 + x];
      if (!(f & 1)) continue;
      ground.fillStyle = f & 2 ? "#24586c" : "#3b5041";
      ground.fillRect(x * 4, y * 4, 4, 4);
    }
  const ctx = canvas.getContext("2d"),
    objects = scene.objects.map((o) => ({
      name: o.asset.split("/").pop().replace(".png", ""),
      x: o.position.X / 1024,
      y: o.position.Y / 1024,
    }));
  let zoom = 16,
    width = 1,
    height = 1,
    ratio = 1,
    stopped = false,
    raf,
    follow = true,
    drag,
    route = [],
    goal = null;
  let latest = {
      x: w.x_q / 1024,
      y: w.y_q / 1024,
      time: Date.now(),
      received: performance.now(),
    },
    previous = latest,
    cam = { x: latest.x, y: latest.y },
    offset = { x: 0, y: 0 };
  const resize = () => {
    const r = canvas.getBoundingClientRect();
    width = Math.max(1, r.width);
    height = Math.max(1, r.height);
    ratio = Math.min(2, devicePixelRatio || 1);
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
  };
  const observer = new ResizeObserver(resize);
  observer.observe(canvas);
  resize();
  const world = (e) => {
    const r = canvas.getBoundingClientRect();
    return {
      x: cam.x + (e.clientX - r.left - width / 2) / zoom,
      y: cam.y + (e.clientY - r.top - height / 2) / zoom,
    };
  };
  function setZoom(value, anchor) {
    const next = Math.max(0.25, Math.min(32, value));
    if (anchor) {
      const before = world(anchor),
        r = canvas.getBoundingClientRect();
      cam = {
        x: before.x - (anchor.clientX - r.left - width / 2) / next,
        y: before.y - (anchor.clientY - r.top - height / 2) / next,
      };
      offset = { x: cam.x - latest.x, y: cam.y - latest.y };
    }
    zoom = next;
  }
  const wheel = (e) => {
    e.preventDefault();
    setZoom(zoom * Math.exp(-e.deltaY * 0.0015), e);
  };
  const down = (e) => {
    if (e.button === 0) {
      const p = world(e);
      goal = { x: Math.floor(p.x) + 0.5, y: Math.floor(p.y) + 0.5 };
      target(Math.floor(p.x) * 1024 + 512, Math.floor(p.y) * 1024 + 512);
    } else if (e.button === 2 || e.button === 1) {
      e.preventDefault();
      drag = { x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y };
      follow = false;
      canvas.setPointerCapture(e.pointerId);
    }
  };
  const move = (e) => {
    if (drag)
      cam = {
        x: drag.cx - (e.clientX - drag.x) / zoom,
        y: drag.cy - (e.clientY - drag.y) / zoom,
      };
  };
  const up = () => {
    drag = null;
  };
  const context = (e) => e.preventDefault();
  canvas.addEventListener("wheel", wheel, { passive: false });
  canvas.addEventListener("pointerdown", down);
  canvas.addEventListener("pointermove", move);
  canvas.addEventListener("pointerup", up);
  canvas.addEventListener("pointercancel", up);
  canvas.addEventListener("contextmenu", context);
  canvas.setAttribute(
    "aria-label",
    I18n.t("Поверхность сектора: ЛКМ — идти, колесо — масштаб, ПКМ — обзор"),
  );
  function draw(now) {
    if (stopped || !current()) return;
    const expected = latest.time + Math.max(0, now - latest.received) - 100;
    const span = latest.time - previous.time;
    const t =
      span > 0
        ? Math.max(0, Math.min(1, (expected - previous.time) / span))
        : 1;
    const player = {
      x: previous.x + (latest.x - previous.x) * t,
      y: previous.y + (latest.y - previous.y) * t,
    };
    if (follow && !drag) {
      cam.x += (player.x + offset.x - cam.x) * 0.2;
      cam.y += (player.y + offset.y - cam.y) * 0.2;
    }
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);
    ctx.translate(width / 2 - cam.x * zoom, height / 2 - cam.y * zoom);
    ctx.scale(zoom, zoom);
    ctx.drawImage(terrain, 0, 0, 256, 256);
    if (route.length) {
      ctx.strokeStyle = "#d9b970";
      ctx.lineWidth = 1.2 / zoom;
      ctx.setLineDash([4 / zoom, 4 / zoom]);
      ctx.beginPath();
      ctx.moveTo(player.x, player.y);
      for (const p of route) ctx.lineTo(p.x, p.y);
      ctx.stroke();
      ctx.setLineDash([]);
    }
    const visible = objects.filter(
      (o) =>
        Math.abs(o.x - cam.x) < width / zoom / 2 + 20 &&
        Math.abs(o.y - cam.y) < height / zoom / 2 + 20,
    );
    visible.push({ name: "colonist", ...player });
    visible.sort((a, b) => a.y - b.y);
    for (const o of visible) {
      const img = images.get(o.name),
        s = specs[o.name];
      if (!img || !s) continue;
      const size = s[0] / 16;
      ctx.drawImage(img, o.x - size * s[1], o.y - size * s[2], size, size);
    }
    if (scene.landing) {
      ctx.strokeStyle = "#80f4d6";
      ctx.lineWidth = 0.8 / zoom;
      ctx.beginPath();
      ctx.ellipse(
        scene.landing.X / 1024,
        scene.landing.Y / 1024,
        117 / 16,
        66 / 16,
        0,
        0,
        Math.PI * 2,
      );
      ctx.stroke();
    }
    ctx.strokeStyle = "#a4f4dc";
    ctx.lineWidth = 1.4 / zoom;
    ctx.beginPath();
    ctx.arc(player.x, player.y, 0.8, 0, Math.PI * 2);
    ctx.stroke();
    if (goal) {
      ctx.strokeStyle = "#f0ce86";
      ctx.beginPath();
      ctx.arc(goal.x, goal.y, 0.6, 0, Math.PI * 2);
      ctx.stroke();
    }
    canvas.dataset.ready = "true";
    canvas.dataset.xq = String(Math.round(player.x * 1024));
    canvas.dataset.yq = String(Math.round(player.y * 1024));
    canvas.dataset.zoom = String(zoom);
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    raf = requestAnimationFrame(draw);
  }
  raf = requestAnimationFrame(draw);
  return {
    update(state) {
      canvas.dataset.moving = String(state.moving);
      if (
        state.server_time < latest.time &&
        latest.received !== previous.received
      )
        return;
      previous = latest;
      latest = {
        x: state.x_q / 1024,
        y: state.y_q / 1024,
        time: state.server_time,
        received: performance.now(),
      };
      if (Number.isInteger(state.remaining))
        route = state.remaining ? route.slice(-state.remaining) : [];
    },
    accepted(state) {
      route = state.path || [];
      if (route.length) goal = route.at(-1);
    },
    zoom(factor) {
      setZoom(zoom * factor);
    },
    centre() {
      follow = true;
      offset = { x: 0, y: 0 };
      cam = { x: latest.x, y: latest.y };
    },
    fit() {
      follow = false;
      zoom = Math.max(0.25, Math.min(width, height) / 272);
      cam = { x: 128, y: 128 };
    },
    stop() {
      if (stopped) return;
      stopped = true;
      cancelAnimationFrame(raf);
      observer.disconnect();
      canvas.removeEventListener("wheel", wheel);
      canvas.removeEventListener("pointerdown", down);
      canvas.removeEventListener("pointermove", move);
      canvas.removeEventListener("pointerup", up);
      canvas.removeEventListener("pointercancel", up);
      canvas.removeEventListener("contextmenu", context);
      images.clear();
      terrain.width = terrain.height = 0;
    },
  };
}
