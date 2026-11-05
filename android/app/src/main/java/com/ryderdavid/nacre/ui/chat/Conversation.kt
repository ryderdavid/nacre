package com.ryderdavid.nacre.ui.chat

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.chat.aboutChip
import com.ryderdavid.nacre.data.chat.contextForChat
import com.ryderdavid.nacre.data.chat.elsewhereHint
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MarkdownText
import kotlinx.coroutines.launch

/** One chat full screen: the conversation, its cards and the composer. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationScreen(key: String, onBack: () -> Unit, onNew: (NewChatRequest) -> Unit) {
    val container = LocalContainer.current
    val hub = container.chats
    val conv = remember(key) { hub.conversation(key) }
    LaunchedEffect(key) { conv.open() }
    val state by conv.state.collectAsState()
    val listing by container.repository.chats.collectAsState()
    val record = listing.chat(key) ?: state.chat
    val computer = container.repository.hostName(Keys.host(key)).ifBlank { record?.hostName ?: "the computer" }
    val shown by hub.screens.current.collectAsState()
    val send = contextForChat(shown, Keys.host(key), container.repository.groups.collectAsState().value, container.repository::hostName)
    val focus by hub.focus.collectAsState()
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()
    var menu by remember { mutableStateOf(false) }

    LaunchedEffect(state.items.size, focus) {
        val target = focus?.let { f -> state.items.indexOfFirst { it is ChatItem.Permission && it.card.requestId == f } }?.takeIf { it >= 0 }
        when {
            target != null -> { list.animateScrollToItem(target); hub.focus.value = null }
            state.items.isNotEmpty() -> list.animateScrollToItem(state.items.lastIndex)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(record?.title?.ifBlank { null } ?: "Chat", maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(computer, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Chats") } },
                actions = {
                    Box {
                        IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(text = { Text("New chat") }, onClick = { menu = false; onNew(NewChatRequest()) })
                            DropdownMenuItem(text = { Text("Delete") }, onClick = {
                                menu = false
                                scope.launch {
                                    runCatching { hub.delete(key) }.onFailure { container.repository.notify("Couldn't delete the chat: ${it.friendly()}") }
                                }
                            })
                        }
                    }
                },
            )
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize().imePadding()) {
            LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(state.items, key = { it.key }) { item ->
                    Box(Modifier.padding(horizontal = 16.dp)) {
                        ChatItemView(item, onAnswer = { req, allow, note -> conv.answer(req, allow, note) })
                    }
                }
            }
            if (state.reconnecting) StatusLine("Reconnecting to $computer…")
            state.error?.let { StatusLine(it, error = true, onDismiss = conv::dismissError) }
            state.limitReason?.let { reason ->
                Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(reason, Modifier.weight(1f), style = MaterialTheme.typography.bodyMedium)
                    Button(onClick = { onNew(NewChatRequest(record?.repo?.let { Keys.of(Keys.host(key), it) }, carry = "Carrying on from “${record?.title.orEmpty()}”: ")) }) { Text("New") }
                }
            }
            send.elsewhere?.let { e ->
                Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(elsewhereHint(computer, e), Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
                    TextButton(onClick = { onNew(NewChatRequest(forceRepoKey = e.repoKey)) }) { Text("New chat on ${e.computer}") }
                }
            }
            Composer(
                chatKey = key,
                about = aboutChip(send.label),
                running = state.turnRunning,
                canSend = state.canSend,
                onSend = { text ->
                    // The context is read at the moment Send is pressed.
                    val at = contextForChat(hub.screens.atSend(), Keys.host(key), container.repository.groups.value, container.repository::hostName)
                    conv.send(text, at.context)
                },
                onStop = conv::stop,
            )
        }
    }
}

@Composable
private fun StatusLine(text: String, error: Boolean = false, onDismiss: (() -> Unit)? = null) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text, Modifier.weight(1f), style = MaterialTheme.typography.bodySmall,
            color = if (error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
        onDismiss?.let { TextButton(onClick = it) { Text("Dismiss") } }
    }
}

@Composable
private fun Composer(chatKey: String, about: String, running: Boolean, canSend: Boolean, onSend: (String) -> Unit, onStop: () -> Unit) {
    val hub = LocalContainer.current.chats
    val drafts by hub.drafts.collectAsState()
    var text by rememberSaveable(chatKey) { mutableStateOf(drafts[chatKey].orEmpty()) }
    Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
        AssistChip(onClick = {}, label = { Text(about, maxLines = 1, overflow = TextOverflow.Ellipsis) })
        Row(verticalAlignment = Alignment.Bottom) {
            OutlinedTextField(
                value = text,
                onValueChange = { text = it; hub.drafts.value += (chatKey to it) },
                modifier = Modifier.weight(1f),
                placeholder = { Text("Message") },
                maxLines = 6,
            )
            if (running) {
                FilledIconButton(onClick = onStop, modifier = Modifier.padding(start = 8.dp)) { Icon(Icons.Rounded.Stop, "Stop") }
            } else {
                FilledIconButton(
                    onClick = {
                        val t = text.trim()
                        if (t.isNotEmpty()) {
                            onSend(t)
                            text = ""
                            hub.drafts.value -= chatKey
                        }
                    },
                    enabled = canSend && text.isNotBlank(),
                    modifier = Modifier.padding(start = 8.dp),
                ) { Icon(Icons.AutoMirrored.Rounded.Send, "Send") }
            }
        }
    }
}

@Composable
internal fun ChatItemView(item: ChatItem, onAnswer: (String, Boolean, String?) -> Unit) {
    when (item) {
        is ChatItem.User -> Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            Column(
                Modifier.widthIn(max = 520.dp)
                    .background(MaterialTheme.colorScheme.primaryContainer, RoundedCornerShape(16.dp))
                    .padding(12.dp),
            ) {
                Text(item.text, color = MaterialTheme.colorScheme.onPrimaryContainer)
                item.about?.let { Text(aboutChip(it), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onPrimaryContainer) }
            }
        }
        is ChatItem.Assistant -> MarkdownText(item.text, Modifier.fillMaxWidth())
        is ChatItem.ToolStep -> ToolStepView(item)
        is ChatItem.Handoff -> Text(item.text, style = MaterialTheme.typography.bodySmall, fontStyle = FontStyle.Italic, color = MaterialTheme.colorScheme.onSurfaceVariant)
        is ChatItem.Permission -> PermissionCardView(item.card, onAnswer)
        is ChatItem.Note -> Text(
            item.text, style = MaterialTheme.typography.bodySmall,
            color = if (item.error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun ToolStepView(step: ChatItem.ToolStep) {
    var open by remember(step.key) { mutableStateOf(false) }
    Column(Modifier.fillMaxWidth().clickable(enabled = step.result != null) { open = !open }) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                step.summary, Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis,
                style = MaterialTheme.typography.labelMedium,
                color = if (step.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (step.result != null) Icon(if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, if (open) "Collapse" else "Expand")
        }
        AnimatedVisibility(open) {
            Text(step.result.orEmpty().take(4000), style = MaterialTheme.typography.bodySmall)
        }
    }
}

/** A permission request: Allow, or Deny with an optional note; read-only once decided. */
@Composable
internal fun PermissionCardView(card: PermissionCard, onAnswer: (String, Boolean, String?) -> Unit) {
    var denying by remember(card.requestId) { mutableStateOf(false) }
    var note by remember(card.requestId) { mutableStateOf("") }
    var sent by remember(card.requestId) { mutableStateOf(false) }
    Card(Modifier.fillMaxWidth(), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(if (card.pending) "Allow this step?" else "Permission", style = MaterialTheme.typography.titleSmall)
            Text(card.step, style = MaterialTheme.typography.bodyMedium)
            if (card.reason.isNotBlank()) Text(card.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (!card.pending) {
                Text(card.outcomeLine, style = MaterialTheme.typography.labelLarge)
            } else if (denying) {
                OutlinedTextField(note, { note = it }, Modifier.fillMaxWidth(), placeholder = { Text("Note for the agent (optional)") })
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    TextButton(onClick = { denying = false }, enabled = !sent) { Text("Back") }
                    Button(onClick = { sent = true; onAnswer(card.requestId, false, note) }, enabled = !sent) { Text("Deny") }
                }
            } else {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { sent = true; onAnswer(card.requestId, true, null) }, enabled = !sent) { Text("Allow") }
                    OutlinedButton(onClick = { denying = true }, enabled = !sent) { Text("Deny") }
                }
            }
        }
    }
}
