package com.encvgo.plugin.openlist

import android.content.ContentProvider
import android.content.ContentValues
import android.content.UriMatcher
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.util.Log

/**
 * Cross-process ContentProvider that exposes OpenList runtime status to the
 * host app (encv-go) via ContentResolver.query().
 *
 * Authority: com.encvgo.plugin.openlist.provider
 * URIs:
 *   - content://com.encvgo.plugin.openlist.provider/status → MatrixCursor snapshot
 *   - content://com.encvgo.plugin.openlist.provider/control → dispatch action
 *
 * The host process reads status every 3s (see useOpenListBridge.ts).
 * The control URI accepts action=start|stop|force_db_sync|set_admin_password.
 *
 * Implementation notes:
 *   - MatrixCursor.addRow needs Array<Any?> (NOT reified Array<Comparable<*> & Serializable>).
 *     Mixing Int/Long/String/Boolean in arrayOf() infers a noisy intersection type
 *     that breaks Kotlin's Array<out Any?> variance. We pass an explicit Any? array.
 *   - The bridge returns Map<String, Any?> (not Bundle), so we use direct map access
 *     with safe `as` casts (default fallback baked in).
 */
class OpenListStatusProvider : ContentProvider() {

    companion object {
        const val TAG = "OpenList-Provider"
        const val AUTHORITY = "com.encvgo.plugin.openlist.provider"
        const val PATH_STATUS = "status"
        const val PATH_CONTROL = "control"

        val STATUS_URI: Uri = Uri.parse("content://$AUTHORITY/$PATH_STATUS")
        val CONTROL_URI: Uri = Uri.parse("content://$AUTHORITY/$PATH_CONTROL")

        private const val CODE_STATUS = 1
        private const val CODE_CONTROL = 2

        const val COL_RUNNING = "running"
        const val COL_PORT = "port"
        const val COL_PID = "pid"
        const val COL_DATA_SIZE = "data_size_bytes"
        const val COL_LAST_ERROR = "last_error"
        const val COL_LAST_UPDATE = "last_update_ts"

        private const val ACTION_START = "start"
        private const val ACTION_STOP = "stop"
        private const val ACTION_FORCE_DB_SYNC = "force_db_sync"
        private const val ACTION_SET_ADMIN_PASSWORD = "set_admin_password"

        private val STATUS_COLUMNS = arrayOf(
            COL_RUNNING, COL_PORT, COL_PID, COL_DATA_SIZE, COL_LAST_ERROR, COL_LAST_UPDATE
        )
    }

    private val uriMatcher = UriMatcher(UriMatcher.NO_MATCH).apply {
        addURI(AUTHORITY, PATH_STATUS, CODE_STATUS)
        addURI(AUTHORITY, PATH_CONTROL, CODE_CONTROL)
    }

    override fun onCreate(): Boolean {
        Log.e(TAG, "[SAT-DBG][OpenList][Provider] onCreate() | caller=${callingPackage ?: "(unknown)"} | ts=${System.currentTimeMillis()}")
        val ctx = context
        if (ctx != null) {
            try {
                OpenListNativeService.init(ctx)
                Log.e(TAG, "[SAT-DBG][OpenList][Provider] OpenListNativeService.init() called")
            } catch (e: Throwable) {
                Log.e(TAG, "[SAT-DBG][OpenList][Provider] init() FAILED", e)
            }
        }
        return true
    }

    override fun query(
        uri: Uri,
        projection: Array<out String>?,
        selection: String?,
        selectionArgs: Array<out String>?,
        sortOrder: String?
    ): Cursor? {
        Log.e(TAG, "[SAT-DBG][OpenList][Provider] query() | uri=$uri | caller=${callingPackage ?: "(unknown)"} | ts=${System.currentTimeMillis()}")
        return when (uriMatcher.match(uri)) {
            CODE_STATUS -> {
                // Phase 26: 改用 OpenListNativeService.snapshot() (ProcessBuilder 启的 Go 进程状态)
                val snap = OpenListNativeService.snapshot()
                // Defensive reads: snapshot is Map<String, Any?>; coerce with safe defaults
                // so a missing field never NPEs the cross-process boundary.
                val running: Boolean = (snap["running"] as? Boolean) ?: false
                val port: Int = (snap["port"] as? Int) ?: 0
                val pid: Int = (snap["pid"] as? Int) ?: 0
                val dataSize: Long = (snap["data_size_bytes"] as? Long) ?: 0L
                val lastError: String = (snap["last_error"] as? String) ?: ""
                val lastUpdate: Long = (snap["last_update_ts"] as? Long) ?: 0L

                val cursor = MatrixCursor(STATUS_COLUMNS, 1)
                // Explicit Array<Any?> avoids the reified-type intersection issue
                // (Int + Long + String + Boolean → Array<Comparable<*> & Serializable>).
                cursor.addRow(
                    arrayOf<Any?>(
                        if (running) 1 else 0,
                        port,
                        pid,
                        dataSize,
                        lastError,
                        lastUpdate,
                    )
                )
                Log.e(TAG, "[SAT-DBG][OpenList][Provider] query(status) returned: running=$running port=$port pid=$pid dataSize=$dataSize")
                cursor
            }
            else -> {
                Log.w(TAG, "[SAT-DBG][OpenList][Provider] query() unknown URI: $uri")
                null
            }
        }
    }

    override fun insert(uri: Uri, values: ContentValues?): Uri? {
        Log.e(TAG, "[SAT-DBG][OpenList][Provider] insert() | uri=$uri | caller=${callingPackage ?: "(unknown)"} | values=$values")
        if (values == null) return null
        return when (uriMatcher.match(uri)) {
            CODE_CONTROL -> {
                val action = values.getAsString("action")
                Log.e(TAG, "[SAT-DBG][OpenList][Provider] insert(control) action=$action")
                when (action) {
                    ACTION_START -> {
                        // Phase 26: 改用 OpenListNativeService.start() (ProcessBuilder 启 Go server)
                        try { OpenListNativeService.start() } catch (e: Throwable) { Log.e(TAG, "[SAT-DBG][OpenList][Provider] start() FAILED", e) }
                    }
                    ACTION_STOP -> {
                        try { OpenListNativeService.shutdown(3000L) } catch (e: Throwable) { Log.e(TAG, "[SAT-DBG][OpenList][Provider] stop() FAILED", e) }
                    }
                    ACTION_FORCE_DB_SYNC -> {
                        // Phase 26: 改用 OpenListNativeService.notifyDbSynced() (内部快照更新)
                        try { OpenListNativeService.notifyDbSynced(System.currentTimeMillis()) } catch (e: Throwable) { Log.e(TAG, "[SAT-DBG][OpenList][Provider] forceDbSync() FAILED", e) }
                    }
                    ACTION_SET_ADMIN_PASSWORD -> {
                        val pwd = values.getAsString("password") ?: ""
                        try { OpenListNativeService.setAdminPassword(pwd) } catch (e: Throwable) { Log.e(TAG, "[SAT-DBG][OpenList][Provider] setAdminPassword() FAILED", e) }
                    }
                    else -> Log.w(TAG, "[SAT-DBG][OpenList][Provider] unknown action: $action")
                }
                Uri.parse("content://$AUTHORITY/$PATH_CONTROL/result/${System.currentTimeMillis()}")
            }
            else -> {
                Log.w(TAG, "[SAT-DBG][OpenList][Provider] insert() unknown URI: $uri")
                null
            }
        }
    }

    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun getType(uri: Uri): String? = null
}
