/**
 * Matrix 展开器 — 将笛卡尔积矩阵策略展开为多组变量绑定
 *
 * 示例：
 *   axes: { plugin: ['video-v4', 'audio-v4'], cipher: [0, 1] }
 *   → [
 *     { plugin: 'video-v4', cipher: 0 },
 *     { plugin: 'video-v4', cipher: 1 },
 *     { plugin: 'audio-v4', cipher: 0 },
 *     { plugin: 'audio-v4', cipher: 1 },
 *   ]
 */

import type { JobStrategy, MatrixStrategy } from "./types";

export interface MatrixBinding {
  [key: string]: string;
}

/**
 * 将 matrix 策略展开为所有变量组合。
 */
export function expandMatrix(strategy: MatrixStrategy): MatrixBinding[] {
  const entries = Object.entries(strategy.axes);
  if (entries.length === 0) return [{}];

  // 递归笛卡尔积
  function cartesian(remaining: [string, string[]][], current: MatrixBinding): MatrixBinding[] {
    if (remaining.length === 0) return [current];

    const [[key, values], ...rest] = remaining;
    const results: MatrixBinding[] = [];

    for (const val of values) {
      const childResults = cartesian(rest, { ...current, [key]: val });
      results.push(...childResults);
    }

    return results;
  }

  return cartesian(entries, {});
}

/**
 * 判断 Job 是否使用 matrix 策略。
 */
export function isMatrixStrategy(strategy?: JobStrategy): strategy is MatrixStrategy {
  return strategy?.type === "matrix";
}
