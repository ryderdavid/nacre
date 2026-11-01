package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalButton
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
    val action = change?.let { changeNextAction(it) }
    val humanChecks = summary.sections.mapNotNull { s -> ix.byId[s.beadId] }.filter { !it.isClosed && "human" in it.labels }
    Surface(color = MaterialTheme.colorScheme.secondaryContainer.copy(alpha = 0.45f), shape = RoundedCornerShape(20.dp), modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(changePhaseLabel(summary.phase), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
            Text(summary.name, style = MaterialTheme.typography.titleLarge)
            if (summary.revision.isNotBlank()) Text(
                "Revision ${summary.revision.take(12)}" + if (summary.approvedRevision == summary.revision) " · approved" else "",
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = MonoFamily), color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (summary.phase == "stale") Text(staleText(summary.staleReason), style = MaterialTheme.typography.bodyMedium)
            SectionStrip(summary.sections)
            Text(
                "${summary.landed} of ${summary.sections.size} landed · ${summary.verified} verified",
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
            Button(onClick = onOpenChange) { Text(action?.label ?: "View change") }
        }
    }
}

/** What broke a change's approval, in a sentence. */
fun staleText(reason: String) = reason.ifBlank { "The project's specifications changed since this was approved." } + " A new review is needed before its sections can run."

/**
 * Where an open epic's work stands and how to bring it home: its beads land on
 * the epic's branch; the rest can be worked until done in one round, and once
 * every bead is closed, finishing merges the branch into the main branch.
 */
@Composable
fun EpicCompletionCard(epic: Issue, ix: BoardIndex, isChange: Boolean, busy: Boolean, onWorkRest: () -> Unit, onFinish: () -> Unit) {
    val kids = ix.childrenOf(epic.id)
    val closed = kids.count { it.isClosed }
    val open = kids.size - closed
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Bringing it home", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                Text("$closed of ${kids.size} closed", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text(
                "Work on its beads lands on ${epicBranch(epic.id)}. Finishing ${if (isChange) "archives its change and " else ""}merges the branch into the main branch with every gate and the build, pushes it and closes the epic.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            when {
                kids.isEmpty() -> Text("It has no beads yet.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                open == 0 -> Button(onClick = onFinish, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(if (isChange) "Finish: archive and merge" else "Finish epic") }
                !isChange -> FilledTonalButton(onClick = onWorkRest, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text("Work the other $open until done") }
                else -> Text("Implement its open sections from the change; finish once they're closed.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
