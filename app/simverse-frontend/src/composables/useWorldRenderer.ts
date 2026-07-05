import { Ellipse, Leafer, PointerEvent, Rect, Text } from "leafer-ui";
import Matter from "matter-js";
import { onUnmounted, ref } from "vue";

export interface WorldEntity {
  id: string;
  type: "npc" | "building" | "tree" | "rock" | "ground" | "water";
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
  label?: string;
  static?: boolean;
  onClick?: (entity: WorldEntity) => void;
}

export interface WorldRendererOptions {
  container: HTMLElement;
  worldWidth: number;
  worldHeight: number;
  gravity?: number;
}

export function useWorldRenderer(options: WorldRendererOptions) {
  const { container, worldWidth, worldHeight, gravity = 0 } = options;

  const isReady = ref(false);

  let leafer: Leafer | null = null;
  let engine: Matter.Engine | null = null;
  let runner: Matter.Runner | null = null;

  const bodyToEntity = new Map<number, WorldEntity>();
  const entityToLeafer = new Map<string, any>();
  const entityToBody = new Map<string, Matter.Body>();

  function init() {
    leafer = new Leafer({
      view: container,
      width: container.clientWidth,
      height: container.clientHeight,
    });

    engine = Matter.Engine.create();
    engine.gravity.y = gravity;

    const worldSize = { width: worldWidth, height: worldHeight };
    Matter.Composite.add(engine.world, [
      Matter.Bodies.rectangle(worldSize.width / 2, -50, worldSize.width, 100, { isStatic: true, label: "wall-top" }),
      Matter.Bodies.rectangle(worldSize.width / 2, worldSize.height + 50, worldSize.width, 100, { isStatic: true, label: "wall-bottom" }),
      Matter.Bodies.rectangle(-50, worldSize.height / 2, 100, worldSize.height, { isStatic: true, label: "wall-left" }),
      Matter.Bodies.rectangle(worldSize.width + 50, worldSize.height / 2, 100, worldSize.height, { isStatic: true, label: "wall-right" }),
    ]);

    runner = Matter.Runner.create();
    Matter.Runner.run(runner, engine);

    requestAnimationFrame(gameLoop);

    isReady.value = true;
  }

  let lastTime = 0;
  function gameLoop(timestamp: number) {
    if (!engine || !leafer) return;

    const delta = timestamp - lastTime;
    lastTime = timestamp;

    Matter.Engine.update(engine, delta);

    for (const [entityId, body] of entityToBody.entries()) {
      const leaf = entityToLeafer.get(entityId);
      if (leaf && !body.isStatic) {
        leaf.x = body.position.x - body.bounds.min.x;
        leaf.y = body.position.y - body.bounds.min.y;
        leaf.rotation = body.angle * 57.2958;
      }
    }

    requestAnimationFrame(gameLoop);
  }

  function addEntity(entity: WorldEntity) {
    if (!leafer || !engine) return;

    const isStatic =
      entity.static ??
      (entity.type === "building" ||
        entity.type === "tree" ||
        entity.type === "ground" ||
        entity.type === "rock" ||
        entity.type === "water");

    let leaf: any;
    if (entity.type === "npc") {
      leaf = new Ellipse({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
        stroke: "#ffffff",
        strokeWidth: 2,
        cursor: "pointer",
      });
    } else if (entity.type === "tree" || entity.type === "rock" || entity.type === "building") {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
        stroke: "#333",
        strokeWidth: 1,
        cornerRadius: 4,
      });
    } else if (entity.type === "water" || entity.type === "ground") {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
      });
    } else {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
      });
    }

    if (entity.label) {
      const labelText = new Text({
        x: entity.x + entity.width / 2,
        y: entity.y - 18,
        text: entity.label,
        fontSize: 12,
        fill: "#fff",
        textAlign: "center",
        verticalAlign: "middle",
        textWrap: false,
      });
      leafer.add(labelText);
    }

    leafer.add(leaf);
    entityToLeafer.set(entity.id, leaf);

    const body = Matter.Bodies.rectangle(entity.x + entity.width / 2, entity.y + entity.height / 2, entity.width, entity.height, {
      isStatic,
      label: entity.id,
      friction: 0.1,
      restitution: 0.3,
    });
    Matter.Composite.add(engine.world, body);
    bodyToEntity.set(body.id, entity);
    entityToBody.set(entity.id, body);

    if (entity.onClick && entity.type === "npc") {
      leaf.on(PointerEvent.DOWN, () => {
        entity.onClick?.(entity);
      });
    }
  }

  function removeEntity(id: string) {
    const leaf = entityToLeafer.get(id);
    if (leaf) {
      leafer?.remove(leaf);
      entityToLeafer.delete(id);
    }
    const body = entityToBody.get(id);
    if (body && engine) {
      Matter.Composite.remove(engine.world, body);
      bodyToEntity.delete(body.id);
      entityToBody.delete(id);
    }
  }

  function updateEntityPosition(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.setPosition(body, { x, y });
    }
  }

  function applyForce(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.applyForce(body, body.position, { x, y });
    }
  }

  function setVelocity(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.setVelocity(body, { x, y });
    }
  }

  function resize() {
    if (leafer) {
      leafer.resize({ width: container.clientWidth, height: container.clientHeight });
    }
  }

  function clearAll() {
    for (const id of Array.from(entityToBody.keys())) {
      removeEntity(id);
    }
  }

  function destroy() {
    clearAll();
    if (runner) {
      Matter.Runner.stop(runner);
      runner = null;
    }
    if (engine) {
      Matter.Engine.clear(engine);
      engine = null;
    }
    if (leafer) {
      leafer.destroy();
      leafer = null;
    }
    bodyToEntity.clear();
    entityToLeafer.clear();
    entityToBody.clear();
    isReady.value = false;
  }

  onUnmounted(() => {
    destroy();
  });

  return {
    isReady,
    init,
    destroy,
    resize,
    addEntity,
    removeEntity,
    updateEntityPosition,
    applyForce,
    setVelocity,
    clearAll,
  };
}
