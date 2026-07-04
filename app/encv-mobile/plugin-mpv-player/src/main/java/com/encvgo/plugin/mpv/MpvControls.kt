package com.encvgo.plugin.mpv

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.FullscreenExit
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.automirrored.filled.VolumeMute
import androidx.compose.material.icons.outlined.MusicNote
import androidx.compose.material.icons.outlined.Subtitles
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.roundToInt
import com.encvgo.plugin.mpv.MpvEngine.TrackInfo

enum class ActivePanel { None, Settings, Subtitles, Audio, Volume }

@Composable
fun MpvControls(
    state: PlayerState,
    fileName: String,
    currentPosition: Long,
    duration: Long,
    isLocked: Boolean,
    isFullscreen: Boolean,
    playbackSpeed: Float,
    volume: Float = 1f,
    showControls: Boolean = true,
    subtitleTracks: List<TrackInfo> = emptyList(),
    audioTracks: List<TrackInfo> = emptyList(),
    currentSubtitleId: Int = -1,
    currentAudioId: Int = -1,
    bgPlaybackEnabled: Boolean = false,
    onPlayPause: () -> Unit,
    onSeek: (Long) -> Unit,
    onSeekDelta: (Long) -> Unit,
    onToggleLock: () -> Unit,
    onChangeSpeed: (Float) -> Unit,
    onToggleFullscreen: () -> Unit,
    onVolumeChange: (Float) -> Unit,
    onSelectSubtitle: (Int) -> Unit,
    onSelectAudio: (Int) -> Unit,
    onAddSubtitleFile: (String) -> Unit,
    onToggleBgPlayback: (Boolean) -> Unit,
    onToggleSubtitle: () -> Unit,
    onCycleAudio: () -> Unit,
    onRetry: () -> Unit,
    onBack: () -> Unit
) {
    val progress = if (duration > 0) currentPosition.toFloat() / duration else 0f
    val isPlaying = state == PlayerState.Playing || state == PlayerState.AudioOnly
    val isLoading = state == PlayerState.Idle || state == PlayerState.Loading
    val isError = state is PlayerState.Error

    when {
        isError -> {
            val errorState = state as PlayerState.Error
            ErrorLayout(
                fileName = fileName,
                errorType = errorState.errorType,
                detail = errorState.detail,
                onRetry = onRetry,
                onBack = onBack
            )
        }
        isLoading -> LoadingLayout(fileName = fileName, onBack = onBack)
        isLocked -> LockedLayout(
            progress = progress,
            currentPosition = currentPosition,
            duration = duration,
            onUnlock = onToggleLock,
            onSeek = { onSeek((it * duration).toLong()) }
        )
        state == PlayerState.AudioOnly -> AudioOnlyLayout(
            fileName = fileName,
            currentPosition = currentPosition,
            duration = duration,
            progress = progress,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
            volume = volume,
            showControls = showControls,
            onPlayPause = onPlayPause,
            onSeek = onSeek,
            onSeekDelta = onSeekDelta,
            onChangeSpeed = onChangeSpeed,
            onVolumeChange = onVolumeChange,
            onBack = onBack
        )
        else -> VideoPlaybackLayout(
            fileName = fileName,
            currentPosition = currentPosition,
            duration = duration,
            progress = progress,
            isPlaying = isPlaying,
            isLocked = isLocked,
            isFullscreen = isFullscreen,
            playbackSpeed = playbackSpeed,
            volume = volume,
            showControls = showControls,
            subtitleTracks = subtitleTracks,
            audioTracks = audioTracks,
            currentSubtitleId = currentSubtitleId,
            currentAudioId = currentAudioId,
            bgPlaybackEnabled = bgPlaybackEnabled,
            onPlayPause = onPlayPause,
            onSeek = onSeek,
            onSeekDelta = onSeekDelta,
            onToggleLock = onToggleLock,
            onChangeSpeed = onChangeSpeed,
            onToggleFullscreen = onToggleFullscreen,
            onVolumeChange = onVolumeChange,
            onSelectSubtitle = onSelectSubtitle,
            onSelectAudio = onSelectAudio,
            onAddSubtitleFile = onAddSubtitleFile,
            onToggleBgPlayback = onToggleBgPlayback,
            onBack = onBack
        )
    }
}

