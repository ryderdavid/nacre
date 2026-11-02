package com.ryderdavid.nacre.ui.bead

import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.DeleteSweep
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.EditNote
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.QuestionAnswer
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.BeadChatItem
import com.ryderdavid.nacre.data.claude.ChatMode
import com.ryderdavid.nacre.data.claude.RefineProposal
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.PriorityPill
import com.ryderdavid.nacre.ui.theme.Nacre

private val REFINE_SUGGESTIONS = listOf(
    "Make it agent-ready",
    "Write acceptance criteria",
    "Tighten the description",
    "Split into subtasks",
    "Add design notes",
    "Suggest priority and type",
)

private val ASK_SUGGESTIONS = listOf(
    "What's blocking this?",
    "Explain the design",
    "What's left to do?",
    "Is it ready for an agent?",
)

/** How each mode presents itself in the sheet. */
private data class ModeLook(val title: String, val icon: ImageVector, val placeholder: String, val hintIcon: ImageVector, val hint: String)

private fun look(mode: ChatMode, id: String) = when (mode) {
    ChatMode.ASK -> ModeLook(
        "Ask about this bead", Icons.Rounded.QuestionAnswer, "Ask anything about $id",
        Icons.Rounded.Lock, "Read-only. Claude answers but won't change the bead.",
    )
    ChatMode.REFINE -> ModeLook(
        "Refine with Claude", AiIcon, "What should change? Leave empty to make it agent-ready.",
        Icons.Rounded.EditNote, "Claude proposes edits. Nothing changes until you apply them.",
    )
}

