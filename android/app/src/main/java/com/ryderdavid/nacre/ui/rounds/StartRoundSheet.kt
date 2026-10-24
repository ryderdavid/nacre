package com.ryderdavid.nacre.ui.rounds

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

/** The beads a round can take from a board: everything open except epics, ready work first. */
fun roundCandidates(index: BoardIndex): List<Issue> {
    val order = listOf(Lane.READY, Lane.DOING, Lane.BLOCKED)
    return index.issues
        .filter { !it.isClosed && it.type != "epic" && index.laneOf(it) in order }
        .sortedWith(compareBy<Issue> { order.indexOf(index.laneOf(it)) }.thenBy { it.priority }.thenBy { it.id })
}

/** Why a repository can't take a round, or null when it can. */
fun roundUnavailable(repo: Repo?): String? = when {
    repo == null -> "This board isn't loaded yet."
    repo.gatesError.isNotBlank() -> "Its nacre.yaml can't be used: ${repo.gatesError}"
    repo.gates.isEmpty() -> "${repo.name} needs a nacre.yaml first: it names the tests and build that decide when work is done."
    else -> null
}

/**
 * Pick several beads and hand them to the computer as one round: it reviews them together,
 * works each on its own branch, checks and merges them, and finishes on one commit.
 */
@Composable
fun StartRoundSheet(repo: Repo?, index: BoardIndex, onDismiss: () -> Unit, onStarted: (roundKey: String) -> Unit) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    var picked by rememberSaveable { mutableStateOf(listOf<String>()) }
    var budget by rememberSaveable { mutableStateOf("") }
    var starting by rememberSaveable { mutableStateOf(false) }
    var error by rememberSaveable { mutableStateOf<String?>(null) }
    val unavailable = roundUnavailable(repo)
    val candidates = roundCandidates(index)

    HandleDismissSheet(onDismiss = onDismiss) {
        Column(Modifier.padding(horizontal = 24.dp).navigationBarsPadding(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Work several beads until done", style = MaterialTheme.typography.headlineSmall)
            Text(
                "${repo?.hostName?.ifBlank { null } ?: "The computer"} reviews them together first and asks you only where they disagree. " +
                    "Then it works each on its own branch, checks it against the repository's gates, and merges them into one commit that builds.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (unavailable != null) {
                Text(unavailable, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            } else {
                Text("Gates: ${repo!!.gates.joinToString(", ")}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 420.dp)) {
                    items(candidates, key = { it.id }) { issue ->
                        val on = issue.id in picked
                        Row(
                            Modifier.fillMaxWidth().clickable { picked = if (on) picked - issue.id else picked + issue.id }.padding(vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Checkbox(checked = on, onCheckedChange = { picked = if (it) picked + issue.id else picked - issue.id })
                            Column(Modifier.weight(1f)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    BeadId(issue.id)
                                    Spacer(Modifier.width(8.dp))
                                    Text("P${issue.priority} · ${issue.type} · ${index.laneOf(issue).title}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                Text(issue.title, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
                OutlinedTextField(
                    value = budget, onValueChange = { budget = it.filter { c -> c.isDigit() || c == '.' } }, singleLine = true,
                    prefix = { Text("$") }, label = { Text("Budget") },
                    placeholder = { Text("The computer's default") },
                    supportingText = { Text("The round pauses when it has spent this much of your Claude subscription.") },
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
                Button(
                    onClick = {
                        starting = true
                        error = null
                        scope.launch {
                            try {
                                val round = container.repository.startRound(repo.key, picked, budget.toDoubleOrNull())
                                onStarted(round.key)
                            } catch (e: Exception) {
                                error = e.friendly()
                            } finally {
                                starting = false
                            }
                        }
                    },
                    enabled = picked.isNotEmpty() && !starting,
                    modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp),
                ) {
                    Text(
                        when (picked.size) {
                            0 -> "Pick the beads to work on"
                            1 -> "Work this one until done"
                            else -> "Work these ${picked.size} until done"
                        },
                    )
                }
            }
        }
    }
}
