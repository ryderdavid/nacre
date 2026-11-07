package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialShapes
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SplitButtonDefaults
import androidx.compose.material3.SplitButtonLayout
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.toShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.WorkSummary
import androidx.compose.ui.semantics.heading
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.LinearProgressIndicator
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.StatusPill
import com.ryderdavid.nacre.ui.workspace.segmentShape
import kotlinx.coroutines.launch

/** The project's jobs and chats in a sheet, for the one-pane layouts; the board's drawer shows the same list as its Jobs tab. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ThreadSwitcher(repoKey: String, current: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val openEvidence = com.ryderdavid.nacre.ui.workspace.LocalOpenEvidence.current
    ModalBottomSheet(onDismissRequest = onDismiss) {
        JobsPane(
            repoKey, current, onPick = onPick,
            onInspect = { w -> onDismiss(); openEvidence(com.ryderdavid.nacre.data.model.DecisionEvidence(label = w.title, kind = w.kind, ref = w.id)) },
            modifier = Modifier.fillMaxWidth().navigationBarsPadding(),
        )
    }
}

/**
 * Jobs: each chat that started or created work, with what it is working on and how far it has come, then
 * work started from the board; below them the project's other conversations. New chat with the default
 * agent, the other in its menu.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun JobsPane(repoKey: String, current: String?, onPick: (String) -> Unit, onInspect: (WorkSummary) -> Unit, modifier: Modifier = Modifier, showTitle: Boolean = true) {
    val container = LocalContainer.current
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val name = repo.repos.collectAsState().value.data?.firstOrNull { it.key == repoKey }?.name ?: Keys.id(repoKey)
    val decisions by repo.decisions.collectAsState()
    val work by repo.work.collectAsState()
    val board by repo.board(repoKey).collectAsState()
    val open = repo.siblingKeys(repoKey).plus(repoKey).flatMap { decisions[it].orEmpty() }.filter { it.pending && !it.chatId.isNullOrBlank() }
        .groupingBy { it.chatId.orEmpty() }.eachCount()
    val jobs = remember(listing.chats, work[repoKey], board.data, open) {
        Jobs.build(listing.chats, repoKey, repo.siblingKeys(repoKey), work[repoKey].orEmpty(), board.data, open)
    }
    val scope = container.scope
    val start: (String) -> Unit = { agent ->
        scope.launch {
            runCatching { container.chats.link.createChat(Keys.host(repoKey), CreateChat(agent, Keys.id(repoKey), null)).copy(hostId = Keys.host(repoKey)).key }
                .onSuccess(onPick).onFailure { repo.notify("Couldn't start the chat: ${it.message}") }
        }
    }
    // New chat floats at the bottom right, where it sits on Projects; the list scrolls clear of it.
    Box(modifier) {
      Column(Modifier.padding(horizontal = 12.dp)) {
        // In the board's drawer the Jobs tab already names it.
        if (showTitle) Text("$name jobs", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.padding(start = 12.dp, top = 4.dp, bottom = 4.dp))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(2.dp), contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 88.dp)) {
            item { SectionLabel(if (jobs.jobs.isEmpty()) "Jobs" else "Jobs · ${jobs.jobs.size}") }
            if (jobs.jobs.isEmpty()) item {
                Text(
                    "A chat becomes a job once it starts work or files beads or a change. Attach a card and ask for it.",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                )
            }
            items(jobs.jobs.size) { i ->
                val j = jobs.jobs[i]
                JobRowView(j, selected = j.chatKey != null && j.chatKey == current, shape = segmentShape(i, jobs.jobs.size)) {
                    val key = j.chatKey
                    if (key != null) onPick(key) else j.work?.let(onInspect)
                }
            }
            if (jobs.conversations.isNotEmpty()) {
                item { SectionLabel("Conversations") }
                items(jobs.conversations.size) { i ->
                    val t = jobs.conversations[i]
                    ThreadRowView(t, selected = t.key == current, shape = segmentShape(i, jobs.conversations.size), host = if (t.here) "" else repo.hostName(t.hostId)) { onPick(t.key) }
                }
            }
        }
      }
      com.ryderdavid.nacre.ui.common.NewChatButton(onStart = start, modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp))
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 12.dp, top = 12.dp, bottom = 6.dp).semantics { heading() })
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun AgentMark(agent: String, selected: Boolean) {
    val c = MaterialTheme.colorScheme
    Surface(
        shape = (if (agent == ChatWire.AGENT_CODEX) MaterialShapes.Square else if (agent.isBlank()) MaterialShapes.Pill else MaterialShapes.Cookie9Sided).toShape(),
        color = if (selected) c.secondary else c.surfaceContainerHighest,
        contentColor = if (selected) c.onSecondary else c.onSurfaceVariant,
        modifier = Modifier.size(40.dp),
    ) {
        Box(contentAlignment = Alignment.Center) {
            Text(when { agent == ChatWire.AGENT_CODEX -> "X"; agent.isBlank() -> "R"; else -> "C" }, style = MaterialTheme.typography.titleMedium)
        }
    }
}

/** A job: its chat, what it is working on, a bar of its progress and where it stands. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun JobRowView(j: JobRow, selected: Boolean, shape: androidx.compose.ui.graphics.Shape, onClick: () -> Unit) {
    val c = MaterialTheme.colorScheme
    val described = listOf(j.title, "working on ${j.subject}", j.status, if (j.total > 0) "${j.done} of ${j.total}" else "", if (j.waiting > 0) "${j.waiting} waiting on you" else "")
        .filter { it.isNotBlank() }.joinToString(". ")
    Surface(
        onClick = onClick, shape = shape, color = if (selected) c.secondaryContainer else c.surfaceContainer,
        modifier = Modifier.fillMaxWidth().heightIn(min = 72.dp).semantics(mergeDescendants = true) { contentDescription = described },
    ) {
        Row(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
            AgentMark(j.agent, selected)
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(j.title, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(j.subject, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (j.total > 0) LinearProgressIndicator(progress = { j.done.toFloat() / j.total }, modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp))
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (j.running) LoadingIndicator(Modifier.size(18.dp))
                    Text(
                        j.status.ifBlank { "Not started" }, style = MaterialTheme.typography.labelMedium,
                        color = if (j.stopped) c.error else c.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            if (j.waiting > 0) StatusPill("${j.waiting}", c.errorContainer, c.onErrorContainer)
            j.chatKey?.let { ChatRowMenu(it, j.title) }
        }
    }
}

@Composable
private fun ThreadRowView(t: ThreadRow, selected: Boolean, shape: androidx.compose.ui.graphics.Shape, host: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick, shape = shape,
        color = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth().heightIn(min = 72.dp),
    ) {
        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
            AgentMark(t.agent, selected)
            Column(Modifier.weight(1f)) {
                Text(t.title, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(
                    listOf(t.agent.replaceFirstChar { it.uppercase() }, t.lastLine).filter { it.isNotBlank() }.joinToString(" · ") + if (host.isNotBlank()) " · on $host" else "",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
            if (t.waiting > 0) StatusPill("${t.waiting}", MaterialTheme.colorScheme.errorContainer, MaterialTheme.colorScheme.onErrorContainer)
            ChatRowMenu(t.key, t.title)
        }
    }
}

/**
 * A chat row's menu: Delete chat, after a confirmation. Deleting removes the conversation from its
 * computer; beads, specs and work it started stay.
 */
