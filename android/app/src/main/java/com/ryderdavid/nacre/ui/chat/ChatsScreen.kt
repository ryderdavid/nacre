package com.ryderdavid.nacre.ui.chat

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Info
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import com.ryderdavid.nacre.data.chat.NewChatTarget
import com.ryderdavid.nacre.data.chat.newChatTarget
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.ui.ScreenOrigin
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.relativeTime
import kotlinx.coroutines.launch

/** "New · <label>": the Chat list's first row, about the screen shown before Chat. */
fun newChatLabel(originLabel: String) = "New · $originLabel"

/** A chat's state as the list shows it. */
fun chatStateLabel(state: String): String = when (state) {
    ChatWire.RUNNING -> "Working"
    ChatWire.WAITING_PERMISSION -> "Needs your OK"
    ChatWire.INTERRUPTED -> "Interrupted"
    ChatWire.LIMIT_REACHED -> "Reached its limit"
    else -> "Idle"
}

/** A list row's second line: computer, start repository (or all boards), state. */
fun chatRowLine(chat: ChatRecord, repoName: (ChatRecord) -> String?): String =
    listOf(
        chat.hostName.ifBlank { "this computer" },
        chat.repo?.let { repoName(chat) ?: it } ?: ScreenOrigin.ALL_BOARDS,
        chatStateLabel(chat.state),
    ).joinToString(" · ")

/**
 * The Chat tab: the current chat full screen, or the list when there is none. [originLabel] names
 * the screen last shown before switching here (or all boards).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatsScreen(originLabel: String) {
    val container = LocalContainer.current
    val hub = container.chats
    val current by hub.current.collectAsState()
    var newChat by remember { mutableStateOf<NewChatRequest?>(null) }
    val key = current
    if (key != null) {
        ConversationScreen(key, onBack = { hub.current.value = null }, onNew = { newChat = it })
    } else {
        ChatList(originLabel, onNew = { newChat = it })
    }
    newChat?.let { req ->
        NewChatSheet(req, onDismiss = { newChat = null }, onCreated = { k ->
            newChat = null
            hub.openChat(k)
        })
    }
}

/** What New was asked for: the screen's computer, or [forceRepoKey]'s, with an optional first-message note. */
data class NewChatRequest(val forceRepoKey: String? = null, val carry: String? = null)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChatList(originLabel: String, onNew: (NewChatRequest) -> Unit) {
    val container = LocalContainer.current
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val repos by repo.repos.collectAsState()
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { runCatching { repo.refreshChats() } }
    Scaffold(topBar = { LargeTopAppBar(title = { Text("Chat") }) }) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize()) {
            item {
                ListItem(
                    headlineContent = { Text(newChatLabel(originLabel)) },
                    leadingContent = { Icon(Icons.Rounded.Add, null) },
                    modifier = Modifier.clickable { onNew(NewChatRequest()) },
                )
                HorizontalDivider()
            }
            items(listing.notices) { n ->
                ListItem(
                    headlineContent = { Text(n, style = MaterialTheme.typography.bodyMedium) },
                    leadingContent = { Icon(Icons.Rounded.Info, null) },
                )
            }
            items(listing.chats, key = { it.key }) { chat ->
                var menu by remember { mutableStateOf(false) }
                ListItem(
                    headlineContent = { Text(chat.title.ifBlank { "Chat" }, maxLines = 1) },
                    supportingContent = {
                        Text(chatRowLine(chat) { c -> repos.data?.firstOrNull { it.hostId == c.hostId && it.id == c.repo }?.name }, maxLines = 1)
                    },
                    overlineContent = { Text(relativeTime(chat.updatedAt.ifBlank { chat.createdAt })) },
                    trailingContent = {
                        Box {
                            IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                                DropdownMenuItem(text = { Text("Delete") }, onClick = {
                                    menu = false
                                    scope.launch {
                                        runCatching { container.chats.delete(chat.key) }
                                            .onFailure { repo.notify("Couldn't delete the chat: ${it.friendly()}") }
                                    }
                                })
                            }
                        }
                    },
                    modifier = Modifier.clickable { container.chats.openChat(chat.key) },
                )
            }
        }
    }
}

/** Where New starts for [req], from the screen Chat is about. */
@Composable
internal fun rememberNewChatTarget(req: NewChatRequest): NewChatTarget? {
    val container = LocalContainer.current
    val shown by container.chats.screens.current.collectAsState()
    val hosts by container.repository.hosts.collectAsState()
    val repos by container.repository.repos.collectAsState()
    return newChatTarget(shown, hosts, repos.data.orEmpty(), req.forceRepoKey)
}
