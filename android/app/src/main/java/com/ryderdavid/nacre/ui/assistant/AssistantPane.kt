package com.ryderdavid.nacre.ui.assistant

import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.signedOutOn
import com.ryderdavid.nacre.ui.common.ClaudeSignInDialog
import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.DeleteSweep
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Lightbulb
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.VerifiedUser
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconToggleButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.material3.ScaffoldDefaults
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.material3.TextButton
import androidx.compose.runtime.rememberCoroutineScope
import com.ryderdavid.nacre.data.claude.BOARDS_STEP
import com.ryderdavid.nacre.data.claude.CONNECTION_STEP
import com.ryderdavid.nacre.data.claude.ChatItem
import com.ryderdavid.nacre.data.claude.ChatItem.ToolCall.State as S
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.sendOnHardwareEnter
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.launch

/** Which conversation the pane shows: one board, or every board at once. */
sealed interface AssistantTarget {
    data class Board(val repoKey: String, val repoName: String) : AssistantTarget
    data object AllBoards : AssistantTarget
}

private val BOARD_SUGGESTIONS = listOf(
    "What should I work on next?",
    "Triage what's blocked",
    "Summarize this board",
    "Which beads need my decision?",
    "Find stale or duplicate beads",
    "Break down the biggest open epic",
)

private val ALL_BOARDS_SUGGESTIONS = listOf(
    "What should I work on next, anywhere?",
    "Which projects are stuck?",
    "What needs my decision on any board?",
    "Summarize every board in a few lines",
    "Where is the same work filed twice?",
    "Pick the ready beads worth running tonight",
)

