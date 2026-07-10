import Phaser from "phaser";
import { BattleScene } from "./BattleScene";
import { RegionScene } from "./RegionScene";
import { WorldScene } from "./WorldScene";

export function createPhaserGame(container: HTMLElement, config?: { width?: number; height?: number; seed?: number }): Phaser.Game {
  const gameConfig: Phaser.Types.Core.GameConfig = {
    type: Phaser.AUTO,
    parent: container,
    width: config?.width || container.clientWidth,
    height: config?.height || container.clientHeight,
    backgroundColor: "#0a0a1a",
    scene: [WorldScene, RegionScene, BattleScene],
    scale: {
      mode: Phaser.Scale.RESIZE,
      autoCenter: Phaser.Scale.CENTER_BOTH,
    },
    render: {
      antialias: true,
      pixelArt: false,
    },
    input: {
      activePointers: 3,
    },
  };

  const game = new Phaser.Game(gameConfig);

  game.scene.start("WorldScene", { seed: config?.seed || 12345 });

  return game;
}

export function destroyPhaserGame(game: Phaser.Game): void {
  if (game) {
    game.destroy(true);
  }
}
