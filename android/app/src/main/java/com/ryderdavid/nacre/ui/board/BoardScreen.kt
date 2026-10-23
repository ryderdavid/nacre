package com.ryderdavid.nacre.ui.board

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.GroupBy
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.ui.assistant.AssistantPane
import com.ryderdavid.nacre.ui.bead.BeadDetail
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.nacreViewModel
import kotlinx.coroutines.launch

private const val PANE_ASSISTANT = "@assistant"

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BoardScreen(
    repoKey: String,
    onBack: () -> Unit,
    onOpenBead: (String) -> Unit,
    onOpenAssistant: () -> Unit,
    onOpenRun: (String) -> Unit,
    onSwitchBoard: (String) -> Unit,
) {
    val vm = nacreViewModel(key = "board:$repoKey") { BoardViewModel(it.repository, repoKey) }
    val group by vm.group.collectAsState()
    val hostCount = vm.hosts.collectAsState().value.size
    val state by vm.state.collectAsState()
    val filter by vm.filter.collectAsState()
    val groupBy by vm.groupBy.collectAsState()
    val container = LocalContainer.current
    val prefs by container.settings.settings.collectAsState(initial = null)

    var searching by rememberSaveable { mutableStateOf(false) }
    var menu by remember { mutableStateOf(false) }
    var newBead by rememberSaveable { mutableStateOf(false) }
    var pane by rememberSaveable { mutableStateOf<String?>(null) }
    var scrollTo by remember { mutableStateOf<Lane?>(null) }
    var rootOffset by remember { mutableStateOf(Offset.Zero) }
    var rootBounds by remember { mutableStateOf(Rect.Zero) }

    val density = LocalDensity.current.density
    val drag = rememberDragController { issue, key -> vm.move(issue, key) }
    val ui = state.data
    val repo = ui?.snapshot?.repo

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "All boards") } },
                title = {
                    Column {
                        Text(repo?.name ?: "Board", maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (ui != null) Text(
                            listOfNotNull(
                                repo?.hostName?.takeIf { hostCount > 1 && it.isNotBlank() },
                                "${ui.stats[Lane.READY] ?: 0} ready · ${ui.stats[Lane.DOING] ?: 0} in progress · ${ui.stats[Lane.BLOCKED] ?: 0} blocked",
                            ).joinToString(" · "),
                            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1,
                        )
                    }
                },
                actions = {
                    IconButton(onClick = { searching = !searching; if (!searching) vm.setQuery("") }) {
                        Icon(if (searching) Icons.Rounded.Close else Icons.Rounded.Search, "Search beads")
                    }
                    IconButton(onClick = { if (rootBounds.width / density >= 760f) pane = PANE_ASSISTANT else onOpenAssistant() }) {
                        Icon(AiIcon, "Ask Claude about this board")
                    }
                    IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "Board options") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        group?.takeIf { it.isShared }?.let { g ->
                            Text("Beads from", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
                            g.members.forEach { m ->
                                DropdownMenuItem(
                                    text = { Text(m.hostName) },
                                    leadingIcon = { if (m.key == repoKey) Icon(Icons.Rounded.Check, null) else Spacer(Modifier.size(24.dp)) },
                                    onClick = {
                                        menu = false
                                        if (m.key != repoKey) { vm.makeHome(m); onSwitchBoard(m.key) }
                                    },
                                )
                            }
                            HorizontalDivider()
                        }
                        Text("Group by", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
                        GroupBy.entries.forEach { g ->
                            DropdownMenuItem(
                                text = { Text(g.title) },
                                leadingIcon = { if (g == groupBy) Icon(Icons.Rounded.Check, null) else Spacer(Modifier.size(24.dp)) },
                                onClick = { menu = false; vm.groupBy.value = g },
                            )
                        }
                        HorizontalDivider()
                        val showAll by vm.showAllDone.collectAsState()
                        DropdownMenuItem(
                            text = { Text("Show all done beads") },
                            leadingIcon = { if (showAll) Icon(Icons.Rounded.Check, null) else Spacer(Modifier.size(24.dp)) },
                            onClick = { menu = false; vm.showAllDone.value = !showAll },
                        )
                        DropdownMenuItem(text = { Text("Refresh") }, leadingIcon = { Icon(Icons.Rounded.Refresh, null) }, onClick = { menu = false; vm.refresh() })
                    }
                },
            )
        },
        floatingActionButton = {
            if (!drag.isDragging && pane == null) ExtendedFloatingActionButton(
                onClick = { newBead = true },
                icon = { Icon(Icons.Rounded.Add, null) },
                text = { Text("New bead") },
            )
        },
    ) { pad ->
        Box(
            Modifier.padding(pad).fillMaxSize().onGloballyPositioned {
                rootBounds = it.boundsInWindow()
                rootOffset = rootBounds.topLeft
            },
        ) {
            when {
                ui == null && state.error != null -> ErrorState(state.error!!, vm::refresh)
                ui == null -> LoadingState("Loading beads…")
                else -> BoxWithConstraints(Modifier.fillMaxSize()) {
                    val canPane = maxWidth >= 760.dp
                    LaunchedEffect(canPane) { if (!canPane) pane = null }

                    val paneWidth = rememberPaneWidth(rootBounds, maxWidth, prefs?.detailPaneDp ?: 440)
                    Row(Modifier.fillMaxSize()) {
                        BoxWithConstraints(Modifier.weight(1f).fillMaxHeight()) {
                        val boardWide = maxWidth >= 600.dp
                        // When the board narrows to one lane at a time, show the open bead's lane.
                        LaunchedEffect(boardWide, pane) {
                            val ix = ui.snapshot.index
                            if (!boardWide && groupBy == GroupBy.WORKFLOW) pane?.let { ix.byId[it] }?.let { scrollTo = ix.laneOf(it) }
                        }
                        Column(Modifier.fillMaxSize()) {
                            if (searching) OutlinedTextField(
                                value = filter.query, onValueChange = vm::setQuery, singleLine = true,
                                placeholder = { Text("Search titles, ids, labels") },
                                shape = RoundedCornerShape(28.dp),
                                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                            )
                            if (boardWide) StatStrip(ui, filter, onLane = { scrollTo = it }, onNeedsYou = vm::toggleNeedsYou, modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp))
                            EpicChips(ui, filter, onEpic = vm::setEpic, onNeedsYou = if (boardWide) null else vm::toggleNeedsYou, modifier = Modifier.padding(vertical = 4.dp))
                            FilterSummary(filter, vm::clearFilters)
                            Box(Modifier.fillMaxSize()) {
                                val open: (com.ryderdavid.nacre.data.model.Issue) -> Unit = { i ->
                                    if (canPane) pane = if (pane == i.id) null else i.id else onOpenBead(i.id)
                                }
                                if (boardWide) WideBoard(
                                    ui, groupBy, selectedId = pane, drag = drag, scrollToLane = scrollTo, onScrolled = { scrollTo = null },
                                    onOpen = open, onShowMoreDone = { vm.showAllDone.value = true },
                                ) else CompactBoard(
                                    ui, groupBy, drag = drag, scrollToLane = scrollTo, onScrolled = { scrollTo = null },
                                    onOpen = open, onShowMoreDone = { vm.showAllDone.value = true },
                                )
                            }
                        }
                        }
                        AnimatedVisibility(visible = canPane && pane != null, enter = expandHorizontally(expandFrom = Alignment.Start), exit = shrinkHorizontally(shrinkTowards = Alignment.Start)) {
                            Row(Modifier.fillMaxHeight()) {
                                PaneDivider(onDrag = { dx -> paneWidth.resize(-dx) }, onDragEnd = {
                                    val dp = paneWidth.width.value.toInt()
                                    container.scope.launch { container.settings.update { it.copy(detailPaneDp = dp) } }
                                })
                                Box(Modifier.width(paneWidth.width).fillMaxHeight()) {
                                    val current = pane
                                    when {
                                        current == PANE_ASSISTANT -> AssistantPane(repoKey, repo?.name ?: "", inPane = true, onClose = { pane = null }, onOpenBead = { pane = it }, onOpenRun = onOpenRun)
                                        current != null -> BeadDetail(repoKey, current, inPane = true, onClose = { pane = null }, onOpenBead = { pane = it }, onOpenRun = onOpenRun)
                                    }
                                }
                            }
                        }
                    }
                }
            }
            DragOverlay(drag, ui?.snapshot?.index, rootOffset)
        }
    }

    if (newBead && ui != null) NewBeadSheet(
        repoKey, repo?.name.orEmpty(), ui.snapshot.index,
        defaultParent = filter.epic.takeIf { it.isNotEmpty() && it != com.ryderdavid.nacre.data.board.BoardFilter.NO_EPIC },
        onDismiss = { newBead = false },
        onCreated = { newBead = false; if (pane != null) pane = it.id },
    )
}