@Composable
private fun TopBar(
    title: String,
    showBack: Boolean = true,
    trailing: @Composable (() -> Unit)? = null,
    onBack: () -> Unit = {}
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.statusBars)
            .background(Brush.verticalGradient(listOf(Color.Black.copy(alpha = 0.6f), Color.Transparent)))
            .padding(start = 8.dp, end = 8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (showBack) {
            IconButton(onClick = onBack, modifier = Modifier.size(40.dp)) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "Back",
                    tint = Color.White
                )
            }
        }
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            color = Color.White,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.weight(1f).padding(horizontal = 4.dp)
        )
        trailing?.invoke()
    }
}

@Composable
private fun CenterPlayButton(
    isPlaying: Boolean,
    onPlayPause: () -> Unit,
    onSeekBack: (() -> Unit)? = null,
    onSeekForward: (() -> Unit)? = null
) {
    Row(
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (onSeekBack != null) {
            SeekDeltaButton(delta = "-10", onClick = onSeekBack)
            Spacer(Modifier.width(32.dp))
        }
        Surface(
            modifier = Modifier.size(72.dp),
            shape = CircleShape,
            color = Color.White.copy(alpha = 0.15f)
        ) {
            IconButton(
                onClick = onPlayPause,
                modifier = Modifier.size(72.dp)
            ) {
                Icon(
                    imageVector = if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                    contentDescription = if (isPlaying) "Pause" else "Play",
                    tint = Color.White.copy(alpha = 0.9f),
                    modifier = Modifier.size(40.dp)
                )
            }
        }
        if (onSeekForward != null) {
            Spacer(Modifier.width(32.dp))
            SeekDeltaButton(delta = "+10", onClick = onSeekForward)
        }
    }
}

@Composable
private fun SeekDeltaButton(delta: String, onClick: () -> Unit) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = Modifier.clickable(onClick = onClick).padding(8.dp)
    ) {
        Text(
            text = "${delta}s",
            color = Color.White.copy(alpha = 0.7f),
            fontSize = 14.sp,
            fontWeight = FontWeight.Bold
        )
    }
}

@Composable
private fun SideLockButton(isLocked: Boolean, onClick: () -> Unit) {
    Surface(
        modifier = Modifier.size(40.dp),
        shape = CircleShape,
        color = if (isLocked) MaterialTheme.colorScheme.primary.copy(alpha = 0.15f) else Color.White.copy(alpha = 0.08f),
        border = BorderStroke(
            1.dp,
            if (isLocked) MaterialTheme.colorScheme.primary.copy(alpha = 0.5f) else Color.White.copy(alpha = 0.15f)
        )
    ) {
        IconButton(onClick = onClick, modifier = Modifier.size(40.dp)) {
            Icon(
                imageVector = if (isLocked) Icons.Default.LockOpen else Icons.Default.Lock,
                contentDescription = if (isLocked) "Unlock" else "Lock",
                tint = if (isLocked) MaterialTheme.colorScheme.primary else Color.White.copy(alpha = 0.6f),
                modifier = Modifier.size(20.dp)
            )
        }
    }
}

