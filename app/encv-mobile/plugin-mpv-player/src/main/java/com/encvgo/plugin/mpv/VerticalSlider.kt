package com.encvgo.plugin.mpv

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import kotlin.math.roundToInt

@Composable
fun VerticalSlider(
    value: Float,
    onValueChange: (Float) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    trackWidth: Float = 4f,
    trackHeight: Float = 100f,
    thumbRadius: Float = 6f,
    activeColor: Color = MaterialTheme.colorScheme.primary,
    inactiveColor: Color = Color.White.copy(alpha = 0.2f),
    thumbColor: Color = MaterialTheme.colorScheme.primary
) {
    val density = LocalDensity.current
    var trackHeightPx by remember { mutableFloatStateOf(0f) }
    val clampedValue = value.coerceIn(0f, 1f)

    Box(
        modifier = modifier
            .width(with(density) { (trackWidth + 20).toDp() })
            .height(with(density) { trackHeight.toDp() }),
        contentAlignment = Alignment.BottomCenter
    ) {
        Box(
            modifier = Modifier
                .width(with(density) { trackWidth.toDp() })
                .fillMaxHeight()
                .clip(RoundedCornerShape(with(density) { (trackWidth / 2).toDp() }))
                .background(inactiveColor)
                .onSizeChanged { trackHeightPx = it.height.toFloat() }
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .fillMaxHeight(clampedValue)
                    .align(Alignment.BottomCenter)
                    .background(activeColor)
            )
        }

        if (trackHeightPx > 0f) {
            val thumbOffsetY = -(clampedValue * trackHeightPx - with(density) { thumbRadius.toDp().toPx() })

            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .offset { IntOffset(0, thumbOffsetY.roundToInt()) }
                    .width(with(density) { (thumbRadius * 2).toDp() })
                    .height(with(density) { (thumbRadius * 2).toDp() })
                    .clip(CircleShape)
                    .background(thumbColor)
                    .shadow(2.dp, CircleShape)
            )
        }

        Box(
            modifier = Modifier
                .width(with(density) { (trackWidth + 20).toDp() })
                .height(with(density) { trackHeight.toDp() })
                .pointerInput(enabled) {
                    if (!enabled) return@pointerInput
                    detectTapGestures { offset ->
                        val ratio = 1f - (offset.y / trackHeightPx).coerceIn(0f, 1f)
                        onValueChange(ratio.coerceIn(0f, 1f))
                    }
                }
                .pointerInput(enabled) {
                    if (!enabled) return@pointerInput
                    detectDragGestures { change, dragAmount ->
                        change.consume()
                        val delta = -dragAmount.y / trackHeightPx
                        val newValue = (clampedValue + delta).coerceIn(0f, 1f)
                        onValueChange(newValue)
                    }
                }
        )
    }
}
