package com.ryderdavid.nacre.ui.board

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.unit.IntSize
import com.ryderdavid.nacre.data.model.Issue

/**
 * Long-press drag of bead cards between drop targets (board columns, or the lane dock on
 * a phone). All geometry is in window coordinates so targets and the floating card can
 * live in different parts of the layout.
 */
@Stable
class DragController(private val haptics: HapticFeedback, private val onDrop: (Issue, String) -> Unit) {
    var dragging by mutableStateOf<Issue?>(null)
        private set
    var sourceKey by mutableStateOf<String?>(null)
        private set
    var hoverKey by mutableStateOf<String?>(null)
        private set
    /** Pointer position in window coordinates. */
    var pointer by mutableStateOf(Offset.Zero)
        private set
    var grabOffset by mutableStateOf(Offset.Zero)
        private set
    var cardSize by mutableStateOf(IntSize.Zero)
        private set

    private val targets = mutableStateMapOf<String, Rect>()

    val isDragging get() = dragging != null

    fun registerTarget(key: String, bounds: Rect) {
        targets[key] = bounds
    }

    fun unregisterTarget(key: String) {
        targets.remove(key)
    }

    fun start(issue: Issue, from: String, cardBounds: Rect, local: Offset) {
        dragging = issue
        sourceKey = from
        grabOffset = local
        cardSize = IntSize(cardBounds.width.toInt(), cardBounds.height.toInt())
        pointer = cardBounds.topLeft + local
        hoverKey = from
        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
    }

    fun drag(delta: Offset) {
        pointer += delta
        val over = targets.entries.firstOrNull { it.value.contains(pointer) }?.key
        if (over != hoverKey) {
            hoverKey = over
            if (over != null) haptics.performHapticFeedback(HapticFeedbackType.SegmentTick)
        }
    }

    fun end() {
        val issue = dragging
        val target = hoverKey
        if (issue != null && target != null && target != sourceKey) {
            haptics.performHapticFeedback(HapticFeedbackType.Confirm)
            onDrop(issue, target)
        }
        cancel()
    }

    fun cancel() {
        dragging = null
        sourceKey = null
        hoverKey = null
    }
}

@Composable
fun rememberDragController(onDrop: (Issue, String) -> Unit): DragController {
    val haptics = LocalHapticFeedback.current
    return remember(haptics) { DragController(haptics, onDrop) }
}

/** Makes a card draggable after a long press. */
fun Modifier.dragSource(controller: DragController, issue: Issue, sourceKey: String): Modifier = composed {
    var coords by remember { mutableStateOf<LayoutCoordinates?>(null) }
    this
        .onGloballyPositioned { coords = it }
        .pointerInput(issue.id, sourceKey) {
            detectDragGesturesAfterLongPress(
                onDragStart = { local -> coords?.let { controller.start(issue, sourceKey, it.boundsInWindow(), local) } },
                onDrag = { change, delta -> change.consume(); controller.drag(delta) },
                onDragEnd = { controller.end() },
                onDragCancel = { controller.cancel() },
            )
        }
}

/** Registers a drop target; unregisters when it leaves composition. */
fun Modifier.dropTarget(controller: DragController, key: String): Modifier = composed {
    androidx.compose.runtime.DisposableEffect(key) { onDispose { controller.unregisterTarget(key) } }
    onGloballyPositioned { controller.registerTarget(key, it.boundsInWindow()) }
}
