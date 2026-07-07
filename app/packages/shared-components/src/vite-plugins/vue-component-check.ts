import { readFileSync, existsSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

export interface VueComponentCheckOptions {
  componentDirs?: string[];
  globalComponents?: string[];
  failOnError?: boolean;
  dev?: boolean;
  exclude?: RegExp[];
}

interface FileState {
  components: Set<string>;
  imports: Set<string>;
  errors: string[];
}

const PASCAL_TAG_RE = /<([A-Z][a-zA-Z0-9]*)(\s|\/|>)/g;
const DYNAMIC_COMPONENT_RE = /<component[^>]+:is\s*=\s*["']([A-Z][a-zA-Z0-9]*)["']/g;
const IMPORT_DEFAULT_RE = /import\s+(\w+)\s+from\s+["'][^"']+["']/g;
const IMPORT_NAMED_RE = /import\s+\{\s*([^}]+)\s*\}\s+from\s+["'][^"']+["']/g;
const VUE_BUILT_INS = new Set([
  "Transition",
  "TransitionGroup",
  "KeepAlive",
  "Teleport",
  "Suspense",
  "Component",
  "Slot",
]);

function extractTemplateComponents(template: string): Set<string> {
  const components = new Set<string>();
  let match;

  PASCAL_TAG_RE.lastIndex = 0;
  while ((match = PASCAL_TAG_RE.exec(template)) !== null) {
    const tag = match[1];
    if (!VUE_BUILT_INS.has(tag)) {
      components.add(tag);
    }
  }

  DYNAMIC_COMPONENT_RE.lastIndex = 0;
  while ((match = DYNAMIC_COMPONENT_RE.exec(template)) !== null) {
    components.add(match[1]);
  }

  return components;
}

function extractScriptImports(script: string): Set<string> {
  const imports = new Set<string>();
  let match;

  IMPORT_DEFAULT_RE.lastIndex = 0;
  while ((match = IMPORT_DEFAULT_RE.exec(script)) !== null) {
    imports.add(match[1]);
  }

  IMPORT_NAMED_RE.lastIndex = 0;
  while ((match = IMPORT_NAMED_RE.exec(script)) !== null) {
    const names = match[1].split(",").map((s) => {
      const trimmed = s.trim();
      const asMatch = trimmed.match(/(\w+)\s+as\s+(\w+)/);
      return asMatch ? asMatch[2] : trimmed;
    });
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
    dev = false,
    exclude = [],
  } = options;

  const globalSet = new Set(globalComponents);
  let rootDir: string;
  const fileStates = new Map<string, FileState>();
  let totalErrors = 0;
  let startTime = 0;
  let totalFiles = 0;

  function resolveComponentDirs(): string[] {
    return componentDirs.map((d) => resolve(rootDir, d)).filter((d) => existsSync(d) && statSync(d).isDirectory());
  }

  function isGlobalComponent(name: string): boolean {
    if (globalSet.has(name)) return true;
    const kebab = pascalToKebab(name);
    if (kebab.startsWith("ion-")) return true;
    return false;
  }

  function shouldExclude(id: string): boolean {
    return exclude.some((re) => re.test(id));
  }

  function checkFile(id: string, code: string): string[] {
    const errors: string[] = [];
    const relativePath = relative(rootDir, id);

    const templateMatch = code.match(/<template>([\s\S]*?)<\/template>/);
    if (!templateMatch) return errors;

    const scriptMatch = code.match(/<script[^>]*>([\s\S]*?)<\/script>/);
    const templateComponents = extractTemplateComponents(templateMatch[1]);
    const scriptImports = scriptMatch ? extractScriptImports(scriptMatch[1]) : new Set<string>();

    for (const comp of templateComponents) {
      if (scriptImports.has(comp)) continue;
      if (isGlobalComponent(comp)) continue;

      errors.push(`${relativePath}: 组件 <${comp}> 在模板中使用但未导入`);
    }

    return errors;
  }

  function reportErrors(allErrors: string[]) {
    if (allErrors.length === 0) return;

    console.error(`\n[vue-component-check] 发现 ${allErrors.length} 个未导入的组件：`);
    for (const err of allErrors) {
      console.error(`  ⚠️  ${err}`);
    }
    console.error("");
  }

  return {
    name: "vue-component-check",
    enforce: "pre" as const,

    configResolved(config: any) {
      rootDir = config.root;
    },

    buildStart() {
      fileStates.clear();
      totalErrors = 0;
      totalFiles = 0;
      startTime = Date.now();
    },

    transform(code: string, id: string) {
      if (!id.endsWith(".vue")) return null;
      if (shouldExclude(id)) return null;

      totalFiles++;
      const errors = checkFile(id, code);

      fileStates.set(id, {
        components: new Set(),
        imports: new Set(),
        errors,
      });

      if (errors.length > 0) {
        totalErrors += errors.length;
        if (dev) {
          for (const err of errors) {
            console.warn(`[vue-component-check] ⚠️  ${err}`);
          }
        }
      }

      return null;
    },

    buildEnd() {
      const duration = Date.now() - startTime;
      console.log(`\n[vue-component-check] 扫描了 ${totalFiles} 个 Vue 文件，用时 ${duration}ms`);

      if (totalErrors > 0) {
        const allErrors: string[] = [];
        for (const state of fileStates.values()) {
          allErrors.push(...state.errors);
        }
        reportErrors(allErrors);

        if (failOnError && !dev) {
          this.error(`发现 ${totalErrors} 个组件未导入，请检查上述文件`);
        } else if (dev) {
          console.warn(`[vue-component-check] ⚠️  dev 模式下仅警告，构建继续\n`);
        }
      } else {
        console.log("[vue-component-check] ✅ 所有组件都已正确导入\n");
      }
    },

    handleHotUpdate(ctx: any) {
      if (!dev) return;
      const file = ctx.file;
      if (!file.endsWith(".vue")) return;
      if (shouldExclude(file)) return;

      try {
        const code = readFileSync(file, "utf-8");
        const errors = checkFile(file, code);
        const prevErrors = fileStates.get(file)?.errors.length || 0;

        if (errors.length > 0) {
          totalErrors += errors.length - prevErrors;
          console.warn(`\n[vue-component-check] ⚠️  ${relative(rootDir, file)} 有 ${errors.length} 个未导入组件：`);
          for (const err of errors) {
            console.warn(`     ${err}`);
          }
          console.warn("");
        } else if (prevErrors > 0) {
          totalErrors -= prevErrors;
          console.log(`[vue-component-check] ✅  ${relative(rootDir, file)} 已修复，剩余 ${totalErrors} 个问题\n`);
        }

        fileStates.set(file, {
          components: new Set(),
          imports: new Set(),
          errors,
        });
      } catch {
        // 读取失败时忽略（比如文件被删除）
      }
    },
  };
}
