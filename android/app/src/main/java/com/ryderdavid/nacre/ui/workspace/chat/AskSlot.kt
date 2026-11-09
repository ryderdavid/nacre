package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.MoreHoriz
import androidx.compose.material3.AssistChip
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.ChatConversation
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionAnswer
import com.ryderdavid.nacre.data.model.DecisionOption
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionDraft
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionDrafts
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/**
 * What the slot above the composer asks: a permission request first, then the decision the chat is
 * asking about, or, in a walk-through between turns, the next decision waiting (marked [next]).
 */
sealed interface AskTarget {
    data class Permission(val card: PermissionCard) : AskTarget
    data class Ask(val decision: Decision, val next: Boolean = false) : AskTarget
}

object AskSlotModel {
    /**
     * [asking] is the decision the chat asks about (nacred's `asking`, or its last ask event); [queue] the
     * project's decisions in queue order; [walkThrough] whether the chat runs the Needs you walk-through.
     */
    fun target(
        permissions: List<PermissionCard>,
        asking: String?,
        queue: List<Decision>,
        walkThrough: Boolean,
        turnRunning: Boolean,
    ): AskTarget? {
        permissions.firstOrNull { it.pending }?.let { return AskTarget.Permission(it) }
        val asked = asking?.let { id -> queue.firstOrNull { it.id == id } }
        if (asked != null && asked.needsYou) return AskTarget.Ask(asked)
        // The agent forgot to name the next one: offer it, so Ryder is never stuck.
        if (walkThrough && !turnRunning) queue.firstOrNull { it.needsYou }?.let { return AskTarget.Ask(it, next = true) }
        return null
    }

    /** The decisions still waiting on Ryder (pending or held). */
    fun left(queue: List<Decision>) = queue.count { it.needsYou }

    /** The slot's small heading: the kind, and how many wait. */
    fun heading(d: Decision, left: Int, next: Boolean): String {
        val kind = DecisionModel.kindLabel(d.kind).removePrefix("Needs you · ")
        val prefix = if (next) "Next · " else ""
        return prefix + kind + if (left > 1) " · $left waiting" else ""
    }

    /** The line under the chips: the focused option's own detail, else nacred's first consequences. */
    fun consequence(d: Decision, focused: DecisionOption?): String =
        focused?.detail?.ifBlank { null } ?: d.consequences.take(2).joinToString(" · ")

    /** What the transcript says once an option was tapped. */
    fun answeredText(label: String) = "Answered: $label"

    /** A chat runs the walk-through once Ryder has sent /needs-you in it. */
    fun walkThrough(items: List<com.ryderdavid.nacre.data.chat.ChatItem>) =
        items.any { it is com.ryderdavid.nacre.data.chat.ChatItem.User && it.text == com.ryderdavid.nacre.data.chat.NEEDS_YOU_SHOWN }
}

/**
 * The composer as an answer box for one decision or permission request: Send answers it instead of
 * sending a chat message. [option] is the option whose input is typed; [rejecting] a reject's note;
 * [permission] a deny note.
 */
data class AnswerMode(
    val decisionKey: String? = null,
    val option: String? = null,
    val rejecting: Boolean = false,
    val permission: String? = null,
    val prompt: String,
)

/** The answer box each chat's composer is, by chat key. */
object AnswerModes {
    val all = MutableStateFlow<Map<String, AnswerMode>>(emptyMap())
    fun set(thread: String, mode: AnswerMode) = all.update { it + (thread to mode) }
    fun clear(thread: String) = all.update { it - thread }
}

/**
 * Answers decision [d] from the phone, then tells the chat with "Answered: <label>" so the walk-through
 * moves on. The message waits (up to two minutes) for a running reply to end.
 */
fun answerAndReport(
    container: com.ryderdavid.nacre.AppContainer,
    conv: ChatConversation,
    d: Decision,
    answer: DecisionAnswer,
    label: String,
    context: ScreenContext?,
    onDone: () -> Unit = {},
) {
    if (!DecisionDrafts.begin(d.key)) return
    container.scope.launch {
        val ok = runCatching { container.repository.answerDecision(d, answer) }
            .onFailure { container.repository.notify("Not sent: ${it.message}") }.isSuccess
        DecisionDrafts.end(d.key)
        if (!ok) return@launch
        DecisionDrafts.clear(d.key)
        onDone()
        withTimeoutOrNull(120_000) { conv.state.first { !it.turnRunning } }
        conv.sendNow(AskSlotModel.answeredText(label), context, answered = d.id)
    }
}

