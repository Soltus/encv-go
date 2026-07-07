import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

interface VueComponentCheckOptions {
  componentDirs?: string[];
  globalComponents?: string[];
  failOnError?: boolean;
}

const PASCAL_CASE_RE = /^[A-Z][a-zA-Z0-9]*$/;
const ION_TAG_RE = /^ion-/;

function extractTemplateComponents(template: string): Set<string> {
  const components = new Set<string>();
  const tagRE = /<([A-Z][a-zA-Z0-9]*)/g;
  let match;
  while ((match = tagRE.exec(template)) !== null) {
    const tag = match[1];
    if (PASCAL_CASE_RE.test(tag) && tag !== "template" && tag !== "script" && tag !== "style") {
      components.add(tag);
    }
  }
  return components;
}

function extractScriptImports(script: string): Set<string> {
  const imports = new Set<string>();
  const importDefaultRE = /import\s+(\w+)\s+from\s+["'][^"']+\.vue["']/g;
  let match;
  while ((match = importDefaultRE.exec(script)) !== null) {
    imports.add(match[1]);
  }
  const importNamedRE = /import\s+\{\s*([^}]+)\s*\}\s+from\s+["']@ionic\/vue["']/g;
  while ((match = importNamedRE.exec(script)) !== null) {
    const names = match[1].split(",").map((s) => s.trim().split(" as ")[0].trim());
    for (const name of names) {
      if (name) imports.add(name);
    }
  }
  return imports;
}

function pascalToKebab(name: string): string {
  return name.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

export function vueComponentCheckPlugin(options: VueComponentCheckOptions = {}) {
  const {
    componentDirs = ["src/components"],
    globalComponents = [],
    failOnError = true,
  } = options;

  const globalSet = new Set(globalComponents);
  let rootDir: string;
  const errors: string[] = [];
  let totalFiles = 0;
  let startTime = 0;

  return {
    name: "vue-component-check",
    enforce: "pre" as const,

    configResolved(config: any) {
      rootDir = config.root;
    },

    buildStart() {
      errors.length = 0;
      totalFiles = 0;
      startTime = Date.now();
    },

    transform(code: string, id: string) {
      if (!id.endsWith(".vue")) return null;
      totalFiles++;

      const templateMatch = code.match(/<template>([\s\S]*?)<\/template>/);
      const scriptMatch = code.match(/<script[^>]*>([\s\S]*?)<\/script>/);
      if (!templateMatch) return null;

      const templateComponents = extractTemplateComponents(templateMatch[1]);
      const scriptImports = scriptMatch ? extractScriptImports(scriptMatch[1]) : new Set<string>();

      for (const comp of templateComponents) {
        if (scriptImports.has(comp)) continue;
        if (globalSet.has(comp)) continue;

        const kebab = pascalToKebab(comp);
        if (ION_TAG_RE.test(kebab)) continue;
        if (comp === "Transition" || comp === "TransitionGroup" || comp === "KeepAlive" || comp === "Teleport" || comp === "Suspense") continue;

        const relativePath = id.replace(rootDir + "/", "");
        errors.push(`[vue-component-check] ${relativePath}: 使用了组件 <${comp}> 但未导入`);
      }

      return null;
    },

    buildEnd() {
      const duration = Date.now() - startTime;
      console.log(`\n[vue-component-check] 扫描了 ${totalFiles} 个 Vue 文件，用时 ${duration}ms`);

      if (errors.length > 0) {
        console.error(`\n[vue-component-check] 发现 ${errors.length} 个未导入的组件：`);
        for (const err of errors) {
          console.error(`  ${err}`);
        }
        console.error("");
        if (failOnError) {
          this.error(`发现 ${errors.length} 个组件未导入，请检查上述文件`);
        }
      } else {
        console.log("[vue-component-check] ✅ 所有组件都已正确导入\n");
      }
    },
  };
}
