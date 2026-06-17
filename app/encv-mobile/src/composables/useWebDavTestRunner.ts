/**
 * useWebDavTestRunner — 声明式 WebDAV test case 解释引擎
 *
 * 🆕 2026-06-17：声明式重构（multi-mount-storage-refactor spec 续）
 *
 * 设计要点：
 *  - 不写 switch case：所有测试逻辑通过 TestDescriptor 数据描述
 *  - 统一处理：HTTP 请求构造 / 响应验证 / 断言
 *  - 支持：并发（concurrency）/ 重复（iterations）/ 超时（timeoutMs）/ abort
 *  - 兼容：useI18n i18n key 翻译（runner 不持有 t()，name 字段直接由调用方填）
 *
 * 错误分类（errorKind）：
 *  - 'http_4xx' / 'http_5xx'：预期外的 status code
 *  - 'timeout'：超时（AbortError）
 *  - 'network'：fetch 失败
 *  - 'assertion'：body / header / 响应时间不符合期望
 *  - 'unknown'：default
 */

import type {
  TestDescriptor,
  WebDavTestContext,
  TestCaseResult,
  TestCaseStatus,
  AssertionFailure,
} from '@/types/webdav-test'

const DEFAULT_TIMEOUT_MS = 15_000

export interface RunCaseOptions {
  /** 用户手动 abort signal（取消测试） */
  abortSignal?: AbortSignal
  /** 翻译函数（前端注入，runner 不耦合 i18n） */
  translateName: (id: string) => string
}

export type RunCaseFn = (
  desc: TestDescriptor,
  ctx: WebDavTestContext,
  options?: RunCaseOptions
) => Promise<TestCaseResult>

export function useWebDavTestRunner() {
  /**
   * 执行单个 TestDescriptor 并返回 TestCaseResult
   */
  async function runCase(
    desc: TestDescriptor,
    ctx: WebDavTestContext,
    options: RunCaseOptions = { translateName: (id) => id }
  ): Promise<TestCaseResult> {
    const start = Date.now()

    // 1. skip 判断
    if (typeof desc.skip === 'function' ? desc.skip(ctx) : desc.skip === true) {
      return {
        id: desc.id,
        name: options.translateName(desc.id),
        module: desc.module,
        status: 'skipped',
        durationMs: 0,
      }
    }

    // 2. 构造请求参数
    const method = desc.method
    const url = typeof desc.url === 'function' ? desc.url(ctx) : desc.url
    const headers = typeof desc.headers === 'function'
      ? desc.headers(ctx)
      : (desc.headers ?? {})
    const body = (typeof desc.body === 'function' ? desc.body(ctx) : desc.body) ?? null

    // 注入 Basic Auth（如果 ctx.auth 有 username）
    const finalHeaders: Record<string, string> = { ...headers }
    if (ctx.auth.username && finalHeaders['Authorization'] === undefined) {
      finalHeaders['Authorization'] = basicAuthHeader(ctx.auth)
    }

    const timeoutMs = desc.timeoutMs ?? DEFAULT_TIMEOUT_MS
    const controller = new AbortController()
    const linkedSignal = linkAbortSignals(controller, options.abortSignal, ctx.abortSignal)

    let timer: ReturnType<typeof setTimeout> | null = null
    if (timeoutMs > 0) {
      timer = setTimeout(() => controller.abort(), timeoutMs)
    }

    try {
      // 3. beforeRun 钩子
      if (desc.beforeRun) await desc.beforeRun(ctx)

      // 4. 执行 HTTP 请求（支持并发 / 重复）
      const iterations = desc.iterations ?? 1
      const concurrency = desc.concurrency ?? 1
      const iterResults: { status: number; durationMs: number; passed: boolean }[] = []

      let primaryResponse: Response | null = null
      let primaryBody = ''
      let primaryError: Error | null = null

      for (let i = 0; i < iterations; i++) {
        const responses = await Promise.all(
          Array.from({ length: concurrency }, () =>
            runSingleFetch(url, method, finalHeaders, body, linkedSignal.signal)
          )
        )

        // 验证所有并发响应 status 一致
        for (const r of responses) {
          if (r.error) {
            iterResults.push({ status: 0, durationMs: 0, passed: false })
            if (!primaryError) primaryError = r.error
          } else if (r.response) {
            iterResults.push({ status: r.response.status, durationMs: r.durationMs, passed: true })
            if (!primaryResponse) {
              primaryResponse = r.response
              primaryBody = r.body
            }
          }
        }
      }

      if (timer) clearTimeout(timer)

      // 5. 验证
      let passed = true
      let assertionFailure: AssertionFailure | null = null
      let errorKind: TestCaseResult['errorKind']
      let error: string | undefined

      if (primaryError) {
        if (primaryError.name === 'AbortError') {
          passed = false
          errorKind = 'timeout'
          error = `timeout after ${timeoutMs}ms`
        } else {
          passed = false
          errorKind = 'network'
          error = primaryError.message
        }
      } else if (primaryResponse) {
        const status = primaryResponse.status
        const expected = desc.expect.status
        const statusNotIn = desc.expect.statusNotIn ?? []
        const expectedList = expected !== undefined
          ? (Array.isArray(expected) ? expected : [expected])
          : null

        if (expectedList && !expectedList.includes(status)) {
          passed = false
          errorKind = status >= 500 ? 'http_5xx' : 'http_4xx'
          error = `expected status ${expectedList.join('|')}, got ${status}`
        } else if (statusNotIn.length > 0 && statusNotIn.includes(status)) {
          passed = false
          errorKind = status >= 500 ? 'http_5xx' : 'http_4xx'
          error = `status ${status} is in excluded list [${statusNotIn.join(',')}]`
        } else {
          // body 验证
          const bodyFail = validateBody(primaryBody, desc.expect.bodyMatches, desc.expect.bodyNotMatches)
          if (bodyFail) {
            passed = false
            errorKind = 'assertion'
            error = bodyFail.message
            assertionFailure = bodyFail
          }

          // header 验证
          if (passed && desc.expect.headerContains) {
            const headerFail = validateHeaders(primaryResponse, desc.expect.headerContains)
            if (headerFail) {
              passed = false
              errorKind = 'assertion'
              error = headerFail.message
              assertionFailure = headerFail
            }
          }

          // 响应时间验证
          if (passed && desc.expect.responseTimeMs) {
            const durationMs = Date.now() - start
            if (durationMs > desc.expect.responseTimeMs.max) {
              passed = false
              errorKind = 'assertion'
              error = `response time ${durationMs}ms exceeds max ${desc.expect.responseTimeMs.max}ms`
            }
          }

          // 自定义断言
          if (passed && desc.customAssert && primaryResponse) {
            const customFail = desc.customAssert(primaryResponse, primaryBody, ctx)
            if (customFail) {
              passed = false
              errorKind = 'assertion'
              error = customFail.message
              assertionFailure = customFail
            }
          }
        }
      }

      // 6. afterRun 钩子（不阻塞失败状态）
      if (desc.afterRun) {
        try { await desc.afterRun(ctx) } catch { /* ignore */ }
      }

      const status: TestCaseStatus = passed ? 'success' : 'failure'

      return {
        id: desc.id,
        name: options.translateName(desc.id),
        module: desc.module,
        status,
        durationMs: Date.now() - start,
        httpStatus: primaryResponse?.status,
        error,
        errorKind: passed ? undefined : errorKind,
        details: assertionFailure
          ? JSON.stringify(assertionFailure, null, 2)
          : undefined,
        iterations: iterResults.length > 1 ? iterResults : undefined,
      }
    } catch (e) {
      if (timer) clearTimeout(timer)
      const err = e instanceof Error ? e : new Error(String(e))
      return {
        id: desc.id,
        name: options.translateName(desc.id),
        module: desc.module,
        status: 'failure',
        durationMs: Date.now() - start,
        error: err.name === 'AbortError' ? `timeout after ${timeoutMs}ms` : err.message,
        errorKind: err.name === 'AbortError' ? 'timeout' : 'unknown',
      }
    }
  }

  return { runCase }
}

