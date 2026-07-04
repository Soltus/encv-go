package com.encvgo.app.workers

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Configuration
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.WorkManagerTestInitHelper
import java.util.concurrent.Executor
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EncvTaskCancelWorker Instrumented Test（真机/模拟器，打真实 Go 后端）。
 *
 * 2026-07-03 spec android-workmanager-split-start-stop Phase 3.5
 *
 * 约束（用户原话）：
 *   - 不 mock backend：Worker 打真实的 127.0.0.1:PORT HTTP 端点
 *   - 业务逻辑在 Go：Worker 只是薄包装
 *
 * 测试策略：
 *   - 使用 WorkManager TestInitHelper（SynchronousExecutor 同步执行）
 *   - 测试 enqueue 功能（Worker 是否正确入队）
 *   - 测试 Worker 在 Go 进程未运行时的 retry 行为
 *   - 注意：真机/模拟器测试需要 Go 后端已启动才能测完整 happy path，
 *         这里只测框架集成部分，真实 E2E 由 Cypress 覆盖
 */
@RunWith(AndroidJUnit4::class)
class EncvTaskCancelWorkerTest {

    private lateinit var context: Context
    private lateinit var workManager: WorkManager

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()

        val config = Configuration.Builder()
            .setMinimumLoggingLevel(android.util.Log.DEBUG)
            .setExecutor(SynchronousExecutor())
            .build()

        WorkManagerTestInitHelper.initializeTestWorkManager(context, config)
        workManager = WorkManager.getInstance(context)
    }

    @Test
    fun test_enqueue_createsUniqueWork() {
        val taskId = "test-task-001"
        val workName = EncvTaskCancelWorker.enqueue(context, taskId)

        assertTrue(workName.isNotEmpty(), "workName should not be empty")
        assertTrue(workName.contains(taskId), "workName should contain taskId")

        // 验证 WorkManager 中有对应的 work
        val future = workManager.getWorkInfosForUniqueWork(workName)
        val workInfos = future.get()
        assertNotNull(workInfos)
        assertTrue(workInfos.isNotEmpty(), "should have at least one work info")
    }

    @Test
    fun test_enqueue_idempotent_replacesExisting() {
        val taskId = "test-task-002"

        // 第一次入队
        val workName1 = EncvTaskCancelWorker.enqueue(context, taskId)
        // 第二次入队（同一个 taskId）
        val workName2 = EncvTaskCancelWorker.enqueue(context, taskId)

        assertEquals(workName1, workName2, "same taskId should produce same workName")

        // REPLACE 策略下，应该只有一个 work
        val workInfos = workManager.getWorkInfosForUniqueWork(workName1).get()
        assertEquals(1, workInfos.size, "REPLACE policy should result in single work")
    }

    @Test
    fun test_worker_withoutGoBackend_shouldRetry() {
        // 不启动 Go 后端，模拟 Go 进程已死的场景
        val taskId = "test-task-no-backend"

        val request = OneTimeWorkRequestBuilder<EncvTaskCancelWorker>()
            .setInputData(
                androidx.work.Data.Builder()
                    .putString("task_id", taskId)
                    .build()
            )
            .build()

        workManager.enqueue(request).result.get()

        // 同步执行器：work 应该已经跑完
        val workInfo = workManager.getWorkInfoById(request.id).get()
        assertNotNull(workInfo)

        // Go 后端不存在 → Worker 应该 retry（状态为 ENQUEUED 或 RETRY）
        // 注意：SynchronousExecutor 下 retry 不会立即重跑，而是回到 ENQUEUED 状态
        val state = workInfo.state
        assertTrue(
            state == WorkInfo.State.ENQUEUED || state == WorkInfo.State.RETRYING,
            "Worker should retry when Go backend is not running, got state=$state"
        )
    }

    @Test
    fun test_worker_nullTaskId_shouldFail() {
        val request = OneTimeWorkRequestBuilder<EncvTaskCancelWorker>()
            .build() // 不传 task_id

        workManager.enqueue(request).result.get()

        val workInfo = workManager.getWorkInfoById(request.id).get()
        assertNotNull(workInfo)
        assertEquals(WorkInfo.State.FAILED, workInfo.state, "null taskId should result in failure")
    }
}
