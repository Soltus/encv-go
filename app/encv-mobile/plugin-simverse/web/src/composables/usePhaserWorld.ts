import { ref, onMounted, onUnmounted, shallowRef } from "vue";
import Phaser from "phaser";
import { createPhaserGame, destroyPhaserGame } from "@/game/main";
import { phaserEventBus, PHASER_EVENTS } from "@/game/PhaserEventBus";
import { WorldScene } from "@/game/WorldScene";
import type { SimverseNPC } from "@/composables/useSimverse";

export function usePhaserWorld() {
  const gameContainer = ref<HTMLElement | null>(null);
  const game = shallowRef<Phaser.Game | null>(null);
  const isReady = ref(false);
  const hasError = ref(false);
  const errorMessage = ref("");
  const currentZoom = ref(1);
  const selectedNPC = ref<SimverseNPC | null>(null);

  const npcClickHandlers: ((npc: SimverseNPC) => void)[] = [];

  function onNPCClick(handler: (npc: SimverseNPC) => void) {
    npcClickHandlers.push(handler);
  }

  function handleNPCClick(npc: SimverseNPC) {
    selectedNPC.value = npc;
    npcClickHandlers.forEach((h) => h(npc));
  }

  function setGameContainer(el: HTMLElement) {
    gameContainer.value = el;
  }

  async function initPhaser(seed?: number): Promise<boolean> {
    if (!gameContainer.value) {
      hasError.value = true;
      errorMessage.value = "Game container not found";
      return false;
    }

    try {
      const phaserGame = createPhaserGame(gameContainer.value, { seed });
      game.value = phaserGame;

      phaserEventBus.on(PHASER_EVENTS.WORLD_READY, () => {
        isReady.value = true;
      });

      phaserEventBus.on(PHASER_EVENTS.NPC_CLICK, (npc: SimverseNPC) => {
        handleNPCClick(npc);
      });

      phaserEventBus.on(PHASER_EVENTS.CAMERA_ZOOM, (zoom: number) => {
        currentZoom.value = zoom;
      });

      return true;
    } catch (e: any) {
      console.error("[Phaser] Initialization failed:", e);
      hasError.value = true;
      errorMessage.value = e.message || "Phaser initialization failed";
      return false;
    }
  }

  function setNPCs(npcs: SimverseNPC[]) {
    if (!game.value || !isReady.value) return;

    const scene = game.value.scene.getScene("WorldScene") as WorldScene;
    if (scene) {
      scene.setNPCs(npcs);
    }
  }

  function centerOnNPC(npcId: number) {
    if (!game.value || !isReady.value) return;

    const scene = game.value.scene.getScene("WorldScene") as WorldScene;
    if (scene) {
      scene.centerOnNPC(npcId);
    }
  }

  function setZoom(zoom: number) {
    if (!game.value || !isReady.value) return;

    const scene = game.value.scene.getScene("WorldScene") as WorldScene;
    if (scene) {
      scene.setZoom(zoom);
    }
  }

  function getZoom(): number {
    return currentZoom.value;
  }

  function destroy() {
    if (game.value) {
      destroyPhaserGame(game.value);
      game.value = null;
    }
    isReady.value = false;
    hasError.value = false;
    phaserEventBus.clear();
    npcClickHandlers.length = 0;
  }

  onUnmounted(() => {
    destroy();
  });

  return {
    gameContainer,
    game,
    isReady,
    hasError,
    errorMessage,
    currentZoom,
    selectedNPC,

    setGameContainer,
    initPhaser,
    setNPCs,
    centerOnNPC,
    setZoom,
    getZoom,
    onNPCClick,
    destroy,
  };
}
