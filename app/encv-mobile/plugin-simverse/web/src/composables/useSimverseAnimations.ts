/**
 * SimverseWorld GSAP 动画 composable（Task 1.3 / 1.4 / 1.5）.
 *
 * 集中托管 SimverseWorld.vue 的全部 GSAP 动效逻辑，避免主组件超过 2000 行门禁：
 * - 替换 CSS @keyframes（bgDrift / valuePop / runPulse / pulse / spin / float）
 * - 替换 Vue <transition> 包裹（bottom-bar / more-pop / gacha-flash / ticker）
 * - 新增游戏级动画（光柱 / 屏幕震动 / 飘字 / 粒子）
 *
 * 调用方在 setup 中传入所需 reactive refs，composable 自动注册 watcher；
 * onMounted 时调用 `startAmbientLoops()`，onUnmounted 时调用 `stopAll()`。
 */
import { nextTick, watch, type Ref } from "vue";
import { gsap, MotionPathPlugin } from "./useGsap";

// MotionPathPlugin 已在 useGsap 内部注册，此处仅引用以标注"已使用"避免 tree-shake 移除
void MotionPathPlugin;

/** composable 入参：主组件持有的 reactive refs（由调用方在 setup 中传入）。 */
export interface SimverseAnimationRefs {
  /** 底部主操作条 DOM 引用（Task 1.4 bottom-bar 转场目标）。 */
  bottomBarRef: Ref<HTMLElement | null>;
  /** 更多面板 DOM 引用（Task 1.4 more-pop 转场目标）。 */
  morePopRef: Ref<HTMLElement | null>;
  /** 抽卡动画 overlay DOM 引用（Task 1.4 gacha-flash 转场目标）。 */
  gachaFlashRef: Ref<HTMLElement | null>;
  /** Phaser 是否加载中（Task 1.3 spin 启停）。 */
  phaserLoading: Ref<boolean>;
  /** 是否启用 Phaser（Task 1.3 spin 启停前置条件）。 */
  usePhaser: Ref<boolean>;
  /** Phaser 是否出错（Task 1.3 spin 启停前置条件）。 */
  phaserHasError: Ref<boolean>;
  /** 世界状态（Task 1.3 runPulse 监听 running 字段）。 */
  worldState: Ref<{ running?: boolean } | null>;
  /** 屏幕状态机（Task 1.3 float / Task 1.4 bottom-bar 监听）。 */
  screen: Ref<string>;
  /** 更多面板是否展开（Task 1.4 more-pop 转场）。 */
  bottomMoreOpen: Ref<boolean>;
  /** 抽卡是否动画中（Task 1.4 gacha-flash 转场）。 */
  isGachaAnimating: Ref<boolean>;
  /** 近期事件列表（Task 1.4 ticker 入场）。 */
  recentEvents: Ref<readonly unknown[]>;
  /** 玩家钻石（Task 1.3 valuePop + Task 1.5 飘字）。 */
  playerDiamond: Ref<number>;
  /** 玩家金币（Task 1.3 valuePop + Task 1.5 飘字）。 */
  playerGold: Ref<number>;
  /** 玩家体力（Task 1.3 valuePop）。 */
  playerStamina: Ref<number>;
  /** 当前激活的侧边面板（Task 1.3 面板 spinner 启停）。 */
  activePanel: Ref<string | null>;
  /** 经济面板 tab（Task 1.3 面板 spinner 启停）。 */
  econTab: Ref<string>;
  /** 经济统计数据（Task 1.3 面板 spinner 启停）。 */
  economyStats: Ref<unknown>;
}

/** composable 返回值：游戏级动画工具函数 + 生命周期钩子。 */
export interface SimverseAnimationApi {
  /** 抽卡光柱（MotionPathPlugin 驱动）。在抽卡揭晓时调用。 */
  playGachaLightBeam: () => void;
  /** 屏幕震动。在抽卡揭晓、重大事件等场景调用。 */
  shakeScreen: (intensity?: number, duration?: number) => void;
  /** 资源变化飘字。在资源数值变化时显示 "+N"。 */
  showComboNumber: (target: HTMLElement, value: string, color?: string) => void;
  /** 粒子效果。在抽卡/升级等场景从指定容器中心向外扩散。 */
  spawnParticles: (container: HTMLElement, count?: number, color?: string) => void;
  /** 常驻循环动画启动（bgDrift + npcPulse）。在 onMounted 中调用。 */
  startAmbientLoops: () => void;
  /** 清理所有 tween。在 onUnmounted 中调用。 */
  stopAll: () => void;
}

