package com.encvgo.plugin.mpv

import android.app.Activity
import android.content.pm.ActivityInfo
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch

private val SPEED_ANCHORS = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f, 3f)
private const val CONTROLS_HIDE_DELAY_MS = 3000L
private const val POSITION_UPDATE_INTERVAL_MS = 1000L

@Composable
fun MpvPlayerScreen(
    filePath: String,
    fileName: String,
    mimeType: String,
    isExternal: Boolean,
    backendUrl: String,
    engine: MpvEngine,
    onBack: () -> Unit
) {
    var playerState by remember { mutableStateOf<PlayerState>(PlayerState.Idle) }
    var currentPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var showControls by remember { mutableStateOf(true) }
    var isLocked by remember { mutableStateOf(false) }
    var isFullscreen by remember { mutableStateOf(false) }
    var playbackSpeed by remember { mutableStateOf(1f) }
    var volume by remember { mutableStateOf(1f) }
    var subtitleTracks by remember { mutableStateOf<List<MpvEngine.TrackInfo>>(emptyList()) }
    var audioTracks by remember { mutableStateOf<List<MpvEngine.TrackInfo>>(emptyList()) }
    var currentSubtitleId by remember { mutableStateOf(-1) }
    var currentAudioId by remember { mutableStateOf(-1) }
    var bgPlaybackEnabled by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    LaunchedEffect(filePath) {
        startPlayback(
            filePath = filePath,
            fileName = fileName,
            isExternal = isExternal,
            mimeType = mimeType,
            engine = engine,
            backendUrl = backendUrl,
            onStateChange = { playerState = it },
            onError = { msg -> playerState = PlayerState.Error(classifyError(msg), msg) }
        )
    }

    DisposableEffect(engine) {
        val listener: (MpvEngine.State) -> Unit = { state ->
            when (state) {
                is MpvEngine.State.Playing -> playerState = PlayerState.Playing
                is MpvEngine.State.Paused -> playerState = PlayerState.Paused
                is MpvEngine.State.AudioOnly -> playerState = PlayerState.AudioOnly
                is MpvEngine.State.Ended -> playerState = PlayerState.Ended
                is MpvEngine.State.Error -> playerState = PlayerState.Error(classifyError(state.message), state.message)
                is MpvEngine.State.SurfaceReady -> { }
                is MpvEngine.State.WaitingSurface -> { }
                is MpvEngine.State.MpvReady -> { }
            }
        }
        engine.stateListener = listener
        onDispose {
            engine.stateListener = null
        }
    }

    LaunchedEffect(playerState) {
        if (playerState == PlayerState.Playing || playerState == PlayerState.Paused || playerState == PlayerState.AudioOnly) {
            showControls = true
            subtitleTracks = engine.getTrackList("sub")
            audioTracks = engine.getTrackList("audio")
        }
    }

    LaunchedEffect(playerState, showControls) {
        snapshotFlow { playerState }
            .distinctUntilChanged()
            .filter { it == PlayerState.Playing || it == PlayerState.AudioOnly }
            .collect {
                while (true) {
                    delay(CONTROLS_HIDE_DELAY_MS)
                    if (playerState == PlayerState.Playing || playerState == PlayerState.AudioOnly) {
                        showControls = false
                    }
                }
            }
    }

    LaunchedEffect(Unit) {
        while (true) {
            delay(POSITION_UPDATE_INTERVAL_MS)
            if (playerState == PlayerState.Playing || playerState == PlayerState.Paused || playerState == PlayerState.AudioOnly) {
                try {
                    currentPosition = engine.getPosition()
                    duration = engine.getDuration()
                } catch (_: Exception) {}
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            try { engine.pause() } catch (_: Exception) {}
            try { engine.destroy() } catch (_: Exception) {}
        }
    }

    Surface(
        modifier = Modifier.fillMaxSize(),
        color = Color.Transparent
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .pointerInput(Unit) {
                    detectTapGestures(
                        onTap = {
                            if (isLocked) {
                                isLocked = false
                            } else {
                                showControls = !showControls
                            }
                        }
                    )
                }
        ) {
            MpvControls(
                state = playerState,
                fileName = fileName.ifEmpty { "Unknown" },
                currentPosition = currentPosition,
                duration = duration,
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
                onPlayPause = {
                    when (playerState) {
                        is PlayerState.Playing -> {
                            engine.pause()
                            playerState = PlayerState.Paused
                        }
                        is PlayerState.Paused -> {
                            engine.resume()
                            playerState = PlayerState.Playing
                        }
                        is PlayerState.AudioOnly -> {
                            if (engine.isPlaying()) engine.pause() else engine.resume()
                        }
                        else -> {
                            scope.launch {
                                startPlayback(
                                    filePath = filePath,
                                    fileName = fileName,
                                    isExternal = isExternal,
                                    mimeType = mimeType,
                                    engine = engine,
                                    backendUrl = backendUrl,
                                    onStateChange = { playerState = it },
                                    onError = { msg -> playerState = PlayerState.Error(classifyError(msg), msg) }
                                )
                            }
                        }
                    }
                    showControls = true
                },
                onSeek = { ms ->
                    engine.seek(ms)
                    currentPosition = ms
                    showControls = true
                },
                onSeekDelta = { deltaMs ->
                    val newPos = (currentPosition + deltaMs).coerceIn(0, duration)
                    engine.seek(newPos)
                    currentPosition = newPos
                    showControls = true
                },
                onToggleLock = {
                    isLocked = !isLocked
                    showControls = true
                },
                onChangeSpeed = { newSpeed ->
                    playbackSpeed = newSpeed
                    engine.setProperty("speed", playbackSpeed.toString())
                    showControls = true
                },
                onToggleFullscreen = {
                    isFullscreen = !isFullscreen
                    val activity = context as? Activity ?: return@MpvControls
                    if (isFullscreen) {
                        activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
                        hideSystemUi(activity)
                    } else {
                        activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
                        showSystemUi(activity)
                    }
                    showControls = true
                },
                onVolumeChange = { vol ->
                    volume = vol
                    engine.setVolume(vol)
                    showControls = true
                },
                onSelectSubtitle = { id ->
                    if (id <= 0) {
                        engine.setTrack("sid", 0)
                    } else {
                        engine.setTrack("sid", id)
                    }
                    currentSubtitleId = id
                    showControls = true
                },
                onSelectAudio = { id ->
                    engine.setTrack("aid", id)
                    currentAudioId = id
                    showControls = true
                },
                onAddSubtitleFile = { path ->
                    engine.addSubtitleFile(path)
                    subtitleTracks = engine.getTrackList("sub")
                    showControls = true
                },
                onToggleBgPlayback = { enabled ->
                    bgPlaybackEnabled = enabled
                    showControls = true
                },
                onToggleSubtitle = {
                    engine.toggleSubtitleVisibility()
                    showControls = true
                },
                onCycleAudio = {
                    engine.cycleAudioTrack()
                    showControls = true
                },
                onRetry = {
                    scope.launch {
                        startPlayback(
                            filePath = filePath,
                            fileName = fileName,
                            isExternal = isExternal,
                            mimeType = mimeType,
                            engine = engine,
                            backendUrl = backendUrl,
                            onStateChange = { playerState = it },
                            onError = { msg -> playerState = PlayerState.Error(classifyError(msg), msg) }
                        )
                    }
                },
                onBack = {
                    engine.pause()
                    onBack()
                }
            )
        }
    }
}