/**
 * A conversation with Claude about one bead. Ask questions in Ask mode, which
 * can't change anything, or switch to Refine to get edits proposed; both share
 * one thread, so a refine can build on what was just discussed. Proposals wait
 * in the thread until they're applied or discarded.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun BeadChatSheet(vm: BeadDetailViewModel, issue: Issue, claudeReady: Boolean, onDismiss: () -> Unit, onDismissChat: () -> Unit, onDevelop: (() -> Unit)? = null) {
    HandleDismissSheet(onDismiss) {
        androidx.compose.foundation.layout.Box(Modifier.fillMaxWidth().heightIn(max = 720.dp)) {
            BeadChatContent(vm, issue, claudeReady, onDismiss, onDismissChat, onDevelop)
        }
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun BeadChatContent(
    vm: BeadDetailViewModel, issue: Issue, claudeReady: Boolean,
    onClose: () -> Unit, onDismissChat: () -> Unit, onDevelop: (() -> Unit)? = null,
    inPane: Boolean = false,
    context: com.ryderdavid.nacre.data.claude.ChatContext = com.ryderdavid.nacre.data.claude.ChatContext.Bead(vm.repoKey, issue.id),
) {
    val items by vm.chat.collectAsState()
    val thinking by vm.chatThinking.collectAsState()
    val failure by vm.chatFailure.collectAsState()
    val mode by vm.chatMode.collectAsState()
    val stack = com.ryderdavid.nacre.ui.common.LocalContainer.current.chatStack
    val text = com.ryderdavid.nacre.ui.assistant.chatDraft(context)
    fun setText(value: String) = stack.updateView(context) { it.copy(draft = value) }
    val look = look(mode, issue.id)
    val list = com.ryderdavid.nacre.ui.assistant.rememberChatList(context)
    var firstComposition by remember { mutableStateOf(true) }
    val messages = items.count { it is BeadChatItem.Mine }

    // Follow the newest entry, including a proposal that folds down once it's applied or discarded.
    LaunchedEffect(items, thinking, failure) {
        val last = list.layoutInfo.totalItemsCount - 1
        if (!firstComposition && last >= 0) list.animateScrollToItem(last)
        firstComposition = false
    }

    fun send(message: String) {
        if (!claudeReady || thinking) return
        if (message.isBlank() && mode == ChatMode.ASK) return
        vm.sendChat(message, mode)
        setText("")
    }

        Column(Modifier.fillMaxSize().then(if (inPane) Modifier else Modifier.navigationBarsPadding()).imePadding()) {
            Row(Modifier.padding(start = 20.dp, end = 8.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(look.icon, null, tint = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(look.title, style = MaterialTheme.typography.titleLarge)
                    Text(
                        if (messages == 0) issue.id else "${issue.id} · $messages message${if (messages == 1) "" else "s"}",
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                // Clearing ends the conversation; closing only puts it away.
                if (messages > 0) IconButton(onClick = onDismissChat) { Icon(Icons.Rounded.DeleteSweep, "Clear this conversation") }
                IconButton(onClick = onClose) { Icon(Icons.Rounded.Close, "Close chat") }
            }
            // Refining the bead itself is the usual move; a reviewed proposal is the quiet alternative.
            if (onDevelop != null) androidx.compose.material3.TextButton(onClick = onDevelop, modifier = Modifier.padding(start = 12.dp)) {
                Text("Develop a proposal instead…")
            }

            if (items.isNotEmpty() || thinking || failure != null) LazyColumn(
                state = list,
                modifier = Modifier.fillMaxWidth().weight(1f),
                contentPadding = PaddingValues(horizontal = 20.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                itemsIndexed(items) { i, item ->
                    when (item) {
                        is BeadChatItem.Mine -> MyMessage(item)
                        is BeadChatItem.Answer -> MarkdownText(item.text, Modifier.fillMaxWidth())
                        is BeadChatItem.Proposal -> ProposalCard(
                            item, issue,
                            onApply = { sel -> vm.applyProposal(i, sel) },
                            onDiscard = { vm.discardProposal(i) },
                        )
                    }
                }
                if (thinking) item {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        LoadingIndicator(Modifier.size(32.dp))
                        Spacer(Modifier.width(8.dp))
                        val last = items.lastOrNull() as? BeadChatItem.Mine
                        Text(
                            if (last?.mode == ChatMode.ASK) "Claude is reading ${issue.id}…" else "Claude is drafting changes…",
                            color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                }
                failure?.let { f ->
                    item {
                        Column {
                            Text(f.message, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                            Row {
                                TextButton(onClick = vm::retryChat) { Text("Try again") }
                                if (f.offerApiKey) TextButton(onClick = { vm.useApiKey(); vm.retryChat() }) { Text("Use my API key") }
                            }
                        }
                    }
                }
            } else Text(
                if (mode == ChatMode.ASK) "Ask about the work, the design or what's in the way. Claude reads the bead and its comments, and on your Mac the code too."
                else "Tell Claude what to improve. You'll review each change before anything is saved.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 20.dp, vertical = 4.dp),
            )

            if (!claudeReady) {
                Text(
                    "Pair a computer, or add a Claude API key, in Settings to use this.",
                    color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(20.dp),
                )
                return@Column
            }

            if (!thinking) LazyRow(
                contentPadding = PaddingValues(horizontal = 20.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(top = 8.dp),
            ) {
                items(if (mode == ChatMode.ASK) ASK_SUGGESTIONS else REFINE_SUGGESTIONS) { s ->
                    SuggestionChip(onClick = { send(s) }, label = { Text(s) })
                }
            }

            Composer(
                text = text, onText = ::setText, mode = mode, look = look, busy = thinking,
                onMode = vm::setChatMode, onSend = { send(text) },
            )
        }
}

@Composable
private fun Composer(
    text: String,
    onText: (String) -> Unit,
    mode: ChatMode,
    look: ModeLook,
    busy: Boolean,
    onMode: (ChatMode) -> Unit,
    onSend: () -> Unit,
) {
    val edge by animateColorAsState(
        if (mode == ChatMode.REFINE) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant,
        label = "composer edge",
    )
    Column(Modifier.padding(horizontal = 12.dp).padding(top = 10.dp, bottom = 12.dp)) {
        Surface(
            shape = RoundedCornerShape(24.dp),
            color = MaterialTheme.colorScheme.surfaceContainerLowest,
            border = BorderStroke(1.dp, edge),
        ) {
            Column(Modifier.padding(start = 4.dp, end = 8.dp, top = 2.dp, bottom = 8.dp)) {
                TextField(
                    value = text, onValueChange = onText,
                    placeholder = { Text(look.placeholder) },
                    minLines = 1, maxLines = 6,
                    colors = TextFieldDefaults.colors(
                        focusedContainerColor = Color.Transparent, unfocusedContainerColor = Color.Transparent,
                        focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(Modifier.padding(start = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    SingleChoiceSegmentedButtonRow(Modifier.widthIn(max = 240.dp)) {
                        ChatMode.entries.forEachIndexed { i, m ->
                            SegmentedButton(
                                selected = m == mode, onClick = { onMode(m) },
                                shape = SegmentedButtonDefaults.itemShape(i, ChatMode.entries.size),
                                icon = { SegmentedButtonDefaults.Icon(m == mode) { Icon(look(m, "").icon, null, Modifier.size(18.dp)) } },
                            ) { Text(if (m == ChatMode.ASK) "Ask" else "Refine") }
                        }
                    }
                    Spacer(Modifier.weight(1f))
                    val canSend = !busy && (mode == ChatMode.REFINE || text.isNotBlank())
                    if (mode == ChatMode.REFINE) FilledIconButton(onClick = onSend, enabled = canSend) {
                        Icon(Icons.AutoMirrored.Rounded.Send, "Ask Claude for changes")
                    } else FilledTonalIconButton(onClick = onSend, enabled = canSend) {
                        Icon(Icons.AutoMirrored.Rounded.Send, "Ask")
                    }
                }
            }
        }
        Row(Modifier.padding(start = 12.dp, top = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(look.hintIcon, null, Modifier.size(14.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.width(6.dp))
            Text(look.hint, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun MyMessage(item: BeadChatItem.Mine) {
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(look(item.mode, "").icon, null, Modifier.size(12.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.width(4.dp))
            Text(if (item.mode == ChatMode.ASK) "Ask" else "Refine", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.size(4.dp))
        Surface(
            color = MaterialTheme.colorScheme.primaryContainer,
            shape = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp, bottomStart = 18.dp, bottomEnd = 4.dp),
            modifier = Modifier.widthIn(max = 320.dp),
        ) {
            Text(
                item.text.ifBlank { "Make it agent-ready" },
                color = MaterialTheme.colorScheme.onPrimaryContainer, style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
            )
        }
    }
}

/** One proposed field change: its key, label, and the old and new values as shown. */
private data class FieldChange(val key: String, val label: String, val old: String, val new: String)

