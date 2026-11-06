package com.ryderdavid.nacre.ui.rounds

import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.data.model.CHOICE_RETRY_WITH_BUDGET
import com.ryderdavid.nacre.data.model.CHOICE_RAISE_LIMITS
import com.ryderdavid.nacre.data.model.CHOICE_CANCEL
import com.ryderdavid.nacre.data.model.RoundStop
import androidx.compose.material.icons.rounded.AutoFixHigh
import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.runtime.produceState
import com.ryderdavid.nacre.data.model.RoundChanges
import com.ryderdavid.nacre.data.friendly
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.MergeType
import androidx.compose.material.icons.rounded.Block
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.FactCheck
import androidx.compose.material.icons.rounded.HelpOutline
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Pause
import androidx.compose.material.icons.rounded.RocketLaunch
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.SkipNext
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material.icons.rounded.TravelExplore
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATING
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_RUNNING
import androidx.compose.runtime.produceState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.data.model.BEAD_SKIPPED
import com.ryderdavid.nacre.data.model.BEAD_VERIFYING
import com.ryderdavid.nacre.data.model.BEAD_WAITING
import com.ryderdavid.nacre.data.model.FINDING_OVERLAP
import com.ryderdavid.nacre.data.model.Question
import com.ryderdavid.nacre.data.model.ROUND_CANCELLED
import com.ryderdavid.nacre.data.model.ROUND_IMPLEMENTING
import com.ryderdavid.nacre.data.model.ROUND_INTEGRATING
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import com.ryderdavid.nacre.data.model.ROUND_PLANNING
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundBead
import com.ryderdavid.nacre.data.model.RoundEvent
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.LabelChip
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.SectionHeader
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/** What a round is doing, in the app's words. */
fun roundStateLabel(state: String): String = when (state) {
    ROUND_PLANNING -> "Reviewing the beads"
    ROUND_NEEDS_DECISION -> "Needs your decision"
    ROUND_IMPLEMENTING -> "Working"
    ROUND_INTEGRATING -> "Merging"
    ROUND_READY -> "Ready to merge"
    ROUND_LANDED -> "Merged"
    ROUND_CANCELLED -> "Cancelled"
    ROUND_PAUSED -> "Paused"
    else -> state
}

/** What a bead in a round is doing. */
fun beadStateLabel(state: String): String = when (state) {
    BEAD_WAITING -> "Waiting"
    BEAD_RUNNING -> "Working"
    BEAD_VERIFYING -> "Checking"
    BEAD_INTEGRATING -> "Verified, waiting to merge"
    BEAD_INTEGRATED -> "Merged"
    BEAD_PARKED -> "Parked"
    BEAD_SKIPPED -> "Skipped"
    else -> state
}

/** A one-line summary of where the round's beads are. */
fun beadSummary(r: Round): String {
    if (r.finishEpic.isNotBlank()) return "Finishing ${r.finishEpic}: merging ${com.ryderdavid.nacre.data.model.epicBranch(r.finishEpic)} into ${r.target}"
    val merged = r.beads.count { it.state == BEAD_INTEGRATED }
    val parked = r.beads.count { it.state == BEAD_PARKED || it.state == BEAD_SKIPPED }
    return buildList {
        add("$merged of ${r.beads.size} merged")
        if (parked > 0) add("$parked parked")
    }.joinToString(" · ")
}

fun usd(v: Double): String = String.format(Locale.US, "$%.2f", v)

@Composable
private fun stateColor(state: String): Color = when (state) {
    ROUND_NEEDS_DECISION, ROUND_READY, ROUND_PAUSED -> Nacre.colors.needsYou
    ROUND_LANDED -> Nacre.colors.doing
    ROUND_CANCELLED -> MaterialTheme.colorScheme.outline
    else -> Nacre.colors.running
}

private fun stateIcon(state: String): ImageVector = when (state) {
    ROUND_PLANNING -> Icons.Rounded.TravelExplore
    ROUND_NEEDS_DECISION -> Icons.Rounded.HelpOutline
    ROUND_IMPLEMENTING -> Icons.Rounded.Terminal
    ROUND_INTEGRATING -> Icons.AutoMirrored.Rounded.MergeType
    ROUND_READY -> Icons.Rounded.RocketLaunch
    ROUND_LANDED -> Icons.Rounded.CheckCircle
    ROUND_PAUSED -> Icons.Rounded.Pause
    else -> Icons.Rounded.Block
}

