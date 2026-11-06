package com.ryderdavid.nacre.ui.workspace.work

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.model.DecisionEvidence
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.rounds.BeadRow
import com.ryderdavid.nacre.ui.rounds.EventRow
import com.ryderdavid.nacre.ui.rounds.RoundChangesList
import com.ryderdavid.nacre.ui.rounds.RoundViewModel
import com.ryderdavid.nacre.ui.rounds.StatusCard
import com.ryderdavid.nacre.ui.runs.RunDetail
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.ui.workspace.InspectTarget
import com.ryderdavid.nacre.ui.workspace.LocalOpenEvidence
import com.ryderdavid.nacre.ui.workspace.LocalViewing

/** What Cancel round says will happen, so it's clear what keeps running. */
const val CANCEL_ROUND_TEXT = "Cancels the round and stops its workers. Branches and anything already merged on its branch stay. Chats keep running."

/** What Cancel run says. */
const val CANCEL_RUN_TEXT = "Stops this run. Its branch and worktree stay. Chats keep running."

/** The latest summary of a round or run, from what nacred reports for any checkout on that computer. */
fun latestWork(all: Map<String, List<WorkSummary>>, target: InspectTarget): WorkSummary? =
    all.filterKeys { Keys.host(it) == target.hostId }.values.flatten().firstOrNull { it.id == target.id }

/** A round's facts the inspector shows beside the bead rows. */
fun roundFacts(r: Round): List<String> = buildList {
    if (r.branch.isNotBlank()) add("Branch ${r.branch}")
    if (r.base.isNotBlank()) add("From ${r.base.take(7)}")
    if (r.finalCommit.isNotBlank()) add("Final commit ${r.finalCommit.take(7)}")
}