/** Width of the side pane: hinge-aligned when the fold separates the screen, else user-sized. */
private class PaneWidth(initial: Dp, private val min: Dp, private val max: Dp, val hingeLocked: Boolean) {
    private var w by mutableFloatStateOf(initial.value)
    val width: Dp get() = w.dp.coerceIn(min, max)
    var density = 1f
    fun resize(dxPx: Float) {
        if (hingeLocked) return
        w = (w + dxPx / density).coerceIn(min.value, max.value)
    }
}

@Composable
private fun rememberPaneWidth(root: Rect, available: Dp, preferredDp: Int): PaneWidth {
    val density = LocalDensity.current
    val hinge = currentWindowAdaptiveInfo().windowPosture.hingeList.firstOrNull { it.isVertical && (it.isSeparating || it.isOccluding) }
    val hingeWidth = hinge?.let { with(density) { (root.right - it.bounds.right).toDp() } }?.takeIf { it > 280.dp && it < available - 300.dp }
    val max = (available - 320.dp).coerceAtLeast(320.dp)
    return remember(hingeWidth, available, preferredDp) {
        PaneWidth(hingeWidth ?: preferredDp.dp, min = 320.dp, max = max, hingeLocked = hingeWidth != null)
    }.also { it.density = density.density }
}

@Composable
private fun PaneDivider(onDrag: (Float) -> Unit, onDragEnd: () -> Unit) {
    Box(
        Modifier.width(16.dp).fillMaxHeight().draggable(
            orientation = Orientation.Horizontal,
            state = rememberDraggableState { onDrag(it) },
            onDragStopped = { onDragEnd() },
        ),
        contentAlignment = Alignment.Center,
    ) {
        Box(Modifier.width(4.dp).height(48.dp).background(MaterialTheme.colorScheme.outline, RoundedCornerShape(2.dp)))
    }
}
