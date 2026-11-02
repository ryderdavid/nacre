package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.epicBranch
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.ui.board.SectionStrip
import com.ryderdavid.nacre.ui.board.evidenceLabel
import com.ryderdavid.nacre.ui.theme.MonoFamily

/**
 * A spec epic's detail leads with its change: name, revision and phase, the
 * section strip with each section's evidence, open human checks, and the next
 * action, before the bead's own description.
 */
@Composable
fun EpicChangeHeader(summary: ChangeSummary, change: SpecChange?, ix: BoardIndex, onOpenChange: () -> Unit, onOpenBead: (String) -> Unit) {
    val humanChecks = summary.sections.mapNotNull { s -> ix.byId[s.beadId] }.filter { !it.isClosed && "human" in it.labels }
    Surface(color = MaterialTheme.colorScheme.secondaryContainer.copy(alpha = 0.45f), shape = RoundedCornerShape(20.dp), modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(change?.let(::changeStatusTitle) ?: changePhaseLabel(summary.phase), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
            Text(summary.name, style = MaterialTheme.typography.titleLarge)
            if (summary.revision.isNotBlank()) Text(
                "Revision ${summary.revision.take(12)}" + if (summary.approvedRevision == summary.revision) " · approved" else "",
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = MonoFamily), color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (summary.phase == "stale") Text(staleText(summary.staleReason), style = MaterialTheme.typography.bodyMedium)
            SectionStrip(summary.sections)
            Text(
                "${summary.landed} of ${summary.sections.size} merged · ${summary.verified} verified",
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            summary.sections.forEachIndexed { n, s ->
                Row(
                    Modifier.fillMaxWidth().clickable(enabled = s.beadId.isNotBlank()) { onOpenBead(s.beadId) }.padding(vertical = 2.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("§ ${n + 1}  ${s.title}", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f), maxLines = 1)
                    Text(evidenceLabel(s.evidence), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (humanChecks.isNotEmpty()) Text(
                "Human checks open: " + humanChecks.joinToString { it.id },
                style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium), color = MaterialTheme.colorScheme.tertiary,
            )
            androidx.compose.material3.TextButton(onClick = onOpenChange) { Text("View change") }
        }
    }
}

/** What broke a change's approval, in a sentence. */
fun staleText(reason: String) = reason.ifBlank { "The project's specifications changed since this was approved." } + " A new review is needed before its sections can run."

/** The epic has one next step, with an active round taking priority. */
fun epicPrimaryAction(openChildren: Int, childCount: Int, running: Boolean): String? = when {
    running -> "Running"
    childCount == 0 -> null
    openChildren > 0 -> "Run epic"
    else -> "Finish epic"
}

@Composable
fun EpicCompletionCard(
    epic: Issue, ix: BoardIndex, isChange: Boolean, busy: Boolean,
    activeRound: Round?, onOpenRound: (String) -> Unit, onRunEpic: () -> Unit, onFinish: () -> Unit,
) {
    val kids = ix.childrenOf(epic.id)
    val closed = kids.count { it.isClosed }
    val action = epicPrimaryAction(kids.size - closed, kids.size, activeRound != null)
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("$closed of ${kids.size} children closed", style = MaterialTheme.typography.titleSmall)
            Text(
                "Each child runs in its own worktree and merges into ${epicBranch(epic.id)}. Finishing ${if (isChange) "archives the spec and " else ""}merges the epic into main automatically.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (activeRound != null) {
                androidx.compose.material3.LinearProgressIndicator(
                    progress = { if (kids.isEmpty()) 0f else closed.toFloat() / kids.size }, modifier = Modifier.fillMaxWidth(),
                )
            }
            if (action == null) Text("It has no children yet.", style = MaterialTheme.typography.bodySmall)
            else Button(
                onClick = { when (action) {
                    "Running" -> activeRound?.let { onOpenRound(it.key) }
                    "Run epic" -> onRunEpic()
                    else -> onFinish()
                } },
                enabled = activeRound != null || !busy, modifier = Modifier.fillMaxWidth(),
            ) { Text(action) }
        }
    }
}
