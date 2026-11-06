package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
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
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.board.BoardScreen
import com.ryderdavid.nacre.ui.chat.ConversationFrame
import com.ryderdavid.nacre.ui.chat.ConversationScreen
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.rounds.RoundDetail

// Interim pane contents for the chat-first shell; sections 5 to 9 replace each of them.

@Composable
fun ChatPane(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit) {
    ConversationScreen(thread, onBack = onProjects, onNew = {}, frame = ConversationFrame.Pane)
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

@Composable
fun BeadsSlot(repoKey: String, modifier: Modifier) {
    Column(modifier) {
        BoardScreen(repoKey, onBack = {}, onOpenBead = {}, onOpenChatBead = { _, _ -> }, onOpenRun = {}, onSwitchBoard = {}, onOpenRound = {})
    }
}

@Composable
fun WorkInspector(repoKey: String, work: WorkSummary, onClose: () -> Unit) {
    Column(Modifier.fillMaxWidth().fillMaxSize(0.9f)) {
        RoundDetail(Keys.of(Keys.host(repoKey), work.id), inPane = true, onBack = onClose, onOpenRun = {}, onOpenBead = { _, _ -> })
    }
}

@Composable
fun DecisionQueue(repoKey: String, onClose: () -> Unit) {
    val repo = LocalContainer.current.repository
    val decisions by repo.decisions.collectAsState()
    LazyColumn(Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        item { Text("Needs you", style = MaterialTheme.typography.titleLargeEmphasized) }
        items(decisions[repoKey].orEmpty()) { d -> Text(d.title, style = MaterialTheme.typography.bodyLarge) }
    }
}
