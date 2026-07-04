/**
 * useFileSystemTests — 文件系统任务测试 composable
 *
 * 覆盖 move/copy/rename/delete + rollback 各边界状态。
 *
 * 测试目录：<servingDir>/.encv-test-fs/（通过 checkServiceGuard 获取 servingDir）
 * 每个用例独立创建临时文件，测试完成后清理整个测试目录。
 *
 * 依赖后端：
 *  - 文件操作接入任务系统（move/copy/rename/delete 作为 Task）
 *  - 回滚机制（POST /api/tasks/:id/rollback）
 *  - 回收站机制（/api/trash）
 */
import { ref } from "vue";
import {
  checkFileExists,
  checkServiceGuard,
  copyFile,
  createDirectory,
  deleteFile,
  type EncvTask,
  emptyTrash,
  getTasks,
  moveFile,
  renameFile,
  rollbackTask,
  type TaskStatus,
  uploadFile,
} from "@encv/shared-components/api/encv";

// ==================== 类型定义 ====================

export interface FSTestCase {
  name: string;
  description: string;
  run: () => Promise<void>;
}

export interface FSTestResult {
  name: string;
  passed: boolean;
  error?: string;
  duration?: number;
}

// ==================== 常量 ====================

const TEST_DIR_NAME = ".encv-test-fs";
const DEFAULT_TASK_TIMEOUT_MS = 30_000;
const POLL_INTERVAL_MS = 500;

/** 终态集合：任务进入这些状态后不再变化 */
const TERMINAL_STATUSES: ReadonlySet<TaskStatus> = new Set<TaskStatus>(["completed", "failed", "cancelled"]);

// ==================== Composable ====================

