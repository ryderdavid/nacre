package com.ryderdavid.nacre.ui.bead

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.ArrowDropDown
import androidx.compose.material.icons.rounded.ChatBubbleOutline
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.DeleteSweep
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.BeadChatItem
import com.ryderdavid.nacre.data.claude.ChatMode
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.sendOnHardwareEnter
import kotlinx.coroutines.delay

private val CHAT_SUGGESTIONS = listOf(
    "What's left to do?" to ChatMode.ASK,
    "Is it ready for an agent?" to ChatMode.ASK,
    "Make it agent-ready" to ChatMode.REFINE,
)

/**
 * The bead's one input. A pill switches it between commenting, which adds to
 * the thread every agent on the bead reads, and chatting with Claude about the
 * bead, which isn't saved; an answer worth keeping goes to the thread with
 * Add to thread. Asking for changes in the chat proposes edits to apply.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalFoundationApi::class)
@Composable
fun ThreadComposer(vm: BeadDetailViewModel, issue: Issue, comments: List<Comment>, claudeReady: Boolean, onComment: (String) -> Unit) {
    var chatting by rememberSaveable(issue.id) { mutableStateOf(false) }
    var text by rememberSaveable(issue.id) { mutableStateOf("") }
    val thinking by vm.chatThinking.collectAsState()
    val canSend = text.isNotBlank() && (!chatting || (claudeReady && !thinking))
    val send = {
        if (chatting) vm.sendChat(text, if (wantsChanges(text)) ChatMode.REFINE else ChatMode.ASK) else onComment(text.trim())
        text = ""
    }
    val scheme = MaterialTheme.colorScheme
    val field by animateColorAsState(if (chatting) scheme.primaryContainer else scheme.surfaceContainerHigh, label = "field")
    val border by animateColorAsState(if (chatting) scheme.primary else Color.Transparent, label = "border")
    val pill by animateColorAsState(if (chatting) scheme.primary else scheme.surfaceContainerHighest, label = "pill")
    val onPill by animateColorAsState(if (chatting) scheme.onPrimary else scheme.onSurface, label = "onPill")

    // Opening the chat grows the card above the field; keep the field in view,
    // with room below it so the Start work button doesn't cover it.
    val fieldInView = remember { BringIntoViewRequester() }
    var size by remember { mutableStateOf(IntSize.Zero) }
    val clearance = with(LocalDensity.current) { 96.dp.toPx() }
    LaunchedEffect(chatting) {
        if (chatting) {
            delay(300)
            fieldInView.bringIntoView(Rect(0f, 0f, size.width.toFloat(), size.height + clearance))
        }
    }

    Column(
        Modifier.fillMaxWidth().padding(top = 8.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        AnimatedVisibility(chatting, enter = expandVertically() + fadeIn(), exit = shrinkVertically() + fadeOut()) {
            BeadChatCard(vm, issue, comments, claudeReady, onFold = { chatting = false }, onSuggest = { s, mode -> vm.sendChat(s, mode) })
        }
        Surface(
            color = field, shape = RoundedCornerShape(30.dp), border = BorderStroke(1.dp, border),
            modifier = Modifier.fillMaxWidth().bringIntoViewRequester(fieldInView).onSizeChanged { size = it },
        ) {
            Row(Modifier.heightIn(min = 60.dp).padding(6.dp), verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    onClick = { chatting = !chatting }, color = pill, contentColor = onPill, shape = RoundedCornerShape(20.dp),
                    modifier = Modifier.heightIn(min = 40.dp),
                ) {
                    Row(Modifier.padding(start = 10.dp, end = 6.dp, top = 8.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(if (chatting) AiIcon else Icons.Rounded.ChatBubbleOutline, null, Modifier.size(18.dp))
                        Text(if (chatting) "Chat" else "Comment", style = MaterialTheme.typography.labelLarge)
                        Icon(Icons.Rounded.ArrowDropDown, if (chatting) "Switch to commenting" else "Switch to chatting with Claude", Modifier.size(18.dp))
                    }
                }
                TextField(
                    value = text, onValueChange = { text = it },
                    placeholder = { Text(if (chatting) "Ask Claude about this bead" else "Add a comment") },
                    modifier = Modifier.weight(1f).sendOnHardwareEnter(canSend, send),
                    maxLines = 5,
                    colors = TextFieldDefaults.colors(
                        focusedContainerColor = Color.Transparent, unfocusedContainerColor = Color.Transparent,
                        focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                    ),
                )
                FilledIconButton(onClick = send, enabled = canSend, modifier = Modifier.size(48.dp)) {
                    Icon(Icons.AutoMirrored.Rounded.Send, if (chatting) "Ask Claude" else "Post comment")
                }
            }
        }
    }
}

/** Asking for an edit goes to Claude as a refine, which proposes changes to apply. */
internal fun wantsChanges(text: String): Boolean {
    val t = text.trim().lowercase()
    return listOf("make it", "rewrite", "tighten", "split ", "add acceptance", "write acceptance", "change the", "update the", "set priority", "suggest edits")
        .any { t.startsWith(it) }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
private fun BeadChatCard(
    vm: BeadDetailViewModel, issue: Issue, comments: List<Comment>, claudeReady: Boolean,
    onFold: () -> Unit, onSuggest: (String, ChatMode) -> Unit,
) {
    val items by vm.chat.collectAsState()
    val thinking by vm.chatThinking.collectAsState()
    val failure by vm.chatFailure.collectAsState()
    val scroll = rememberScrollState()
    // Follow the newest message, including when the card first lays out.
    LaunchedEffect(items.size, thinking, scroll.maxValue) { scroll.animateScrollTo(scroll.maxValue) }
    val saved = comments.filter { it.author == "Claude" }.map { it.text.trim() }.toSet()
    Surface(
        shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant), modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(AiIcon, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.primary)
                Column(Modifier.weight(1f)) {
                    Text("Chat about ${issue.id}", style = MaterialTheme.typography.titleSmall)
                    Text("Not saved to the bead", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (items.isNotEmpty() && !thinking) IconButton(onClick = vm::clearChat) { Icon(Icons.Rounded.DeleteSweep, "Clear this chat") }
                IconButton(onClick = onFold) { Icon(Icons.Rounded.ExpandMore, "Fold chat away") }
            }
            Column(
                Modifier.padding(end = 8.dp).heightIn(max = 420.dp).verticalScroll(scroll),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                items.forEachIndexed { i, item ->
                    when (item) {
                        is BeadChatItem.Mine -> Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                            Surface(
                                color = MaterialTheme.colorScheme.primaryContainer,
                                shape = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp, bottomStart = 18.dp, bottomEnd = 4.dp),
                                modifier = Modifier.widthIn(max = 320.dp),
                            ) {
                                Text(
                                    item.text.ifBlank { "Make it agent-ready" }, style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                                )
                            }
                        }
                        is BeadChatItem.Answer -> Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            MarkdownText(item.text, Modifier.fillMaxWidth())
                            if (item.text.trim() in saved) Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                                Icon(Icons.Rounded.Check, null, Modifier.size(14.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                                Text("Added to the thread", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            } else OutlinedButton(onClick = { vm.addToThread(item.text.trim()) }) {
                                Icon(Icons.Rounded.Add, null, Modifier.size(16.dp))
                                Text("Add to thread", Modifier.padding(start = 6.dp))
                            }
                        }
                        is BeadChatItem.Proposal -> ProposalCard(
                            item, issue,
                            onApply = { sel -> vm.applyProposal(i, sel) },
                            onDiscard = { vm.discardProposal(i) },
                        )
                    }
                }
                if (thinking) Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    LoadingIndicator(Modifier.size(28.dp))
                    Text("Claude is reading ${issue.id}…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                failure?.let { f ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(f.message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, modifier = Modifier.weight(1f))
                        TextButton(onClick = vm::retryChat) { Text("Retry") }
                    }
                }
            }
            if (!claudeReady) Text(
                "Pair a computer, or add a Claude API key, in Settings to chat.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error,
            )
            if (items.isEmpty() && !thinking && claudeReady) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CHAT_SUGGESTIONS.forEach { (label, mode) ->
                    SuggestionChip(onClick = { onSuggest(if (mode == ChatMode.REFINE) "" else label, mode) }, label = { Text(label) })
                }
            }
        }
    }
}
