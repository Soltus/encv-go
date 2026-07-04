package com.encvgo.app

import android.content.Context
import android.net.Uri
import java.io.File

object UriUtils {

    fun copyUriToFile(context: Context, uri: Uri, targetDir: File): File? {
        return try {
            val cursor = context.contentResolver.query(uri, null, null, null, null)
            var displayName = ""
            if (cursor != null && cursor.moveToFirst()) {
                val nameIndex = cursor.getColumnIndex("_display_name")
                if (nameIndex >= 0) displayName = cursor.getString(nameIndex)
                cursor.close()
            }
            if (displayName.isEmpty()) displayName = uri.lastPathSegment ?: "file"

            val inputStream = context.contentResolver.openInputStream(uri) ?: return null
            targetDir.mkdirs()
            val targetFile = File(targetDir, displayName)
            targetFile.outputStream().use { output -> inputStream.copyTo(output) }
            inputStream.close()
            targetFile
        } catch (e: Exception) { null }
    }
}
