package com.ryderdavid.nacre.ui.workspace.decisions

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.snap
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Help
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.MergeType
import androidx.compose.material.icons.rounded.PhoneAndroid
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialShapes
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.toShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionAnswer
import com.ryderdavid.nacre.data.model.DecisionEvidence
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.theme.LocalReducedMotion
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.ui.workspace.LocalAskAbout
import com.ryderdavid.nacre.ui.workspace.LocalOpenQueue
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Unsent card input, kept per decision for as long as the app runs (switching thread or pane keeps it), and
 * which decisions are being sent, so the same card can't be sent twice from the queue and the thread.
 */
object DecisionDrafts {
    val all = MutableStateFlow<Map<String, DecisionDraft>>(emptyMap())
    val sending = MutableStateFlow<Set<String>>(emptySet())
    fun set(key: String, draft: DecisionDraft) = all.update { it + (key to draft) }
    fun clear(key: String) = all.update { it - key }
    fun begin(key: String): Boolean {
        var started = false
        sending.update { if (key in it) it else { started = true; it + key } }
        return started
    }
    fun end(key: String) = sending.update { it - key }
}

private fun kindIcon(kind: String): ImageVector = when (kind) {
    "approve", "adopt", "limit" -> Icons.Rounded.Lock
    "land", "finish" -> Icons.Rounded.MergeType
    "signoff" -> Icons.Rounded.PhoneAndroid
    "recovery" -> Icons.Rounded.ErrorOutline
    "found" -> Icons.Rounded.Search
    else -> Icons.Rounded.Help
}

/** The exact facts a decision is bound to: revision, commit, round and change. */
fun decisionFacts(d: Decision): List<String> = buildList {
    val t = d.target
    if (t.change.isNotBlank()) add(t.change)
    if (t.revision.isNotBlank()) add("revision ${t.revision.take(7)}")
    if (t.commit.isNotBlank()) add(t.commit.take(7))
    if (t.round.isNotBlank()) add("round ${t.round}")
    if (t.bead.isNotBlank() && t.change.isBlank()) add(t.bead)
}

