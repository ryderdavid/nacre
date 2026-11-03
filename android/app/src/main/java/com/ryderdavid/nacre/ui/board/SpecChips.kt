package com.ryderdavid.nacre.ui.board

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.SectionSummary
import com.ryderdavid.nacre.data.model.foundChange
import com.ryderdavid.nacre.data.model.isPendingAmendment
import com.ryderdavid.nacre.data.model.touchesSpec
import com.ryderdavid.nacre.ui.bead.changePhaseLabel
import com.ryderdavid.nacre.ui.theme.Nacre

/** How a spec chip is colored: spec state, waiting on Ryder, verified, or a plain note. */
enum class ChipTone { SPEC, NEEDS_YOU, VERIFIED, NOTE, CHECK }

/** The one spec chip a card shows, and an epic's section strip. */
data class SpecChip(val text: String, val tone: ChipTone, val strip: List<SectionSummary> = emptyList())

/** Labels a card's footer leaves out because the spec chip speaks for them. */
fun isSpecLabel(label: String) = label.startsWith("specflow:") || label.startsWith("spec-section:") ||
    label.startsWith("found:") || label == "found-amend" || label.startsWith("spec-touch:")

/** Phases where the change waits on Ryder. */
private val WAITING = setOf("awaiting_approval", "needs_attention", "stale", "interrupted")

fun evidenceLabel(evidence: String) = when (evidence) {
    "landed" -> "merged"
    "verified" -> "verified"
    "in_round" -> "in a round"
    else -> "no evidence yet"
}

/**
 * The spec chip for [issue], at most one: a seed's or epic's phase (with the
 * section strip on epics), a section's place and evidence, "Closed, not verified",
 * found work, or "Touches a spec". Null without the board's changes (older
 * bridge), so those cards look as they did.
 */
fun specChip(issue: Issue, changes: ChangesList?, toCheck: Set<String> = emptySet()): SpecChip? {
    if (changes == null) return null
    for (c in changes.changes) {
        if (issue.id == c.epicId || issue.id in c.seedIds) {
            val strip = if (issue.id == c.epicId) c.sections else emptyList()
            // Built and merged, only Ryder's checks left: say that, not the change's phase.
            val checks = strip.count { it.beadId in toCheck }
            if (checks > 0) return SpecChip("$checks to check", ChipTone.CHECK, strip)
            val tone = if (c.phase in WAITING) ChipTone.NEEDS_YOU else ChipTone.SPEC
            return SpecChip(changePhaseLabel(c.phase), tone, strip)
        }
        val i = c.sections.indexOfFirst { it.beadId == issue.id }
        if (i >= 0) {
            val s = c.sections[i]
            if (issue.isClosed && s.evidence != "verified" && s.evidence != "landed") return SpecChip("Closed, not verified", ChipTone.NOTE)
            val tone = if (s.evidence == "verified" || s.evidence == "landed") ChipTone.VERIFIED else ChipTone.SPEC
            return SpecChip("§ ${i + 1} of ${c.name} · ${evidenceLabel(s.evidence)}", tone)
        }
    }
    val found = issue.foundChange
    if (found.isNotBlank()) {
        val name = changes.changes.firstOrNull { it.id == found }?.name ?: "its change"
        return if (issue.isPendingAmendment) SpecChip("Changes $name", ChipTone.NEEDS_YOU) else SpecChip("Added to $name", ChipTone.SPEC)
    }
    if (issue.touchesSpec) return SpecChip("Touches a spec", ChipTone.NEEDS_YOU)
    return null
}

@Composable
fun SpecChipView(chip: SpecChip, modifier: Modifier = Modifier) {
    val (bg, fg) = when (chip.tone) {
        ChipTone.SPEC -> MaterialTheme.colorScheme.secondaryContainer to MaterialTheme.colorScheme.onSecondaryContainer
        ChipTone.NEEDS_YOU -> Nacre.colors.needsYouContainer to Nacre.colors.needsYou
        ChipTone.VERIFIED -> Nacre.colors.doing.copy(alpha = 0.16f) to Nacre.colors.doing
        ChipTone.NOTE -> MaterialTheme.colorScheme.surfaceContainerHigh to MaterialTheme.colorScheme.onSurfaceVariant
        ChipTone.CHECK -> MaterialTheme.colorScheme.tertiaryContainer to MaterialTheme.colorScheme.onTertiaryContainer
    }
    Surface(color = bg, contentColor = fg, shape = RoundedCornerShape(8.dp), modifier = modifier) {
        Text(chip.text, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp))
    }
}

/** An epic's sections as a strip: green verified or landed, teal in a round, the check colour while waiting on Ryder's sign-off, grey otherwise. */
@Composable
fun SectionStrip(sections: List<SectionSummary>, modifier: Modifier = Modifier, toCheck: Set<String> = emptySet()) {
    if (sections.isEmpty()) return
    val verified = sections.count { it.evidence == "verified" || it.evidence == "landed" }
    val landed = sections.count { it.evidence == "landed" }
    Row(
        modifier.semantics { contentDescription = "${sections.size} sections: $verified verified, $landed merged" },
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        sections.take(16).forEach { s ->
            val color = when {
                s.beadId in toCheck -> Nacre.colors.check
                s.evidence == "landed" || s.evidence == "verified" -> Nacre.colors.doing
                s.evidence == "in_round" -> MaterialTheme.colorScheme.primary
                else -> MaterialTheme.colorScheme.outlineVariant
            }
            Box(Modifier.width(14.dp).height(6.dp).background(color, RoundedCornerShape(3.dp)))
        }
        val checks = sections.count { it.beadId in toCheck }
        Text("  $verified of ${sections.size} verified" + if (checks > 0) " · $checks to check" else "", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
