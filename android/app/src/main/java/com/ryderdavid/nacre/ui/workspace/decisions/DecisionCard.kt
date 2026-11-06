package com.ryderdavid.nacre.ui.workspace.decisions

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateDpAsState
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionEvidence
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Unsent card input, kept per decision for as long as the app runs (switching thread or pane keeps it). */
object DecisionDrafts {
    val all = MutableStateFlow<Map<String, DecisionDraft>>(emptyMap())
    fun get(key: String, d: Decision) = all.value[key] ?: DecisionDraft(option = DecisionModel.initialOption(d))
    fun set(key: String, draft: DecisionDraft) = all.update { it + (key to draft) }
    fun clear(key: String) = all.update { it - key }
}

private fun kindIcon(kind: String): ImageVector = when (kind) {
    "approve", "adopt", "limit" -> Icons.Rounded.Lock
    "land", "finish" -> Icons.Rounded.MergeType
    "signoff" -> Icons.Rounded.PhoneAndroid
    "recovery" -> Icons.Rounded.ErrorOutline
    "found" -> Icons.Rounded.Search
    else -> Icons.Rounded.Help
}

/**
 * One owner decision: kind, title, the exact revision, consequences and evidence; the options (each with
 * its own consequence) and any input they need; Defer and Reject apart; then the truthful state.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun DecisionCard(d: Decision, onEvidence: (DecisionEvidence) -> Unit, modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    val drafts by DecisionDrafts.all.collectAsState()
    val draft = drafts[d.key] ?: DecisionDraft(option = DecisionModel.initialOption(d))
    val set: (DecisionDraft) -> Unit = { DecisionDrafts.set(d.key, it) }
    var sending by remember(d.key) { mutableStateOf(false) }
    var expanded by remember(d.key) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val line = DecisionModel.stateLine(d)

    if (line != null && !expanded) {
        // An answered card collapses to its receipt; it can be expanded to read it again.
        Surface(
            onClick = { expanded = true }, shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.surfaceContainerLow,
            modifier = modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = "${DecisionModel.kindLabel(d.kind)}: ${d.title}. $line. Show details" },
        ) {
            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                val ok = d.state == "done"
                Icon(if (ok) Icons.Rounded.CheckCircle else if (d.state == "executing") Icons.Rounded.Schedule else Icons.Rounded.ErrorOutline, null,
                    tint = if (ok) MaterialTheme.colorScheme.primary else if (d.state in setOf("failed", "unknown")) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                Column(Modifier.weight(1f)) {
                    Text(d.title, style = MaterialTheme.typography.bodyMedium, maxLines = 1)
                    Text(line, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (DecisionModel.canRenew(d)) TextButton(onClick = {
                    scope.launch { runCatching { container.repository.renewDecision(d) }.onFailure { container.repository.notify("Couldn't ask again: ${it.message}") } }
                }) { Text("Ask again") }
            }
        }
        return
    }

    Surface(
        shape = RoundedCornerShape(32.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = modifier.fillMaxWidth().animateContentSize(MaterialTheme.motionScheme.defaultSpatialSpec()),
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
                if (d.subtitle.isNotBlank()) Text(d.subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                d.evidence.forEach { e -> AssistChip(onClick = { onEvidence(e) }, label = { Text(e.label) }) }
            }

            if (d.pending && !draft.rejecting && d.options.isNotEmpty()) Column(Modifier.selectableGroup(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                d.options.forEach { o ->
                    val selected = draft.option == o.id
                    val radius by animateDpAsState(if (selected) 28.dp else 20.dp, MaterialTheme.motionScheme.fastSpatialSpec(), label = "option")
                    Surface(
                        shape = RoundedCornerShape(radius),
                        color = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
                        contentColor = if (selected) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
                            .selectable(selected = selected, onClick = { set(draft.copy(option = o.id)) }, role = Role.RadioButton),
                    ) {
                        Row(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            RadioButton(selected = selected, onClick = null)
                            Column(Modifier.weight(1f)) {
                                Text(o.label + if (o.recommended) " · Recommended" else "", style = MaterialTheme.typography.labelLarge)
                                if (o.detail.isNotBlank()) Text(o.detail, style = MaterialTheme.typography.bodySmall)
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
                                    androidx.compose.material3.FilterChip(selected = draft.input == c.id, onClick = { set(draft.copy(input = if (draft.input == c.id) "" else c.id)) }, label = { Text(c.label) })
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

            if (d.pending && d.options.isEmpty() && !d.allowReject) {
                // Nothing to answer here (an older nacred's item): its links above, or ask the agent about it.
                val ask = com.ryderdavid.nacre.ui.workspace.LocalAskAbout.current
                androidx.compose.material3.FilledTonalButton(
                    onClick = { ask(com.ryderdavid.nacre.ui.workspace.ChatReference("decision", d.id, d.title, d.body)) },
                    modifier = Modifier.heightIn(min = 48.dp),
                ) { Text("Ask about this") }
            } else if (d.pending) {
                if (draft.rejecting || draft.note.isNotEmpty()) OutlinedTextField(
                    value = draft.note, onValueChange = { set(draft.copy(note = it)) },
                    label = { Text(if (draft.rejecting) "Why? (sent with the rejection)" else "Note (optional)") },
                    modifier = Modifier.fillMaxWidth(), minLines = 2,
                )
                val problem = DecisionModel.problem(d, draft)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                    val send = {
                        sending = true
                        scope.launch {
                            runCatching { container.repository.answerDecision(d, DecisionModel.answer(d, draft)) }
                                .onSuccess { DecisionDrafts.clear(d.key) }
                                .onFailure { container.repository.notify("Not sent: ${it.message}") }
                            sending = false
                        }
                    }
                    if (draft.rejecting) {
                        Button(onClick = { send() }, enabled = problem == null && !sending, modifier = Modifier.heightIn(min = 48.dp)) { Text(d.rejectLabel.ifBlank { "Reject" }) }
                        TextButton(onClick = { set(draft.copy(rejecting = false)) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Back") }
                    } else {
                        Button(onClick = { send() }, enabled = problem == null && !sending, modifier = Modifier.heightIn(min = 48.dp)) { Text(DecisionModel.sendLabel(d)) }
                        if (d.allowDefer) TextButton(onClick = {
                            scope.launch { runCatching { container.repository.answerDecision(d, com.ryderdavid.nacre.data.model.DecisionAnswer(defer = true)) }.onFailure { container.repository.notify("Not deferred: ${it.message}") } }
                        }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Defer") }
                        if (draft.note.isEmpty()) TextButton(onClick = { set(draft.copy(note = " ")) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Add a note") }
                        if (d.allowReject) TextButton(onClick = { set(draft.copy(rejecting = true, note = draft.note.trim())) }, modifier = Modifier.heightIn(min = 48.dp)) {
                            Text(d.rejectLabel.ifBlank { "Reject" } + "…", color = MaterialTheme.colorScheme.error)
                        }
                    }
                    if (sending) LoadingIndicator(Modifier.size(32.dp))
                }
                if (problem != null && problem != "Pick an answer") Text(problem, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            } else if (line != null) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) {
                    if (d.state == "executing") LoadingIndicator(Modifier.size(28.dp))
                    Text(line, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                    TextButton(onClick = { expanded = false }) { Text("Collapse") }
                }
            }
        }
    }
}

/** A tool step the agent asks to take: the full step, the reason, then Deny and Allow. */
@Composable
fun PermissionCardM3(card: PermissionCard, agent: String, onAnswer: (String, Boolean, String?) -> Unit, modifier: Modifier = Modifier) {
    if (!card.pending) {
        Text(card.tool + ": " + card.outcomeLine, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = modifier.padding(horizontal = 4.dp))
        return
    }
    var denying by remember(card.requestId) { mutableStateOf(false) }
    var note by remember(card.requestId) { mutableStateOf("") }
    Surface(shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest, modifier = modifier.fillMaxWidth().semantics { stateDescription = "Waiting for your answer" }) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(Icons.Rounded.Terminal, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("${agent.replaceFirstChar { it.uppercase() }} asks to use ${card.tool}", style = MaterialTheme.typography.titleSmall)
            }
            Text(
                card.step, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace,
                modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceContainerLowest, RoundedCornerShape(12.dp)).padding(10.dp),
            )
            if (card.reason.isNotBlank()) Text(card.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (denying) OutlinedTextField(value = note, onValueChange = { note = it }, label = { Text("Tell it why (optional)") }, modifier = Modifier.fillMaxWidth())
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                if (denying) {
                    TextButton(onClick = { denying = false }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Back") }
                    Button(onClick = { onAnswer(card.requestId, false, note.ifBlank { null }) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Deny") }
                } else {
                    OutlinedButton(onClick = { denying = true }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Deny…") }
                    Button(onClick = { onAnswer(card.requestId, true, null) }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Allow") }
                }
            }
        }
    }
}