@Composable
private fun ChatRowMenu(key: String, title: String) {
    val container = LocalContainer.current
    var open by remember { mutableStateOf(false) }
    var confirming by remember { mutableStateOf(false) }
    val name = title.ifBlank { "this chat" }
    Box {
        androidx.compose.material3.IconButton(onClick = { open = true }, modifier = Modifier.size(48.dp)) {
            Icon(androidx.compose.material.icons.Icons.Rounded.MoreVert, "More for $name")
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text("Delete chat") },
                leadingIcon = { Icon(androidx.compose.material.icons.Icons.Rounded.Delete, null) },
                onClick = { open = false; confirming = true },
            )
        }
    }
    if (confirming) androidx.compose.material3.AlertDialog(
        onDismissRequest = { confirming = false },
        title = { Text("Delete “$name”?") },
        text = { Text("Its messages and images are removed from the computer, and it can't be brought back. Beads, specs and work it started stay.") },
        confirmButton = {
            androidx.compose.material3.TextButton(onClick = {
                confirming = false
                container.scope.launch {
                    runCatching { container.chats.delete(key) }.onFailure { container.repository.notify("Couldn't delete the chat: ${it.message}") }
                }
            }) { Text("Delete", color = MaterialTheme.colorScheme.error) }
        },
        dismissButton = { androidx.compose.material3.TextButton(onClick = { confirming = false }) { Text("Cancel") } },
    )
}