/**
 * The work inspector: live state, spend, the stop and the ways forward, each bead (opens its peek) with
 * Retry, its branch and verification, gates, artifacts, the round's changes, the log, and Cancel. Every
 * control works without the agent.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun WorkInspector(repoKey: String, target: InspectTarget, onClose: () -> Unit, onQueue: () -> Unit) {
    val container = LocalContainer.current
    val all by container.repository.work.collectAsState()
    val work = latestWork(all, target) ?: WorkSummary(kind = target.kind, id = target.id, title = target.label)
    val viewing = LocalViewing.current
    val open = LocalOpenEvidence.current
    DisposableEffect(target.key) {
        viewing(ChatReference(target.kind, target.id, work.title.ifBlank { "${target.kind.replaceFirstChar { it.uppercase() }} ${target.id}" }))
        onDispose { }
    }
    if (target.kind == "run") {
        Column(Modifier.fillMaxWidth().fillMaxHeight(0.92f)) {
            RunDetail(target.key, inPane = true, onBack = onClose, onOpenBead = { _, bead -> open(DecisionEvidence(bead, "bead", bead)) })
        }
        return
    }
    val vm = nacreViewModel(key = "round:${target.key}") { RoundViewModel(it.repository, target.key) }
    val round by vm.round.collectAsState()
    val events by vm.events.collectAsState()
    val busy by vm.busy.collectAsState()
    var cancelling by remember { mutableStateOf(false) }
    var log by remember { mutableStateOf(false) }
    // A land card's "Review the changes" opens the inspector with them already showing.
    var reviewing by remember(target.key) { mutableStateOf(target.review) }
    val r = round
    val landed = r?.beads?.count { it.state == com.ryderdavid.nacre.data.model.BEAD_INTEGRATED } ?: work.landed
    val total = r?.beads?.size ?: work.total

    LazyColumn(
        Modifier.fillMaxWidth().fillMaxHeight(0.92f).navigationBarsPadding(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 16.dp, end = 16.dp, bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(work.title.ifBlank { "Round ${target.id}" }, style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.semantics { heading() })
                    Text(listOf(work.change, r?.branch ?: work.branch, container.repository.hostName(target.hostId)).filter { it.isNotBlank() }.joinToString(" · "),
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                IconButton(onClick = onClose) { Icon(Icons.Rounded.Close, "Close") }
            }
        }
        item {
            val frac = if (total > 0) landed.toFloat() / total else 0f
            val label = "$landed of $total landed"
            if (r?.isActive == true && !work.stalled) LinearWavyProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth().semantics { contentDescription = label })
            else LinearProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth().heightIn(min = 8.dp).semantics { contentDescription = label })
        }
        if (work.gates.isNotEmpty()) item { GateChips(work) }
        if (r != null) item {
            StatusCard(r, busy, onBudget = vm::setBudget, onChoose = { c, usd -> vm.choose(c, usd) }, onCancel = { cancelling = true })
        }
        if (r != null && roundFacts(r).isNotEmpty()) item {
            SelectionContainer { Text(roundFacts(r).joinToString(" · "), style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace) }
        }
        if (work.state == "ready_to_land" || work.decisionId != null || r?.state == com.ryderdavid.nacre.data.model.ROUND_READY) item {
            Surface(onClick = onQueue, shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.tertiaryContainer, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp)) {
                Text("Landing is a decision for you: open the queue to review and land", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onTertiaryContainer)
            }
        }
        if (r != null && r.artifacts.isNotEmpty()) item {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text("Artifacts", style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
                SelectionContainer { Column { r.artifacts.forEach { Text(it, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace) } } }
            }
        }
        if (r != null && r.beads.isNotEmpty()) {
            item { Text("Beads", style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 4.dp).semantics { heading() }) }
            items(r.beads, key = { it.id }) { b ->
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    BeadRow(
                        r, b, busy,
                        onOpenBead = { onClose(); open(DecisionEvidence(b.id, "bead", b.id)) },
                        onOpenRun = { if (b.runId.isNotBlank()) open(DecisionEvidence(b.title, "run", Keys.of(target.hostId, b.runId))) },
                        onRetry = { vm.retry(b.id) },
                    )
                    val facts = listOfNotNull(
                        b.branch.takeIf { it.isNotBlank() }?.let { "Branch $it" },
                        b.verification?.let { v -> if (v.ok) "Verified" else null },
                    )
                    if (facts.isNotEmpty()) Text(facts.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 12.dp))
                }
            }
        }
        item {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                if (r?.isActive == true) OutlinedButton(onClick = { cancelling = true }, enabled = !busy, modifier = Modifier.heightIn(min = 48.dp)) {
                    Icon(Icons.Rounded.Stop, null); Text("Cancel round…", Modifier.padding(start = 6.dp), color = MaterialTheme.colorScheme.error)
                }
                if (r != null && r.base.isNotBlank()) TextButton(onClick = { reviewing = !reviewing }, modifier = Modifier.heightIn(min = 48.dp)) { Text(if (reviewing) "Hide the changes" else "Review the changes") }
                TextButton(onClick = { log = !log }, modifier = Modifier.heightIn(min = 48.dp)) { Text(if (log) "Hide the log" else "Full log") }
            }
        }
        if (reviewing && r != null) item {
            val changes by produceState<Result<com.ryderdavid.nacre.data.model.RoundChanges>?>(null, r.key, r.finalCommit) {
                value = runCatching { container.repository.roundChanges(r.key) }
            }
            RoundChangesList(changes)
        }
        if (log) items(events, key = { it.seq }) { e -> EventRow(e) }
    }

    if (cancelling) AlertDialog(
        onDismissRequest = { cancelling = false },
        title = { Text("Cancel the round and its workers?") },
        text = { Text(CANCEL_ROUND_TEXT) },
        confirmButton = { TextButton(onClick = { cancelling = false; vm.cancel() }) { Text("Cancel round", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = { cancelling = false }) { Text("Keep it running") } },
    )
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun GateChips(work: WorkSummary) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        work.gates.forEach { g ->
            val mark = when (g.state) { "pass" -> "passed"; "fail" -> "failed"; else -> "pending" }
            AssistChip(onClick = {}, label = { Text("${g.name}: $mark") })
        }
    }
}

/**
 * The work card in the thread a round or run started from: progress, gates and spend, Inspect, and Stop
 * with its scope stated: this reply only, or the round (or run) and its workers.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun WorkCard(work: WorkSummary, replyRunning: Boolean, onInspect: () -> Unit, onStopReply: () -> Unit, onCancelWork: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf(false) }
    val run = work.kind == "run"
    Surface(onClick = onInspect, shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(work.title.ifBlank { (if (run) "Run " else "Round ") + work.id }, style = MaterialTheme.typography.titleMedium)
                    Text(ChatFirstData.statusText(work) + if (work.budgetUsd > 0) " · $%.2f of $%.0f".format(work.spendUsd, work.budgetUsd) else "",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (work.open) androidx.compose.foundation.layout.Box {
                    IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "Stop…") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        if (replyRunning) DropdownMenuItem(text = { Text("Stop this reply") }, onClick = { menu = false; onStopReply() })
                        DropdownMenuItem(text = { Text(if (run) "Cancel this run…" else "Cancel round and its workers…") }, onClick = { menu = false; confirm = true })
                        DropdownMenuItem(text = { Text("Inspect") }, onClick = { menu = false; onInspect() })
                    }
                }
            }
            val frac = if (work.total > 0) work.landed.toFloat() / work.total else 0f
            if (work.running && !work.stalled) LinearWavyProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth())
            else LinearProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth())
            if (work.gates.isNotEmpty()) GateChips(work)
        }
    }
    if (confirm) AlertDialog(
        onDismissRequest = { confirm = false },
        title = { Text(if (run) "Cancel this run?" else "Cancel the round and its workers?") },
        text = { Text(if (run) CANCEL_RUN_TEXT else CANCEL_ROUND_TEXT) },
        confirmButton = { TextButton(onClick = { confirm = false; onCancelWork() }) { Text(if (run) "Cancel run" else "Cancel round", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = { confirm = false }) { Text("Keep it running") } },
    )
}
