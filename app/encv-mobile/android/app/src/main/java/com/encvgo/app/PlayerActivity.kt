package com.encvgo.app

import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.OpenableColumns
import android.util.Log
import androidx.appcompat.app.AppCompatActivity
import java.io.File
import java.io.FileOutputStream

class PlayerActivity : AppCompatActivity() {
    companion object {
        private const val TAG = "PlayerActivity"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val (filePath, fileName, mimeType, isExternal) = resolveFileInfo(intent)
        val mode = intent.getStringExtra(PlayerEntry.EXTRA_MODE) ?: ""
        Log.i(TAG, "onCreate: path=$filePath, name=$fileName, mimeType=$mimeType, external=$isExternal, mode=$mode")

        if (filePath.isNotEmpty()) {
            PlayerEntry.play(this, filePath, fileName, mimeType, isExternal, mode)
        } else {
            Log.w(TAG, "No valid file path resolved from intent")
        }

        finish()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)

        val (filePath, fileName, mimeType, isExternal) = resolveFileInfo(intent)
        val mode = intent.getStringExtra(PlayerEntry.EXTRA_MODE) ?: ""
        Log.i(TAG, "onNewIntent: path=$filePath, name=$fileName, mimeType=$mimeType, external=$isExternal, mode=$mode")

        if (filePath.isNotEmpty()) {
            PlayerEntry.play(this, filePath, fileName, mimeType, isExternal, mode)
        }

        finish()
    }

    private fun resolveFileInfo(intent: Intent?): ResolvedFileInfo {
        if (intent == null) return ResolvedFileInfo("", "", "", false)

        val internalPath = intent.getStringExtra("file_path")
        if (!internalPath.isNullOrEmpty()) {
            return ResolvedFileInfo(
                internalPath,
                intent.getStringExtra("file_name") ?: File(internalPath).name,
                intent.getStringExtra("mime_type") ?: intent.getStringExtra("file_mime_type") ?: "",
                false
            )
        }

        val uri = intent.data ?: if (Build.VERSION.SDK_INT >= 33) {
            intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
        } else {
            @Suppress("DEPRECATION")
            intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)
        }

        if (uri == null) {
            Log.w(TAG, "resolveFileInfo: no URI found in intent")
            return ResolvedFileInfo("", "", "", false)
        }

        val intentMimeType = intent.type ?: ""
        Log.i(TAG, "resolveFileInfo: URI scheme=${uri.scheme}, uri=$uri, mimeType=$intentMimeType")

        return when (uri.scheme) {
            "content" -> resolveContentUri(uri, intentMimeType)
            "file" -> resolveFileUri(uri, intentMimeType)
            else -> {
                Log.w(TAG, "resolveFileInfo: unsupported URI scheme: ${uri.scheme}")
                ResolvedFileInfo("", "", "", false)
            }
        }
    }

    private fun resolveContentUri(uri: Uri, fallbackMimeType: String): ResolvedFileInfo {
        var fileName = ""
        var filePath = ""
        contentResolver.query(uri, null, null, null, null)?.use { cursor ->
            if (cursor.moveToFirst()) {
                val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                if (nameIndex >= 0) {
                    fileName = cursor.getString(nameIndex)
                }
            }
        }
        if (fileName.isEmpty()) {
            fileName = uri.lastPathSegment ?: "unknown_file"
        }

        val projection = arrayOf(android.provider.MediaStore.MediaColumns.DATA)
        try {
            contentResolver.query(uri, projection, null, null, null)?.use { cursor ->
                if (cursor.moveToFirst()) {
                    val dataIndex = cursor.getColumnIndexOrThrow(android.provider.MediaStore.MediaColumns.DATA)
                    filePath = cursor.getString(dataIndex)
                }
            }
        } catch (_: Exception) {}

        if (filePath.isEmpty() || !File(filePath).exists()) {
            filePath = copyContentToCache(uri)
        }

        val mimeType = if (fallbackMimeType.isNotEmpty()) fallbackMimeType else run {
            try { contentResolver.getType(uri) ?: "" } catch (_: Exception) { "" }
        }

        return ResolvedFileInfo(filePath, fileName, mimeType, true)
    }

    private fun resolveFileUri(uri: Uri, fallbackMimeType: String): ResolvedFileInfo {
        val path = uri.path ?: ""
        val fileName = uri.lastPathSegment ?: File(path).name
        val mimeType = if (fallbackMimeType.isNotEmpty()) fallbackMimeType else run {
            try { contentResolver.getType(uri) ?: "" } catch (_: Exception) { "" }
        }
        return ResolvedFileInfo(path, fileName, mimeType, false)
    }

    private fun copyContentToCache(uri: Uri): String {
        val fileName = try {
            contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                if (cursor.moveToFirst()) {
                    val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (nameIndex >= 0) cursor.getString(nameIndex) else null
                } else null
            }
        } catch (_: Exception) {
            null
        } ?: uri.lastPathSegment ?: "cached_file"

        val cacheDir = File(cacheDir, "player_cache")
        cacheDir.mkdirs()
        val destFile = File(cacheDir, fileName)
        if (destFile.exists()) {
            destFile.delete()
        }
        try {
            contentResolver.openInputStream(uri)?.use { input ->
                FileOutputStream(destFile).use { output ->
                    val buffer = ByteArray(8192)
                    var len: Int
                    while (input.read(buffer).also { len = it } != -1) {
                        output.write(buffer, 0, len)
                    }
                }
            }
            Log.i(TAG, "copyContentToCache: copied to ${destFile.absolutePath}")
        } catch (e: Exception) {
            Log.e(TAG, "copyContentToCache failed", e)
            return ""
        }
        return destFile.absolutePath
    }

    private data class ResolvedFileInfo(
        val filePath: String,
        val fileName: String,
        val mimeType: String,
        val isExternal: Boolean
    )
}
