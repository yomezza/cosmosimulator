// Small canonical geography for renderer tests; not a second planet generator.
export function planetFixture(kind = "Садовый мир") {
  const heights = Array.from({ length: 81 }, (_, i) => (i < 27 ? 300 : 700));
  return {
    id: 11,
    star: 1,
    planet: 1,
    revision: 1,
    name: "Test surface",
    kind,
    geography: {
      sea_level: 460,
      width: 4,
      height: 4,
      columns: 2,
      rows: 1,
      grid_w: 9,
      grid_h: 9,
      heights,
      water: heights.map((h) => (h < 460 ? 1 : 0)),
      rivers: [
        [
          { x: 1, y: 1 },
          { x: 2, y: 3 },
        ],
      ],
      temperature: 10,
      area: 100000,
      size: "Маленькая",
      sectors: [1, 2].map((number) => ({
        number,
        column: number - 1,
        row: 0,
        center: { x: number, y: 2 },
        neighbors: [number === 1 ? 2 : 1],
        land: 70,
        type: "Прибрежный сектор",
        biome: number === 1 ? "Равнины" : "Холодные равнины",
        area: 50000,
        estimates: { iron: "Умеренные залежи" },
      })),
    },
  };
}
export const planetState = () => ({
  claims: [],
  habitable: true,
  level: 30,
  company: 0,
  can_corporate: false,
  personal_count: 0,
  settings: { min_land: 50, min_level: 6, personal_limit: 5 },
});
