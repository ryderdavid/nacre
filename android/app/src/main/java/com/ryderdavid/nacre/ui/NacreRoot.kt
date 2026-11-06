package com.ryderdavid.nacre.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import com.ryderdavid.nacre.AppContainer
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.settings.PairRequest
import com.ryderdavid.nacre.ui.theme.NacreTheme
import com.ryderdavid.nacre.ui.workspace.ChatFirstShell

/** Something the activity was opened with: a pairing link or a run notification. */
sealed interface DeepLink {
    data class Pair(val request: PairRequest) : DeepLink
    data class OpenRun(val runKey: String) : DeepLink
    data class OpenRound(val roundKey: String) : DeepLink
    /** A change notification: the bead whose change view to open. */
    data class OpenChange(val repoKey: String, val beadId: String) : DeepLink
    /** A decision notification: the project's workspace, in [chatKey] when a chat raised it, with the queue pointed at it. */
    data class OpenDecision(val repoKey: String, val decisionId: String?, val chatKey: String?) : DeepLink
    /** A "Chat needs your OK" notification: the chat to open, at the card for [requestId]. */
    data class OpenChat(val chatKey: String, val requestId: String?) : DeepLink
}

@Composable
fun NacreRoot(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    val prefs by container.settings.settings.collectAsState(initial = null)
    NacreTheme(themeMode = prefs?.themeMode ?: com.ryderdavid.nacre.data.settings.ThemeMode.SYSTEM, palette = prefs?.palette) {
        CompositionLocalProvider(LocalContainer provides container) {
            ChatFirstShell(container, deepLink, onDeepLinkHandled)
        }
    }
}

