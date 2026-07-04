package com.encvgo.plugin.mpv

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.MusicNote
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.automirrored.filled.VolumeMute
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay

private val SPEED_OPTIONS = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f)
private const val POSITION_UPDATE_INTERVAL_MS = 1000L

@Composable
fun MpvAudioPlayerScreen(
    filePath: String,
    fileName: String,
    mimeType: String,
    isExternal: Boolean,
    backendUrl: String,
    engine: MpvEngine,
    onBack: () -> Unit
) {
    var playerState by remember { mutableStateOf<PlayerState>(PlayerState.Loading) }
    var currentPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var playbackSpeed by remember { mutableStateOf(1f) }
    var volume by remember { mutableFloatStateOf(1f) }
    val isPlaying = playerState == PlayerState.AudioOnly || playerState == PlayerState.Playing
    val progress = if (duration > 0) currentPosition.toFloat() / duration else 0f

    val gradientBg = Brush.linearGradient(
        colors = listOf(Color(0xFF1A1A2E), Color(0xFF16213E), Color(0xFF0F3460)),
        start = Offset(0f, 0f),
        end = Offset(600f, 1200f)
    )

    LaunchedEffect(filePath) {
        try {
            val streamUrl = resolveStreamUrl(filePath, isExternal, backendUrl)
            if (streamUrl.isNotEmpty()) {
                engine.playAudioOnly(streamUrl)
            } else {
                playerState = PlayerState.Error(MpvError.FILE_NOT_FOUND, "Empty stream URL")
            }
        } catch (e: Exception) {
            playerState = PlayerState.Error(classifyError(e.message ?: ""), e.message ?: "")
        }
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

    LaunchedEffect(Unit) {
        while (true) {
            delay(POSITION_UPDATE_INTERVAL_MS)
            if (playerState == PlayerState.AudioOnly || playerState == PlayerState.Playing) {
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

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(gradientBg)
            .windowInsetsPadding(WindowInsets.statusBars)
            .padding(horizontal = 20.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            IconButton(onClick = { engine.pause(); onBack() }, modifier = Modifier.size(40.dp)) {
                Icon(imageVector = Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = Color.White)
            }
            Text(
                text = "Now Playing",
                modifier = Modifier.weight(1f),
                textAlign = TextAlign.Center,
                color = Color.White.copy(alpha = 0.6f),
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                letterSpacing = 1.sp
            )
            Spacer(Modifier.size(40.dp))
        }

        Spacer(Modifier.weight(1f))

        Box(
            modifier = Modifier.fillMaxWidth(),
            contentAlignment = Alignment.Center
        ) {
            SpinningDisc(isPlaying = isPlaying)
        }

        Spacer(Modifier.height(28.dp))

        Column(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = fileName.ifEmpty { "Unknown Track" },
                color = Color.White,
                fontSize = 20.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Spacer(Modifier.height(4.dp))
            Text(
                text = "Unknown Artist",
                color = Color.White.copy(alpha = 0.5f),
                fontSize = 14.sp
            )
        }

        Spacer(Modifier.weight(1f))

        MpvProgressBar(
            progress = progress,
            currentPosition = currentPosition,
            duration = duration,
            onSeek = { ratio ->
                val newPos = (ratio * duration).toLong()
                engine.seek(newPos)
                currentPosition = newPos
            },
            modifier = Modifier.padding(bottom = 16.dp)
        )

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.Center,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Surface(
                modifier = Modifier.size(48.dp),
                shape = CircleShape,
                color = Color.White.copy(alpha = 0.08f)
            ) {
                IconButton(onClick = {
                    val newPos = (currentPosition - 10_000L).coerceAtLeast(0)
                    engine.seek(newPos)
                    currentPosition = newPos
                }) {
                    Icon(
                        imageVector = Icons.Default.MusicNote,
                        contentDescription = "Skip Back",
                        tint = Color.White.copy(alpha = 0.8f),
                        modifier = Modifier.size(28.dp)
                    )
                }
            }
            Spacer(Modifier.width(36.dp))
            val playBtnBrush = Brush.linearGradient(listOf(Color(0xFFBB86FC), Color(0xFF7C4DFF)))
            Box(
                modifier = Modifier
                    .size(64.dp)
                    .shadow(20.dp, CircleShape, ambientColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.4f))
                    .clip(CircleShape)
                    .background(playBtnBrush)
                    .clickable {
                        if (isPlaying) {
                            engine.pause()
                            playerState = PlayerState.Paused
                        } else {
                            engine.resume()
                            playerState = PlayerState.AudioOnly
                        }
                    },
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                    contentDescription = if (isPlaying) "Pause" else "Play",
                    tint = Color.White,
                    modifier = Modifier.size(36.dp)
                )
            }
            Spacer(Modifier.width(36.dp))
            Surface(
                modifier = Modifier.size(48.dp),
                shape = CircleShape,
                color = Color.White.copy(alpha = 0.08f)
            ) {
                IconButton(onClick = {
                    val newPos = (currentPosition + 10_000L).coerceAtMost(duration)
                    engine.seek(newPos)
                    currentPosition = newPos
                }) {
                    Icon(
                        imageVector = Icons.Default.MusicNote,
                        contentDescription = "Skip Forward",
                        tint = Color.White.copy(alpha = 0.8f),
                        modifier = Modifier.size(28.dp)
                    )
                }
            }
        }

        Spacer(Modifier.height(20.dp))

        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp),
            horizontalArrangement = Arrangement.Center,
            verticalAlignment = Alignment.CenterVertically
        ) {
            OutlinedButton(
                onClick = {
                    val idx = SPEED_OPTIONS.indexOf(playbackSpeed)
                    playbackSpeed = SPEED_OPTIONS[(idx + 1) % SPEED_OPTIONS.size]
                    engine.setProperty("speed", playbackSpeed.toString())
                },
                modifier = Modifier.height(32.dp),
                shape = CircleShape
            ) {
                Text(
                    text = "${if (playbackSpeed == playbackSpeed.toInt().toFloat()) "${playbackSpeed.toInt()}.0" else String.format("%.2f", playbackSpeed)}x",
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = MaterialTheme.colorScheme.primary
                )
            }
            Spacer(Modifier.width(12.dp))
            IconButton(
                onClick = { volume = if (volume > 0f) 0f else 1f; engine.setVolume(volume) },
                modifier = Modifier.size(32.dp)
            ) {
                Icon(
                    imageVector = if (volume > 0f) Icons.AutoMirrored.Filled.VolumeUp else Icons.AutoMirrored.Filled.VolumeMute,
                    contentDescription = "Volume",
                    tint = Color.White.copy(alpha = 0.7f),
                    modifier = Modifier.size(20.dp)
                )
            }
            Spacer(Modifier.width(8.dp))
            Box(modifier = Modifier.width(100.dp)) {
                var sliderVol by remember { mutableFloatStateOf(volume) }
                Slider(
                    value = sliderVol,
                    onValueChange = { sliderVol = it; volume = it; engine.setVolume(it) },
                    valueRange = 0f..1f,
                    modifier = Modifier.height(4.dp),
                    colors = SliderDefaults.colors(
                        thumbColor = MaterialTheme.colorScheme.primary,
                        activeTrackColor = MaterialTheme.colorScheme.primary,
                        inactiveTrackColor = Color.White.copy(alpha = 0.15f)
                    )
                )
            }
        }

        Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
    }
}