@Composable
private fun BottomBar(
    progress: Float,
    currentPosition: Long,
    duration: Long,
    playbackSpeed: Float,
    isFullscreen: Boolean,
    volume: Float,
    activePanel: ActivePanel,
    onSeek: (Float) -> Unit,
    onChangeSpeed: (Float) -> Unit,
    onToggleFullscreen: () -> Unit,
    onVolumeChange: (Float) -> Unit,
    onPanelChange: (ActivePanel) -> Unit
) {
    Column(modifier = Modifier.fillMaxWidth().background(Brush.verticalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = 0.6f))))) {
        MpvProgressBar(
            progress = progress,
            currentPosition = currentPosition,
            duration = duration,
            onSeek = onSeek,
            modifier = Modifier.padding(bottom = 4.dp)
        )
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            SpeedChip(speed = playbackSpeed, onClick = { onChangeSpeed(playbackSpeed) })
            Spacer(Modifier.weight(1f))
            IconButton(
                onClick = { onPanelChange(if (activePanel == ActivePanel.Subtitles) ActivePanel.None else ActivePanel.Subtitles) },
                modifier = Modifier.size(36.dp)
            ) {
                Icon(
                    imageVector = Icons.Outlined.Subtitles,
                    contentDescription = "Subtitle",
                    tint = if (activePanel == ActivePanel.Subtitles) MaterialTheme.colorScheme.primary else Color.White.copy(alpha = 0.7f)
                )
            }
            IconButton(
                onClick = { onPanelChange(if (activePanel == ActivePanel.Audio) ActivePanel.None else ActivePanel.Audio) },
                modifier = Modifier.size(36.dp)
            ) {
                Icon(
                    imageVector = Icons.Outlined.MusicNote,
                    contentDescription = "Audio Track",
                    tint = if (activePanel == ActivePanel.Audio) MaterialTheme.colorScheme.primary else Color.White.copy(alpha = 0.7f)
                )
            }
            IconButton(
                onClick = { onPanelChange(if (activePanel == ActivePanel.Volume) ActivePanel.None else ActivePanel.Volume) },
                modifier = Modifier.size(36.dp)
            ) {
                Icon(
                    imageVector = if (volume > 0f) Icons.AutoMirrored.Filled.VolumeUp else Icons.AutoMirrored.Filled.VolumeMute,
                    contentDescription = "Volume",
                    tint = Color.White.copy(alpha = 0.7f)
                )
            }
            IconButton(onClick = onToggleFullscreen) {
                Icon(
                    imageVector = if (isFullscreen) Icons.Default.FullscreenExit else Icons.Default.Fullscreen,
                    contentDescription = "Fullscreen",
                    tint = Color.White.copy(alpha = 0.7f)
                )
            }
        }
    }
}

@Composable
private fun SpeedChip(speed: Float, onClick: () -> Unit) {
    OutlinedButton(
        onClick = onClick,
        modifier = Modifier.height(28.dp),
        shape = RoundedCornerShape(14.dp)
    ) {
        Text(
            text = "${if (speed == speed.toInt().toFloat()) "${speed.toInt()}.0" else String.format("%.2f", speed)}x",
            fontSize = 12.sp,
            color = MaterialTheme.colorScheme.primary
        )
    }
}