/** A round in the Runs list. */
@Composable
fun RoundRow(r: Round, selected: Boolean, showComputer: Boolean, onClick: () -> Unit) {
    ListItem(
        modifier = Modifier.clickable(onClick = onClick),
        colors = ListItemDefaults.colors(containerColor = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surface),
        leadingContent = { Icon(stateIcon(r.state), roundStateLabel(r.state), tint = stateColor(r.state)) },
        overlineContent = { Text(listOfNotNull(r.hostName.takeIf { showComputer }, r.repoName, "round").joinToString(" · "), maxLines = 1) },
        headlineContent = { Text(r.beads.joinToString(", ") { it.id }, maxLines = 2, overflow = TextOverflow.Ellipsis) },
        supportingContent = {
            Text(
                listOf(roundStateLabel(r.state), beadSummary(r), "${usd(r.spentUsd)} of ${usd(r.budgetUsd)}").joinToString(" · "),
                style = MaterialTheme.typography.bodySmall,
            )
        },
        trailingContent = { if (r.needsYou) NeedsYouChip() },
    )
}

@Composable
private fun NeedsYouChip() {
    Surface(color = Nacre.colors.needsYouContainer, contentColor = Nacre.colors.needsYou, shape = RoundedCornerShape(8.dp)) {
        Text("Needs you", style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp))
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
internal fun StatusCard(
    r: Round,
    busy: Boolean,
    onBudget: (Double) -> Unit,
    onChoose: (String, Double) -> Unit,
    onCancel: () -> Unit,
) {
    var raising by remember { mutableStateOf(false) }
    var retrying by remember { mutableStateOf(false) }
    val stop = r.stop?.takeIf { r.state == ROUND_PAUSED }
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(20.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(stateIcon(r.state), null, tint = stateColor(r.state), modifier = Modifier.size(28.dp))
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(roundStateLabel(r.state), style = MaterialTheme.typography.titleLarge)
                    Text(beadSummary(r), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (r.finish) Text("Then merges into main", style = MaterialTheme.typography.bodyMedium)
            if (r.state == ROUND_READY && r.mergesAutomatically) Text("Merging automatically…", style = MaterialTheme.typography.bodyMedium)
            if (r.pauseReason.isNotBlank()) Text(r.pauseReason, style = MaterialTheme.typography.bodyMedium)
            if (r.error.isNotBlank()) Text(r.error, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            val frac = if (r.budgetUsd > 0) (r.spentUsd / r.budgetUsd).toFloat().coerceIn(0f, 1f) else 0f
            LinearProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth())
            Text(
                listOfNotNull(
                    "${usd(r.spentUsd)} of ${usd(r.budgetUsd)}",
                    if (r.finishEpic.isNotBlank()) "from ${com.ryderdavid.nacre.data.model.epicBranch(r.finishEpic)} at ${r.base.take(7)}" else "merges into ${r.target} · from ${r.base.take(7)}",
                    r.startedAt.takeIf { it.isNotBlank() }?.let { "started ${relativeTime(it)}" },
                ).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            when {
                stop != null -> StopChoices(
                    stop, busy,
                    onChoose = { choice ->
                        when (choice) {
                            CHOICE_RAISE_LIMITS -> raising = true
                            CHOICE_RETRY_WITH_BUDGET -> retrying = true
                            CHOICE_CANCEL -> onCancel()
                            else -> onChoose(choice, 0.0)
                        }
                    },
                )
                // An older nacred: no stop to read, so the plain controls.
                r.state == ROUND_PAUSED -> Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = { raising = true }, enabled = !busy) { Text("Raise budget") }
                }
                r.isActive -> TextButton(onClick = { raising = true }, enabled = !busy) { Text("Change budget") }
            }
        }
    }
    if (raising) BudgetDialog(r.budgetUsd, onDismiss = { raising = false }, onSet = {
        raising = false
        if (stop?.choices?.any { c -> c.id == CHOICE_RAISE_LIMITS } == true) onChoose(CHOICE_RAISE_LIMITS, it) else onBudget(it)
    })
    if (retrying) ExtraBudgetDialog(onDismiss = { retrying = false }, onSet = { retrying = false; onChoose(CHOICE_RETRY_WITH_BUDGET, it) })
}

/**
 * What a stopped round found and the ways forward: Claude's read of the
 * problem, what was already tried, the spend on a limits stop, and one
 * button per choice with what it would do.
 */