@Composable
private fun SpinningDisc(isPlaying: Boolean) {
    val infiniteTransition = rememberInfiniteTransition(label = "discSpin")
    val rotation by infiniteTransition.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 8000, easing = LinearEasing),
            repeatMode = RepeatMode.Restart
        ),
        label = "discRotation"
    )

    val displayRotation = if (isPlaying) rotation else 0f

    Box(
        modifier = Modifier
            .size(200.dp)
            .rotate(displayRotation)
            .clip(CircleShape)
            .shadow(40.dp, CircleShape, ambientColor = Color(0xFFBB86FC).copy(alpha = 0.15f)),
        contentAlignment = Alignment.Center
    ) {
        val discBrush = Brush.radGradient(
            colors = listOf(
                Color(0xFF2A2A3E), Color(0xFF1E1E30), Color(0xFF2A2A3E),
                Color(0xFF1E1E30), Color(0xFF2A2A3E), Color(0xFF1E1E30),
                Color(0xFF2A2A3E), Color(0xFF1A1A2E)
            ),
            radius = 100f
        )
        Box(modifier = Modifier.fillMaxSize().background(discBrush))

        Box(
            modifier = Modifier
                .size(140.dp)
                .clip(CircleShape)
                .border(1.dp, Color.White.copy(alpha = 0.04f), CircleShape)
        )
        Box(
            modifier = Modifier
                .size(180.dp)
                .clip(CircleShape)
                .border(1.dp, Color.White.copy(alpha = 0.04f), CircleShape)
        )

        val centerBrush = Brush.linearGradient(listOf(Color(0xFFBB86FC), Color(0xFF7C4DFF)))
        Box(
            modifier = Modifier
                .size(56.dp)
                .background(centerBrush, CircleShape),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.MusicNote,
                contentDescription = null,
                tint = Color.White.copy(alpha = 0.6f),
                modifier = Modifier.size(32.dp)
            )
        }
    }
}

private fun Brush.Companion.radGradient(colors: List<Color>, radius: Float): Brush {
    return radialGradient(colors = colors, center = Offset(radius, radius), radius = radius)
}
