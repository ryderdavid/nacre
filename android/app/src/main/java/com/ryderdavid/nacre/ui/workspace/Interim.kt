package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.common.LocalContainer

// Interim pane contents for the chat-first shell; sections 5 to 9 replace each of them.

@Composable
fun ChatPane(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit, showHeader: Boolean = true) {
    com.ryderdavid.nacre.ui.workspace.chat.ChatPaneM3(repoKey, thread, onThread, onProjects, onEvidence = LocalOpenEvidence.current, showHeader = showHeader)
}

/**
 * The project's Needs-you queue: every open decision, pending first, then held, unresolved outcomes, work
 * still executing and deferred. Opened at a decision (a notification, or a change waiting in Spec), that
 * one is listed first.
 */
@Composable
fun DecisionQueue(repoKey: String, onClose: () -> Unit) {
    val repo = LocalContainer.current.repository
    val decisions by repo.decisions.collectAsState()
    val focus by repo.queueFocus.collectAsState()
    val focusId = focus?.takeIf { it.repoKey == repoKey }?.decisionId
    val list = com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel.queue(decisions[repoKey].orEmpty(), focusId)
    androidx.compose.runtime.DisposableEffect(repoKey) { onDispose { repo.clearQueueFocus(repoKey) } }
    val open = LocalOpenEvidence.current
    LazyColumn(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 32.dp)) {
        item { Text("Needs you", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.padding(bottom = 4.dp)) }
        if (list.isEmpty()) item { Text("Nothing needs you in this project.", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        items(list, key = { it.key }) { d -> com.ryderdavid.nacre.ui.workspace.decisions.DecisionCard(d, onEvidence = { onClose(); open(it) }) }
    }
}
