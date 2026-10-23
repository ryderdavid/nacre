package com.ryderdavid.nacre.ui.common

import androidx.compose.animation.core.Animatable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.offset
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/** Drag past this far, or flick down faster than this, to dismiss the sheet by its handle. */
private val DismissDragDistance = 72.dp
private const val DismissFlingVelocity = 800f

/**
 * A [ModalBottomSheet] that can only be dragged away by its handle (NAC-boc). Sheet gestures are
 * off so that scrolling or flinging the content never drags the sheet closed with half-finished
 * input in it; the drag handle carries the only dismiss gesture.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HandleDismissSheet(onDismiss: () -> Unit, content: @Composable ColumnScope.() -> Unit) {
    val scope = rememberCoroutineScope()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val handleDrag = remember { Animatable(0f) }
    val dismissDistancePx = with(LocalDensity.current) { DismissDragDistance.toPx() }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        modifier = Modifier.offset { IntOffset(0, handleDrag.value.roundToInt()) },
        sheetState = sheetState,
        sheetGesturesEnabled = false,
        dragHandle = {
            BottomSheetDefaults.DragHandle(
                modifier = Modifier.draggable(
                    state = rememberDraggableState { delta ->
                        scope.launch { handleDrag.snapTo((handleDrag.value + delta).coerceAtLeast(0f)) }
                    },
                    orientation = Orientation.Vertical,
                    onDragStopped = { velocity ->
                        if (handleDrag.value >= dismissDistancePx || velocity >= DismissFlingVelocity) {
                            sheetState.hide()
                            onDismiss()
                        } else {
                            handleDrag.animateTo(0f)
                        }
                    },
                ),
            )
        },
        content = content,
    )
}
