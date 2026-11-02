package com.ryderdavid.nacre.ui.rounds

import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.collectAsState
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
import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.GoalOptions
import com.ryderdavid.nacre.ui.common.GoalOptionsPicker
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.data.model.hasSpecWorkflow
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.isPendingAmendment

/**
 * The beads a round can take from a board: everything open except epics, ready work
 * first. Beads whose proposal is still being planned, and pending amendments, aren't work yet.
 */
fun roundCandidates(index: BoardIndex, changes: ChangesList? = null): List<Issue> {
    val order = listOf(Lane.READY, Lane.DOING, Lane.BLOCKED)
    return index.issues
        .filter { !it.isClosed && it.type != "epic" && !it.hasSpecWorkflow && !it.isPendingAmendment && index.workflowLane(it, changes) in order }
        .sortedWith(compareBy<Issue> { order.indexOf(index.laneOf(it)) }.thenBy { it.priority }.thenBy { it.id })
}

/** Preserve the approved section order and never offer unrelated board work in a spec round. */
fun specificationCandidates(index: BoardIndex, change: SpecChange): List<Issue> =
    change.implementationIds.mapNotNull(index.byId::get).filterNot { it.isClosed }

fun specificationUnavailable(index: BoardIndex, change: SpecChange): String? = when {
    // Explain a stale approval instead of failing with a conflict when the round starts.
    change.phase == "stale" -> com.ryderdavid.nacre.ui.bead.staleText(change.staleReason)
    change.implementationIds.isEmpty() -> "This change needs a current approval and a complete section mapping."
    change.implementationIds.any { it !in index.byId } -> "Some approved beads aren't loaded yet. Close this sheet and refresh the board before starting."
    change.implementationIds.any { index.byId[it]?.type == "epic" } -> "A task section points to an epic. Repair the section mapping before starting."
    specificationCandidates(index, change).isEmpty() -> "All mapped beads are closed. Closure alone does not verify this change. Review its evidence, or reopen a section bead that still needs implementation."
    else -> null
}

/**
 * The candidates to list given the board's filter (NAC-3mc): with [keepBoardFilter], the board's
 * epic, type and needs-you scope; always the sheet's own [query], which starts as the board's search.
 */
fun scopedCandidates(candidates: List<Issue>, index: BoardIndex, boardFilter: BoardFilter?, query: String, keepBoardFilter: Boolean): List<Issue> {
    val scope = if (keepBoardFilter && boardFilter != null) boardFilter.copy(query = query) else BoardFilter(query = query)
    return candidates.filter { index.matches(it, scope) }
}

