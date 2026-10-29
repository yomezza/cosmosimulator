// Synthetic API DTOs, not saved production data or an alternative game engine.
export function fixture() {
  const spec = {
    id: "pilgrim", name: "Пилигрим", model: "pilgrim.svg", role: "Универсальный",
    length: 24, cargo: 60, fuel: 40, jump_seconds: 5, jump_fuel: 2, mining: 3,
    description: "Test starter vessel",
  };
  const ships = [101, 102].map((id, i) => ({
    id, name: `Test vessel ${i + 1}`, star: 1, object_id: "port-1",
    position_mode: "docked", fuel: 30, cargo: { iron: 6, agni: 2 },
    spec: { ...spec }, mining: { current: 5, min: 5, max: 5 }, extractor: "universal",
    reserved_fuel: 0, destination: 0, local_target: "", route: [],
  }));
  const stars = [1, 2, 3].map((id) => ({
    id, name: ["Test Sol", "Test Vega", "Test Altair"][id - 1],
    x: id * 100, y: id * 20, region: 1, galaxy: "edem", class: "G", mass: 1, luminosity: 1,
  }));
  const station = { star: 1, object_id: "port-1", name: "Test Port", specialization: "Материалы" };
  const sites = [
    { id: "port-1", name: "Test Port", kind: "Орбитальная станция", orbit: 2, resources: [] },
    { id: "belt-1", name: "Test Belt", kind: "Пояс астероидов", orbit: 4, resources: ["iron"] },
  ];
  return {
    state: { user: { id: 51, username: "test_pilot", role: "player" }, date: "ЭД 001.001", tick: 1, tick_seconds: 1000 },
    profile: { level: 6, xp: 25, required_xp: 100, upgrade_cost: 100, company: "", company_id: 0 },
    expedition: {
      credits: 1000000, ship: ships[0], system: stars[0], region: "Test Region", sites,
      resources: [
        { id: "iron", name: "Железо", code: "FE", kind: "Руды", description: "Test iron", sources: "Test belt", rarity: "Обычный", uses: ["Промышленность"] },
        { id: "agni", name: "Водород", code: "H", kind: "Газы", description: "Test hydrogen", sources: "Test ice", rarity: "Обычный", uses: ["Топливо"] },
      ],
      inventory: {}, docked: true, capacity: 60, explored: true,
    },
    fleet: { ships, ship_limit: 3 },
    gameplay: { server_time: new Date().toISOString(), tick_seconds: 1000 },
    workbench: {
      stations: [station], notes: [], bookmarks: [], ship_classes: [spec],
      known: [{ star: 1, source: "survey", received_at: "2026-01-01T00:00:00Z" }],
      research: [], population: [{ players: 1, bots: 2 }], server_name: "Иггдрасиль",
      universe_revision: 1,
    },
    notifications: { count: 0, items: [] },
    galaxy: {
      name: "Edem", seed: 42, revision: 1, stars,
      lanes: [{ from: 1, to: 2 }, { from: 2, to: 3 }],
      regions: [{ id: 1, name: "Test Region" }],
      galaxies: [{ id: "edem", name: "Edem", style: "spiral", opened: true }],
    },
    system: { star: stars[0], region: "Test Region", surveyed: true, sites },
    planner: [
      { id: 1, ship_id: 101, kind: "mine", resource: "iron", status: "running", priority: 2, progress: 1, quantity: 30 },
      { id: 2, ship_id: 101, kind: "mine", resource: "iron", status: "queued", priority: 1, progress: 0, quantity: 30 },
      { id: 3, ship_id: 102, kind: "mine", resource: "iron", status: "paused", priority: 1, progress: 0, quantity: 30 },
      { id: 4, ship_id: 102, kind: "mine", resource: "iron", status: "completed", priority: 0, progress: 30, quantity: 30 },
    ],
    users: { page: 1, pages: 1, total: 1, items: [{ id: 71, username: "test_other", credits: 100, role: "player", level: 1 }] },
  };
}