private fun changesOf(p: RefineProposal, issue: Issue) = buildList {
    if (p.title != null && p.title != issue.title) add(FieldChange("title", "Title", issue.title, p.title))
    if (p.description != null && p.description != issue.description) add(FieldChange("description", "Description", issue.description, p.description))
    if (p.acceptance_criteria != null && p.acceptance_criteria != issue.acceptance) add(FieldChange("acceptance_criteria", "Acceptance criteria", issue.acceptance, p.acceptance_criteria))
    if (p.design != null && p.design != issue.design) add(FieldChange("design", "Design", issue.design, p.design))
    if (p.notes != null && p.notes != issue.notes) add(FieldChange("notes", "Notes", issue.notes, p.notes))
    if (p.priority != null && p.priority != issue.priority) add(FieldChange("priority", "Priority", "P${issue.priority}", "P${p.priority}"))
    if (p.issue_type != null && p.issue_type != issue.type) add(FieldChange("issue_type", "Type", issue.type, p.issue_type))
    if (p.labels != null && p.labels.toSet() != issue.labels.toSet()) add(FieldChange("labels", "Labels", issue.labels.joinToString(), p.labels.joinToString()))
}

/**
 * A proposal in the thread. While it waits, each change can be kept or dropped
 * and opened to compare with what's there now; once decided, it folds down to
 * its summary and what happened to it.
 */