/** What the board's filter narrows a round to, beyond its search, or null when nothing. */
fun boardScopeLabel(index: BoardIndex, f: BoardFilter?): String? {
    if (f == null) return null
    val parts = listOfNotNull(
        when (f.epic) {
            "" -> null
            BoardFilter.NO_EPIC -> "outside any epic"
            else -> "under ${index.byId[f.epic]?.title ?: f.epic}"
        },
        f.types.takeIf { it.isNotEmpty() }?.sorted()?.joinToString(", "),
        "needing you".takeIf { f.needsYou },
    )
    return parts.takeIf { it.isNotEmpty() }?.joinToString(" · ")
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
fun StartRoundSheet(
    repo: Repo?, index: BoardIndex, onDismiss: () -> Unit, onStarted: (roundKey: String) -> Unit, specChange: SpecChange? = null,
    /** The board's search and epic filter when the sheet opened; the list starts narrowed to it. */
    boardFilter: BoardFilter? = null,
) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    val prefs by container.settings.settings.collectAsState(initial = null)
    val saved = prefs?.runGoal
    var goalEnabled by rememberSaveable(saved) { mutableStateOf(saved?.enabled ?: false) }
    var goalCommitted by rememberSaveable(saved) { mutableStateOf(saved?.committed ?: false) }
    var goalPushed by rememberSaveable(saved) { mutableStateOf(saved?.pushed ?: false) }
    val goal = GoalOptions(goalEnabled, goalCommitted, goalPushed)
    var picked by rememberSaveable(specChange?.id, specChange?.revision) { mutableStateOf(specChange?.let { specificationCandidates(index, it).map(Issue::id) }.orEmpty()) }
    var budget by rememberSaveable(specChange?.id) { mutableStateOf(if (specChange == null) "" else "5") }
    var starting by rememberSaveable { mutableStateOf(false) }
    var error by rememberSaveable { mutableStateOf<String?>(null) }
    val unavailable = roundUnavailable(repo) ?: specChange?.let { specificationUnavailable(index, it) }
    val candidates = specChange?.let { specificationCandidates(index, it) } ?: roundCandidates(index, repo?.let { container.repository.changes(it.key) })
    var query by rememberSaveable { mutableStateOf(boardFilter?.query.orEmpty()) }
    val scopeLabel = if (specChange == null) boardScopeLabel(index, boardFilter) else null
    var keepScope by rememberSaveable { mutableStateOf(scopeLabel != null) }
    val shown = if (specChange != null) candidates else scopedCandidates(candidates, index, boardFilter, query, keepScope)
    val hiddenPicks = picked.count { id -> shown.none { it.id == id } }
    val budgetValid = (budget.isBlank() && specChange == null) || budget.toDoubleOrNull()?.let { it.isFinite() && it > 0 } == true

    HandleDismissSheet(onDismiss = onDismiss) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).navigationBarsPadding().imePadding(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(if (specChange == null) "Work several beads until done" else "Implement this change", style = MaterialTheme.typography.headlineSmall)
            Text(
                if (specChange != null) "Only sections from the approved proposal are included. The computer checks this revision again, installs its specification, then implements and verifies the selected work. Landing remains your decision."
                else "${repo?.hostName?.ifBlank { null } ?: "The computer"} reviews them together first and asks you only where they disagree. " +
                    "Then it works each on its own branch, checks it against the repository's gates, and merges them into one commit that builds.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (unavailable != null) {
                Text(unavailable, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            } else {
                Text("Gates: ${repo!!.gates.joinToString(", ")}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (specChange != null) Text("${specChange.changeName} · revision ${specChange.revision.take(12)}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                if (specChange != null && candidates.size < specChange.implementationIds.size) Text("${specChange.implementationIds.size - candidates.size} closed section beads are omitted. Their evidence remains in the change.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (candidates.isEmpty()) Text("No ordinary open work is available. Specification work starts from its Change card.", style = MaterialTheme.typography.bodyMedium)
                if (specChange == null && candidates.isNotEmpty()) {
                    OutlinedTextField(
                        value = query, onValueChange = { query = it }, singleLine = true,
                        placeholder = { Text("Search titles, ids, labels") },
                        shape = RoundedCornerShape(28.dp),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    if (scopeLabel != null) FilterChip(
                        selected = keepScope, onClick = { keepScope = !keepScope },
                        label = { Text("Only $scopeLabel", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    )
                    if (shown.isEmpty()) Text("Nothing open matches. Clear the search to see all ${candidates.size}.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (hiddenPicks > 0) Text("$hiddenPicks picked not shown by this search", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 280.dp)) {
                    items(shown, key = { it.id }) { issue ->
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
                    isError = !budgetValid,
                    modifier = Modifier.fillMaxWidth(),
                )
                GoalOptionsPicker(goal, { goalEnabled = it.enabled; goalCommitted = it.committed; goalPushed = it.pushed }, enabled = !starting && prefs != null)
                if (goal.enabled) Text("One session works every selected bead and commits it. Nacre bumps the version and, if pushed is selected, pushes when it lands the round.", style = MaterialTheme.typography.bodySmall)
                error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
                Button(
                    onClick = {
                        starting = true
                        error = null
                        scope.launch {
                            try {
                                val round = container.repository.startRound(repo.key, picked, budget.toDoubleOrNull(), specChange?.id, specChange?.revision, goal = goal)
                                onStarted(round.key)
                            } catch (e: Exception) {
                                error = e.friendly()
                            } finally {
                                starting = false
                            }
                        }
                    },
                    enabled = picked.isNotEmpty() && !starting && budgetValid && prefs != null,
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
