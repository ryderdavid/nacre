package com.ryderdavid.nacre.ui.board

import androidx.activity.compose.BackHandler
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
import androidx.compose.material.icons.rounded.RocketLaunch
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
import com.ryderdavid.nacre.data.claude.ChatContext
import com.ryderdavid.nacre.ui.assistant.ChatHost
import com.ryderdavid.nacre.ui.assistant.ChatHeaderAction
import com.ryderdavid.nacre.ui.common.isPaneWide
import com.ryderdavid.nacre.ui.bead.BeadDetail
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.rounds.StartRoundSheet
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BoardScreen(
    repoKey: String,
    onBack: () -> Unit,
    onOpenBead: (String) -> Unit,
    onOpenChatBead: (String, String) -> Unit,
    onOpenRun: (String) -> Unit,
    onSwitchBoard: (String) -> Unit,
    onOpenRound: (String) -> Unit,
) {
    val vm = nacreViewModel(key = "board:$repoKey") { BoardViewModel(it.repository, repoKey) }
    val group by vm.group.collectAsState()
    val hostCount = vm.hosts.collectAsState().value.size
    val state by vm.state.collectAsState()
    val filter by vm.filter.collectAsState()
    val groupBy by vm.groupBy.collectAsState()
    val container = LocalContainer.current
    val chatContext = ChatContext.Board(repoKey)
    val presentations by container.chatStack.presentations.collectAsState()
    val chat = presentations[chatContext]

    var searching by rememberSaveable { mutableStateOf(false) }
    var menu by remember { mutableStateOf(false) }
    var newBead by rememberSaveable { mutableStateOf(false) }
    var pane by rememberSaveable { mutableStateOf<String?>(null) }
    var scrollTo by remember { mutableStateOf<Lane?>(null) }
    var rootOffset by remember { mutableStateOf(Offset.Zero) }
    var rootBounds by remember { mutableStateOf(Rect.Zero) }

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
                    ChatHeaderAction(chatContext, repo?.name ?: repoKey)
                    IconButton(onClick = { searching = !searching; if (!searching) vm.setQuery("") }) {
                        Icon(if (searching) Icons.Rounded.Close else Icons.Rounded.Search, "Search beads")
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
                        val showAll by vm.showAllDone.collectAsState()
                        DropdownMenuItem(
                            text = { Text("Show all done beads") },
                            leadingIcon = { if (showAll) Icon(Icons.Rounded.Check, null) else Spacer(Modifier.size(24.dp)) },
                            onClick = { menu = false; vm.showAllDone.value = !showAll },
                        )
                        HorizontalDivider()
                        DropdownMenuItem(text = { Text("Refresh") }, leadingIcon = { Icon(Icons.Rounded.Refresh, null) }, onClick = { menu = false; vm.refresh() })
                    }
                },
            )
        },
        floatingActionButton = {
            if (!drag.isDragging && pane == null && (chat?.selected == null || chat.collapsed)) Column(
                horizontalAlignment = Alignment.End,
                verticalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(12.dp),
            ) {
                ExtendedFloatingActionButton(
                    onClick = { newBead = true },
                    icon = { Icon(Icons.Rounded.Add, null) },
                    text = { Text(if (ui?.snapshot?.changes?.openspec == true) "New effort" else "New") },
                )
            }
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
                    val canPane = isPaneWide(maxWidth)
                    LaunchedEffect(canPane) { if (!canPane) pane = null }
                    ChatHost(
                        screenContext = chatContext, onOpenBead = onOpenChatBead, onOpenRun = onOpenRun,
                        beadPaneOpen = canPane && pane != null, onCloseBead = { pane = null },
                        beadPane = {
                            pane?.let { current ->
                                BeadDetail(repoKey, current, inPane = true, onClose = { pane = null },
                                    onOpenBead = { pane = it }, onOpenRun = onOpenRun, onOpenRound = onOpenRound,
                                    onOpenChatBead = onOpenChatBead,
                                    onSelectChat = { container.chatStack.select(chatContext, it) })
                            }
                        },
                    ) {
                        BoxWithConstraints(Modifier.fillMaxSize()) {
                        val boardWide = maxWidth >= 600.dp
                        // When the board narrows to one lane at a time, show the open bead's lane.
                        LaunchedEffect(boardWide, pane) {
                            val ix = ui.snapshot.index
                            if (!boardWide && groupBy == GroupBy.WORKFLOW) pane?.let { ix.byId[it] }?.let { scrollTo = ix.workflowLane(it, ui.snapshot.changes) }
                        }
                        Column(Modifier.fillMaxSize()) {
                            if (searching) OutlinedTextField(
                                value = filter.query, onValueChange = vm::setQuery, singleLine = true,
                                placeholder = { Text("Search titles, ids, labels") },
                                shape = RoundedCornerShape(28.dp),
                                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                            )
                            EpicChips(ui, filter, onEpic = vm::setEpic, modifier = Modifier.padding(vertical = 4.dp))
                            FilterSummary(filter, vm::clearFilters)
                            Box(Modifier.fillMaxSize()) {
                                val open: (com.ryderdavid.nacre.data.model.Issue) -> Unit = { i ->
                                    if (canPane) {
                                        container.chatStack.collapse(chatContext)
                                        pane = if (pane == i.id) null else i.id
                                    } else onOpenBead(i.id)
                                }
                                if (boardWide) WideBoard(
                                    ui, groupBy, selectedId = pane, drag = drag, scrollToLane = scrollTo, onScrolled = { scrollTo = null },
                                    onOpen = open, onShowMoreDone = { vm.showAllDone.value = true }, onClose = { vm.move(it, Lane.DONE.key) },
                                ) else CompactBoard(
                                    ui, groupBy, drag = drag, scrollToLane = scrollTo, onScrolled = { scrollTo = null }, query = filter.query,
                                    onOpen = open, onShowMoreDone = { vm.showAllDone.value = true }, onClose = { vm.move(it, Lane.DONE.key) },
                                )
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
        effort = ui.snapshot.changes?.openspec == true,
    )
}

