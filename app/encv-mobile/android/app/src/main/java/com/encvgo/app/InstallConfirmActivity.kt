package com.encvgo.app

import android.content.Intent
import android.graphics.drawable.BitmapDrawable
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.res.ResourcesCompat
import androidx.core.graphics.drawable.toBitmap
import java.io.File
import java.text.DecimalFormat

class InstallConfirmActivity : ComponentActivity() {

    companion object {
        private const val TAG = "InstallConfirm"
        const val EXTRA_APK_PATH = "apk_path"
        const val EXTRA_FILE_NAME = "file_name"
    }

    @OptIn(ExperimentalMaterial3Api::class)
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        Log.i(TAG, "SATURATION-DEBUG onCreate: intent=$intent, extras=${intent.extras}")

        val apkPath = intent.getStringExtra(EXTRA_APK_PATH) ?: run {
            Log.w(TAG, "SATURATION-DEBUG onCreate: No APK path provided, finishing")
            setResult(RESULT_CANCELED)
            finish()
            return
        }
        val fileName = intent.getStringExtra(EXTRA_FILE_NAME) ?: File(apkPath).name

        val apkFile = File(apkPath)
        Log.i(TAG, "SATURATION-DEBUG onCreate: apkPath=$apkPath, exists=${apkFile.exists()}, size=${if (apkFile.exists()) apkFile.length() else -1}")

        setContent {
            MaterialTheme {
                InstallConfirmContent(
                    apkPath = apkPath,
                    fileName = fileName,
                    onConfirm = {
                        Log.i(TAG, "User confirmed installation: $fileName")
                        val resultIntent = Intent().apply {
                            putExtra(EXTRA_APK_PATH, apkPath)
                        }
                        setResult(RESULT_OK, resultIntent)
                        finish()
                    },
                    onCancel = {
                        Log.i(TAG, "User cancelled installation: $fileName")
                        setResult(RESULT_CANCELED)
                        finish()
                    },
                    onBack = {
                        setResult(RESULT_CANCELED)
                        finish()
                    }
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun InstallConfirmContent(
    apkPath: String,
    fileName: String,
    onConfirm: () -> Unit,
    onCancel: () -> Unit,
    onBack: () -> Unit
) {
    val context = LocalContext.current

    val apkInfo = remember(apkPath) { extractApkInfo(context, apkPath) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Rounded.ArrowBack,
                            contentDescription = "返回"
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                    titleContentColor = MaterialTheme.colorScheme.onSurface
                )
            )
        },
        containerColor = MaterialTheme.colorScheme.background
    ) { paddingValues ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(paddingValues)
                .padding(horizontal = 24.dp, vertical = 16.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (apkInfo.icon != null) {
                    Image(
                        bitmap = apkInfo.icon!!.toBitmap().asImageBitmap(),
                        contentDescription = "插件图标",
                        modifier = Modifier
                            .size(56.dp)
                            .clip(RoundedCornerShape(12.dp))
                    )
                } else {
                    Image(
                        painter = painterResource(id = android.R.mipmap.sym_def_app_icon),
                        contentDescription = "默认图标",
                        modifier = Modifier
                            .size(56.dp)
                            .clip(RoundedCornerShape(12.dp))
                    )
                }
                Spacer(modifier = Modifier.width(16.dp))
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        text = apkInfo.label.ifBlank { fileName },
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    if (apkInfo.versionName.isNotBlank()) {
                        Text(
                            text = "版本 ${apkInfo.versionName}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(24.dp))

            Row(
                verticalAlignment = Alignment.Top,
                modifier = Modifier
                    .fillMaxWidth()
                    .background(
                        MaterialTheme.colorScheme.tertiaryContainer,
                        RoundedCornerShape(10.dp)
                    )
                    .padding(horizontal = 12.dp, vertical = 8.dp)
            ) {
                Icon(
                    modifier = Modifier.size(20.dp),
                    imageVector = Icons.Rounded.Warning,
                    contentDescription = "警告",
                    tint = MaterialTheme.colorScheme.tertiary
                )
                Spacer(modifier = Modifier.width(8.dp))
                Text(
                    text = "即将安装以下插件到本应用，是否继续？",
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.Medium,
                    color = MaterialTheme.colorScheme.tertiary
                )
            }

            Spacer(modifier = Modifier.height(24.dp))

            Column(
                modifier = Modifier.fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                InfoRow("文件名", fileName)
                if (apkInfo.packageName.isNotBlank()) {
                    InfoRow("包名", apkInfo.packageName)
                }
                InfoRow("大小", formatFileSize(apkInfo.fileSize))
            }

            Spacer(modifier = Modifier.weight(1f))

            Column(
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Button(
                    onClick = onConfirm,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("确认安装")
                }
                TextButton(onClick = onCancel) {
                    Text("取消")
                }
            }
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f).padding(start = 16.dp)
        )
    }
}

private data class ApkInfo(
    val icon: BitmapDrawable? = null,
    val label: String = "",
    val packageName: String = "",
    val versionName: String = "",
    val fileSize: Long = 0L
)

private fun extractApkInfo(context: android.content.Context, apkPath: String): ApkInfo {
    try {
        val file = File(apkPath)
        if (!file.exists()) return ApkInfo(fileSize = 0L)

        val pm = context.packageManager
        val packageInfo = pm.getPackageArchiveInfo(apkPath, 0) ?: return ApkInfo(
            fileSize = file.length()
        )

        packageInfo.applicationInfo?.let { appInfo ->
            appInfo.sourceDir = apkPath
            appInfo.publicSourceDir = apkPath

            val iconResId = appInfo.icon
            val icon = try {
                val pluginRes = pm.getResourcesForApplication(appInfo)
                val drawable = ResourcesCompat.getDrawable(pluginRes, iconResId, null)
                if (drawable is BitmapDrawable) drawable else null
            } catch (e: Exception) {
                Log.w("InstallConfirm", "Failed to load icon from APK: $apkPath", e)
                null
            }

            return ApkInfo(
                icon = icon,
                label = appInfo.loadLabel(pm).toString(),
                packageName = packageInfo.packageName ?: "",
                versionName = packageInfo.versionName ?: "",
                fileSize = file.length()
            )
        }

        return ApkInfo(fileSize = file.length())
    } catch (e: Exception) {
        Log.w("InstallConfirm", "Failed to extract APK info: $apkPath", e)
        val file = File(apkPath)
        return ApkInfo(fileSize = if (file.exists()) file.length() else 0L)
    }
}

private fun formatFileSize(bytes: Long): String {
    if (bytes <= 0) return "未知"
    val mb = bytes / (1024.0 * 1024.0)
    return DecimalFormat("#.##").format(mb) + " MB"
}