/** The slot above the composer: the asked decision's own options as chips, or a permission's Allow and Deny. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun AskSlot(target: AskTarget, thread: String, left: Int, agent: String, context: () -> ScreenContext?) {
    val container = LocalContainer.current
    val conv = remember(thread) { container.chats.conversation(thread) }
    Surface(
        shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier.fillMaxWidth().semantics { liveRegion = LiveRegionMode.Polite },
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            when (target) {
                is AskTarget.Permission -> PermissionAsk(target.card, thread, agent, conv)
                is AskTarget.Ask -> DecisionAsk(target.decision, target.next, thread, left, conv, context)
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun PermissionAsk(card: PermissionCard, thread: String, agent: String, conv: ChatConversation) {
    Text(
        "${agent.replaceFirstChar { it.uppercase() }} asks to use ${card.tool}",
        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.semantics { heading() },
    )
    Text(card.step, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace, maxLines = 3, overflow = TextOverflow.Ellipsis)
    if (card.reason.isNotBlank()) Text(card.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        FilterChip(
            selected = true, onClick = { AnswerModes.clear(thread); conv.answer(card.requestId, true, null) },
            label = { Text("Allow") }, modifier = Modifier.heightIn(min = 48.dp),
        )
        AssistChip(
            onClick = { AnswerModes.set(thread, AnswerMode(permission = card.requestId, prompt = "Why deny it? (optional)")) },
            label = { Text("Deny…") }, modifier = Modifier.heightIn(min = 48.dp),
        )
    }
}

@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun DecisionAsk(d: Decision, next: Boolean, thread: String, left: Int, conv: ChatConversation, context: () -> ScreenContext?) {
    val container = LocalContainer.current
    val inFlight by DecisionDrafts.sending.collectAsState()
    val sending = d.key in inFlight
    var showAll by remember(d.key) { mutableStateOf(false) }
    var menu by remember(d.key) { mutableStateOf(false) }
    val options = DecisionModel.ordered(d)
    var focused by remember(d.key) { mutableStateOf(options.firstOrNull { it.disabled.isBlank() }) }

    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            AskSlotModel.heading(d, left, next), style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f),
        )
        if (sending) LoadingIndicator(Modifier.size(24.dp))
    }
    Text(d.title, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.semantics { heading() })

    val choose: (DecisionOption) -> Unit = choose@{ o ->
        focused = o
        if (o.input != null || d.ask != null) {
            // Something to type first: the composer becomes this decision's answer box.
            AnswerModes.set(thread, AnswerMode(decisionKey = d.key, option = o.id, prompt = o.input?.label?.ifBlank { null } ?: d.ask?.label?.ifBlank { null } ?: "Answer: ${o.label}"))
            return@choose
        }
        answerAndReport(container, conv, d, DecisionAnswer(option = o.id), o.label, context())
    }

    if (d.state == "failed" || d.state == "unknown") {
        // An outcome to take in: what nacred reports, then Got it (and Ask again where nacred can rebuild it).
        (d.result?.message?.ifBlank { null } ?: d.reason.ifBlank { null })?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, maxLines = 3, overflow = TextOverflow.Ellipsis)
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            FilterChip(selected = true, enabled = !sending, onClick = { answerAndReport(container, conv, d, DecisionAnswer(acknowledge = true), "Got it", context()) },
                label = { Text("Got it") }, modifier = Modifier.heightIn(min = 48.dp))
        }
        return
    }
    if (d.state == "held") {
        // Held work keeps its next actions: land it after all (a fresh decision), or cancel the round.
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            FilterChip(selected = true, onClick = {
                container.scope.launch { runCatching { container.repository.renewDecision(d) }.onFailure { container.repository.notify("Couldn't ask again: ${it.message}") } }
            }, label = { Text("Land now…") }, modifier = Modifier.heightIn(min = 48.dp))
            if (d.target.round.isNotBlank()) AssistChip(onClick = { container.repository.cancelRoundQuietly(Keys.of(d.hostId, d.target.round)) }, label = { Text("Cancel round") }, modifier = Modifier.heightIn(min = 48.dp))
        }
        return
    }

    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        val folded = if (showAll) emptySet() else DecisionModel.foldedOptions(d, null)
        var foldShown = false
        options.forEach { o ->
            if (o.id in folded) {
                if (!foldShown) {
                    foldShown = true
                    SuggestionChip(onClick = { showAll = true }, label = { Text("${folded.size} more") }, modifier = Modifier.heightIn(min = 48.dp))
                }
                return@forEach
            }
            FilterChip(
                selected = o.recommended, enabled = o.disabled.isBlank() && !sending,
                onClick = { choose(o) },
                label = { Text(o.label, maxLines = 2, overflow = TextOverflow.Ellipsis) },
                leadingIcon = if (o.protected) ({ Icon(Icons.Rounded.Lock, null, Modifier.size(16.dp)) }) else null,
                modifier = Modifier.heightIn(min = 48.dp).semantics {
                    contentDescription = o.label + (if (o.recommended) ", recommended" else "") + (o.detail.takeIf { it.isNotBlank() }?.let { ". $it" } ?: "")
                },
            )
        }
        if (d.allowDefer) AssistChip(
            enabled = !sending,
            onClick = { answerAndReport(container, conv, d, DecisionAnswer(defer = true), "Skip", context()) },
            label = { Text("Skip") }, modifier = Modifier.heightIn(min = 48.dp),
        )
        if (d.allowReject || options.isNotEmpty()) androidx.compose.foundation.layout.Box {
            AssistChip(
                onClick = { menu = true }, label = { Text("More") },
                leadingIcon = { Icon(Icons.Rounded.MoreHoriz, null, Modifier.size(18.dp)) },
                modifier = Modifier.heightIn(min = 48.dp),
            )
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                if (d.allowReject) DropdownMenuItem(
                    text = { Text(d.rejectLabel.ifBlank { "Reject" } + "…") },
                    onClick = {
                        menu = false
                        AnswerModes.set(thread, AnswerMode(decisionKey = d.key, rejecting = true, prompt = DecisionModel.rejectNoteLabel(d)))
                    },
                )
                focused?.takeIf { it.input == null && it.disabled.isBlank() }?.let { o ->
                    DropdownMenuItem(
                        text = { Text("${o.label}, with a note…") },
                        onClick = {
                            menu = false
                            AnswerModes.set(thread, AnswerMode(decisionKey = d.key, option = o.id, prompt = "Note for ${o.label} (optional)"))
                        },
                    )
                }
            }
        }
    }
    val line = AskSlotModel.consequence(d, focused)
    if (line.isNotBlank()) Text(line, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
}

/**
 * Sends what the composer holds as the answer [mode] stands for. Returns false when it can't be sent as
 * it stands (a required note or input is missing), leaving the composer as it is.
 */