@Composable
internal fun ProposalCard(item: BeadChatItem.Proposal, issue: Issue, onApply: (RefineSelection) -> Unit, onDiscard: () -> Unit) {
    val p = item.proposal
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp)) {
            if (p.summary.isNotBlank()) Text(p.summary, style = MaterialTheme.typography.bodyMedium)
            when (item.outcome) {
                BeadChatItem.Outcome.APPLIED -> Outcome(Icons.Rounded.Check, "Applied ${item.applied}", MaterialTheme.colorScheme.primary)
                BeadChatItem.Outcome.DISCARDED -> Outcome(null, "Discarded", MaterialTheme.colorScheme.onSurfaceVariant)
                BeadChatItem.Outcome.PENDING -> PendingChanges(p, issue, onApply, onDiscard)
            }
        }
    }
}

@Composable
private fun Outcome(icon: ImageVector?, label: String, color: Color) {
    Row(Modifier.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        icon?.let { Icon(it, null, Modifier.size(16.dp), tint = color); Spacer(Modifier.width(4.dp)) }
        Text(label, style = MaterialTheme.typography.labelLarge, color = color)
    }
}

@Composable
private fun PendingChanges(p: RefineProposal, issue: Issue, onApply: (RefineSelection) -> Unit, onDiscard: () -> Unit) {
    val changes = changesOf(p, issue)
    var fields by remember(p) { mutableStateOf(changes.map { it.key }.toSet()) }
    var children by remember(p) { mutableStateOf(p.children.indices.toSet()) }
    var open by remember(p) { mutableStateOf<String?>(null) }

    if (changes.isEmpty() && p.children.isEmpty()) {
        Text(
            "Nothing here differs from the bead as it is now.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 8.dp),
        )
    }
    changes.forEach { c ->
        val on = c.key in fields
        val expanded = open == c.key
        Column(Modifier.fillMaxWidth().clickable { open = if (expanded) null else c.key }.padding(top = 6.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                Checkbox(checked = on, onCheckedChange = { fields = if (on) fields - c.key else fields + c.key })
                Column(Modifier.weight(1f).padding(top = 12.dp)) {
                    Text(c.label, style = MaterialTheme.typography.titleSmall)
                    if (!expanded) Text(
                        c.new.lineSequence().firstOrNull { it.isNotBlank() }.orEmpty().trimStart('#', '-', '*', ' '),
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 2, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            if (expanded) Column(Modifier.padding(start = 12.dp, end = 4.dp, bottom = 4.dp)) {
                if (c.old.isNotBlank()) Text(
                    c.old.take(600), style = MaterialTheme.typography.bodySmall.copy(textDecoration = TextDecoration.LineThrough),
                    color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 6, overflow = TextOverflow.Ellipsis,
                )
                Surface(color = Nacre.colors.doing.copy(alpha = 0.12f), shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth().padding(top = 6.dp)) {
                    MarkdownText(c.new, Modifier.padding(10.dp))
                }
            }
        }
    }
    if (p.children.isNotEmpty()) {
        Text("New child beads", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp))
        p.children.forEachIndexed { i, c ->
            val on = i in children
            Row(Modifier.fillMaxWidth().clickable { children = if (on) children - i else children + i }, verticalAlignment = Alignment.Top) {
                Checkbox(checked = on, onCheckedChange = { children = if (on) children - i else children + i })
                Column(Modifier.weight(1f).padding(top = 12.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        PriorityPill(c.priority.coerceIn(0, 4)); Spacer(Modifier.width(6.dp))
                        Text(c.issue_type.uppercase(), style = MaterialTheme.typography.labelSmall, color = Nacre.colors.type(c.issue_type))
                    }
                    Text(c.title, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
    Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
        TextButton(onClick = onDiscard) { Text("Discard") }
        Spacer(Modifier.width(8.dp))
        val n = fields.size + children.size
        Button(onClick = { onApply(RefineSelection(fields, children)) }, enabled = n > 0) { Text("Apply $n") }
    }
}
