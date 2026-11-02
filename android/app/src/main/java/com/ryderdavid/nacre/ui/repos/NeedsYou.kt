package com.ryderdavid.nacre.ui.repos

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.AttentionOption
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre

/** How many items show before "Show all". */
const val NEEDS_YOU_SHOWN = 5

/** The chip naming what kind of thing waits on Ryder. */
fun attentionLabel(kind: String) = when (kind) {
    AttentionItem.BEAD_DECISION -> "Decision"
    AttentionItem.PROPOSAL_READY -> "Proposal ready"
    AttentionItem.REVIEW_BLOCKED -> "Review blocked"
    AttentionItem.APPROVAL_STALE -> "Review needs updating"
    AttentionItem.WORKFLOW_INTERRUPTED -> "Interrupted"
    AttentionItem.ROUND_QUESTION -> "Round question"
    AttentionItem.BEAD_PARKED -> "Parked"
    AttentionItem.ROUND_READY -> "Ready to merge"
    AttentionItem.CHANGE_ARCHIVABLE -> "Ready to archive"
    AttentionItem.FOUND_WORK -> "Found work"
    AttentionItem.EPIC_FINISHABLE -> "Epic done"
    AttentionItem.BEAD_STUCK -> "Stuck in progress"
    AttentionItem.BEAD_UNCLOSED -> "Done, still open"
    else -> kind.replace('_', ' ').replaceFirstChar { it.uppercase() }
}

/** Spec state is violet; anything that's Ryder's call uses the needs-you color. */
@Composable
private fun labelColors(kind: String): Pair<Color, Color> = when (kind) {
    AttentionItem.PROPOSAL_READY, AttentionItem.CHANGE_ARCHIVABLE -> MaterialTheme.colorScheme.secondaryContainer to MaterialTheme.colorScheme.onSecondaryContainer
    AttentionItem.ROUND_READY, AttentionItem.EPIC_FINISHABLE -> MaterialTheme.colorScheme.primaryContainer to MaterialTheme.colorScheme.onPrimaryContainer
    else -> Nacre.colors.needsYouContainer to Nacre.colors.needsYou
}

/**
 * Needs you at the top of Boards: every item waiting on Ryder across computers,
 * each explaining itself and offering its answers inline. Hidden when empty.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun NeedsYouSection(
    items: List<AttentionItem>,
    busy: Set<String>,
    showComputers: Boolean,
    onOption: (AttentionItem, AttentionOption) -> Unit,
    modifier: Modifier = Modifier,
    initiallyAll: Boolean = false,
) {
    if (items.isEmpty()) return
    var all by rememberSaveable { mutableStateOf(initiallyAll) }
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Needs you", style = MaterialTheme.typography.titleLarge)
            val (bg, fg) = labelColors(AttentionItem.BEAD_DECISION)
            Surface(color = bg, contentColor = fg, shape = RoundedCornerShape(12.dp)) {
                Text("${items.size}", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(horizontal = 9.dp, vertical = 2.dp))
            }
        }
        (if (all) items else items.take(NEEDS_YOU_SHOWN)).forEach { item ->
            NeedsYouCard(item, busy = item.id in busy, showComputer = showComputers, onOption = { onOption(item, it) })
        }
        if (items.size > NEEDS_YOU_SHOWN) {
            TextButton(onClick = { all = !all }) { Text(if (all) "Show fewer" else "Show all ${items.size}") }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun NeedsYouCard(item: AttentionItem, busy: Boolean, showComputer: Boolean, onOption: (AttentionOption) -> Unit) {
    Surface(
        shape = RoundedCornerShape(20.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        color = MaterialTheme.colorScheme.surface,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                val (bg, fg) = labelColors(item.kind)
                Surface(color = bg, contentColor = fg, shape = RoundedCornerShape(8.dp)) {
                    Text(attentionLabel(item.kind), style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp))
                }
                val where = listOfNotNull(
                    item.repoName.ifBlank { null },
                    item.hostName.takeIf { showComputer && it.isNotBlank() },
                    item.beadId.ifBlank { item.roundId }.ifBlank { null },
                ).joinToString(" · ")
                Text(
                    where, style = MaterialTheme.typography.labelSmall.copy(fontFamily = MonoFamily),
                    color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).padding(start = 8.dp), textAlign = androidx.compose.ui.text.style.TextAlign.End,
                )
            }
            Text(item.title, style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Medium), maxLines = 2, overflow = TextOverflow.Ellipsis)
            if (item.why.isNotBlank()) Text(item.why, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            val disabled = item.options.filter { it.disabled.isNotBlank() }.map { "${it.label}: ${it.disabled}" }
            disabled.forEach { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            if (item.options.isNotEmpty()) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
                item.options.forEachIndexed { i, o ->
                    val enabled = !busy && o.disabled.isBlank()
                    if (i == 0) Button(onClick = { onOption(o) }, enabled = enabled) { Text(o.label) }
                    else OutlinedButton(onClick = { onOption(o) }, enabled = enabled) { Text(o.label) }
                }
            }
        }
    }
}


/**
 * Says which computers run a nacred too old for Needs you and what came with it
 * (proposals, Work until done, finishing epics), instead of those simply being
 * empty there.
 */
@Composable
fun OutdatedNacredNote(computers: List<String>) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("Update nacred on ${computers.joinToString(", ")}", style = MaterialTheme.typography.titleSmall)
            Text(
                "Its nacred is older than this app, so Needs you, proposals, Work until done and finishing epics stay empty there. Update it from Settings › Update connected computers, or install it on that computer.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
