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
fun ChatPane(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit) {
    com.ryderdavid.nacre.ui.workspace.chat.ChatPaneM3(repoKey, thread, onThread, onProjects, onEvidence = LocalOpenEvidence.current)
}

@Composable
fun SpecSlot(repoKey: String, modifier: Modifier) {
    val repo = LocalContainer.current.repository
    val board by repo.board(repoKey).collectAsState()
    val changes = board.data?.changes?.changes.orEmpty()
    LazyColumn(modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        item { Text("Spec", style = MaterialTheme.typography.titleLargeEmphasized) }
        items(changes) { c -> Text(c.id + " · " + c.phase, style = MaterialTheme.typography.bodyLarge) }
    }
}

/** The project's Needs-you queue: every open decision, pending first, then held, failed and deferred. */
@Composable
fun DecisionQueue(repoKey: String, onClose: () -> Unit) {
    val repo = LocalContainer.current.repository
    val decisions by repo.decisions.collectAsState()
    val list = com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel.queueOrder(decisions[repoKey].orEmpty())
    val open = LocalOpenEvidence.current
    LazyColumn(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 32.dp)) {
        item { Text("Needs you", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.padding(bottom = 4.dp)) }
        if (list.isEmpty()) item { Text("Nothing needs you in this project.", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        items(list, key = { it.key }) { d -> com.ryderdavid.nacre.ui.workspace.decisions.DecisionCard(d, onEvidence = { onClose(); open(it) }) }
    }
}