@Composable
internal fun StopChoices(stop: RoundStop, busy: Boolean, onChoose: (String) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        stop.diagnosis?.takeIf { it.summary.isNotBlank() }?.let { d ->
            Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, shape = RoundedCornerShape(14.dp), modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(AiIcon, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(6.dp))
                        Text("What's wrong", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                    }
                    Text(d.summary, style = MaterialTheme.typography.bodyMedium)
                    if (d.cause.isNotBlank()) Text(d.cause, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        if (stop.attempted.isNotEmpty()) Text(
            "Already tried: " + stop.attempted.joinToString(", ", transform = ::attemptLabel),
            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (stop.spend.isNotEmpty()) Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            stop.spend.forEach { s ->
                Row {
                    BeadId(s.bead)
                    Spacer(Modifier.width(8.dp))
                    Text(usd(s.spentUsd) + if (s.done) " · done" else "", style = MaterialTheme.typography.bodySmall)
                }
            }
            if (stop.estimateUsd > 0) Text(
                "The unfinished beads need about ${usd(stop.estimateUsd)} more, going by the finished ones.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        stop.choices.forEachIndexed { i, c ->
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                when {
                    c.id == CHOICE_CANCEL -> TextButton(onClick = { onChoose(c.id) }, enabled = !busy) { Text(c.label.ifBlank { "Cancel the round" }) }
                    i == 0 -> FilledTonalButton(onClick = { onChoose(c.id) }, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(c.label) }
                    else -> OutlinedButton(onClick = { onChoose(c.id) }, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(c.label) }
                }
                if (c.explanation.isNotBlank() && c.id != CHOICE_CANCEL) Text(
                    c.explanation, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 4.dp),
                )
            }
        }
    }
}

