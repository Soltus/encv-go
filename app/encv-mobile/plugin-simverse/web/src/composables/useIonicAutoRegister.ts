import * as IonicVueComponents from "@ionic/vue";
import type { App } from "vue";

const ION_PREFIX = "Ion";

export interface IonicRegisterOptions {
  extraComponents?: Record<string, any>;
  skip?: string[];
}

function isIonComponent(name: string): boolean {
  return name.startsWith(ION_PREFIX) && name.length > ION_PREFIX.length;
}

export function registerIonicComponents(app: App, options: IonicRegisterOptions = {}): { registered: string[]; skipped: string[] } {
  const { extraComponents = {}, skip = [] } = options;

  const registered: string[] = [];
  const skipped: string[] = [];

  const skipSet = new Set(
    skip.map(name => {
      if (name.startsWith(ION_PREFIX)) return name;
      return ION_PREFIX + name.charAt(0).toUpperCase() + name.slice(1);
    })
  );

  for (const [name, component] of Object.entries(IonicVueComponents)) {
    if (!isIonComponent(name)) continue;
    if (skipSet.has(name)) {
      skipped.push(name);
      continue;
    }
    if (typeof component !== "object" && typeof component !== "function") {
      continue;
    }
    app.component(name, component as any);
    registered.push(name);
  }

  for (const [name, component] of Object.entries(extraComponents)) {
    const displayName = isIonComponent(name) ? name : ION_PREFIX + name;
    app.component(displayName, component);
    registered.push(displayName);
  }

  return { registered, skipped };
}

export function getIonicComponentNames(): string[] {
  return Object.keys(IonicVueComponents).filter(isIonComponent).sort();
}

export default registerIonicComponents;
