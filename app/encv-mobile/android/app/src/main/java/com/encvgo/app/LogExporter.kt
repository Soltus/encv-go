package com.encvgo.app

import android.content.Context
import android.content.Intent
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

object LogExporter {

    data class ExportResult(val success: Boolean, val path: String = "")

    fun export(context: Context): ExportResult {
        return try {
            val logDir = File(context.cacheDir, "encv_logs_export")
            logDir.mkdirs()
            val timestamp = SimpleDateFormat("yyyyMMdd_HHmmss", Locale.US).format(Date())

            val appLogFile = File(logDir, "app_log_${timestamp}.txt")
            appLogFile.writeText(AppLogger.getLogs().ifEmpty { "(no app log entries)" })

            val logcatFile = File(logDir, "logcat_${timestamp}.txt")
            try {
                val pid = android.os.Process.myPid()
                Runtime.getRuntime().exec(arrayOf("logcat", "-d", "--pid=$pid", "-t", "5000", "-v", "threadtime")).inputStream.bufferedReader().use { reader ->
                    logcatFile.outputStream().bufferedWriter().use { writer ->
                        var line: String?
                        while (reader.readLine().also { line = it } != null) { writer.write(line); writer.newLine() }
                    }
                }
            } catch (_: Exception) {}
            if (!logcatFile.exists() || logcatFile.length() == 0L) logcatFile.writeText("(logcat empty)\n")

            val goBackendLogFile = File(logDir, "go_backend_${timestamp}.txt")
            goBackendLogFile.writeText(EncvGoService.getOutputSnapshot().ifEmpty { "(Go backend not running or no output)" })

            val zipFile = File(context.cacheDir, "encv_logs_${timestamp}.zip")
            java.util.zip.ZipOutputStream(zipFile.outputStream()).use { zos ->
                fun addToZip(file: File, entryName: String) {
                    if (!file.exists()) return
                    try { zos.putNextEntry(java.util.zip.ZipEntry(entryName)); file.inputStream().use { it.copyTo(zos) }; zos.closeEntry() } catch (_: Exception) {}
                }
                addToZip(appLogFile, "app_log_${timestamp}.txt")
                addToZip(logcatFile, "logcat_${timestamp}.txt")
                addToZip(goBackendLogFile, "go_backend/go_backend_${timestamp}.txt")
                val devLogsJson = File(context.cacheDir, "devlogs_export.json")
                if (devLogsJson.exists()) addToZip(devLogsJson, "frontend/devlogs.json")
                val infoFile = File(logDir, "device_info_${timestamp}.txt")
                infoFile.writeText(buildString {
                    appendLine("Device: ${android.os.Build.MANUFACTURER} ${android.os.Build.MODEL}")
                    appendLine("Android: ${android.os.Build.VERSION.RELEASE} (API ${android.os.Build.VERSION.SDK_INT})")
                    appendLine("App: ${context.packageName}")
                    appendLine("GoBackend running: ${EncvGoService.isRunning}")
                    appendLine("GoBackend port: ${EncvGoService.lastKnownPort}")
                    appendLine("Timestamp: $timestamp")
                })
                addToZip(infoFile, "device_info_${timestamp}.txt")
            }

            appLogFile.delete(); logcatFile.delete(); goBackendLogFile.delete()

            val uri = androidx.core.content.FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", zipFile)
            val shareIntent = Intent(Intent.ACTION_SEND).apply {
                type = "application/zip"; putExtra(Intent.EXTRA_STREAM, uri); addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                putExtra(Intent.EXTRA_SUBJECT, "ENCV Logs $timestamp")
            }
            val chooser = Intent.createChooser(shareIntent, "\u5bfc\u51fa\u65e5\u5fd7")
            if (context !is android.app.Activity) chooser.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            context.startActivity(chooser)
            ExportResult(success = true, path = zipFile.absolutePath)
        } catch (e: Exception) { ExportResult(false) }
    }

    fun clear(context: Context): Boolean {
        return try {
            AppLogger.clear()
            try { Runtime.getRuntime().exec(arrayOf("logcat", "-c")) } catch (_: Exception) {}
            EncvGoService.clearOutputSnapshot()
            File(context.cacheDir, "encv_logs_export").listFiles()?.forEach { it.delete() }
            true
        } catch (e: Exception) { false }
    }

    fun openViewer(context: Context): Boolean {
        return try {
            val logFile = File(context.cacheDir, "encv_logs_export/app_log_latest.txt")
            logFile.parentFile?.mkdirs()
            logFile.writeText(AppLogger.getLogs().ifEmpty { "(no app log entries)" })
            val uri = androidx.core.content.FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", logFile)
            context.startActivity(Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, "text/plain"); addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                if (context !is android.app.Activity) addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            })
            true
        } catch (e: Exception) { false }
    }

    fun saveDevLogs(context: Context, logsJson: String): String? {
        return try {
            val file = File(context.cacheDir, "devlogs_export.json")
            file.writeText(logsJson)
            file.absolutePath
        } catch (e: Exception) { null }
    }

    private inline fun String.ifEmpty(default: () -> String): String = if (isEmpty()) default() else this
}
