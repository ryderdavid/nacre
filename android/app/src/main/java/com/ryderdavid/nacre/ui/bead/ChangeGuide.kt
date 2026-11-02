package com.ryderdavid.nacre.ui.bead

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.GuideOption
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.ui.rounds.usd

/** Two cards side by side on a wide screen, stacked on a narrow one. */
@Composable
fun AdaptivePair(wide: Boolean, first: @Composable () -> Unit, second: @Composable () -> Unit) {
    if (wide) Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) { first() }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) { second() }
    } else Column(verticalArrangement = Arrangement.spacedBy(12.dp)) { first(); second() }
}

@Composable
fun GuideHeader(change: SpecChange, title: String) {
    val (label, bg, fg) = when (change.phase) {
        "awaiting_approval" -> Triple("Ready for your approval", MaterialTheme.colorScheme.primaryContainer, MaterialTheme.colorScheme.onPrimaryContainer)
        "covered" -> Triple("Already covered", MaterialTheme.colorScheme.secondaryContainer, MaterialTheme.colorScheme.onSecondaryContainer)
        else -> Triple("Needs you", MaterialTheme.colorScheme.errorContainer, MaterialTheme.colorScheme.onErrorContainer)
    }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Surface(color = bg, shape = RoundedCornerShape(8.dp)) {
            Text(label, style = MaterialTheme.typography.labelLarge, color = fg, modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp))
        }
        Text(title, style = MaterialTheme.typography.headlineSmall)
        Text("${change.seedId} · ${usd(change.costUsd)} so far", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun GuideSummary(change: SpecChange) {
    val (summary, status, blocking) = guideSummary(change)
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Summary", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (summary.isNotBlank()) Text(summary, style = MaterialTheme.typography.bodyLarge)
            if (status.isNotBlank()) Text(status, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (change.guide == null) Text("Claude is writing a plain summary and a recommendation…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (blocking.isNotEmpty()) {
                Text("Blocking it", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
                blocking.forEachIndexed { i, b ->
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        Text("${i + 1}", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium, color = MaterialTheme.colorScheme.error)
                        Text(b, style = MaterialTheme.typography.bodyMedium)
                    }
                }
            }
        }
    }
}

/**
 * The step Ryder is about to take: the recommendation until he picks another.
 * A revise shows its feedback, editable, since that is what gets sent.
 */
@Composable
fun GuideDecision(
    option: GuideOption,
    recommended: Boolean,
    seedId: String,
    feedback: String,
    onFeedback: (String) -> Unit,
    enabled: Boolean,
    sending: Boolean,
    onGo: () -> Unit,
) {
    Surface(
        shape = RoundedCornerShape(16.dp), border = BorderStroke(2.dp, MaterialTheme.colorScheme.primary),
        color = MaterialTheme.colorScheme.surfaceContainerLowest, modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Surface(color = MaterialTheme.colorScheme.primaryContainer, shape = RoundedCornerShape(8.dp)) {
                    Text(if (recommended) "Recommended" else "Your choice", style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp))
                }
                Text(option.label, style = MaterialTheme.typography.titleMedium)
            }
            if (option.action == "revise") OutlinedTextField(
                value = feedback, onValueChange = onFeedback, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp),
                label = { Text("Your decisions, sent as feedback") }, placeholder = { Text("Leave empty to try again as it is") },
                minLines = 2, maxLines = 8,
            )
            if (option.why.isNotBlank()) Text(option.why, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Button(onClick = onGo, enabled = enabled, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text(if (sending) "Sending…" else guideButton(option, seedId))
            }
        }
    }
}

/** The other steps, each with what it costs or leads to; tapping one makes it the step above. */
@Composable
fun GuideOptions(options: List<GuideOption>, recommended: GuideOption?, onPick: (GuideOption) -> Unit) {
    if (options.isEmpty()) return
    Surface(shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
            Text("Other options", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 8.dp))
            options.forEach { o ->
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                Row(
                    Modifier.fillMaxWidth().heightIn(min = 56.dp).clickable { onPick(o) }.padding(vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(if (o == recommended) "${o.label} (recommended)" else o.label, style = MaterialTheme.typography.bodyLarge)
                        if (o.why.isNotBlank()) Text(o.why, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

/** A fold-out chat about the change, open on a wide screen; what Ryder writes can become his decision. */
@Composable
fun ChangeChat(change: SpecChange, repoKey: String, claudeReady: Boolean, busy: Boolean, startOpen: Boolean, onUse: (String) -> Unit) {
    var open by rememberSaveable(change.id) { mutableStateOf(startOpen) }
    Surface(shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
            Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).clickable { open = !open }, verticalAlignment = Alignment.CenterVertically) {
                Text("Ask Claude about this change", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                Icon(if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, if (open) "Fold chat away" else "Open chat")
            }
            AnimatedVisibility(open) {
                AboutChat(
                    repoKey = repoKey, beadId = change.seedId, about = changeContext(change), claudeReady = claudeReady,
                    hint = "Ask what a finding means, why Claude recommends this step, or what another choice would lead to.",
                    placeholder = "Ask about this change", useLabel = "Use what I wrote as the decision", busy = busy, onUse = onUse,
                    modifier = Modifier.padding(bottom = 8.dp),
                )
            }
        }
    }
}

/** What Claude is told about the change being discussed. */
internal fun changeContext(change: SpecChange): String = buildString {
    val (summary, status, blocking) = guideSummary(change)
    append("Ryder is deciding the next step on the OpenSpec change ").append(change.changeName.ifBlank { change.seedId }).append(" (phase ").append(change.phase).append(").\n")
    if (summary.isNotBlank()) append("What it does: ").append(summary).append('\n')
    if (status.isNotBlank()) append("How it went: ").append(status).append('\n')
    change.findings.filter { it.disposition != "resolved" && it.disposition != "rejected" }.forEach {
        append("Open finding [").append(it.severity).append("] ").append(it.summary).append(": ").append(it.detail.take(600)).append('\n')
    }
    if (blocking.isNotEmpty() && change.findings.isEmpty()) append("Blocking: ").append(blocking.joinToString("; ")).append('\n')
    val (rec, _) = guideChoices(change)
    append("Recommended step: ").append(rec.label)
    if (rec.feedback.isNotBlank()) append(" (feedback: ").append(rec.feedback).append(')')
    append("\nAnswer plainly and briefly, and say which step you'd take.")
}