/**
 * SimverseWorld GSAP 动画 composable.
 *
 * 注册全部 Task 1.3（@keyframes 替换）与 Task 1.4（<transition> 替换）的 watcher，
 * 并暴露 Task 1.5 的游戏级动画工具函数。
 *
 * @param refs 主组件持有的 reactive refs。
 * @returns 游戏级动画工具函数 + 生命周期钩子。
 */
export function useSimverseAnimations(refs: SimverseAnimationRefs): SimverseAnimationApi {
  // Tween 引用：用于在 stopAll 中统一 kill
  let bgTween: gsap.core.Tween | null = null;
  let runPulseTween: gsap.core.Tween | null = null;
  let npcPulseTween: gsap.core.Tween | null = null;
  let loadingSpinTween: gsap.core.Tween | null = null;
  let bannerFloatTween: gsap.core.Tween | null = null;
  let panelLoadingSpinTween: gsap.core.Tween | null = null;

  // 资源数值跳动飘字：缓存上一次的数值以计算 delta（Task 1.5 showComboNumber）
  let prevDiamond = refs.playerDiamond.value;
  let prevGold = refs.playerGold.value;

  // —— Task 1.5: 新增游戏级动画工具函数 ——

  /**
   * 抽卡光柱：沿曲线路径运动的光束（使用 MotionPathPlugin）。
   * 在抽卡触发时调用，附加到 .gacha-modal-overlay 内。
   */
  function playGachaLightBeam(): void {
    const modal = document.querySelector(".gacha-modal-overlay");
    if (!modal) return;
    const beam = document.createElement("div");
    beam.className = "gacha-light-beam";
    modal.appendChild(beam);

    gsap.to(beam, {
      motionPath: {
        path: [
          { x: 0, y: 200 },
          { x: 100, y: 100 },
          { x: 200, y: 0 },
          { x: 100, y: -100 },
          { x: 0, y: -200 },
        ],
        curviness: 1.5,
        autoRotate: false,
      },
      duration: 1.5,
      ease: "power2.inOut",
      onComplete: () => beam.remove(),
    });
  }

  /**
   * 屏幕震动：在抽卡揭晓、重大事件（imp-4）等场景调用。
   */
  function shakeScreen(intensity: number = 8, duration: number = 0.4): void {
    const container = document.querySelector(".game-container");
    if (!container) return;
    gsap.to(container, {
      x: () => gsap.utils.random(-intensity, intensity),
      y: () => gsap.utils.random(-intensity, intensity),
      duration: 0.05,
      repeat: duration / 0.05,
      yoyo: true,
      onComplete: () => gsap.set(container, { x: 0, y: 0 }),
    });
  }

  /**
   * 资源变化飘字：在资源数值变化时显示 "+N" 飘字（Task 1.5）。
   */
  function showComboNumber(target: HTMLElement, value: string, color: string = "#22c55e"): void {
    const combo = document.createElement("div");
    combo.textContent = value;
    combo.className = "gacha-combo-number";
    combo.style.cssText = `
      position: absolute;
      color: ${color};
      font-weight: 700;
      font-size: 18px;
      pointer-events: none;
      z-index: 100;
      text-shadow: 0 2px 4px rgba(0,0,0,0.5);
    `;
    target.parentElement?.appendChild(combo);

    gsap.fromTo(
      combo,
      { y: 0, opacity: 1, scale: 1 },
      { y: -40, opacity: 0, scale: 1.5, duration: 1, ease: "power2.out", onComplete: () => combo.remove() }
    );
  }

  /**
   * 粒子效果：在抽卡/升级等场景调用，从指定容器中心向外扩散（Task 1.5）。
   */
  function spawnParticles(container: HTMLElement, count: number = 12, color: string = "#8b5cf6"): void {
    for (let i = 0; i < count; i++) {
      const p = document.createElement("div");
      p.className = "gacha-particle";
      p.style.cssText = `
        position: absolute;
        width: 4px;
        height: 4px;
        border-radius: 50%;
        background: ${color};
        pointer-events: none;
      `;
      container.appendChild(p);

      const angle = (i / count) * Math.PI * 2;
      const distance = 80 + Math.random() * 40;

      gsap.to(p, {
        x: Math.cos(angle) * distance,
        y: Math.sin(angle) * distance,
        opacity: 0,
        scale: 0,
        duration: 0.8 + Math.random() * 0.4,
        ease: "power2.out",
        onComplete: () => p.remove(),
      });
    }
  }

  // —— Task 1.3: 替换 CSS @keyframes 的 GSAP watcher ——

  // 加载旋转：当 phaserLoading 变化时启停（替换 @keyframes spin）
  watch(
    () => refs.phaserLoading.value,
    (loading) => {
      if (loading && refs.usePhaser.value && !refs.phaserHasError.value) {
        nextTick(() => {
          loadingSpinTween?.kill();
          loadingSpinTween = gsap.to(".loading-spinner", {
            rotation: 360,
            duration: 1,
            ease: "none",
            repeat: -1,
          });
        });
      } else {
        loadingSpinTween?.kill();
        loadingSpinTween = null;
      }
    }
  );

  // 运行脉冲：当 worldState.running 变化时启停（替换 @keyframes runPulse）
  watch(
    () => refs.worldState.value?.running,
    (running) => {
      if (running) {
        nextTick(() => {
          runPulseTween?.kill();
          runPulseTween = gsap.to(".play-btn.running", {
            boxShadow: "0 0 20px rgba(34,197,94,0.85)",
            duration: 0.8,
            ease: "sine.inOut",
            yoyo: true,
            repeat: -1,
          });
        });
      } else {
        runPulseTween?.kill();
        runPulseTween = null;
      }
    }
  );

  // 抽卡 banner 浮动：当进入 gacha 场景时启停（替换 @keyframes float）
  watch(
    () => refs.screen.value === "gacha",
    (isGacha) => {
      if (isGacha) {
        nextTick(() => {
          bannerFloatTween?.kill();
          bannerFloatTween = gsap.to(".banner-icon-large", {
            y: -8,
            duration: 2,
            ease: "sine.inOut",
            yoyo: true,
            repeat: -1,
          });
        });
      } else {
        bannerFloatTween?.kill();
        bannerFloatTween = null;
      }
    }
  );

  // 资源数值跳动 + 飘字：当 playerDiamond/Gold 变化时触发（替换 @keyframes valuePop + Task 1.5 combo）
  watch([refs.playerDiamond, refs.playerGold, refs.playerStamina], ([newDiamond, newGold]) => {
    nextTick(() => {
      gsap.fromTo(
        ".resource-value",
        { scale: 1.3, color: "#c4b5fd" },
        { scale: 1, color: "#fff", duration: 0.45, ease: "back.out(2)", stagger: 0.05 }
      );

      // 飘字（仅 diamond / gold 显示，stamina 不显示）
      if (newDiamond !== prevDiamond) {
        const delta = newDiamond - prevDiamond;
        const targets = document.querySelectorAll(".resource-item.game-resource .resource-value");
        if (targets[0] instanceof HTMLElement) {
          showComboNumber(targets[0], (delta >= 0 ? "+" : "") + delta, delta >= 0 ? "#22c55e" : "#ef4444");
        }
        prevDiamond = newDiamond;
      }
      if (newGold !== prevGold) {
        const delta = newGold - prevGold;
        const targets = document.querySelectorAll(".resource-item.game-resource .resource-value");
        if (targets[1] instanceof HTMLElement) {
          showComboNumber(targets[1], (delta >= 0 ? "+" : "") + delta, delta >= 0 ? "#22c55e" : "#ef4444");
        }
        prevGold = newGold;
      }
    });
  });

  // 面板内 loading-spinner（经济/排行榜加载占位）：当面板切换时启停
  watch([refs.activePanel, refs.econTab, refs.economyStats], () => {
    nextTick(() => {
      const panelSpinner = document.querySelector(".side-panel .loading-spinner");
      if (panelSpinner) {
        panelLoadingSpinTween?.kill();
        panelLoadingSpinTween = gsap.to(panelSpinner, {
          rotation: 360,
          duration: 0.8,
          ease: "none",
          repeat: -1,
        });
      } else {
        panelLoadingSpinTween?.kill();
        panelLoadingSpinTween = null;
      }
    });
  });

  // —— Task 1.4: 替换 <transition> 包裹的 GSAP watcher ——

  // 底部主操作条：screen === 'world' 时滑入（替换 <transition name="bottom-bar">）
  watch(
    () => refs.screen.value === "world",
    (isWorld) => {
      if (isWorld) {
        nextTick(() => {
          if (refs.bottomBarRef.value) {
            gsap.fromTo(
              refs.bottomBarRef.value,
              { y: 20, opacity: 0 },
              { y: 0, opacity: 1, duration: 0.25, ease: "power2.out" }
            );
          }
        });
      }
    }
  );

  // 更多面板：bottomMoreOpen 切换时淡入 + 网格上滑（替换 <transition name="more-pop">）
  watch(refs.bottomMoreOpen, (open) => {
    if (open) {
      nextTick(() => {
        if (refs.morePopRef.value) {
          gsap.fromTo(refs.morePopRef.value, { opacity: 0 }, { opacity: 1, duration: 0.2 });
          const grid = refs.morePopRef.value.querySelector(".more-grid");
          if (grid) {
            gsap.fromTo(grid, { y: 24 }, { y: 0, duration: 0.24, ease: "power2.out" });
          }
        }
      });
    }
  });

  // 抽卡动画 overlay：isGachaAnimating 切换时闪现（替换 <transition name="gacha-flash">）
  watch(refs.isGachaAnimating, (animating) => {
    if (animating) {
      nextTick(() => {
        if (refs.gachaFlashRef.value) {
          gsap.fromTo(
            refs.gachaFlashRef.value,
            { opacity: 0.8 },
            { opacity: 0, duration: 0.5 }
          );
        }
      });
    }
  });

  // Ticker：recentEvents 变化时新条目滑入（替换 <transition-group name="ticker">）
  watch(refs.recentEvents, () => {
    nextTick(() => {
      gsap.fromTo(
        ".ticker-item",
        { x: 20, opacity: 0 },
        { x: 0, opacity: 1, stagger: 0.1, duration: 0.3, ease: "power2.out" }
      );
    });
  });

  // —— 生命周期钩子 ——

  /** 常驻循环动画启动（bgDrift + npcPulse）。在 onMounted 中调用。 */
  function startAmbientLoops(): void {
    // 背景缓动（替换 @keyframes bgDrift）—— 常驻循环
    bgTween = gsap.to(".game-container", {
      backgroundPosition: "100% 100%",
      duration: 26,
      ease: "sine.inOut",
      yoyo: true,
      repeat: -1,
    });

    // NPC 标记脉冲（替换 @keyframes pulse）—— 仅对 alive NPC
    npcPulseTween = gsap.to(".npc-dot.alive", {
      boxShadow: "0 0 12px rgba(34,197,94,0.8)",
      duration: 1,
      ease: "sine.inOut",
      yoyo: true,
      repeat: -1,
    });
  }

  /** 清理所有 tween。在 onUnmounted 中调用。 */
  function stopAll(): void {
    bgTween?.kill();
    runPulseTween?.kill();
    npcPulseTween?.kill();
    loadingSpinTween?.kill();
    bannerFloatTween?.kill();
    panelLoadingSpinTween?.kill();
  }

  return {
    playGachaLightBeam,
    shakeScreen,
    showComboNumber,
    spawnParticles,
    startAmbientLoops,
    stopAll,
  };
}