@Composable
private fun SettingsPanel(
    playbackSpeed: Float,
    bgPlaybackEnabled: Boolean,
    onChangeSpeed: (Float) -> Unit,
    onToggleBgPlayback: (Boolean) -> Unit,
    onDismiss: () -> Unit
) {
    val speedMin = 0.5f
    val speedMax = 3.0f
    val speedAnchors = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f, 3f)
    val snapThreshold = 0.08f

    fun snapSpeed(value: Float): Float {
        for (anchor in speedAnchors) {
            if (kotlin.math.abs(value - anchor) < snapThreshold) return anchor
        }
        return value
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.4f))
            .clickable(onClick = onDismiss),
        contentAlignment = Alignment.Center
    ) {
        Surface(
            modifier = Modifier.width(300.dp),
            shape = RoundedCornerShape(16.dp),
            color = Color(0xFF18181E).copy(alpha = 0.97f)
        ) {
            Column(modifier = Modifier.padding(16.dp)) {
                Text(
                    text = "Settings",
                    style = MaterialTheme.typography.labelMedium,
                    color = Color.White.copy(alpha = 0.5f),
                    fontWeight = FontWeight.Bold,
                    letterSpacing = 0.5.sp
                )
                Spacer(Modifier.height(12.dp))

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = "Playback Speed",
                        style = MaterialTheme.typography.labelSmall,
                        color = Color.White.copy(alpha = 0.5f)
                    )
                    Text(
                        text = "${if (playbackSpeed == playbackSpeed.toInt().toFloat()) "${playbackSpeed.toInt()}.0" else String.format("%.2f", playbackSpeed)}x",
                        fontFamily = FontFamily.Monospace,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = MaterialTheme.colorScheme.primary
                    )
                }
                Spacer(Modifier.height(8.dp))

                var sliderSpeed by remember { mutableStateOf(playbackSpeed) }
                Slider(
                    value = sliderSpeed,
                    onValueChange = {
                        val snapped = snapSpeed((it * 10).toInt() / 10f)
                        sliderSpeed = snapped
                    },
                    onValueChangeFinished = { onChangeSpeed(sliderSpeed) },
                    valueRange = speedMin..speedMax,
                    colors = SliderDefaults.colors(
                        thumbColor = MaterialTheme.colorScheme.primary,
                        activeTrackColor = MaterialTheme.colorScheme.primary,
                        inactiveTrackColor = Color.White.copy(alpha = 0.12f)
                    )
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    speedAnchors.forEach { anchor ->
                        Text(
                            text = "${anchor}x",
                            fontSize = 10.sp,
                            color = if (kotlin.math.abs(playbackSpeed - anchor) < 0.05f) MaterialTheme.colorScheme.primary else Color.White.copy(alpha = 0.3f),
                            fontWeight = if (kotlin.math.abs(playbackSpeed - anchor) < 0.05f) FontWeight.SemiBold else FontWeight.Normal
                        )
                    }
                }

                Spacer(Modifier.height(16.dp))
                HorizontalDivider(color = Color.White.copy(alpha = 0.06f))
                Spacer(Modifier.height(12.dp))

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = "Background Playback",
                            fontSize = 14.sp,
                            color = Color.White.copy(alpha = 0.9f)
                        )
                        Text(
                            text = "Audio only in background, resume position on return",
                            fontSize = 11.sp,
                            color = Color.White.copy(alpha = 0.4f),
                            lineHeight = 14.sp
                        )
                    }
                    Switch(
                        checked = bgPlaybackEnabled,
                        onCheckedChange = onToggleBgPlayback,
                        colors = SwitchDefaults.colors(
                            checkedTrackColor = MaterialTheme.colorScheme.primary,
                            checkedThumbColor = Color.White
                        )
                    )
                }
            }
        }
    }
}

@Composable
private fun TrackSelectionPopup(
    title: String,
    tracks: List<TrackInfo>,
    currentTrackId: Int,
    includeNone: Boolean = false,
    includeFilePicker: Boolean = false,
    onSelect: (Int) -> Unit,
    onAddFile: (() -> Unit)? = null,
    onDismiss: () -> Unit
) {
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = Color(0xFF18181E).copy(alpha = 0.96f)
    ) {
        Column(modifier = Modifier.padding(vertical = 6.dp)) {
            Text(
                text = title,
                modifier = Modifier.padding(horizontal = 14.dp, vertical = 6.dp),
                style = MaterialTheme.typography.labelMedium,
                color = Color.White.copy(alpha = 0.5f),
                fontWeight = FontWeight.Bold,
                letterSpacing = 0.5.sp
            )
            if (includeNone) {
                TrackItem(
                    label = "None",
                    selected = currentTrackId <= 0,
                    onClick = { onSelect(0); onDismiss() }
                )
            }
            tracks.forEach { track ->
                val label = track.title ?: track.lang ?: "Track ${track.id}"
                val suffix = if (track.codec != null) " (${track.codec})" else ""
                TrackItem(
                    label = label + suffix,
                    selected = track.id == currentTrackId,
                    onClick = { onSelect(track.id); onDismiss() }
                )
            }
            if (includeFilePicker && onAddFile != null) {
                HorizontalDivider(color = Color.White.copy(alpha = 0.08f), modifier = Modifier.padding(vertical = 4.dp))
                TrackItem(
                    label = "Choose subtitle file…",
                    selected = false,
                    isSpecial = true,
                    onClick = { onAddFile(); onDismiss() }
                )
            }
        }
    }
}

