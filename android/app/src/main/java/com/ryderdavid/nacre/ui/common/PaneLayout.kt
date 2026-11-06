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
