// Hand-authored public renderer input. No second geography generator or gameplay.
export function localArtScene({ wet = true, origin = { x: 1, y: 1 } } = {}) {
  const cells = Buffer.alloc(256 * 256, 1),
    heights = Buffer.alloc(256 * 256 * 2);
  for (let y = 0; y < 256; y++)
    for (let x = 0; x < 256; x++) {
      const water = wet && (x < 70 || (x > 170 && x < 176));
      cells[y * 256 + x] = water ? 3 : 1;
      heights.writeUInt16LE(water ? 350 : 650, (y * 256 + x) * 2);
    }
  return {
    id: "public-art-fixture",
    size: 256,
    cells: cells.toString("base64"),
    heights: heights.toString("base64"),
    origin,
    biome: "Леса и равнины",
    materials: [
      { x: 2, y: 2, biome: "Леса и равнины" },
      { x: 3.732, y: 2, biome: "Сухие равнины" },
    ],
    landing: { X: 98 * 1024, Y: 120 * 1024 },
    exits: [],
    objects: [
      {
        id: "tree",
        asset: "nature/tree_old.png",
        position: { X: 115 * 1024, Y: 140 * 1024 },
      },
      {
        id: "tree2",
        asset: "nature/tree_young.png",
        position: { X: 83 * 1024, Y: 118 * 1024 },
      },
      {
        id: "rock",
        asset: "nature/rock_flat.png",
        position: { X: 110 * 1024, Y: 122 * 1024 },
      },
      {
        id: "bush",
        asset: "nature/bush_sparse.png",
        position: { X: 92 * 1024, Y: 143 * 1024 },
      },
    ],
  };
}