export function useFileSystemTests() {
  const results = ref<FSTestResult[]>([]);
  const isRunning = ref(false);

  /** 测试目录绝对路径（懒初始化，由 ensureTestDir 设置） */
  let testDir = "";

  // ============ 辅助函数 ============

  /**
   * 初始化测试目录：获取 servingDir，拼接测试目录路径，通过 API 创建。
   * 若目录已存在则忽略错误。后续调用直接返回已缓存的路径。
   */
  async function ensureTestDir(): Promise<string> {
    if (testDir) return testDir;
    const guard = await checkServiceGuard();
    // servingDir 形如 /storage/emulated/0（无尾斜杠）
    const base = guard.servingDir.replace(/\/+$/, "");
    testDir = `${base}/${TEST_DIR_NAME}`;
    try {
      await createDirectory(base, TEST_DIR_NAME);
    } catch {
      // 目录已存在或其他非致命错误 — 后续操作会处理
    }
    return testDir;
  }

  /**
   * 在测试目录下创建临时文件。
   * @param name 文件名（不含路径）
   * @param content 文本内容
   * @returns 文件绝对路径
   */
  async function createTempFile(name: string, content: string): Promise<string> {
    const dir = await ensureTestDir();
    const blob = new Blob([content], { type: "text/plain" });
    const file = new File([blob], name);
    await uploadFile(dir, file);
    return `${dir}/${name}`;
  }

  /**
   * 删除临时文件（通过 delete 任务系统），等待任务完成。
   * 文件不存在或已删除时静默忽略。
   */
  async function deleteTempFile(path: string): Promise<void> {
    try {
      const { taskId } = await deleteFile(path);
      await waitForTaskComplete(taskId);
    } catch {
      // 文件可能不存在或已删除 — 忽略
    }
  }

  /** 检查文件是否存在 */
  async function fileExists(path: string): Promise<boolean> {
    return checkFileExists(path);
  }

  /**
   * 轮询 getTasks 直到指定任务进入终态（completed/failed/cancelled）。
   * @param taskId 任务 ID
   * @param timeoutMs 超时（默认 30s）
   * @returns 终态任务对象
   * @throws 超时抛 Error
   */
  async function waitForTaskComplete(taskId: string, timeoutMs: number = DEFAULT_TASK_TIMEOUT_MS): Promise<EncvTask> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const tasks = await getTasks();
      const task = tasks.find(t => t.id === taskId);
      if (task && TERMINAL_STATUSES.has(task.status)) {
        return task;
      }
      await sleep(POLL_INTERVAL_MS);
    }
    throw new Error(`Task ${taskId} did not complete within ${timeoutMs}ms`);
  }

  /** 断言文件存在，否则抛错 */
  async function assertExists(path: string, label: string): Promise<void> {
    if (!(await fileExists(path))) {
      throw new Error(`Assertion failed: ${label} should exist at ${path}`);
    }
  }

  /** 断言文件不存在，否则抛错 */
  async function assertNotExists(path: string, label: string): Promise<void> {
    if (await fileExists(path)) {
      throw new Error(`Assertion failed: ${label} should NOT exist at ${path}`);
    }
  }

  /**
   * 清理测试目录：删除整个 .encv-test-fs 目录。
   * 尽力而为，不抛错。
   */
  async function cleanup(): Promise<void> {
    if (!testDir) return;
    // deleteTempFile 内部已处理错误吞没 + 等待任务完成
    await deleteTempFile(testDir);
  }

  // ============ 测试用例构建 ============

  function buildTestCases(): FSTestCase[] {
    return [
      // 1. move + rollback
      {
        name: "move + rollback",
        description: "创建临时文件 → move → 验证原位无文件、新位有文件 → rollback → 验证原位有文件、新位无文件",
        run: async () => {
          const src = await createTempFile("move_src.txt", "move-test-content");
          const dest = `${testDir}/move_dest.txt`;

          const { taskId } = await moveFile(src, dest);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`move task failed: ${task.error || task.status}`);
          }
          await assertNotExists(src, "源文件");
          await assertExists(dest, "目标文件");

          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "completed") {
            throw new Error(`rollback task failed: ${rbTask.error || rbTask.status}`);
          }
          await assertExists(src, "回滚后源文件");
          await assertNotExists(dest, "回滚后目标文件");
        },
      },

      // 2. copy + rollback
      {
        name: "copy + rollback",
        description: "创建临时文件 → copy → 验证原位和新位都有文件 → rollback → 验证新位文件已删",
        run: async () => {
          const src = await createTempFile("copy_src.txt", "copy-test-content");
          const dest = `${testDir}/copy_dest.txt`;

          const { taskId } = await copyFile(src, dest);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`copy task failed: ${task.error || task.status}`);
          }
          await assertExists(src, "源文件");
          await assertExists(dest, "目标文件");

          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "completed") {
            throw new Error(`rollback task failed: ${rbTask.error || rbTask.status}`);
          }
          await assertExists(src, "回滚后源文件");
          await assertNotExists(dest, "回滚后目标文件");
        },
      },

      // 3. rename + rollback
      {
        name: "rename + rollback",
        description: "创建临时文件 → rename → 验证旧名无文件、新名有文件 → rollback → 验证旧名有文件、新名无文件",
        run: async () => {
          const src = await createTempFile("rename_old.txt", "rename-test-content");
          const newPath = `${testDir}/rename_new.txt`;

          // renameFile 接受 newName（仅文件名，不含路径）
          const { taskId } = await renameFile(src, "rename_new.txt");
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`rename task failed: ${task.error || task.status}`);
          }
          await assertNotExists(src, "旧名文件");
          await assertExists(newPath, "新名文件");

          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "completed") {
            throw new Error(`rollback task failed: ${rbTask.error || rbTask.status}`);
          }
          await assertExists(src, "回滚后旧名文件");
          await assertNotExists(newPath, "回滚后新名文件");
        },
      },

      // 4. delete + rollback
      {
        name: "delete + rollback",
        description: "创建临时文件 → delete → 验证原位无文件 → rollback → 验证原位有文件（从回收站还原）",
        run: async () => {
          const src = await createTempFile("delete_test.txt", "delete-test-content");

          const { taskId } = await deleteFile(src);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`delete task failed: ${task.error || task.status}`);
          }
          await assertNotExists(src, "删除后源文件");

          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "completed") {
            throw new Error(`rollback task failed: ${rbTask.error || rbTask.status}`);
          }
          await assertExists(src, "回滚后源文件");
        },
      },

      // 5. 边界：move 到已存在目标
      {
        name: "boundary: move to existing target",
        description: "创建两个文件 → move file1 到 file2 路径 → 期望任务 failed",
        run: async () => {
          const src = await createTempFile("move_exist_src.txt", "src-content");
          const dest = await createTempFile("move_exist_dest.txt", "dest-content");

          const { taskId } = await moveFile(src, dest);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "failed") {
            throw new Error(`Expected move task to fail (target exists), but status=${task.status}`);
          }
        },
      },

      // 6. 边界：rollback 已被回滚的任务
      {
        name: "boundary: rollback already rolled back task",
        description: "完成 move → rollback → 再次 rollback 原 move 任务 → 期望 400 错误",
        run: async () => {
          const src = await createTempFile("rb_twice_src.txt", "rb-twice-content");
          const dest = `${testDir}/rb_twice_dest.txt`;

          const { taskId } = await moveFile(src, dest);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`move task failed: ${task.error || task.status}`);
          }

          // 第一次 rollback（应成功）
          const { taskId: rbTaskId } = await rollbackTask(taskId);
          await waitForTaskComplete(rbTaskId);

          // 第二次 rollback（应失败 — 400 错误或任务 failed）
          let secondRollbackFailed = false;
          try {
            const { taskId: secondRbTaskId } = await rollbackTask(taskId);
            const secondRbTask = await waitForTaskComplete(secondRbTaskId);
            if (secondRbTask.status === "failed") {
              secondRollbackFailed = true;
            }
          } catch {
            // API 调用失败（400 错误）
            secondRollbackFailed = true;
          }
          if (!secondRollbackFailed) {
            throw new Error("Expected second rollback to fail, but it succeeded");
          }
        },
      },

      // 7. 边界：rollback 已被清空的 trash
      {
        name: "boundary: rollback with emptied trash",
        description: "完成 delete → 清空 trash → rollback delete 任务 → 期望 failed",
        run: async () => {
          const src = await createTempFile("del_empty_trash.txt", "del-trash-content");

          const { taskId } = await deleteFile(src);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`delete task failed: ${task.error || task.status}`);
          }

          // 清空回收站
          await emptyTrash();

          // rollback delete 任务（应失败 — trash 已空）
          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "failed") {
            throw new Error(`Expected rollback to fail (trash emptied), but status=${rbTask.status}`);
          }
        },
      },

      // 8. 边界：rollback 时原位已被占用
      {
        name: "boundary: rollback with occupied original location",
        description: "完成 move → 在原位创建新文件 → rollback → 期望 failed",
        run: async () => {
          const src = await createTempFile("rb_occupied_src.txt", "occupied-content");
          const dest = `${testDir}/rb_occupied_dest.txt`;

          const { taskId } = await moveFile(src, dest);
          const task = await waitForTaskComplete(taskId);
          if (task.status !== "completed") {
            throw new Error(`move task failed: ${task.error || task.status}`);
          }

          // 在原位创建新文件（move 后原位已空，可重新创建同名文件）
          await createTempFile("rb_occupied_src.txt", "new-occupying-content");

          // rollback（应失败 — 原位已被占用）
          const { taskId: rbTaskId } = await rollbackTask(taskId);
          const rbTask = await waitForTaskComplete(rbTaskId);
          if (rbTask.status !== "failed") {
            throw new Error(`Expected rollback to fail (original location occupied), but status=${rbTask.status}`);
          }
        },
      },
    ];
  }

  // ============ 主入口 ============

  async function runAllTests(): Promise<void> {
    if (isRunning.value) return;
    isRunning.value = true;
    results.value = [];

    try {
      await ensureTestDir();
      const testCases = buildTestCases();
      for (const tc of testCases) {
        const result: FSTestResult = { name: tc.name, passed: false };
        const start = Date.now();
        try {
          await tc.run();
          result.passed = true;
        } catch (err: any) {
          result.error = err?.message || String(err);
        }
        result.duration = Date.now() - start;
        results.value.push(result);
      }
    } finally {
      // 尽力清理临时目录
      await cleanup();
      isRunning.value = false;
    }
  }

  return { results, isRunning, runAllTests };
}

// ==================== 内部工具 ====================

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
