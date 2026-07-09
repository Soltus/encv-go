import { ref, onMounted, onUnmounted, shallowRef } from "vue";
import Phaser from "phaser";
import { createPhaserGame, destroyPhaserGame } from "@/game/main";
import { phaserEventBus, PHASER_EVENTS } from "@/game/PhaserEventBus";
import { WorldScene } from "@/game/WorldScene";
import type { SimverseNPC } from "@/composables/useSimverse";
import { useWorldRenderSettings, QUALITY_RESOLUTION, type RenderQuality } from "@/composables/useWorldRenderSettings";

export interface RegionEnterData {
  regionId: string;
  regionName: string;
  regionType: string;
}

export interface BattleStartData {
  enemyName: string;
  enemyLevel: number;
  enemyEmoji: string;
  playerEmoji?: string;
  playerName?: string;
  playerLevel?: number;
}

export function usePhaserWorld() {
  const gameContainer = ref<HTMLElement | null>(null);
  const game = shallowRef<Phaser.Game | null>(null);
  const isReady = ref(false);
  const hasError = ref(false);
  const errorMessage = ref("");
  const currentZoom = ref(1);
  const selectedNPC = ref<SimverseNPC | null>(null);
  const currentScene = ref("WorldScene");
  const renderSettings = useWorldRenderSettings();

  const npcClickHandlers: ((npc: SimverseNPC) => void)[] = [];
  const regionEnterHandlers: ((data: RegionEnterData) => void)[] = [];
  const battleStartHandlers: ((data: BattleStartData) => void)[] = [];
  const battleEndHandlers: ((result: string) => void)[] = [];

  function onNPCClick(handler: (npc: SimverseNPC) => void) {
    npcClickHandlers.push(handler);
  }

  function onRegionEnter(handler: (data: RegionEnterData) => void) {
    regionEnterHandlers.push(handler);
  }

  function onBattleStart(handler: (data: BattleStartData) => void) {
    battleStartHandlers.push(handler);
  }

  function onBattleEnd(handler: (result: string) => void) {
    battleEndHandlers.push(handler);
  }

  function handleNPCClick(npc: SimverseNPC) {
    selectedNPC.value = npc;
    npcClickHandlers.forEach((h) => h(npc));
  }

  // 将 WorldSettings 的帧率/等效渲染等级应用到运行中的 Phaser 游戏
  function resolutionScale(q: RenderQuality): number {
    const base = 1080;
    return QUALITY_RESOLUTION[q].height / base;
  }

  function applyRenderSettings(): void {
    if (!game.value || !isReady.value) return;
    try {
      const fps = renderSettings.fps.value;
      const quality = renderSettings.quality.value;
      const loop = game.value.loop as any;
      loop.targetFps = fps;
      // 低于 60 时强制 setTimeout 限速以省电；60/90/120 走 RAF（显示器刷新率）
      loop.forceSetTimeOut = fps < 60;
      const renderer = game.value.renderer as any;
      if (renderer && typeof renderer.setResolution === "function") {
        renderer.setResolution(resolutionScale(quality));
      }
    } catch (e) {
      console.warn("[Phaser] applyRenderSettings failed:", e);
    }
  }

  function onRenderSettingsEvent(): void {
    applyRenderSettings();
  }

  function handleRegionEnter(data: RegionEnterData) {
    regionEnterHandlers.forEach((h) => h(data));
  }

  function handleBattleStart(data: BattleStartData) {
    battleStartHandlers.forEach((h) => h(data));
  }

  function handleBattleEnd(result: string) {
    battleEndHandlers.forEach((h) => h(result));
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
        currentScene.value = "WorldScene";
        applyRenderSettings();
      });

      window.addEventListener("simverse:render-settings", onRenderSettingsEvent);

      phaserEventBus.on(PHASER_EVENTS.REGION_READY, (data: any) => {
        currentScene.value = "RegionScene";
        handleRegionEnter(data);
      });

      phaserEventBus.on(PHASER_EVENTS.BATTLE_START, (data: any) => {
        currentScene.value = "BattleScene";
        handleBattleStart(data);
      });

      phaserEventBus.on(PHASER_EVENTS.BATTLE_END, (data: any) => {
        handleBattleEnd(data?.result || "end");
      });

      phaserEventBus.on(PHASER_EVENTS.BACK_TO_WORLD, () => {
        currentScene.value = "WorldScene";
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

  function enterRegion(data: RegionEnterData) {
    if (!game.value) return;
    game.value.scene.start("RegionScene", {
      ...data,
      backTo: "WorldScene",
    });
  }

  function startBattle(data: BattleStartData) {
    if (!game.value) return;
    game.value.scene.start("BattleScene", {
      ...data,
      backTo: "WorldScene",
    });
  }

  function goBackToWorld() {
    if (!game.value) return;
    game.value.scene.start("WorldScene");
    currentScene.value = "WorldScene";
  }

  function setNPCs(npcs: SimverseNPC[]) {
    if (!game.value || !isReady.value) return;

    const scene = game.value.scene.getScene("WorldScene") as WorldScene;
    if (scene) {
      scene.setNPCs(npcs);
    }
  }

  function setNPCBehaviors(behaviors: Map<number, string>) {
    if (!game.value || !isReady.value) return;

    const scene = game.value.scene.getScene("WorldScene") as WorldScene;
    if (scene) {
      scene.setNPCBehaviors(behaviors);
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
    currentScene.value = "WorldScene";
    phaserEventBus.clear();
    window.removeEventListener("simverse:render-settings", onRenderSettingsEvent);
    npcClickHandlers.length = 0;
    regionEnterHandlers.length = 0;
    battleStartHandlers.length = 0;
    battleEndHandlers.length = 0;
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
    currentScene,

    setGameContainer,
    initPhaser,
    setNPCs,
    setNPCBehaviors,
    centerOnNPC,
    setZoom,
    getZoom,
    enterRegion,
    startBattle,
    goBackToWorld,
    onNPCClick,
    onRegionEnter,
    onBattleStart,
    onBattleEnd,
    destroy,
  };
}
