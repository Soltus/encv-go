export class NoiseGenerator {
  private perm: number[] = [];

  constructor(seed: number = Math.random() * 10000) {
    this.perm = this.generatePermutation(seed);
  }

  private generatePermutation(seed: number): number[] {
    const p: number[] = [];
    for (let i = 0; i < 256; i++) {
      p[i] = i;
    }

    let n = seed;
    for (let i = 255; i > 0; i--) {
      n = (n * 16807) % 2147483647;
      const j = n % (i + 1);
      [p[i], p[j]] = [p[j], p[i]];
    }

    return [...p, ...p];
  }

  private fade(t: number): number {
    return t * t * t * (t * (t * 6 - 15) + 10);
  }

  private lerp(a: number, b: number, t: number): number {
    return a + t * (b - a);
  }

  private grad(hash: number, x: number, y: number): number {
    const h = hash & 3;
    const u = h < 2 ? x : y;
    const v = h < 2 ? y : x;
    return ((h & 1) === 0 ? u : -u) + ((h & 2) === 0 ? v : -v);
  }

  noise2D(x: number, y: number): number {
    const X = Math.floor(x) & 255;
    const Y = Math.floor(y) & 255;

    x -= Math.floor(x);
    y -= Math.floor(y);

    const u = this.fade(x);
    const v = this.fade(y);

    const A = this.perm[X] + Y;
    const B = this.perm[X + 1] + Y;

    return this.lerp(
      this.lerp(this.grad(this.perm[A], x, y), this.grad(this.perm[B], x - 1, y), u),
      this.lerp(this.grad(this.perm[A + 1], x, y - 1), this.grad(this.perm[B + 1], x - 1, y - 1), u),
      v
    );
  }

  fbm(x: number, y: number, octaves = 4, persistence = 0.5, lacunarity = 2): number {
    let value = 0;
    let amplitude = 1;
    let frequency = 1;
    let maxValue = 0;

    for (let i = 0; i < octaves; i++) {
      value += this.noise2D(x * frequency, y * frequency) * amplitude;
      maxValue += amplitude;
      amplitude *= persistence;
      frequency *= lacunarity;
    }

    return value / maxValue;
  }
}

export enum TerrainType {
  DEEP_WATER = "deep_water",
  SHALLOW_WATER = "shallow_water",
  BEACH = "beach",
  PLAIN = "plain",
  FOREST = "forest",
  HILLS = "hills",
  MOUNTAIN = "mountain",
  SNOW = "snow",
  DESERT = "desert",
}

// 地形配色：与 encv-mobile 紫色主题协调。
// 水体偏蓝紫，平原暖米色，森林略去饱和的绿，山地冷灰带紫调，沙漠暖橙。
export const TERRAIN_COLORS: Record<TerrainType, number> = {
  [TerrainType.DEEP_WATER]: 0x2a2a5a, // 深水：蓝紫底
  [TerrainType.SHALLOW_WATER]: 0x3a3a7a, // 浅水：蓝紫
  [TerrainType.BEACH]: 0xd4c4a0, // 沙滩：暖沙
  [TerrainType.PLAIN]: 0xb8a878, // 平原：暖米色
  [TerrainType.FOREST]: 0x3a5a3a, // 森林：略去饱和的绿
  [TerrainType.HILLS]: 0x6b7a6b, // 丘陵：森林→山地过渡
  [TerrainType.MOUNTAIN]: 0x6b6b85, // 山地：冷灰带紫调
  [TerrainType.SNOW]: 0xe8e8f0, // 雪山：偏冷的近白
  [TerrainType.DESERT]: 0xd4a06e, // 沙漠：暖橙
};

export class TerrainGenerator {
  private noise: NoiseGenerator;
  private moistureNoise: NoiseGenerator;

  constructor(seed: number = 12345) {
    this.noise = new NoiseGenerator(seed);
    this.moistureNoise = new NoiseGenerator(seed + 9999);
  }

  getHeight(x: number, y: number, scale = 0.01): number {
    return this.noise.fbm(x * scale, y * scale, 5, 0.5, 2);
  }

  getMoisture(x: number, y: number, scale = 0.008): number {
    return this.moistureNoise.fbm(x * scale, y * scale, 4, 0.6, 2);
  }

  getTerrainType(x: number, y: number): TerrainType {
    const height = this.getHeight(x, y);
    const moisture = this.getMoisture(x, y);

    if (height < -0.3) return TerrainType.DEEP_WATER;
    if (height < -0.1) return TerrainType.SHALLOW_WATER;
    if (height < -0.05) return TerrainType.BEACH;
    if (height > 0.6) return TerrainType.SNOW;
    if (height > 0.4) return TerrainType.MOUNTAIN;
    if (height > 0.25) return TerrainType.HILLS;

    if (moisture < -0.3) return TerrainType.DESERT;
    if (moisture > 0.2) return TerrainType.FOREST;
    return TerrainType.PLAIN;
  }

  isWalkable(x: number, y: number): boolean {
    const terrain = this.getTerrainType(x, y);
    return terrain !== TerrainType.DEEP_WATER && terrain !== TerrainType.SHALLOW_WATER;
  }

  generateTileMap(width: number, height: number): TerrainType[][] {
    const map: TerrainType[][] = [];
    for (let y = 0; y < height; y++) {
      map[y] = [];
      for (let x = 0; x < width; x++) {
        map[y][x] = this.getTerrainType(x, y);
      }
    }
    return map;
  }

  findSettlementLocations(mapWidth: number, mapHeight: number, count: number): { x: number; y: number; size: number }[] {
    const locations: { x: number; y: number; size: number }[] = [];
    const minDist = Math.min(mapWidth, mapHeight) / Math.sqrt(count);

    let attempts = 0;
    const maxAttempts = count * 50;

    while (locations.length < count && attempts < maxAttempts) {
      attempts++;
      const x = Math.floor(Math.random() * (mapWidth - 10)) + 5;
      const y = Math.floor(Math.random() * (mapHeight - 10)) + 5;

      if (!this.isWalkable(x, y)) continue;

      let tooClose = false;
      for (const loc of locations) {
        const dist = Math.sqrt((loc.x - x) ** 2 + (loc.y - y) ** 2);
        if (dist < minDist) {
          tooClose = true;
          break;
        }
      }

      if (tooClose) continue;

      const height = this.getHeight(x, y);
      const moisture = this.getMoisture(x, y);
      let size = 1;
      if (height > -0.05 && height < 0.2 && moisture > 0) size = 2;
      if (height > -0.05 && height < 0.15 && moisture > 0.1) size = 3;

      locations.push({ x, y, size });
    }

    return locations;
  }
}
