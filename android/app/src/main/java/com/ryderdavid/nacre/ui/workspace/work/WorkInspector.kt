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
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.rounds.BeadRow
import com.ryderdavid.nacre.ui.rounds.EventRow
import com.ryderdavid.nacre.ui.rounds.RoundViewModel
import com.ryderdavid.nacre.ui.rounds.StatusCard
import com.ryderdavid.nacre.ui.runs.RunDetail

/** What Cancel round says will happen, so it's clear what keeps running. */
const val CANCEL_ROUND_TEXT = "Cancels the round and stops its workers. Branches and anything already merged on its branch stay. Chats keep running."

/**
 * The work inspector: a round's state and spend, its stop and the ways forward, each bead with Retry, the
 * gates and the log, and Cancel round. Every control works without the agent.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun WorkInspector(repoKey: String, work: WorkSummary, onClose: () -> Unit, onQueue: () -> Unit) {
    val host = Keys.host(repoKey)
    if (work.kind == "run") {
        Column(Modifier.fillMaxWidth().fillMaxHeight(0.92f)) {
            RunDetail(Keys.of(host, work.id), inPane = true, onBack = onClose, onOpenBead = { _, _ -> })
        }
        return
    }
    val roundKey = Keys.of(host, work.id)
    val vm = nacreViewModel(key = "round:$roundKey") { RoundViewModel(it.repository, roundKey) }
    val round by vm.round.collectAsState()
    val events by vm.events.collectAsState()
    val busy by vm.busy.collectAsState()
    var cancelling by remember { mutableStateOf(false) }
    var log by remember { mutableStateOf(false) }
    val r = round

    LazyColumn(
        Modifier.fillMaxWidth().fillMaxHeight(0.92f).navigationBarsPadding(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 16.dp, end = 16.dp, bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(work.title.ifBlank { "Round ${work.id}" }, style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.semantics { heading() })
                    Text(listOf(work.change, r?.branch ?: work.branch).filter { it.isNotBlank() }.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                IconButton(onClick = onClose) { Icon(Icons.Rounded.Close, "Close") }
            }
        }
        item {
            val frac = if (work.total > 0) work.landed.toFloat() / work.total else 0f
            if (work.running && !work.stalled) LinearWavyProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth().semantics { contentDescription = "${work.landed} of ${work.total} landed" })
            else LinearProgressIndicator(progress = { frac }, modifier = Modifier.fillMaxWidth().heightIn(min = 8.dp).semantics { contentDescription = "${work.landed} of ${work.total} landed" })
        }
        if (work.gates.isNotEmpty()) item { GateChips(work) }
        if (r != null) item {
            StatusCard(r, busy, onBudget = vm::setBudget, onChoose = { c, usd -> vm.choose(c, usd) }, onCancel = { cancelling = true })
        }
        if (work.state == "ready_to_land" || work.decisionId != null) item {
            Surface(onClick = onQueue, shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.tertiaryContainer, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp)) {
                Text("Landing is a decision for you: open the queue to review and land", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onTertiaryContainer)
            }
        }
        if (r != null && r.beads.isNotEmpty()) {
            item { Text("Beads", style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 4.dp).semantics { heading() }) }
            items(r.beads, key = { it.id }) { b ->
                BeadRow(r, b, busy, onOpenBead = {}, onOpenRun = {}, onRetry = { vm.retry(b.id) })
            }
        }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (r?.isActive == true) OutlinedButton(onClick = { cancelling = true }, enabled = !busy, modifier = Modifier.heightIn(min = 48.dp)) {
                    Icon(Icons.Rounded.Stop, null); Text("Cancel round…", Modifier.padding(start = 6.dp), color = MaterialTheme.colorScheme.error)
                }
                TextButton(onClick = { log = !log }, modifier = Modifier.heightIn(min = 48.dp)) { Text(if (log) "Hide the log" else "Full log") }
            }
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
 * The work card in the thread a round started from: progress, gates and spend, Inspect, and Stop with its
 * scope stated: this reply only, or the round and its workers.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun WorkCard(work: WorkSummary, replyRunning: Boolean, onInspect: () -> Unit, onStopReply: () -> Unit, onCancelRound: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf(false) }
    Surface(onClick = onInspect, shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(work.title.ifBlank { "Round ${work.id}" }, style = MaterialTheme.typography.titleMedium)
                    Text(ChatFirstData.statusText(work) + if (work.budgetUsd > 0) " · $%.2f of $%.0f".format(work.spendUsd, work.budgetUsd) else "",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                androidx.compose.foundation.layout.Box {
                    IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "Stop…") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        if (replyRunning) DropdownMenuItem(text = { Text("Stop this reply") }, onClick = { menu = false; onStopReply() })
                        DropdownMenuItem(text = { Text("Cancel round and its workers…") }, onClick = { menu = false; confirm = true })
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
        title = { Text("Cancel the round and its workers?") },
        text = { Text(CANCEL_ROUND_TEXT) },
        confirmButton = { TextButton(onClick = { confirm = false; onCancelRound() }) { Text("Cancel round", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = { confirm = false }) { Text("Keep it running") } },
    )
}
