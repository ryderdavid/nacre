package com.ryderdavid.nacre.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

fun isPaneWide(maxWidth: Dp): Boolean = maxWidth >= 760.dp

/** Width of the side pane: hinge-aligned when the fold separates the screen, else user-sized. */
class PaneWidth(initial: Dp, private val min: Dp, private val max: Dp, val hingeLocked: Boolean) {
    private var w by mutableFloatStateOf(initial.value)
    val width: Dp get() = w.dp.coerceIn(min, max)
    var density = 1f
    fun resize(dxPx: Float) {
        if (hingeLocked) return
        w = (w + dxPx / density).coerceIn(min.value, max.value)
    }
}

@Composable
fun rememberPaneWidth(root: Rect, available: Dp, preferredDp: Int): PaneWidth {
    val density = LocalDensity.current
    val hinge = currentWindowAdaptiveInfo().windowPosture.hingeList.firstOrNull { it.isVertical && (it.isSeparating || it.isOccluding) }
    val hingeWidth = hinge?.let { with(density) { (root.right - it.bounds.right).toDp() } }?.takeIf { it > 280.dp && it < available - 300.dp }
    val max = (available - 320.dp).coerceAtLeast(320.dp)
    return remember(hingeWidth, available, preferredDp) {
        PaneWidth(hingeWidth ?: preferredDp.dp, min = 320.dp, max = max, hingeLocked = hingeWidth != null)
    }.also { it.density = density.density }
}

@Composable
fun PaneDivider(onDrag: (Float) -> Unit, onDragEnd: () -> Unit) {
    Box(
        Modifier.width(16.dp).fillMaxHeight().draggable(
            orientation = Orientation.Horizontal,
            state = rememberDraggableState { onDrag(it) },
            onDragStopped = { onDragEnd() },
        ),
        contentAlignment = Alignment.Center,
    ) {
        Box(Modifier.width(4.dp).height(48.dp).background(MaterialTheme.colorScheme.outline, RoundedCornerShape(2.dp)))
    }
}