internal suspend fun startPlayback(
    filePath: String,
    fileName: String,
    isExternal: Boolean,
    mimeType: String,
    engine: MpvEngine,
    backendUrl: String,
    onStateChange: (PlayerState) -> Unit,
    onError: (String) -> Unit
) {
    if (filePath.isEmpty()) {
        onError("File path is empty")
        return
    }

    onStateChange(PlayerState.Loading)

    try {
        val streamUrl = resolveStreamUrl(filePath, isExternal, backendUrl)

        if (streamUrl.isEmpty()) {
            onError("Unable to get stream URL")
            return
        }

        engine.play(streamUrl)
    } catch (e: Exception) {
        val msg = e.message ?: e.toString()
        onError("Playback error: $msg")
    }
}

internal suspend fun resolveStreamUrl(filePath: String, isExternal: Boolean, backendUrl: String): String {
    if (backendUrl.isEmpty()) {
        android.util.Log.w("MpvPlayer", "resolveStreamUrl: backendUrl is empty")
        return ""
    }
    val encodedPath = java.net.URLEncoder.encode(filePath, "UTF-8")
    val url = if (isExternal) {
        "$backendUrl/api/stream/external?path=$encodedPath"
    } else {
        "$backendUrl/stream?path=$encodedPath"
    }
    android.util.Log.i("MpvPlayer", "resolveStreamUrl: $url")
    return url
}

private fun hideSystemUi(activity: Activity) {
    WindowCompat.setDecorFitsSystemWindows(activity.window, false)
    val controller = WindowInsetsControllerCompat(
        activity.window, activity.window.decorView
    )
    controller.hide(WindowInsetsCompat.Type.systemBars())
    controller.systemBarsBehavior =
        WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
}

private fun showSystemUi(activity: Activity) {
    WindowCompat.setDecorFitsSystemWindows(activity.window, true)
}