@Composable
private fun TrackItem(label: String, selected: Boolean, isSpecial: Boolean = false, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = label,
            fontSize = 13.sp,
            color = if (isSpecial) MaterialTheme.colorScheme.primary.copy(alpha = 0.9f)
                   else if (selected) MaterialTheme.colorScheme.primary
                   else Color.White.copy(alpha = 0.85f)
        )
        if (selected) {
            Icon(
                imageVector = Icons.Filled.Check,
                contentDescription = "Selected",
                tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(16.dp)
            )
        }
    }
}

@Composable
private fun VolumePopover(volume: Float, onVolumeChange: (Float) -> Unit) {
    Surface(
        shape = RoundedCornerShape(20.dp),
        color = Color(0xFF1E1E1E).copy(alpha = 0.92f)
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 12.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            var sliderVolume by remember { mutableStateOf(volume) }
            VerticalSlider(
                value = sliderVolume,
                onValueChange = { sliderVolume = it },
                modifier = Modifier.width(24.dp).height(100.dp),
                trackWidth = 4f,
                trackHeight = 100f,
                thumbRadius = 6f
            )
            Spacer(Modifier.height(6.dp))
            Text(
                text = "${(sliderVolume * 100).roundToInt()}",
                fontFamily = FontFamily.Monospace,
                fontSize = 10.sp,
                color = Color.White.copy(alpha = 0.7f)
            )
        }
    }
}

