package com.ryderdavid.nacre.ui.workspace

import androidx.compose.runtime.Composable

/** The workspace's Chat pane. Needs you is a conversation there: see [com.ryderdavid.nacre.ui.workspace.chat.AskSlot]. */
@Composable
fun ChatPane(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit, showHeader: Boolean = true) {
    com.ryderdavid.nacre.ui.workspace.chat.ChatPaneM3(repoKey, thread, onThread, onProjects, onEvidence = LocalOpenEvidence.current, showHeader = showHeader)
}
