package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.InputChip
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.chatComposerGate
import com.ryderdavid.nacre.data.model.DecisionEvidence
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.chat.ChatItemView
import com.ryderdavid.nacre.ui.common.AttachImageButton
import com.ryderdavid.nacre.ui.common.ComposerReason
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.PendingImageRow
import com.ryderdavid.nacre.ui.common.rememberImageAttacher
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionCard
import com.ryderdavid.nacre.ui.workspace.decisions.PermissionCardM3
import com.ryderdavid.nacre.ui.workspace.projectStrip
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** How long after the last output the loading indicator keeps moving: it shows output arriving, not mere waiting. */
private const val ACTIVE_WINDOW_MS = 20_000L

/**
 * The workspace's Chat: the project and the thread, the agent's state, the conversation with its cards and
 * work cards, and the composer with the Viewing chip, the thread's reference chips and suggestions.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun ChatPaneM3(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit, onEvidence: (DecisionEvidence) -> Unit) {
    val container = LocalContainer.current
    val hub = container.chats
    val repo = container.repository
    val conv = remember(thread) { hub.conversation(thread) }
    LaunchedEffect(thread) { conv.open() }
    val state by conv.state.collectAsState()
    val listing by repo.chats.collectAsState()
    val record = listing.chat(thread) ?: state.chat
    val project = repo.repos.collectAsState().value.data?.firstOrNull { it.key == repoKey }
    val computer = repo.hostName(Keys.host(thread)).ifBlank { record?.hostName ?: "this computer" }
    val attachedAll by hub.references.attachedAll.collectAsState()
    val viewingAll by hub.references.viewingAll.collectAsState()
    val viewing = viewingAll[repoKey]
    val refs = com.ryderdavid.nacre.data.chat.ChatReferences.State(attachedAll[thread].orEmpty(), viewing?.first, viewing?.second ?: false)
    val decisionsAll by repo.decisions.collectAsState()
    val mine = decisionsAll[repoKey].orEmpty().filter { it.chatId == Keys.id(thread) }.sortedBy { it.createdAt }
    val workAll by repo.work.collectAsState()
    val myWork = workAll[repoKey].orEmpty().filter { it.chatId == Keys.id(thread) }
    val (work, needsYou) = projectStrip(repoKey)
    val views by hub.stack.views.collectAsState()
    val draft = views[thread]?.draft.orEmpty()
    var switching by remember { mutableStateOf(false) }
    val list = com.ryderdavid.nacre.ui.chat.rememberChatList(thread)
    val compact = LocalConfiguration.current.screenWidthDp < 600

    // Silence is measured from the last thing the turn produced.
    var lastOutput by remember(thread) { mutableLongStateOf(0L) }
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(state.items.size, state.lastSeq) { lastOutput = System.currentTimeMillis(); repo.refreshDecisions(repoKey) }
    LaunchedEffect(Unit) { while (true) { delay(10_000); now = System.currentTimeMillis() } }
    val since = if (lastOutput == 0L) 0L else (now - lastOutput).coerceAtLeast(0)
    val agent = AgentStates.of(state, since, stalled = work?.stalled == true && state.turnRunning, pendingDecisions = mine.count { it.pending })
    val outputFlowing = state.turnRunning && lastOutput > 0 && since < ACTIVE_WINDOW_MS

    // Follow the bottom only when Ryder is already there; a thread he scrolled up in keeps its place.
    val total = state.items.size + mine.size + myWork.size
    var following by remember(thread) { mutableStateOf(views[thread]?.firstVisibleIndex == null || views[thread]?.firstVisibleIndex == 0) }
    LaunchedEffect(list.canScrollForward, list.isScrollInProgress) { if (!list.isScrollInProgress) following = !list.canScrollForward }
    LaunchedEffect(total) { if (total > 0 && following) list.animateScrollToItem(total - 1) }

    Column(Modifier.fillMaxSize().imePadding()) {
        Row(Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(horizontal = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            if (compact) IconButton(onClick = onProjects) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back to projects") }
            Surface(
                onClick = { switching = true }, shape = RoundedCornerShape(16.dp), color = Color.Transparent,
                modifier = Modifier.weight(1f).semantics { contentDescription = "Switch chat. ${project?.name.orEmpty()}, $computer, ${record?.agent ?: "Claude"}" },
            ) {
                Column(Modifier.padding(horizontal = 8.dp, vertical = 4.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(project?.name ?: Keys.id(repoKey), style = MaterialTheme.typography.titleLargeEmphasized, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Icon(Icons.Rounded.ExpandMore, null, Modifier.size(20.dp))
                    }
                    Text(
                        listOfNotNull(computer, (record?.agent ?: "claude").replaceFirstChar { it.uppercase() }, record?.title?.ifBlank { null }).joinToString(" · "),
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
        if (agent.phase != AgentPhase.IDLE) Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 2.dp).semantics { liveRegion = LiveRegionMode.Polite },
            verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (agent.phase == AgentPhase.WORKING && outputFlowing) LoadingIndicator(Modifier.size(24.dp))
            Text(
                agent.text, style = MaterialTheme.typography.labelLarge,
                color = when (agent.phase) {
                    AgentPhase.FAILED, AgentPhase.STALLED -> MaterialTheme.colorScheme.error
                    AgentPhase.WAITING -> MaterialTheme.colorScheme.tertiary
                    else -> MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
        }

        LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
            items(state.items, key = { it.key }) { item ->
                when (item) {
                    is ChatItem.Permission -> PermissionCardM3(item.card, record?.agent ?: "claude", onAnswer = { r, allow, note -> conv.answer(r, allow, note) })
                    is ChatItem.User -> Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterEnd) {
                        Surface(shape = RoundedCornerShape(22.dp, 22.dp, 6.dp, 22.dp), color = MaterialTheme.colorScheme.primaryContainer, modifier = Modifier.fillMaxWidth(0.84f)) {
                            Column(Modifier.padding(horizontal = 16.dp, vertical = 10.dp)) {
                                if (item.images.isNotEmpty()) com.ryderdavid.nacre.ui.common.ChatSentImages(thread, item.images, Modifier.padding(bottom = 4.dp))
                                Text(item.text, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onPrimaryContainer)
                            }
                        }
                    }
                    else -> ChatItemView(item, onAnswer = { r, allow, note -> conv.answer(r, allow, note) }, chatKey = thread)
                }
            }
            items(mine, key = { "d:" + it.id }) { d -> DecisionCard(d, onEvidence) }
            items(myWork, key = { "w:" + it.kind + it.id }) { w ->
                com.ryderdavid.nacre.ui.workspace.work.WorkCard(
                    w, replyRunning = state.turnRunning,
                    onInspect = { onEvidence(DecisionEvidence(label = w.title, kind = w.kind, ref = Keys.of(Keys.host(repoKey), w.id))) },
                    onStopReply = conv::stop,
                    onCancelWork = {
                        val key = Keys.of(Keys.host(repoKey), w.id)
                        if (w.kind == "run") container.scope.launch { runCatching { repo.cancelRun(key) }.onFailure { repo.notify("Couldn't cancel the run: ${it.message}") } }
                        else repo.cancelRoundQuietly(key)
                    },
                )
            }
        }

        Composer(repoKey, thread, record, state, refs, draft, work, needsYou)
    }

    if (switching) ThreadSwitcher(repoKey, current = thread, onPick = { switching = false; onThread(it) }, onDismiss = { switching = false })
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun Composer(
    repoKey: String,
    thread: String,
    record: com.ryderdavid.nacre.data.model.ChatRecord?,
    state: com.ryderdavid.nacre.data.chat.ConversationState,
    refs: com.ryderdavid.nacre.data.chat.ChatReferences.State,
    draft: String,
    work: com.ryderdavid.nacre.data.model.WorkSummary?,
    needsYou: Int,
) {
    val container = LocalContainer.current
    val hub = container.chats
    val conv = remember(thread) { hub.conversation(thread) }
    val views by hub.stack.views.collectAsState()
    val pending = views[thread]?.images.orEmpty()
    val gate = chatComposerGate(draft, pending, state.chat ?: record, state.canSend)
    val attach = rememberImageAttacher(thread, gate.remaining)
    val scope = rememberCoroutineScope()
    var sending by remember(thread) { mutableStateOf(false) }
    // The agent may later need an OK while the app is closed, so notifications are asked for on the first send.
    val askNotifications = com.ryderdavid.nacre.ui.common.rememberNotificationPermission()

    // References are spent only once nacred accepts the message; a failed send keeps the draft as written.
    val send: () -> Unit = send@{
        if (!gate.canSend || sending) return@send
        askNotifications()
        val original = draft
        val repoId = record?.repo ?: Keys.id(repoKey)
        val ctx = ChatSend.context(repoId, refs.viewing, refs.viewingDropped)
        hub.stack.updateView(thread) { it.copy(draft = ChatSend.text(original, refs.attached)) }
        sending = true
        scope.launch {
            val turn = runCatching { conv.sendDraftNow(ctx) }.getOrNull()
            if (turn != null) hub.references.sent(repoKey, thread)
            else hub.stack.updateView(thread) { it.copy(draft = original) }
            sending = false
        }
    }

    Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        val viewing = refs.viewing
        if (viewing != null || refs.attached.isNotEmpty()) LazyRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            if (viewing != null) item {
                InputChip(
                    selected = !refs.viewingDropped, onClick = { hub.references.dropViewing(repoKey, !refs.viewingDropped) },
                    label = { Text((if (refs.viewingDropped) "Not sending: " else "Viewing: ") + viewing.label, maxLines = 1) },
                    leadingIcon = { Icon(Icons.Rounded.Visibility, null, Modifier.size(18.dp)) },
                    trailingIcon = { Icon(Icons.Rounded.Close, null, Modifier.size(18.dp)) },
                    modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = if (refs.viewingDropped) "Viewing ${viewing.label} is left out of the next message. Tap to send it" else "Viewing ${viewing.label}. Tap to leave it out of the next message" },
                )
            }
            items(refs.attached) { r: ChatReference ->
                InputChip(
                    selected = true, onClick = { hub.references.remove(thread, r) },
                    label = { Text(r.label, maxLines = 1) },
                    leadingIcon = { Icon(Icons.Rounded.Link, null, Modifier.size(18.dp)) },
                    trailingIcon = { Icon(Icons.Rounded.Close, null, Modifier.size(18.dp)) },
                    modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "Reference ${r.label}. Remove reference" },
                )
            }
        }
        if (draft.isEmpty() && !state.turnRunning) LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(suggestions(needsYou, work, refs.viewing.takeUnless { refs.viewingDropped })) { s ->
                SuggestionChip(onClick = { hub.stack.updateView(thread) { it.copy(draft = s) } }, label = { Text(s) }, modifier = Modifier.heightIn(min = 48.dp))
            }
        }
        PendingImageRow(pending, onRemove = { id -> hub.stack.removePending(thread, id) })
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TextField(
                value = draft, onValueChange = { v -> hub.stack.updateView(thread) { it.copy(draft = v) } },
                modifier = Modifier.weight(1f).heightIn(min = 56.dp).onPreviewKeyEvent { e ->
                    // Ctrl+Enter sends from a hardware keyboard.
                    if (e.type == KeyEventType.KeyDown && e.key == Key.Enter && e.isCtrlPressed) { send(); true } else false
                },
                placeholder = { Text("Message ${(record?.agent ?: "claude").replaceFirstChar { it.uppercase() }}") },
                leadingIcon = { AttachImageButton(gate, attach) },
                shape = RoundedCornerShape(28.dp), maxLines = 6,
                colors = TextFieldDefaults.colors(
                    focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent, disabledIndicatorColor = Color.Transparent,
                    unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh, focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                ),
            )
            if (state.turnRunning) {
                FilledIconButton(onClick = conv::stop, modifier = Modifier.size(56.dp), shape = RoundedCornerShape(18.dp),
                    colors = IconButtonDefaults.filledIconButtonColors(containerColor = MaterialTheme.colorScheme.secondaryContainer, contentColor = MaterialTheme.colorScheme.onSecondaryContainer)) {
                    Icon(Icons.Rounded.Stop, "Stop this reply")
                }
            } else {
                FilledIconButton(onClick = send, enabled = gate.canSend && !sending, modifier = Modifier.size(56.dp), shape = RoundedCornerShape(18.dp)) {
                    if (sending) LoadingIndicator(Modifier.size(28.dp)) else Icon(Icons.AutoMirrored.Rounded.Send, "Send")
                }
            }
        }
        ComposerReason(gate, Modifier.padding(start = 8.dp))
    }
}