@Composable
private fun ErrorLayout(
    fileName: String,
    errorType: MpvError,
    detail: String,
    onRetry: () -> Unit,
    onBack: () -> Unit
) {
    Box(modifier = Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        Column(modifier = Modifier.align(Alignment.TopStart)) {
            TopBar(title = fileName, onBack = onBack)
        }
        Column(
            modifier = Modifier.align(Alignment.Center).padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(text = "⚠", fontSize = 56.sp, modifier = Modifier.padding(bottom = 16.dp))
            Text(
                text = "播放失败 / Playback Failed",
                style = MaterialTheme.typography.headlineSmall,
                color = MaterialTheme.colorScheme.onError,
                fontWeight = FontWeight.Bold
            )
            Spacer(Modifier.height(8.dp))
            Text(text = errorType.displayMessage(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (detail.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                Text(text = detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f), maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
            Spacer(Modifier.height(24.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedButton(onClick = onRetry) {
                    Icon(imageVector = Icons.Default.Refresh, contentDescription = "Retry", modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("重试 / Retry")
                }
                OutlinedButton(onClick = onBack) { Text("返回 / Back") }
            }
        }
    }
}

@Composable
private fun LoadingLayout(fileName: String, onBack: () -> Unit) {
    Box(modifier = Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        Column(modifier = Modifier.align(Alignment.TopStart)) {
            TopBar(title = fileName, onBack = onBack)
        }
        Column(modifier = Modifier.align(Alignment.Center), horizontalAlignment = Alignment.CenterHorizontally) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.primary, modifier = Modifier.size(52.dp))
            Spacer(Modifier.height(16.dp))
            Text(text = fileName.ifEmpty { "Loading..." }, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

@Composable
private fun LockedLayout(
    progress: Float,
    currentPosition: Long,
    duration: Long,
    onUnlock: () -> Unit,
    onSeek: (Float) -> Unit
) {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .align(Alignment.CenterStart)
                .offset(x = 12.dp)
        ) {
            SideLockButton(isLocked = true, onClick = onUnlock)
        }
        Column(
            modifier = Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = 0.5f))))
                .windowInsetsPadding(WindowInsets.navigationBars)
        ) {
            MpvProgressBar(progress = progress, currentPosition = currentPosition, duration = duration, onSeek = onSeek)
        }
    }
}

@Composable
private fun AudioOnlyLayout(
    fileName: String,
    currentPosition: Long,
    duration: Long,
    progress: Float,
    isPlaying: Boolean,
    playbackSpeed: Float,
    volume: Float,
    showControls: Boolean,
    onPlayPause: () -> Unit,
    onSeek: (Long) -> Unit,
    onSeekDelta: (Long) -> Unit,
    onChangeSpeed: (Float) -> Unit,
    onVolumeChange: (Float) -> Unit,
    onBack: () -> Unit
) {
    val alpha by animateFloatAsState(targetValue = if (showControls) 1f else 0.3f, label = "audioAlpha")

    Column(
        modifier = Modifier.fillMaxSize().alpha(alpha).background(MaterialTheme.colorScheme.background).padding(horizontal = 20.dp)
    ) {
        TopBar(title = fileName, onBack = onBack)
        Spacer(Modifier.weight(1f))
        Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(vertical = 32.dp)) {
            Box(modifier = Modifier.size(220.dp).background(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.large), contentAlignment = Alignment.Center) {
                Text(text = "\uD83C\uDFB5", fontSize = 80.sp)
            }
            Spacer(Modifier.height(28.dp))
            Text(text = fileName, style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.onSurface, maxLines = 2, overflow = TextOverflow.Ellipsis, fontWeight = FontWeight.Medium)
            Spacer(Modifier.height(8.dp))
            Text(text = formatTime(duration), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.weight(1f))
        Column {
            MpvProgressBar(progress = progress, currentPosition = currentPosition, duration = duration, onSeek = { onSeek((it * duration).toLong()) }, modifier = Modifier.padding(bottom = 20.dp))
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
                SeekDeltaButton(delta = "-10") { onSeekDelta(-10_000L) }
                Spacer(Modifier.width(32.dp))
                IconButton(onClick = onPlayPause, modifier = Modifier.size(64.dp)) {
                    Icon(imageVector = if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow, contentDescription = if (isPlaying) "Pause" else "Play", tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(40.dp))
                }
                Spacer(Modifier.width(32.dp))
                SeekDeltaButton(delta = "+10") { onSeekDelta(10_000L) }
                Spacer(Modifier.width(24.dp))
                SpeedChip(speed = playbackSpeed, onClick = { onChangeSpeed(playbackSpeed) })
                Spacer(Modifier.width(24.dp))
                IconButton(onClick = { onVolumeChange(if (volume > 0f) 0f else 1f) }, modifier = Modifier.size(36.dp)) {
                    Icon(imageVector = if (volume > 0f) Icons.AutoMirrored.Filled.VolumeUp else Icons.AutoMirrored.Filled.VolumeMute, contentDescription = "Volume", tint = Color.White.copy(alpha = 0.7f))
                }
            }
            Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
        }
    }
}