@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun AssistantPane(
    target: AssistantTarget,
    inPane: Boolean,
    onClose: () -> Unit,
    onOpenBead: (repoKey: String, beadId: String) -> Unit,
    onOpenRun: (String) -> Unit,
    onDismissChat: () -> Unit,
) {
    val container = LocalContainer.current
    val everyBoard = target is AssistantTarget.AllBoards
    val boardName = when (target) {
        is AssistantTarget.Board -> target.repoName.ifBlank { container.repository.repo(target.repoKey)?.name ?: target.repoKey }
        AssistantTarget.AllBoards -> "Every board"
    }
    val assistant = remember(target) {
        when (target) {
            is AssistantTarget.Board -> container.assistant(target.repoKey, boardName)
            AssistantTarget.AllBoards -> container.orchestrator()
        }
    }
    val items by assistant.items.collectAsState()
    val busy by assistant.busy.collectAsState()
    val auto by assistant.autoApprove.collectAsState()
    val prefs by container.settings.settings.collectAsState(initial = null)
    val scope = rememberCoroutineScope()
    val chatContext = when (target) {
        is AssistantTarget.Board -> com.ryderdavid.nacre.data.claude.ChatContext.Board(target.repoKey)
        AssistantTarget.AllBoards -> com.ryderdavid.nacre.data.claude.ChatContext.Multi
    }
    val input = chatDraft(chatContext)
    fun setInput(text: String) = container.chatStack.updateView(chatContext) { it.copy(draft = text) }
    val list = rememberChatList(chatContext)
    var signInHost by remember { mutableStateOf<String?>(null) }
    signInHost?.let { ClaudeSignInDialog(it, onDismiss = { signInHost = null }) }
    var firstComposition by remember { mutableStateOf(true) }

    LaunchedEffect(items.size, items.lastOrNull()) {
        if (!firstComposition && items.isNotEmpty()) list.animateScrollToItem(items.lastIndex)
        firstComposition = false
    }

    Scaffold(
        containerColor = if (inPane) MaterialTheme.colorScheme.surfaceContainerLow else MaterialTheme.colorScheme.surface,
        contentWindowInsets = if (inPane) WindowInsets(0) else ScaffoldDefaults.contentWindowInsets,
        topBar = {
            TopAppBar(
                windowInsets = if (inPane) WindowInsets(0) else TopAppBarDefaults.windowInsets,
                colors = if (inPane) TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow) else TopAppBarDefaults.topAppBarColors(),
                navigationIcon = { IconButton(onClick = onClose) { Icon(if (inPane) Icons.Rounded.Close else Icons.AutoMirrored.Rounded.ArrowBack, "Close assistant") } },
                title = {
                    Column {
                        Text("Ask Claude")
                        Text(boardName, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                    }
                },
                actions = {
                    IconToggleButton(checked = auto, onCheckedChange = { assistant.autoApprove.value = it }) {
                        Icon(Icons.Rounded.VerifiedUser, if (auto) "Changes apply without asking" else "Ask before changing beads", tint = if (auto) Nacre.colors.needsYou else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    IconButton(onClick = onDismissChat) { Icon(Icons.Rounded.DeleteSweep, "Dismiss chat") }
                },
            )
        },
        bottomBar = {
            Surface(color = if (inPane) MaterialTheme.colorScheme.surfaceContainerLow else MaterialTheme.colorScheme.surface) {
                Row(Modifier.fillMaxWidth().then(if (inPane) Modifier else Modifier.navigationBarsPadding()).imePadding().padding(12.dp), verticalAlignment = Alignment.Bottom) {
                    val canSend = !busy && input.isNotBlank() && prefs?.claudeConfigured == true
                    val send = { assistant.send(input); setInput("") }
                    OutlinedTextField(
                        value = input,
                        onValueChange = ::setInput,
                        placeholder = { Text(if (everyBoard) "Ask about any board" else "Ask about this board") },
                        maxLines = 5,
                        shape = RoundedCornerShape(24.dp),
                        modifier = Modifier.weight(1f).sendOnHardwareEnter(canSend, send),
                    )
                    Spacer(Modifier.width(8.dp))
                    if (busy) FilledIconButton(onClick = assistant::stop, modifier = Modifier.size(52.dp)) { Icon(Icons.Rounded.Stop, "Stop") }
                    else FilledIconButton(
                        onClick = send,
                        enabled = canSend,
                        modifier = Modifier.size(52.dp),
                    ) { Icon(Icons.AutoMirrored.Rounded.Send, "Send") }
                }
            }
        },
    ) { pad ->
        when {
            prefs?.claudeConfigured == false -> if (prefs?.claudeRunner == AppSettings.RUNNER_MAC) MessageState(
                AiIcon, "Pair a computer first",
                "The assistant runs on your Mac's Claude subscription. Pair a computer in Settings, or add a Claude API key there.",
                modifier = Modifier.padding(pad),
            ) else MessageState(
                AiIcon, "Add your Claude API key",
                "The board assistant uses your own Claude API key, set in Settings.",
                modifier = Modifier.padding(pad),
            )
            items.isEmpty() -> EmptyAssistant(everyBoard, Modifier.padding(pad)) { assistant.send(it) }
            else -> LazyColumn(
                state = list,
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
                modifier = Modifier.padding(pad).fillMaxSize(),
            ) {
                items(items, key = { it.key }) { item ->
                    when (item) {
                        is ChatItem.User -> UserBubble(item.text)
                        is ChatItem.Assistant -> MarkdownText(item.text.ifEmpty { "…" }, Modifier.fillMaxWidth())
                        is ChatItem.Thinking -> ThinkingRow(item)
                        is ChatItem.ToolCall -> ToolRow(
                            item,
                            onApprove = { assistant.decide(item.id, true) },
                            onDecline = { assistant.decide(item.id, false) },
                            onOpenBead = { id -> onOpenBead(item.boardKey, id) },
                        )
                        is ChatItem.Error -> Surface(color = MaterialTheme.colorScheme.errorContainer, shape = RoundedCornerShape(12.dp)) {
                            Column(Modifier.padding(12.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Icon(Icons.Rounded.ErrorOutline, null, tint = MaterialTheme.colorScheme.onErrorContainer)
                                    Spacer(Modifier.width(8.dp))
                                    Text(item.text, color = MaterialTheme.colorScheme.onErrorContainer, style = MaterialTheme.typography.bodyMedium)
                                }
                                Row {
                                    signedOutOn(item.text)?.let { name ->
                                        val host = (target as? AssistantTarget.Board)?.repoKey?.let(Keys::host) ?: container.repository.hostCalled(name)
                                        if (host != null) TextButton(onClick = { signInHost = host }) { Text("Sign in to Claude") }
                                    }
                                    if (item.canRetry) TextButton(onClick = assistant::retry, enabled = !busy) { Text("Retry") }
                                    if (item.offerApiKey) TextButton(
                                        onClick = { scope.launch { container.settings.update { it.copy(claudeRunner = AppSettings.RUNNER_API) } } },
                                    ) { Text("Use my Claude API key instead") }
                                }
                            }
                        }
                    }
                }
                if (busy && items.none { (it is ChatItem.Assistant && it.streaming) || (it is ChatItem.ToolCall && it.state == ChatItem.ToolCall.State.NEEDS_APPROVAL) }) item {
                    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterStart) { LoadingIndicator(Modifier.size(36.dp)) }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun EmptyAssistant(everyBoard: Boolean, modifier: Modifier, onAsk: (String) -> Unit) {
    Column(modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Icon(AiIcon, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(40.dp))
        Spacer(Modifier.height(12.dp))
        Text(if (everyBoard) "Every board, with Claude" else "Your board, with Claude", style = MaterialTheme.typography.titleLarge)
        Text(
            if (everyBoard) "Claude can read the beads on all of your projects, answer questions about any one of them, and line up work across them. You approve each change."
            else "Claude can read every bead here and propose edits, new beads and dependencies. You approve each change.",
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 8.dp).widthIn(max = 420.dp),
        )
        Spacer(Modifier.height(20.dp))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally)) {
            val suggestions = if (everyBoard) ALL_BOARDS_SUGGESTIONS else BOARD_SUGGESTIONS
            suggestions.forEach { s -> SuggestionChip(onClick = { onAsk(s) }, label = { Text(s) }, icon = { Icon(Icons.Rounded.Lightbulb, null, Modifier.size(16.dp)) }) }
        }
    }
}

@Composable
private fun UserBubble(text: String) {
    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterEnd) {
        Surface(color = MaterialTheme.colorScheme.primaryContainer, shape = RoundedCornerShape(20.dp, 20.dp, 4.dp, 20.dp), modifier = Modifier.widthIn(max = 520.dp)) {
            Text(text, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp))
        }
    }
}

@Composable
private fun ThinkingRow(item: ChatItem.Thinking) {
    var open by remember { mutableStateOf(false) }
    Column(Modifier.fillMaxWidth().clickable { open = !open }.animateContentSize()) {
        Text(
            if (item.streaming) "Thinking…" else "Thought it through",
            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (open || item.streaming) Text(
            item.text, style = MaterialTheme.typography.bodySmall.copy(fontStyle = FontStyle.Italic),
            color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = if (open) Int.MAX_VALUE else 3, overflow = TextOverflow.Ellipsis,
        )
    }
}

private fun toolTitle(name: String) = when (name) {
    "bd" -> "Read the board on the Mac"
    CONNECTION_STEP -> "Connection"
    BOARDS_STEP -> "Boards not read"
    "list_boards" -> "Looked at the boards"
    "list_beads" -> "Looked at beads"
    "get_bead" -> "Read bead"
    "update_bead" -> "Edit bead"
    "create_bead" -> "Create bead"
    "close_bead" -> "Close bead"
    "add_comment" -> "Comment"
    "add_dependency" -> "Add dependency"
    "run_in_claude_code" -> "Run in Claude Code"
    "start_round" -> "Work these until done"
    else -> name
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun ToolRow(item: ChatItem.ToolCall, onApprove: () -> Unit, onDecline: () -> Unit, onOpenBead: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val needs = item.state == S.NEEDS_APPROVAL
    Surface(
        color = if (needs) Nacre.colors.needsYouContainer else MaterialTheme.colorScheme.surfaceContainer,
        shape = RoundedCornerShape(16.dp),
        modifier = Modifier.fillMaxWidth().animateContentSize(),
        onClick = { open = !open },
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    when (item.state) {
                        S.DONE -> Icons.Rounded.Check
                        S.FAILED -> Icons.Rounded.ErrorOutline
                        S.REJECTED -> Icons.Rounded.Close
                        else -> if (item.isWrite) AiIcon else Icons.Rounded.Search
                    },
                    null, Modifier.size(18.dp),
                    tint = when (item.state) {
                        S.FAILED -> MaterialTheme.colorScheme.error
                        S.NEEDS_APPROVAL -> Nacre.colors.needsYou
                        else -> MaterialTheme.colorScheme.primary
                    },
                )
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(toolTitle(item.name) + if (item.state == S.REJECTED) " · declined" else "", style = MaterialTheme.typography.labelLarge)
                    if (item.summary.isNotBlank()) Text(item.summary, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = if (open) 6 else 1, overflow = TextOverflow.Ellipsis)
                }
                if (item.state == S.RUNNING) LoadingIndicator(Modifier.size(24.dp))
            }
            if (needs) {
                ChangePreview(item)
                Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                    OutlinedButton(onClick = onDecline) { Text("Decline") }
                    Button(onClick = onApprove) { Text("Approve") }
                }
            }
            if (open && item.result.isNotBlank()) {
                Text(
                    item.result.take(4000),
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = MonoFamily),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            val beadId = (item.input["id"] as? kotlinx.serialization.json.JsonPrimitive)?.content
            if (open && beadId != null && item.boardKey.isNotEmpty() && item.state == S.DONE) {
                androidx.compose.material3.TextButton(onClick = { onOpenBead(beadId) }) { Text("Open $beadId") }
            }
        }
    }
}

/** What a write tool will change, shown before approval. */
@Composable
private fun ChangePreview(item: ChatItem.ToolCall) {
    val fields = item.input.entries.filter { (k, v) -> k != "id" && v.toString() != "null" }
    if (fields.isEmpty()) return
    Column(Modifier.padding(top = 8.dp)) {
        fields.forEach { (k, v) ->
            val text = (v as? kotlinx.serialization.json.JsonPrimitive)?.content ?: v.toString()
            Text(k.replace('_', ' '), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
            Text(text.take(800), style = MaterialTheme.typography.bodySmall, maxLines = 8, overflow = TextOverflow.Ellipsis)
        }
    }
}
