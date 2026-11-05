package com.ryderdavid.nacre.ui.common

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf

/**
 * A full-screen layer drawn over the app's content area, beside the rail and above the tab bar,
 * so a focused document (the change view) covers the screen without covering the navigation.
 * The app shell draws [content]; a screen fills it with [OverContent].
 */
class ScreenOverlay {
    private var owner: Any? = null
    var content by mutableStateOf<(@Composable () -> Unit)?>(null)
        private set

    internal fun show(token: Any, body: @Composable () -> Unit) { owner = token; content = body }

    internal fun clear(token: Any) {
        if (owner === token) { owner = null; content = null }
    }
}

val LocalScreenOverlay = staticCompositionLocalOf<ScreenOverlay?> { null }

/** Shows [content] in the shell's overlay layer while this is composed, or in place when there is none. */
@Composable
fun OverContent(content: @Composable () -> Unit) {
    val host = LocalScreenOverlay.current
    if (host == null) { content(); return }
    val latest by rememberUpdatedState(content)
    val token = remember { Any() }
    DisposableEffect(host, token) {
        host.show(token) { latest() }
        onDispose { host.clear(token) }
    }
}