@Composable
private fun VideoPlaybackLayout(
    fileName: String,
    currentPosition: Long,
    duration: Long,
    progress: Float,
    isPlaying: Boolean,
    isLocked: Boolean,
    isFullscreen: Boolean,
    playbackSpeed: Float,
    showControls: Boolean,
    subtitleTracks: List<TrackInfo>,
    audioTracks: List<TrackInfo>,
    currentSubtitleId: Int,
    currentAudioId: Int,
    bgPlaybackEnabled: Boolean,
    onPlayPause: () -> Unit,
    onSeek: (Long) -> Unit,
    onSeekDelta: (Long) -> Unit,
    onToggleLock: () -> Unit,
    onChangeSpeed: (Float) -> Unit,
    onToggleFullscreen: () -> Unit,
    volume: Float = 1f,
    onVolumeChange: (Float) -> Unit,
    onSelectSubtitle: (Int) -> Unit,
    onSelectAudio: (Int) -> Unit,
    onAddSubtitleFile: (String) -> Unit,
    onToggleBgPlayback: (Boolean) -> Unit,
    onBack: () -> Unit
) {
    var activePanel by remember { mutableStateOf(ActivePanel.None) }
    val subtitlePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        uri?.let { onAddSubtitleFile(it.toString()) }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        AnimatedVisibility(
            visible = showControls,
            enter = fadeIn(),
            exit = fadeOut()
        ) {
            Box(modifier = Modifier.fillMaxSize()) {
                Column(modifier = Modifier.fillMaxSize()) {
                    TopBar(
                        title = fileName,
                        onBack = onBack,
                        trailing = {
                            IconButton(onClick = { activePanel = if (activePanel == ActivePanel.Settings) ActivePanel.None else ActivePanel.Settings }, modifier = Modifier.size(36.dp)) {
                                Icon(imageVector = Icons.Default.Settings, contentDescription = "Settings", tint = Color.White)
                            }
                        }
                    )
                    Spacer(Modifier.weight(1f))
                    Box(modifier = Modifier.fillMaxWidth().height(160.dp), contentAlignment = Alignment.Center) {
                        CenterPlayButton(isPlaying = isPlaying, onPlayPause = onPlayPause, onSeekBack = { onSeekDelta(-10_000L) }, onSeekForward = { onSeekDelta(10_000L) })
                    }
                    Spacer(Modifier.weight(1f))
                    BottomBar(
                        progress = progress,
                        currentPosition = currentPosition,
                        duration = duration,
                        playbackSpeed = playbackSpeed,
                        isFullscreen = isFullscreen,
                        volume = volume,
                        activePanel = activePanel,
                        onSeek = { onSeek((it * duration).toLong()) },
                        onChangeSpeed = onChangeSpeed,
                        onToggleFullscreen = onToggleFullscreen,
                        onVolumeChange = onVolumeChange,
                        onPanelChange = { panel -> activePanel = if (activePanel == panel) ActivePanel.None else panel }
                    )
                }

                Box(modifier = Modifier.align(Alignment.CenterStart).offset(x = 12.dp)) {
                    SideLockButton(isLocked = false, onClick = onToggleLock)
                }

                when (activePanel) {
                    ActivePanel.Subtitles -> {
                        Box(modifier = Modifier.align(Alignment.BottomEnd).offset(y = (-48).dp).padding(end = 12.dp)) {
                            TrackSelectionPopup(
                                title = "Subtitles",
                                tracks = subtitleTracks,
                                currentTrackId = currentSubtitleId,
                                includeNone = true,
                                includeFilePicker = true,
                                onSelect = onSelectSubtitle,
                                onAddFile = { subtitlePicker.launch(arrayOf("*/*")) },
                                onDismiss = { activePanel = ActivePanel.None }
                            )
                        }
                    }
                    ActivePanel.Audio -> {
                        Box(modifier = Modifier.align(Alignment.BottomEnd).offset(y = (-48).dp).padding(end = 12.dp)) {
                            TrackSelectionPopup(
                                title = "Audio Track",
                                tracks = audioTracks,
                                currentTrackId = currentAudioId,
                                onSelect = onSelectAudio,
                                onDismiss = { activePanel = ActivePanel.None }
                            )
                        }
                    }
                    ActivePanel.Volume -> {
                        Box(modifier = Modifier.align(Alignment.BottomEnd).offset(y = (-48).dp).padding(end = 12.dp)) {
                            VolumePopover(volume = volume, onVolumeChange = onVolumeChange)
                        }
                    }
                    ActivePanel.Settings -> {
                        SettingsPanel(
                            playbackSpeed = playbackSpeed,
                            bgPlaybackEnabled = bgPlaybackEnabled,
                            onChangeSpeed = onChangeSpeed,
                            onToggleBgPlayback = onToggleBgPlayback,
                            onDismiss = { activePanel = ActivePanel.None }
                        )
                    }
                    ActivePanel.None -> {}
                }
            }
        }
    }
}