/**
 * One owner decision: kind, title, the exact revision it's bound to, consequences and evidence; the
 * options (recommended first, each with its own consequence) and any input they need; Defer and Reject apart;
 * then the truthful state, which an answered card keeps as a receipt.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun DecisionCard(d: Decision, onEvidence: (DecisionEvidence) -> Unit, modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    val drafts by DecisionDrafts.all.collectAsState()
    val inFlight by DecisionDrafts.sending.collectAsState()
    val draft = drafts[d.key] ?: DecisionDraft(option = DecisionModel.initialOption(d))
    val set: (DecisionDraft) -> Unit = { DecisionDrafts.set(d.key, it) }
    val sending = d.key in inFlight
    var expanded by remember(d.key) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val reduced = LocalReducedMotion.current
    val ask = LocalAskAbout.current
    val openQueue = LocalOpenQueue.current
    val line = DecisionModel.stateLine(d)
    val facts = decisionFacts(d)
    val stateText = line ?: "Waiting for your answer"

    val submit: (DecisionAnswer, Boolean) -> Unit = { answer, clearDraft ->
        if (DecisionDrafts.begin(d.key)) scope.launch {
            runCatching { container.repository.answerDecision(d, answer) }
                .onSuccess { if (clearDraft) DecisionDrafts.clear(d.key) }
                .onFailure { container.repository.notify("Not sent: ${it.message}") }
            DecisionDrafts.end(d.key)
        }
    }

    if (line != null && !expanded) {
        // An answered card collapses to its receipt; it can be expanded to read it again.
        Surface(
            onClick = { expanded = true }, shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.surfaceContainerLow,
            modifier = modifier.fillMaxWidth().heightIn(min = 48.dp).semantics {
                contentDescription = "${DecisionModel.kindLabel(d.kind)}: ${d.title}. $line. Show details"
                liveRegion = LiveRegionMode.Polite
            },
        ) {
            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                val ok = d.state == "done"
                Icon(if (ok) Icons.Rounded.CheckCircle else if (d.state == "executing") Icons.Rounded.Schedule else Icons.Rounded.ErrorOutline, null,
                    tint = if (ok) MaterialTheme.colorScheme.primary else if (d.state in setOf("failed", "unknown")) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                Column(Modifier.weight(1f)) {
                    Text(d.title, style = MaterialTheme.typography.bodyMedium)
                    Text(line, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (DecisionModel.canRenew(d)) TextButton(onClick = {
                    scope.launch { runCatching { container.repository.renewDecision(d) }.onFailure { container.repository.notify("Couldn't ask again: ${it.message}") } }
                }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Ask again") }
                if (d.state == "superseded") TextButton(onClick = openQueue, modifier = Modifier.heightIn(min = 48.dp)) { Text("See the newer one") }
            }
        }
        return
    }

    Surface(
        shape = RoundedCornerShape(32.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = modifier.fillMaxWidth()
            .then(if (reduced) Modifier else Modifier.animateContentSize(MaterialTheme.motionScheme.defaultSpatialSpec()))
            .semantics { stateDescription = stateText },
    ) {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Box(Modifier.size(36.dp).background(MaterialTheme.colorScheme.tertiaryContainer, MaterialShapes.Cookie9Sided.toShape()), contentAlignment = Alignment.Center) {
                    Icon(kindIcon(d.kind), null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.onTertiaryContainer)
                }
                Text(DecisionModel.kindLabel(d.kind), style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onTertiaryContainer)
            }
            Column {
                Text(d.title, style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.semantics { heading() })
                val sub = (listOf(d.subtitle).filter { it.isNotBlank() } + facts.filterNot { f -> d.subtitle.contains(f) }).joinToString(" · ")
                if (sub.isNotBlank()) SelectionContainer { Text(sub, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            }
            if (d.body.isNotBlank()) {
                if (d.kind == "recovery") Text(
                    d.body, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace,
                    modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceContainer, RoundedCornerShape(12.dp)).padding(10.dp),
                ) else com.ryderdavid.nacre.ui.common.MarkdownText(d.body)
            }
            if (d.consequences.isNotEmpty()) Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                d.consequences.forEach { c ->
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        Icon(Icons.Rounded.CheckCircle, null, Modifier.size(18.dp).padding(top = 1.dp), tint = MaterialTheme.colorScheme.primary)
                        Text(c, style = MaterialTheme.typography.bodyMedium)
                    }
                }
            }
            if (d.evidence.isNotEmpty()) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                d.evidence.forEach { e -> AssistChip(onClick = { onEvidence(e) }, label = { Text(e.label) }, modifier = Modifier.heightIn(min = 48.dp)) }
            }

            if (d.pending && !draft.rejecting && d.options.isNotEmpty()) Column(Modifier.selectableGroup(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                DecisionModel.ordered(d).forEach { o ->
                    val selected = draft.option == o.id
                    val radius by animateDpAsState(if (selected) 28.dp else 20.dp, if (reduced) snap() else MaterialTheme.motionScheme.fastSpatialSpec(), label = "option")
                    Surface(
                        shape = RoundedCornerShape(radius),
                        color = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
                        contentColor = if (selected) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
                            .selectable(selected = selected, enabled = o.disabled.isBlank(), onClick = { set(draft.copy(option = o.id)) }, role = Role.RadioButton),
                    ) {
                        Row(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            RadioButton(selected = selected, onClick = null)
                            Column(Modifier.weight(1f)) {
                                Text(o.label + if (o.recommended) " · Recommended" else "", style = MaterialTheme.typography.labelLarge)
                                if (o.detail.isNotBlank()) Text(o.detail, style = MaterialTheme.typography.bodySmall)
                                if (o.disabled.isNotBlank()) Text(o.disabled, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                            }
                            if (o.protected) Icon(Icons.Rounded.Lock, "Needs your approval", Modifier.size(16.dp))
                        }
                    }
                }
                val option = d.options.firstOrNull { it.id == draft.option }
                option?.input?.let { spec ->
                    when (spec.kind) {
                        "choice" -> Column(Modifier.padding(top = 4.dp)) {
                            Text(spec.label.ifBlank { "Pick one" }, style = MaterialTheme.typography.labelLarge)
                            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                spec.choices.forEach { c ->
                                    androidx.compose.material3.FilterChip(selected = draft.input == c.id, onClick = { set(draft.copy(input = if (draft.input == c.id) "" else c.id)) }, label = { Text(c.label) }, modifier = Modifier.heightIn(min = 48.dp))
                                }
                            }
                        }
                        else -> OutlinedTextField(
                            value = draft.input, onValueChange = { set(draft.copy(input = it)) },
                            label = { Text(spec.label.ifBlank { if (spec.kind == "amount") "Amount" else "Your answer" }) },
                            prefix = if (spec.kind == "amount" && spec.unit == "usd") ({ Text("$") }) else null,
                            keyboardOptions = if (spec.kind == "amount") KeyboardOptions(keyboardType = KeyboardType.Decimal) else KeyboardOptions.Default,
                            isError = DecisionModel.inputProblem(option, draft.input) != null && draft.input.isNotEmpty(),
                            supportingText = DecisionModel.inputProblem(option, draft.input)?.takeIf { draft.input.isNotEmpty() }?.let { p -> { Text(p) } },
                            modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                            minLines = if (spec.kind == "text") 2 else 1,
                        )
                    }
                }
            }

            val askable = d.kind in setOf("question", "intake", "found", "recovery") || (d.options.isEmpty() && !d.allowReject)
            if (d.pending) {
                if (draft.rejecting || draft.note.isNotEmpty()) OutlinedTextField(
                    value = draft.note, onValueChange = { set(draft.copy(note = it)) },
                    label = { Text(if (draft.rejecting) "Why? (sent with the rejection)" else "Note (optional)") },
                    modifier = Modifier.fillMaxWidth(), minLines = 2,
                )
                val problem = DecisionModel.problem(d, draft)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                    if (d.options.isNotEmpty() || draft.rejecting) {
                        if (draft.rejecting) {
                            Button(onClick = { submit(DecisionModel.answer(d, draft), true) }, enabled = problem == null && !sending, modifier = Modifier.heightIn(min = 48.dp)) { Text(d.rejectLabel.ifBlank { "Reject" }) }
                            TextButton(onClick = { set(draft.copy(rejecting = false)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Back") }
                        } else {
                            Button(onClick = { submit(DecisionModel.answer(d, draft), true) }, enabled = problem == null && !sending, modifier = Modifier.heightIn(min = 48.dp)) { Text(DecisionModel.sendLabel(d)) }
                        }
                    }
                    if (!draft.rejecting) {
                        if (d.allowDefer) TextButton(onClick = { submit(DecisionAnswer(defer = true), false) }, enabled = !sending, modifier = Modifier.heightIn(min = 48.dp)) { Text("Defer") }
                        if (draft.note.isEmpty() && d.options.isNotEmpty()) TextButton(onClick = { set(draft.copy(note = " ")) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Add a note") }
                        if (askable) FilledTonalButton(onClick = { ask(ChatReference("decision", d.id, d.title, d.body)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Ask about this") }
                        if (d.allowReject) TextButton(onClick = { set(draft.copy(rejecting = true, note = draft.note.trim())) }, enabled = !sending, modifier = Modifier.heightIn(min = 48.dp)) {
                            Text(d.rejectLabel.ifBlank { "Reject" } + "…", color = MaterialTheme.colorScheme.error)
                        }
                    }
                    if (sending) LoadingIndicator(Modifier.size(32.dp))
                }
                if (problem != null && problem != "Pick an answer" && d.options.isNotEmpty()) Text(problem, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            } else if (line != null) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) {
                    if (d.state == "executing") LoadingIndicator(Modifier.size(28.dp))
                    Text(line, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                }
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (d.state == "held") {
                        // Held work keeps its next actions: land it after all (a fresh decision), or cancel the round.
                        Button(onClick = { scope.launch { runCatching { container.repository.renewDecision(d) }.onFailure { container.repository.notify("Couldn't ask again: ${it.message}") } } }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Land now…") }
                        if (d.target.round.isNotBlank()) OutlinedButton(onClick = { container.repository.cancelRoundQuietly(Keys.of(d.hostId, d.target.round)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Cancel round") }
                    }
                    if (DecisionModel.canRenew(d)) TextButton(onClick = { scope.launch { runCatching { container.repository.renewDecision(d) } } }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Ask again") }
                    if (d.state == "superseded") TextButton(onClick = openQueue, modifier = Modifier.heightIn(min = 48.dp)) { Text("See the newer one") }
                    if (d.state in setOf("failed", "unknown")) TextButton(onClick = { submit(DecisionAnswer(acknowledge = true), false) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Got it") }
                    TextButton(onClick = { expanded = false }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Collapse") }
                }
            }
        }
    }
}

/** A tool step the agent asks to take: the full step, the reason, then Deny and Allow. A deny note is kept per request. */
@Composable
fun PermissionCardM3(card: PermissionCard, agent: String, onAnswer: (String, Boolean, String?) -> Unit, modifier: Modifier = Modifier) {
    if (!card.pending) {
        Text(card.tool + ": " + card.outcomeLine, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = modifier.padding(horizontal = 4.dp))
        return
    }
    val key = "perm:" + card.requestId
    val drafts by DecisionDrafts.all.collectAsState()
    val draft = drafts[key] ?: DecisionDraft()
    val denying = draft.rejecting
    Surface(shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest, modifier = modifier.fillMaxWidth().semantics { stateDescription = "Waiting for your answer" }) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(Icons.Rounded.Terminal, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("${agent.replaceFirstChar { it.uppercase() }} asks to use ${card.tool}", style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
            }
            SelectionContainer {
                Text(
                    card.step, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace,
                    modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceContainerLowest, RoundedCornerShape(12.dp)).padding(10.dp),
                )
            }
            if (card.reason.isNotBlank()) Text(card.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (denying) OutlinedTextField(value = draft.note, onValueChange = { DecisionDrafts.set(key, draft.copy(note = it)) }, label = { Text("Tell it why (optional)") }, modifier = Modifier.fillMaxWidth())
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                if (denying) {
                    TextButton(onClick = { DecisionDrafts.set(key, draft.copy(rejecting = false)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Back") }
                    Button(onClick = { onAnswer(card.requestId, false, draft.note.ifBlank { null }); DecisionDrafts.clear(key) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Deny") }
                } else {
                    OutlinedButton(onClick = { DecisionDrafts.set(key, draft.copy(rejecting = true)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Deny…") }
                    Button(onClick = { onAnswer(card.requestId, true, null); DecisionDrafts.clear(key) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Allow") }
                }
            }
        }
    }
}
