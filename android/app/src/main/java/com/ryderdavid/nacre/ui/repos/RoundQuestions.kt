package com.ryderdavid.nacre.ui.repos

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.ChatBubbleOutline
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.BeadChatItem
import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.AttentionOption
import com.ryderdavid.nacre.data.model.AttentionQuestion
import com.ryderdavid.nacre.ui.bead.BeadDetailViewModel
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject

/**
 * A round's open questions in one card. Each question lists Claude's
 * recommended answer first, marked and with its reason, and folds out a chat
 * about it; Accept all answers every question with its recommendation.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun RoundQuestionsCard(item: AttentionItem, busy: Boolean, showComputer: Boolean, claudeReady: Boolean, onOption: (AttentionOption) -> Unit) {
    Surface(
        shape = RoundedCornerShape(20.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        color = MaterialTheme.colorScheme.surface,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Surface(color = Nacre.colors.needsYouContainer, contentColor = Nacre.colors.needsYou, shape = RoundedCornerShape(8.dp)) {
                    Text("Before work starts", style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp))
                }
                val where = listOfNotNull(
                    item.repoName.ifBlank { null },
                    item.hostName.takeIf { showComputer && it.isNotBlank() },
                    item.beadId.ifBlank { null },
                ).joinToString(" · ")
                Text(
                    where, style = MaterialTheme.typography.labelSmall.copy(fontFamily = MonoFamily),
                    color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).padding(start = 8.dp), textAlign = TextAlign.End,
                )
            }
            Text(item.title, style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Medium))
            if (item.why.isNotBlank()) Text(item.why, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            item.questions.forEachIndexed { i, q ->
                HorizontalDivider(Modifier.padding(vertical = 6.dp), color = MaterialTheme.colorScheme.outlineVariant)
                QuestionBlock(i + 1, q, item, busy, claudeReady, onOption)
            }
            HorizontalDivider(Modifier.padding(vertical = 6.dp), color = MaterialTheme.colorScheme.outlineVariant)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                item.options.forEach { o ->
                    if (o.recommended) Button(onClick = { onOption(o) }, enabled = !busy) { Text(o.label) }
                    else OutlinedButton(onClick = { onOption(o) }, enabled = !busy) { Text(o.label) }
                }
            }
        }
    }
}

@Composable
private fun QuestionBlock(n: Int, q: AttentionQuestion, item: AttentionItem, busy: Boolean, claudeReady: Boolean, onOption: (AttentionOption) -> Unit) {
    var open by rememberSaveable(item.roundId, q.id) { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text("$n. ${q.text}", style = MaterialTheme.typography.bodyLarge)
        q.options.forEach { o ->
            AnswerButton(o, busy) { onOption(o) }
            if (o.recommended && q.reason.isNotBlank()) Text(
                q.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 12.dp),
            )
        }
        TextButton(onClick = { open = !open }) {
            Icon(Icons.Rounded.ChatBubbleOutline, null, Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text(if (open) "Hide chat" else "Ask Claude")
            Icon(if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, null)
        }
        AnimatedVisibility(open) {
            if (item.beadId.isNotBlank()) QuestionChat(item, q, busy, claudeReady, onOption)
        }
    }
}

/** An answer: the recommended one outlined in the accent color and labelled. */
@Composable
private fun AnswerButton(o: AttentionOption, busy: Boolean, onClick: () -> Unit) {
    OutlinedButton(
        onClick = onClick, enabled = !busy && o.disabled.isBlank(), modifier = Modifier.fillMaxWidth(),
        border = if (o.recommended) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            if (o.recommended) {
                Surface(color = MaterialTheme.colorScheme.primaryContainer, contentColor = MaterialTheme.colorScheme.onPrimaryContainer, shape = RoundedCornerShape(6.dp)) {
                    Text("Recommended", style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp))
                }
                Spacer(Modifier.width(8.dp))
            }
            Text(o.label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurface)
        }
    }
}

/**
 * The fold-out chat about one question, in the bead's own thread. What Ryder
 * types can be asked of Claude or sent as his own answer.
 */
@Composable
private fun QuestionChat(item: AttentionItem, q: AttentionQuestion, busy: Boolean, claudeReady: Boolean, onOption: (AttentionOption) -> Unit) {
    val key = item.repoKey
    val vm = nacreViewModel(key = "bead:$key:${item.beadId}") { BeadDetailViewModel(it.repository, it.backends, it.settings, it.beadChats, key, item.beadId) }
    val chat by vm.chat.collectAsState()
    val thinking by vm.chatThinking.collectAsState()
    val failure by vm.chatFailure.collectAsState()
    var text by rememberSaveable(item.roundId, q.id) { mutableStateOf("") }
    val about = questionContext(q)
    // Only this question's exchange: from its first message on.
    val start = chat.indexOfFirst { it is BeadChatItem.Mine && it.about == about }
    val shown = if (start < 0) emptyList() else chat.drop(start)
    val scroll = rememberScrollState()
    LaunchedEffect(shown.size, thinking) { scroll.animateScrollTo(scroll.maxValue) }
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (shown.isEmpty() && !thinking) Text(
                "Ask why Claude recommends this, what the other answers would mean, or anything else about it.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (shown.isNotEmpty() || thinking) Column(
                Modifier.heightIn(max = 320.dp).verticalScroll(scroll), verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                shown.forEach { m ->
                    when (m) {
                        is BeadChatItem.Mine -> Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                            Surface(color = MaterialTheme.colorScheme.primaryContainer, shape = RoundedCornerShape(14.dp), modifier = Modifier.widthIn(max = 280.dp)) {
                                Text(m.text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                            }
                        }
                        is BeadChatItem.Answer -> MarkdownText(m.text, Modifier.fillMaxWidth())
                        else -> Unit
                    }
                }
                if (thinking) Text("Claude is thinking…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                failure?.let { Text(it.message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
            }
            if (!claudeReady) Text("Pair a computer, or add a Claude API key, in Settings to chat.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = text, onValueChange = { text = it }, modifier = Modifier.weight(1f),
                    placeholder = { Text("Ask about this question") }, shape = RoundedCornerShape(24.dp), maxLines = 4,
                )
                IconButton(
                    onClick = { vm.askAbout(about, text); text = "" },
                    enabled = claudeReady && !thinking && text.isNotBlank(),
                ) { Icon(Icons.AutoMirrored.Rounded.Send, "Ask Claude") }
            }
            if (text.isNotBlank()) TextButton(onClick = { onOption(ownAnswer(item.roundId, q.id, text.trim())); text = "" }, enabled = !busy) {
                Text("Answer with what I wrote")
            }
        }
    }
}

/** What Claude is told about the question being discussed. */
internal fun questionContext(q: AttentionQuestion): String = buildString {
    append("A round asks Ryder this before work starts: ").append(q.text).append('\n')
    append("Answers: ").append(q.options.joinToString(" | ") { if (it.recommended) "${it.label} (recommended)" else it.label })
    if (q.reason.isNotBlank()) append("\nWhy that one is recommended: ").append(q.reason)
    append("\nAnswer his question about it plainly and say which answer you'd pick.")
}

/** Ryder's own answer, sent the way the listed answers are. */
internal fun ownAnswer(roundId: String, questionId: String, answer: String) = AttentionOption(
    type = "action", label = "Your answer", method = "POST", path = "/v1/rounds/$roundId/answers",
    body = buildJsonObject { put("question_id", JsonPrimitive(questionId)); put("answer", JsonPrimitive(answer)) },
)
