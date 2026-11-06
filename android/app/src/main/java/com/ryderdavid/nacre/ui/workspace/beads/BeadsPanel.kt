package com.ryderdavid.nacre.ui.workspace.beads

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.ArrowDropDown
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.board.MoveAction
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.board.BoardUi
import com.ryderdavid.nacre.ui.board.BoardViewModel
import com.ryderdavid.nacre.ui.board.DraftHandlers
import com.ryderdavid.nacre.ui.board.rememberDragController
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.workspace.LocalAskAbout
import com.ryderdavid.nacre.ui.workspace.LocalOpenEvidence
import com.ryderdavid.nacre.ui.workspace.LocalViewing
import kotlinx.coroutines.launch

/**
 * The Beads panel: the project checkout's kanban. Columns of at least 272dp when the pane is wide or
 * focused; otherwise the compact board, one lane at a time. Moving is by long-press drag or Move to…;
 * everything beyond moving is asked for in chat, from the bead peek or the card's menu.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BeadsSlot(repoKey: String, modifier: Modifier) {
    val container = LocalContainer.current
    val repo = container.repository
    val vm = nacreViewModel(key = "board:$repoKey") { BoardViewModel(it.repository, repoKey) }
    val state by vm.state.collectAsState()
    val filter by vm.filter.collectAsState()
    val draftsBusy by vm.draftsBusy.collectAsState()
    val ask = LocalAskAbout.current
    val viewing = LocalViewing.current
    val openEvidence = LocalOpenEvidence.current

    var searching by rememberSaveable { mutableStateOf(false) }
    var laneKey by rememberSaveable(repoKey) { mutableStateOf(Lane.READY.key) }
    var peekId by rememberSaveable(repoKey) { mutableStateOf<String?>(null) }
    var moving by remember { mutableStateOf<Pair<Issue, Lane>?>(null) }
    var rootOffset by remember { mutableStateOf(Offset.Zero) }

    val ui = state.data

    fun perform(issue: Issue, from: Lane, to: Lane) {
        when (val o = BeadMoves.outcome(from, to)) {
            is MoveOutcome.Refused -> repo.notify(o.reason)
            MoveOutcome.Unchanged -> Unit
            // The repository moves optimistically and its notice offers Undo.
            is MoveOutcome.Moves -> repo.move(repoKey, issue, o.action)
        }
    }
    // The drag controller is remembered once, so it reads the board as it is at the drop.
    val board by rememberUpdatedState(ui)
    fun laneOf(issue: Issue): Lane = board?.snapshot?.let { it.index.workflowLane(issue, it.changes) } ?: Lane.READY
    val drag = rememberDragController { issue, key -> BeadMoves.laneByKey(key)?.let { perform(issue, laneOf(issue), it) } }

    // The peeked bead is what the pane is viewing, for the next message's context.
    DisposableEffect(peekId) {
        peekId?.let { viewing(beadReference(it)) }
        onDispose { }
    }

    val beadTargets by container.chats.references.beadTarget.collectAsState()
    LaunchedEffect(beadTargets[repoKey]) {
        beadTargets[repoKey]?.let { peekId = it; container.chats.references.beadOpened(repoKey) }
    }

    val handlers = CardHandlers(
        selectedId = peekId,
        onOpen = { peekId = it.id },
        onMoveTo = { issue, from -> moving = issue to from },
        onAsk = { ask(beadReference(it.id)) },
        onCloseCovered = { repo.move(repoKey, it, MoveAction.Close) },
        onShowMoreDone = { vm.showAllDone.value = true },
        drafts = DraftHandlers(busy = draftsBusy, onResume = vm::resumeDraft, onDismiss = vm::dismissDraft),
    )

    BoxWithConstraints(modifier.fillMaxSize().onGloballyPositioned { rootOffset = it.boundsInWindow().topLeft }) {
        val widthDp = maxWidth.value.toInt()
        val columns = BeadsLayout.usesColumns(widthDp)
        val row = rememberLazyListState()
        // A filter that leaves the lanes in view empty scrolls to the nearest lane with matches.
        LaunchedEffect(filter, columns, ui?.columns?.map { it.total }) {
            val cols = ui?.columns ?: return@LaunchedEffect
            if (!columns || !filter.isActive) return@LaunchedEffect
            val inView = row.layoutInfo.visibleItemsInfo.map { it.index }
            if (inView.any { cols.getOrNull(it)?.let { c -> c.total > 0 } == true }) return@LaunchedEffect
            com.ryderdavid.nacre.ui.board.laneWithMatches(cols, row.firstVisibleItemIndex)?.let { row.animateScrollToItem(it) }
        }
        Column(Modifier.fillMaxSize()) {
            Header(ui, columns, widthDp, row, searching, onSearch = { searching = !searching; if (!searching) vm.setQuery("") })
            if (searching) SearchField(filter.query, vm::setQuery)
            if (ui != null) {
                FilterRow(ui, filter, vm)
                BeadsFilters.note(ui.snapshot.index, filter)?.let { FilterNote(it, vm::clearFilters) }
            }
            Box(Modifier.weight(1f).fillMaxWidth()) {
                when {
                    ui == null && state.error != null -> ErrorState(state.error!!, vm::refresh)
                    ui == null -> LoadingState("Loading beads…")
                    columns -> ColumnBeads(ui, widthDp, row, drag, handlers)
                    else -> CompactBeads(ui, laneKey, onLane = { laneKey = it }, drag = drag, query = filter.query, h = handlers)
                }
            }
        }
        LiftedCard(drag, ui, rootOffset, inColumn = columns)
    }

    moving?.let { (issue, from) ->
        MoveToDialog(issue, from, onDismiss = { moving = null }) { to -> moving = null; perform(issue, from, to) }
    }

    val peeked = peekId?.let { id -> ui?.snapshot?.index?.byId?.get(id) }
    if (peeked != null && ui != null) BeadPeekSheet(
        repoKey, peeked, ui,
        onDismiss = { peekId = null },
        onMoveTo = { issue, from -> moving = issue to from },
        onOpenBead = { peekId = it },
        // The project's work inspector, the same one the chat's work card opens.
        onInspect = { kind, id, label -> peekId = null; openEvidence(inspectorEvidence(kind, id, label)) },
    )
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun Header(
    ui: BoardUi?,
    columns: Boolean,
    widthDp: Int,
    row: androidx.compose.foundation.lazy.LazyListState,
    searching: Boolean,
    onSearch: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    Row(Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(start = 16.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text("Beads", style = MaterialTheme.typography.titleLargeEmphasized)
            if (ui != null) Text(
                listOfNotNull(
                    ui.snapshot.repo.name,
                    "${ui.stats[Lane.READY] ?: 0} ready · ${ui.stats[Lane.DOING] ?: 0} in progress",
                ).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis,
            )
        }
        if (columns && ui != null) {
            val fit = BeadsLayout.columnsThatFit(widthDp)
            val total = ui.columns.size
            // Lanes fully in view, not the partly scrolled one at the edge.
            val shownLanes by remember(row) { derivedStateOf {
                val info = row.layoutInfo
                val full = info.visibleItemsInfo.filter { it.offset >= info.viewportStartOffset - 1 && it.offset + it.size <= info.viewportEndOffset + 1 }
                (full.ifEmpty { info.visibleItemsInfo }).let { v -> (v.firstOrNull()?.index ?: 0) to (v.lastOrNull()?.index ?: 0) }
            } }
            val first = shownLanes.first
            val last = shownLanes.second
            val label = BeadsLayout.rangeLabel(first, last - first + 1, total)
            if (label.isNotEmpty()) {
                Text(label, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 4.dp))
                IconButton(onClick = { scope.launch { row.animateScrollToItem((first - fit).coerceAtLeast(0)) } }, enabled = first > 0) {
                    Icon(Icons.AutoMirrored.Rounded.KeyboardArrowLeft, "Previous lanes")
                }
                IconButton(onClick = { scope.launch { row.animateScrollToItem((first + fit).coerceAtMost(total - 1)) } }, enabled = last < total - 1) {
                    Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, "Next lanes")
                }
            }
        }
        IconButton(onClick = onSearch) { Icon(if (searching) Icons.Rounded.Close else Icons.Rounded.Search, if (searching) "Close search" else "Search beads") }
    }
}

@Composable
private fun SearchField(query: String, onQuery: (String) -> Unit) {
    TextField(
        value = query, onValueChange = onQuery, singleLine = true,
        placeholder = { Text("Search titles, ids, labels") },
        leadingIcon = { Icon(Icons.Rounded.Search, null) },
        shape = RoundedCornerShape(28.dp),
        colors = TextFieldDefaults.colors(focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
    )
}

/** Filters: an epic or change, Needs you, and a few types. */
@Composable
private fun FilterRow(ui: BoardUi, filter: BoardFilter, vm: BoardViewModel) {
    val index = ui.snapshot.index
    Row(
        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically,
    ) {
        if (ui.epics.isNotEmpty()) {
            var open by remember { mutableStateOf(false) }
            Box {
                val epicTitle = when (filter.epic) {
                    "" -> "All work"
                    BoardFilter.NO_EPIC -> "Not in an epic"
                    else -> index.byId[filter.epic]?.title ?: filter.epic
                }
                FilterChip(
                    selected = filter.epic.isNotEmpty(), onClick = { open = true },
                    label = { Text(epicTitle, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.widthIn(max = 220.dp)) },
                    leadingIcon = if (filter.epic.isNotEmpty()) ({ Icon(Icons.Rounded.Check, null) }) else null,
                    trailingIcon = { Icon(Icons.Rounded.ArrowDropDown, "Choose an epic or change") },
                )
                DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                    DropdownMenuItem(text = { Text("All work") }, onClick = { open = false; if (filter.epic.isNotEmpty()) vm.setEpic(filter.epic) })
                    ui.epics.forEach { e ->
                        DropdownMenuItem(text = { Text("${e.title} · ${e.done} of ${e.total} done") }, onClick = { open = false; if (filter.epic != e.id) vm.setEpic(e.id) })
                    }
                    if (ui.looseCount > 0) DropdownMenuItem(text = { Text("Not in an epic") }, onClick = { open = false; if (filter.epic != BoardFilter.NO_EPIC) vm.setEpic(BoardFilter.NO_EPIC) })
                }
            }
        }
        FilterChip(
            selected = filter.needsYou, onClick = vm::toggleNeedsYou,
            label = { Text(if (ui.needsYou > 0) "Needs you · ${ui.needsYou}" else "Needs you") },
            leadingIcon = if (filter.needsYou) ({ Icon(Icons.Rounded.Check, null) }) else null,
        )
        BeadsFilters.TYPES.forEach { (type, label) ->
            val on = type in filter.types
            FilterChip(selected = on, onClick = { vm.toggleType(type) }, label = { Text(label) }, leadingIcon = if (on) ({ Icon(Icons.Rounded.Check, null) }) else null)
        }
    }
}

/** Says what the filters hide, and offers to clear them. Announced politely when it changes. */
@Composable
private fun FilterNote(note: String, onClear: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
    ) {
        Row(Modifier.padding(start = 14.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                note, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.weight(1f).padding(vertical = 8.dp).semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = onClear) { Text("Clear") }
        }
    }
}