/** A stop's "already tried" entry in words. */
internal fun attemptLabel(a: String): String {
    val (what, detail) = a.split(':', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
    return when (what) {
        "fix" -> "settled ${detail.substringBefore(':').replace('-', ' ')} without Claude"
        "turn" -> {
            val times = detail.substringAfter('×', "1")
            "Claude ${if (times == "1") "once" else "$times times"}"
        }
        "retry" -> "retried $detail×"
        "review" -> "reviewed $detail times"
        else -> a
    }
}

@Composable
private fun ExtraBudgetDialog(onDismiss: () -> Unit, onSet: (Double) -> Unit) {
    var text by rememberSaveable { mutableStateOf("5") }
    val value = text.toDoubleOrNull()
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Let Claude try again") },
        text = {
            OutlinedTextField(
                value = text, onValueChange = { text = it }, singleLine = true, prefix = { Text("$") },
                supportingText = { Text("Added to what's left of the round's budget for the new attempts.") },
                isError = value == null || value <= 0,
            )
        },
        confirmButton = { TextButton(onClick = { value?.let(onSet) }, enabled = value != null && value > 0) { Text("Try again") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
internal fun BudgetDialog(current: Double, onDismiss: () -> Unit, onSet: (Double) -> Unit) {
    var text by rememberSaveable { mutableStateOf(String.format(Locale.US, "%.0f", current + 10)) }
    val value = text.toDoubleOrNull()
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Round budget") },
        text = {
            OutlinedTextField(
                value = text, onValueChange = { text = it }, singleLine = true, prefix = { Text("$") },
                supportingText = { Text("A round paused by its budget carries on once it has room.") },
                isError = value == null || value <= 0,
            )
        },
        confirmButton = { TextButton(onClick = { value?.let(onSet) }, enabled = value != null && value > 0) { Text("Set budget") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** The commits and files a round adds, loaded on demand. */
@Composable
internal fun RoundChangesList(changes: Result<RoundChanges>?) {
    val c = changes?.getOrNull()
    when {
        changes == null -> Text("Loading the changes…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        c == null -> Text("Couldn't load the changes: ${changes.exceptionOrNull()?.friendly()}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
        else -> SelectionContainer {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    "${c.commits.size + c.moreCommits} commits · ${c.files.size} files · +${c.added} −${c.deleted} since ${c.base.take(7)}",
                    style = MaterialTheme.typography.labelLarge,
                )
                c.commits.forEach { Text("${it.sha.take(7)} ${it.subject}", style = MaterialTheme.typography.bodySmall, fontFamily = MonoFamily, maxLines = 2, overflow = TextOverflow.Ellipsis) }
                if (c.moreCommits > 0) Text("and ${c.moreCommits} earlier", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.height(4.dp))
                c.files.forEach { f ->
                    Text(
                        if (f.binary) "${f.path} (binary)" else "+${f.added} −${f.deleted}  ${f.path}",
                        style = MaterialTheme.typography.bodySmall, fontFamily = MonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun BeadRow(r: Round, b: RoundBead, busy: Boolean, onOpenBead: () -> Unit, onOpenRun: () -> Unit, onRetry: () -> Unit) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerLow, shape = RoundedCornerShape(16.dp),
        modifier = Modifier.fillMaxWidth().animateContentSize().clickable(onClick = onOpenBead),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                BeadStateIcon(b.state)
                Spacer(Modifier.width(8.dp))
                BeadId(b.id)
                Spacer(Modifier.width(8.dp))
                Text(beadStateLabel(b.state), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                if (b.runId.isNotBlank()) TextButton(onClick = onOpenRun) { Text("Run") }
            }
            Text(b.title, style = MaterialTheme.typography.bodyLarge.copy(fontWeight = FontWeight.Medium))
            val notes = buildList {
                if (b.after.isNotEmpty() && b.state == BEAD_WAITING) add("After " + b.after.joinToString(", "))
                if (b.needs.isNotEmpty()) add("Uses " + b.needs.joinToString(", "))
                if (b.fixAttempts > 0) add("Sent back ${b.fixAttempts}×")
                if (b.stalls > 0) add("Resumed after stalling")
                if (b.mergeCommit.isNotBlank()) add("Merged at ${b.mergeCommit.take(7)}")
                if (b.carriedFrom.isNotBlank()) add("Continues earlier work from ${b.carriedFrom}")
                if (b.pickedUpBy.isNotBlank()) add("Continued in round ${b.pickedUpBy}")
            }
            if (notes.isNotEmpty()) Text(notes.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (b.reason.isNotBlank()) Text(b.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            b.verification?.takeIf { !it.ok && it.reason.isNotBlank() && b.state != BEAD_PARKED }?.let {
                Text("Last check: ${it.reason}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (b.humanCheck.isNotBlank()) Text("Left for you: ${b.humanCheck}", style = MaterialTheme.typography.bodySmall, color = Nacre.colors.needsYou)
            if (r.isActive) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (b.state == BEAD_PARKED || b.state == BEAD_SKIPPED) OutlinedButton(onClick = onRetry, enabled = !busy) { Text("Try again") }
            }
        }
    }
}

@Composable
private fun BeadStateIcon(state: String) {
    val (icon, tint) = when (state) {
        BEAD_INTEGRATED -> Icons.Rounded.CheckCircle to Nacre.colors.doing
        BEAD_RUNNING -> Icons.Rounded.Terminal to Nacre.colors.running
        BEAD_VERIFYING -> Icons.Rounded.FactCheck to Nacre.colors.running
        BEAD_INTEGRATING -> Icons.AutoMirrored.Rounded.MergeType to Nacre.colors.running
        BEAD_PARKED -> Icons.Rounded.Pause to Nacre.colors.needsYou
        BEAD_SKIPPED -> Icons.Rounded.SkipNext to MaterialTheme.colorScheme.outline
        else -> Icons.Rounded.Schedule to MaterialTheme.colorScheme.outline
    }
    Icon(icon, beadStateLabel(state), tint = tint, modifier = Modifier.size(20.dp))
}

fun findingLabel(kind: String): String = when (kind) {
    "contradiction" -> "Contradiction"
    "ambiguity" -> "Unclear"
    "overlap" -> "Overlap, run one after the other"
    "ordering" -> "Order"
    "shared-artifact" -> "Shared, the round owns it"
    "unmerged-work" -> "Unmerged work"
    else -> kind
}

private val timeFmt = DateTimeFormatter.ofPattern("HH:mm").withZone(ZoneId.systemDefault())

@Composable
internal fun EventRow(e: RoundEvent) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Top) {
        Text(
            runCatching { timeFmt.format(Instant.parse(e.ts)) }.getOrDefault(""),
            style = MaterialTheme.typography.labelSmall, fontFamily = MonoFamily, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(44.dp).padding(top = 2.dp),
        )
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.Top) {
                // Something the round fixed by itself.
                if (e.kind == "recover") Icon(
                    Icons.Rounded.AutoFixHigh, "Recovered", tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(14.dp).padding(top = 2.dp),
                )
                if (e.kind == "recover") Spacer(Modifier.width(4.dp))
                Text(
                    (if (e.bead.isNotBlank()) "${e.bead}: " else "") + e.text,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            e.gates.filter { !it.ok }.forEach { g ->
                Text("${g.name}: ${g.tail.lines().lastOrNull { it.isNotBlank() }.orEmpty()}", style = MaterialTheme.typography.labelSmall, fontFamily = MonoFamily, color = MaterialTheme.colorScheme.error, maxLines = 2)
            }
        }
    }
}
