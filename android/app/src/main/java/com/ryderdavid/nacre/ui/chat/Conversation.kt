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
import androidx.compose.foundation.layout.heightIn
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
import androidx.compose.material.icons.rounded.ChevronRight
import androidx.compose.material.icons.rounded.Close
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

@Composable
internal fun ChatItemView(
    item: ChatItem,
    onAnswer: (String, Boolean, String?) -> Unit,
    onContinue: ((ChatItem.Draft) -> Unit)? = null,
    chatKey: String = "",
    draftNote: String? = null,
    draftBusy: Boolean = false,
) {
    when (item) {
        is ChatItem.User -> Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
            if (item.images.isNotEmpty()) com.ryderdavid.nacre.ui.common.ChatSentImages(chatKey, item.images, Modifier.padding(bottom = 4.dp))
            if (item.text.isNotBlank()) Column(
                Modifier.widthIn(max = 520.dp)
                    .background(MaterialTheme.colorScheme.primaryContainer, RoundedCornerShape(16.dp))
                    .padding(12.dp),
            ) {
                Text(item.text, color = MaterialTheme.colorScheme.onPrimaryContainer)
                item.about?.let { Text(aboutChip(it), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onPrimaryContainer) }
            } else item.about?.let { Text(aboutChip(it), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        is ChatItem.Assistant -> {
            // A reply's MEDIA: lines are mockups, shown where they were.
            val parts = androidx.compose.runtime.remember(item.text) { com.ryderdavid.nacre.data.chat.Mockups.split(item.text) }
            if (parts.size == 1 && parts[0] is com.ryderdavid.nacre.data.chat.ReplyPart.Prose) MarkdownText(item.text, Modifier.fillMaxWidth())
            else Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                parts.forEach { p ->
                    when (p) {
                        is com.ryderdavid.nacre.data.chat.ReplyPart.Prose -> MarkdownText(p.text, Modifier.fillMaxWidth())
                        is com.ryderdavid.nacre.data.chat.ReplyPart.Mockup -> com.ryderdavid.nacre.ui.common.MockupCard(chatKey, p.ref)
                    }
                }
            }
        }
        is ChatItem.ToolStep -> ToolStepView(item)
        is ChatItem.Handoff -> Text(item.text, style = MaterialTheme.typography.bodySmall, fontStyle = FontStyle.Italic, color = MaterialTheme.colorScheme.onSurfaceVariant)
        is ChatItem.Permission -> PermissionCardView(item.card, onAnswer)
        // The workspace's Chat shows these as receipts and the heads-up; nothing else draws them.
        is ChatItem.Ask, is ChatItem.HeadsUp -> Unit
        is ChatItem.Note -> Text(
            item.text, style = MaterialTheme.typography.bodySmall,
            color = if (item.error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
        is ChatItem.Draft -> if (item.latest) DraftCardView(item, onContinue, draftNote, draftBusy)
    }
}

/**
 * The newest New effort draft: its fields, and "Where does this belong?" when its computer raises intake
 * from a recorded draft. [note] is what the last tap came to when it didn't raise a card.
 */
@Composable
internal fun DraftCardView(item: ChatItem.Draft, onContinue: ((ChatItem.Draft) -> Unit)?, note: String? = null, busy: Boolean = false) {
    val draft = item.draft
    Card(Modifier.fillMaxWidth(), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.tertiaryContainer), shape = RoundedCornerShape(28.dp)) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text("Draft" + draft.type.takeIf { it.isNotBlank() }?.let { " · $it" }.orEmpty(), style = MaterialTheme.typography.labelLarge)
            Text(draft.title, style = MaterialTheme.typography.titleMedium)
            if (draft.description.isNotBlank()) Text(draft.description, style = MaterialTheme.typography.bodyMedium, maxLines = 8, overflow = TextOverflow.Ellipsis)
            if (draft.acceptance.isNotBlank()) {
                Text("Acceptance", style = MaterialTheme.typography.labelMedium)
                Text(draft.acceptance, style = MaterialTheme.typography.bodySmall, maxLines = 8, overflow = TextOverflow.Ellipsis)
            }
            if (draft.digest.isNotBlank()) {
                // What the author will be told was settled, so Ryder sees it before filing.
                Text(DRAFT_SETTLED, style = MaterialTheme.typography.labelMedium)
                Text(draft.digest, style = MaterialTheme.typography.bodySmall, maxLines = 10, overflow = TextOverflow.Ellipsis)
            }
            if (draft.design.isNotBlank()) {
                Text("Design", style = MaterialTheme.typography.labelMedium)
                Text(draft.design, style = MaterialTheme.typography.bodySmall, maxLines = 6, overflow = TextOverflow.Ellipsis)
            }
            if (onContinue != null) {
                Row(Modifier.fillMaxWidth().padding(top = 6.dp), horizontalArrangement = Arrangement.End) {
                    Button(onClick = { onContinue(item) }, enabled = !busy, modifier = Modifier.heightIn(min = 48.dp)) { Text(DRAFT_ACTION) }
                }
                Text(DRAFT_HINT, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onTertiaryContainer)
            }
            if (!note.isNullOrBlank()) Text(
                note, style = MaterialTheme.typography.bodySmall,
                color = if (note == com.ryderdavid.nacre.data.chat.DRAFT_ASKING || note.startsWith(com.ryderdavid.nacre.data.chat.DRAFT_FILED)) MaterialTheme.colorScheme.onTertiaryContainer else MaterialTheme.colorScheme.error,
            )
        }
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

/** Bead ids in [text] that are on the chat's board, in order of first mention, at most [max]. */
fun beadMentions(text: String, known: Set<String>, max: Int = 3): List<String> {
    if (known.isEmpty()) return emptyList()
    return BEAD_ID.findAll(text).map { it.value.trimEnd('.') }.filter { it in known }.distinct().take(max).toList()
}

private val BEAD_ID = Regex("""\b[A-Za-z][A-Za-z0-9]*-[A-Za-z0-9]+(?:\.[A-Za-z0-9]+)*""")

/** The Draft card's one action: raise intake on this draft. */
const val DRAFT_ACTION = "Where does this belong?"

/** Under the Draft card's action: the other way forward. */
const val DRAFT_HINT = "Or tell the chat what to change."

/** The Draft card's heading for the brainstorm digest. */
const val DRAFT_SETTLED = "What we settled"