fun sendAnswer(
    container: com.ryderdavid.nacre.AppContainer,
    thread: String,
    mode: AnswerMode,
    text: String,
    decisions: List<Decision>,
    context: ScreenContext?,
    onSent: () -> Unit,
): Boolean {
    val conv = container.chats.conversation(thread)
    mode.permission?.let { req ->
        conv.answer(req, false, text.trim().ifBlank { null })
        AnswerModes.clear(thread)
        onSent()
        return true
    }
    val d = decisions.firstOrNull { it.key == mode.decisionKey } ?: run { AnswerModes.clear(thread); return false }
    val option = d.options.firstOrNull { it.id == mode.option }
    val draft = when {
        mode.rejecting -> DecisionDraft(rejecting = true, note = text)
        option?.input != null -> DecisionDraft(option = option.id, input = text)
        d.ask != null -> DecisionDraft(option = option?.id, ask = text)
        else -> DecisionDraft(option = option?.id, note = text)
    }
    if (DecisionModel.problem(d, draft) != null) return false
    val label = when {
        mode.rejecting -> (d.rejectLabel.ifBlank { "Reject" }) + ": " + text.trim()
        option != null && text.isNotBlank() -> "${option.label}: ${text.trim()}"
        option != null -> option.label
        else -> text.trim()
    }
    answerAndReport(container, conv, d, DecisionModel.answer(d, draft), label, context) { AnswerModes.clear(thread) }
    onSent()
    return true
}
