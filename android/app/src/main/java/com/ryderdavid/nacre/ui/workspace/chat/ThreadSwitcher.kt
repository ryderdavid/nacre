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
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.StatusPill
import com.ryderdavid.nacre.ui.workspace.segmentShape
import kotlinx.coroutines.launch

/** The project's threads: this checkout's, then other computers'; New chat with the default agent, the other in its menu. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ThreadSwitcher(repoKey: String, current: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val container = LocalContainer.current
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val name = repo.repos.collectAsState().value.data?.firstOrNull { it.key == repoKey }?.name ?: Keys.id(repoKey)
    val decisions by repo.decisions.collectAsState()
    val open = repo.siblingKeys(repoKey).plus(repoKey).flatMap { decisions[it].orEmpty() }.filter { it.pending && !it.chatId.isNullOrBlank() }
        .groupingBy { it.chatId.orEmpty() }.eachCount()
    val rows = threadsFor(listing.chats, repoKey, repo.siblingKeys(repoKey), open)
    val scope = container.scope
    val start: (String) -> Unit = { agent ->
        scope.launch {
            runCatching { container.chats.link.createChat(Keys.host(repoKey), CreateChat(agent, Keys.id(repoKey), null)).copy(hostId = Keys.host(repoKey)).key }
                .onSuccess(onPick).onFailure { repo.notify("Couldn't start the chat: ${it.message}") }
        }
    }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal = 12.dp)) {
            Row(Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp, bottom = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("$name chats", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.weight(1f))
                com.ryderdavid.nacre.ui.common.NewChatButton(onStart = start, height = 48.dp)
            }
            val groups = rows.groupBy { it.hostId }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(2.dp), modifier = Modifier.padding(bottom = 16.dp)) {
                groups.forEach { (host, list) ->
                    item { Text(repo.hostName(host).ifBlank { "Computer" }, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 12.dp, top = 12.dp, bottom = 6.dp)) }
                    items(list.size) { i ->
                        val t = list[i]
                        val selected = t.key == current
                        Surface(
                            onClick = { onPick(t.key) }, shape = segmentShape(i, list.size),
                            color = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainer,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 72.dp),
                        ) {
                            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                                Surface(
                                    shape = (if (t.agent == ChatWire.AGENT_CODEX) MaterialShapes.Square else MaterialShapes.Cookie9Sided).toShape(),
                                    color = if (selected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.surfaceContainerHighest,
                                    contentColor = if (selected) MaterialTheme.colorScheme.onSecondary else MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.size(40.dp),
                                ) { Box(contentAlignment = Alignment.Center) { Text(if (t.agent == ChatWire.AGENT_CODEX) "X" else "C", style = MaterialTheme.typography.titleMedium) } }
                                Column(Modifier.weight(1f)) {
                                    Text(t.title, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    Text(
                                        listOf(t.agent.replaceFirstChar { it.uppercase() }, t.lastLine).filter { it.isNotBlank() }.joinToString(" · ") + if (!t.here) " · another checkout" else "",
                                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                                    )
                                }
                                if (t.waiting > 0) StatusPill("${t.waiting}", MaterialTheme.colorScheme.errorContainer, MaterialTheme.colorScheme.onErrorContainer)
                            }
                        }
                    }
                }
                if (rows.isEmpty()) item { Text("No chats yet in this project.", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onSurfaceVariant) }
            }
        }
    }
}