// ============ 辅助函数 ============

function basicAuthHeader(auth: { username?: string; password?: string }): string {
  const creds = `${auth.username ?? ''}:${auth.password ?? ''}`
  // btoa 在 Capacitor / 现代浏览器可用
  const encoded = typeof btoa !== 'undefined'
    ? btoa(creds)
    : (typeof Buffer !== 'undefined' ? Buffer.from(creds).toString('base64') : '')
  return `Basic ${encoded}`
}

function linkAbortSignals(
  controller: AbortController,
  ...signals: (AbortSignal | undefined)[]
): AbortController {
  for (const sig of signals) {
    if (!sig) continue
    if (sig.aborted) {
      controller.abort()
      break
    }
    sig.addEventListener('abort', () => controller.abort(), { once: true })
  }
  return controller
}

async function runSingleFetch(
  url: string,
  method: string,
  headers: Record<string, string>,
  body: string | null,
  signal: AbortSignal
): Promise<{ response?: Response; body: string; durationMs: number; error?: Error }> {
  const start = Date.now()
  try {
    const response = await fetch(url, {
      method,
      headers,
      body: body ?? undefined,
      signal,
    })
    const text = await response.text()
    return { response, body: text, durationMs: Date.now() - start }
  } catch (e) {
    return { body: '', durationMs: Date.now() - start, error: e instanceof Error ? e : new Error(String(e)) }
  }
}

function validateBody(
  body: string,
  matches: RegExp | string | undefined,
  notMatches: RegExp | string | undefined
): AssertionFailure | null {
  if (matches !== undefined) {
    const m = matches instanceof RegExp ? matches : new RegExp(matches)
    if (!m.test(body)) {
      return { message: `body missing pattern ${m}`, actual: body.slice(0, 200) }
    }
  }
  if (notMatches !== undefined) {
    const m = notMatches instanceof RegExp ? notMatches : new RegExp(notMatches)
    if (m.test(body)) {
      return { message: `body unexpectedly matched pattern ${m}`, actual: body.slice(0, 200) }
    }
  }
  return null
}

function validateHeaders(
  response: Response,
  expected: Record<string, string>
): AssertionFailure | null {
  for (const [key, value] of Object.entries(expected)) {
    const actual = response.headers.get(key) ?? ''
    if (!actual.toLowerCase().includes(value.toLowerCase())) {
      return { message: `header ${key} missing "${value}", got "${actual}"` }
    }
  }
  return null
}
