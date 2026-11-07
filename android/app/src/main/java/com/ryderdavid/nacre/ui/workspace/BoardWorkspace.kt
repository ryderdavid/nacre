package com.ryderdavid.nacre.ui.workspace

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Chat
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.SearchOff
import androidx.compose.material.icons.rounded.WarningAmber
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.IconToggleButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.board.BoardViewModel
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.workspace.beads.AttachButton
import com.ryderdavid.nacre.ui.workspace.beads.BeadMoves
import com.ryderdavid.nacre.ui.workspace.beads.BeadPeekBody
import com.ryderdavid.nacre.ui.workspace.beads.BeadsSlot
import com.ryderdavid.nacre.ui.workspace.beads.CardAttach
import com.ryderdavid.nacre.ui.workspace.beads.LocalCardAttach
import com.ryderdavid.nacre.ui.workspace.beads.MoveOutcome
import com.ryderdavid.nacre.ui.workspace.beads.MoveToDialog
import com.ryderdavid.nacre.ui.workspace.beads.beadReference
import com.ryderdavid.nacre.ui.workspace.beads.inspectorEvidence
import com.ryderdavid.nacre.ui.workspace.spec.ChangeRow
import com.ryderdavid.nacre.ui.workspace.spec.ProgressTrack
import com.ryderdavid.nacre.ui.workspace.spec.SpecGroup
import com.ryderdavid.nacre.ui.workspace.spec.SpecList
import com.ryderdavid.nacre.ui.workspace.spec.SpecPanelViewModel
import com.ryderdavid.nacre.ui.workspace.spec.SpecScreen
import com.ryderdavid.nacre.ui.workspace.spec.failedBeads

/** The width from which the board is the whole app: tablets and the unfolded phone (about 790dp). */
const val BOARD_MIN_WIDTH = 700

/** The chat drawer's width beside the board and the viewers: 38% of the window, 320–420dp. */
private fun chatWidth(windowDp: Int) = (windowDp * 38 / 100).coerceIn(320, 420)

/** The Specs column, a little wider than a bead lane. */
private const val SPECS_WIDTH = 300

private const val BOARD = "board"
private const val SPEC = "spec"
private const val BEAD = "bead:"

/**
 * The board is the app on wide screens: the Specs column and the bead lanes, with the chat drawer on the
 * right. Tapping a card opens its viewer in the board's place (Back returns); each card's Send to chat
 * attaches it to the next message. Opening something never attaches it: only Send to chat does.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun BoardWorkspace(
    repoKey: String,
    projectName: String,
    thread: String?,
    work: WorkSummary?,
    needsYou: Int,
    chat: @Composable (Modifier) -> Unit,
    onSwitchProject: () -> Unit,
    onWork: (WorkSummary) -> Unit,
    onQueue: () -> Unit,
    askAbout: (ChatReference) -> Unit,
    /** Shows a chat in the drawer: picked from Jobs. */
    onThread: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val container = LocalContainer.current
    val refs = container.chats.references
    var view by rememberSaveable(repoKey) { mutableStateOf(BOARD) }
    var chatOpen by rememberSaveable { mutableStateOf(true) }
    var jobsTab by rememberSaveable { mutableStateOf(false) }
    // The drawer opens on the chats as cards; a picked chat unfolds into the conversation.
    var unfolded by rememberSaveable(repoKey) { mutableStateOf(false) }
    val specVm = nacreViewModel(key = "spec:$repoKey") { SpecPanelViewModel(it.repository, repoKey) }
    val board by container.repository.board(repoKey).collectAsState()
    val specStack by specVm.stack.collectAsState()

    // Links from chat, decision cards and notifications open the viewers.
    val specTargets by refs.specTarget.collectAsState()
    val boardLoaded = board.data != null
    LaunchedEffect(specTargets[repoKey], boardLoaded) {
        val t = specTargets[repoKey] ?: return@LaunchedEffect
        if (!boardLoaded && board.loading) return@LaunchedEffect
        specVm.show(SpecList.opening(t.ref, t.requirement, board.data?.changes?.forSpec.orEmpty()))
        refs.specOpened(repoKey)
        view = SPEC
    }
    val beadTargets by refs.beadTarget.collectAsState()
    LaunchedEffect(beadTargets[repoKey]) {
        beadTargets[repoKey]?.let { view = BEAD + it; refs.beadOpened(repoKey) }
    }
    QueueWatcher(repoKey, onQueue)
    // The board has no Viewing context: a pane's leftover one would ride along unasked.
    LaunchedEffect(repoKey) { refs.view(repoKey, null) }

    val attachedAll by refs.attachedAll.collectAsState()
    val attached = thread?.let { attachedAll[it] }.orEmpty()
    val attach = CardAttach(attached.map { "${it.kind}:${it.id}" }.toSet()) { ref ->
        val t = thread
        val existing = attached.firstOrNull { it.kind == ref.kind && it.id == ref.id }
        if (t != null && existing != null) refs.remove(t, existing) else { chatOpen = true; jobsTab = false; unfolded = true; askAbout(ref) }
    }

    val here = when {
        view == SPEC -> (specStack.lastOrNull { it is SpecScreen.Change } as? SpecScreen.Change)?.let { c ->
            board.data?.changes?.forSpec?.firstOrNull { it.id == c.changeId }?.name?.ifBlank { null } ?: c.changeId
        } ?: "Spec"
        view.startsWith(BEAD) -> view.removePrefix(BEAD)
        else -> null
    }

    androidx.activity.compose.BackHandler(enabled = chatOpen && unfolded && !jobsTab && view == BOARD) { unfolded = false }

    CompositionLocalProvider(LocalCardAttach provides attach, LocalViewing provides {}) {
        Column(modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface)) {
            BoardTopBar(
                projectName, here, work, needsYou, chatOpen,
                onSwitchProject = onSwitchProject, onBoard = { view = BOARD }, onWork = onWork, onQueue = onQueue,
                onChat = { chatOpen = !chatOpen },
            )
            Row(Modifier.weight(1f).fillMaxWidth()) {
                Box(Modifier.weight(1f).fillMaxHeight()) {
                    when {
                        view == SPEC -> SpecSlot(repoKey, Modifier.fillMaxSize(), onList = { view = BOARD })
                        view.startsWith(BEAD) -> BeadViewer(repoKey, view.removePrefix(BEAD), onClose = { view = BOARD }, onOpenBead = { view = BEAD + it })
                        else -> BeadsSlot(
                            repoKey, Modifier.fillMaxSize(),
                            onOpenBead = { view = BEAD + it },
                            leading = { SpecsColumn(repoKey, onOpen = { id -> specVm.openChange(id); view = SPEC }) },
                        )
                    }
                }
                if (chatOpen) Surface(
                    shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerLow,
                    modifier = Modifier.width(chatWidth(androidx.compose.ui.platform.LocalConfiguration.current.screenWidthDp).dp).fillMaxHeight().padding(end = 8.dp, bottom = 8.dp),
                ) {
                    com.ryderdavid.nacre.ui.workspace.chat.ChatDrawer(
                        repoKey, thread, jobs = jobsTab, onJobs = { jobsTab = it }, unfolded = unfolded, onUnfold = { unfolded = it },
                        onThread = onThread, onInspect = onWork, chat = chat, modifier = Modifier.fillMaxSize(),
                    )
                }
            }
        }
    }
}

/** The project, where the board is, the project's work and Needs you, and the chat drawer's toggle. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun BoardTopBar(
    projectName: String,
    here: String?,
    work: WorkSummary?,
    needsYou: Int,
    chatOpen: Boolean,
    onSwitchProject: () -> Unit,
    onBoard: () -> Unit,
    onWork: (WorkSummary) -> Unit,
    onQueue: () -> Unit,
    onChat: () -> Unit,
) {
    val c = MaterialTheme.colorScheme
    Row(
        Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(horizontal = 12.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Surface(
            onClick = onSwitchProject, shape = RoundedCornerShape(24.dp), color = c.surfaceContainerHigh,
            modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "$projectName. Switch project" },
        ) {
            Row(Modifier.padding(start = 6.dp, end = 16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Surface(shape = projectAvatarShape(projectName), color = c.primary, contentColor = c.onPrimary, modifier = Modifier.size(36.dp)) {
                    Box(contentAlignment = Alignment.Center) { Text(projectName.take(1).uppercase(), style = MaterialTheme.typography.titleMedium) }
                }
                Text(projectName, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.widthIn(max = 220.dp))
            }
        }
        if (here == null) {
            Text("Board", style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.padding(start = 8.dp).semantics { heading() })
        } else {
            androidx.compose.material3.TextButton(onClick = onBoard, modifier = Modifier.semantics { contentDescription = "Back to the board" }) {
                Text("Board", style = MaterialTheme.typography.titleMedium)
            }
            Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, tint = c.onSurfaceVariant)
            Text(here, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false).semantics { heading() })
        }
        Spacer(Modifier.weight(1f))
        if (work != null) Surface(
            onClick = { onWork(work) }, shape = RoundedCornerShape(20.dp), color = c.surfaceContainerHigh,
            modifier = Modifier.heightIn(min = 48.dp).widthIn(max = 320.dp).semantics { contentDescription = "${ChatFirstData.statusText(work)}. Open the work inspector" },
        ) {
            Row(Modifier.padding(horizontal = 12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (work.running && !work.stalled) LoadingIndicator(Modifier.size(24.dp))
                else Icon(Icons.Rounded.WarningAmber, null, tint = if (work.stopped || work.stalled) c.error else c.onSurfaceVariant)
                Text(ChatFirstData.statusText(work), style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
        if (needsYou > 0) Surface(
            onClick = onQueue, shape = RoundedCornerShape(20.dp), color = c.errorContainer, contentColor = c.onErrorContainer,
            modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "$needsYou need you: open the queue" },
        ) {
            Box(Modifier.padding(horizontal = 14.dp), contentAlignment = Alignment.Center) {
                Text("$needsYou need${if (needsYou == 1) "s" else ""} you", style = MaterialTheme.typography.labelLarge)
            }
        }
        IconToggleButton(checked = chatOpen, onCheckedChange = { onChat() }, modifier = Modifier.semantics { contentDescription = if (chatOpen) "Hide chat" else "Show chat" }) {
            Icon(if (chatOpen) Icons.AutoMirrored.Rounded.Chat else Icons.AutoMirrored.Outlined.Chat, null)
        }
    }
}

/** The board's first column: the project's open changes, each with its progress toward being archived. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun SpecsColumn(repoKey: String, onOpen: (String) -> Unit) {
    val repo = LocalContainer.current.repository
    val board by repo.board(repoKey).collectAsState()
    val rounds by repo.rounds.collectAsState()
    val changes = board.data?.changes?.forSpec.orEmpty()
    val failed = remember(rounds) { failedBeads(rounds, repoKey) }
    val rows = remember(changes, failed) { SpecList.build(changes, failedBeads = failed).rows }
    val c = MaterialTheme.colorScheme
    Surface(shape = RoundedCornerShape(28.dp), color = c.tertiaryContainer.copy(alpha = 0.55f), modifier = Modifier.width(SPECS_WIDTH.dp).fillMaxHeight()) {
        Column {
            Row(Modifier.fillMaxWidth().padding(start = 18.dp, end = 12.dp, top = 14.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("Specs", style = MaterialTheme.typography.titleMediumEmphasized, color = c.onTertiaryContainer, modifier = Modifier.weight(1f).semantics { heading() })
                Surface(shape = RoundedCornerShape(12.dp), color = c.tertiaryContainer, contentColor = c.onTertiaryContainer) {
                    Text("${rows.size}", style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 2.dp))
                }
            }
            when {
                board.data == null -> LoadingState("Loading changes…")
                rows.isEmpty() -> Text(
                    "No open changes. Ask in chat to propose one.",
                    style = MaterialTheme.typography.bodyMedium, color = c.onTertiaryContainer, modifier = Modifier.padding(18.dp),
                )
                else -> LazyColumn(
                    contentPadding = PaddingValues(start = 8.dp, end = 8.dp, bottom = 96.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(rows, key = { it.id }) { row -> SpecCard(row, changes.firstOrNull { it.id == row.id }, onOpen = { onOpen(row.id) }) }
                }
            }
        }
    }
}

/** A change on the board: its name, what it's doing, and a bar toward archive (tasks, else sections). */
@Composable
private fun SpecCard(row: ChangeRow, change: ChangeSummary?, onOpen: () -> Unit) {
    val c = MaterialTheme.colorScheme
    val done = change?.tasksDone ?: 0
    val total = change?.tasksTotal ?: 0
    val archivable = row.group == SpecGroup.READY_TO_ARCHIVE || (total > 0 && done >= total)
    val waiting = row.group == SpecGroup.WAITING
    val attach = LocalCardAttach.current
    Surface(
        onClick = onOpen, shape = RoundedCornerShape(24.dp),
        color = if (waiting) c.tertiaryContainer else c.surfaceContainerLowest,
        contentColor = if (waiting) c.onTertiaryContainer else c.onSurface,
        modifier = Modifier.fillMaxWidth().semantics { contentDescription = listOf(row.name, row.group.label, row.status).joinToString(". ") },
    ) {
        Column(Modifier.padding(start = 18.dp, end = 4.dp, top = 6.dp, bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (archivable) "Ready to archive" else row.group.label,
                    style = MaterialTheme.typography.labelMedium, color = if (archivable) c.primary else c.onSurfaceVariant, modifier = Modifier.weight(1f),
                )
                if (archivable) Icon(Icons.Rounded.CheckCircle, null, tint = c.primary, modifier = Modifier.size(18.dp))
                if (attach != null) AttachButton(ChatReference("change", change?.name?.ifBlank { null } ?: row.id, row.name), attach)
                else Spacer(Modifier.size(48.dp))
            }
            Column(Modifier.padding(end = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(row.name, style = MaterialTheme.typography.titleSmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
                when {
                    total > 0 -> {
                        LinearProgressIndicator(progress = { done.toFloat() / total }, modifier = Modifier.fillMaxWidth())
                        Text("$done of $total tasks", style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant)
                    }
                    row.progress != null -> {
                        ProgressTrack(row.progress)
                        Text(row.progress.headline, style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant)
                    }
                }
                Text(row.status, style = MaterialTheme.typography.bodySmall, color = if (waiting) c.onTertiaryContainer else c.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/** A bead in the board's place: its details, comments and actions, with Back to the board. */
@Composable
private fun BeadViewer(repoKey: String, id: String, onClose: () -> Unit, onOpenBead: (String) -> Unit) {
    val repo = LocalContainer.current.repository
    val vm = nacreViewModel(key = "board:$repoKey") { BoardViewModel(it.repository, repoKey) }
    val state by vm.state.collectAsState()
    val ui = state.data
    val issue = ui?.snapshot?.index?.byId?.get(id)
    val openEvidence = LocalOpenEvidence.current
    val attach = LocalCardAttach.current
    var moving by remember { mutableStateOf<Pair<Issue, Lane>?>(null) }
    BackHandler(onBack = onClose)

    Column(Modifier.fillMaxSize()) {
        Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.TopCenter) {
            when {
                ui == null -> LoadingState("Loading the bead…")
                issue == null -> MessageState(Icons.Rounded.SearchOff, "$id isn't on the board", "It may have been closed long ago or removed.", action = "Back to the board", onAction = onClose)
                else -> Box(Modifier.widthIn(max = 880.dp).fillMaxSize()) {
                    BeadPeekBody(
                        repoKey, issue, ui, dismiss = {},
                        onMoveTo = { i, from -> moving = i to from },
                        onOpenBead = onOpenBead,
                        onInspect = { kind, rid, label -> openEvidence(inspectorEvidence(kind, rid, label)) },
                        modifier = Modifier.fillMaxSize().padding(top = 8.dp),
                    )
                    if (attach != null) AttachButton(beadReference(id), attach, Modifier.align(Alignment.TopEnd).padding(top = 8.dp, end = 56.dp))
                }
            }
        }
    }

    moving?.let { (issue, from) ->
        MoveToDialog(issue, from, onDismiss = { moving = null }) { to ->
            moving = null
            when (val o = BeadMoves.outcome(from, to)) {
                is MoveOutcome.Refused -> repo.notify(o.reason)
                MoveOutcome.Unchanged -> Unit
                is MoveOutcome.Moves -> repo.move(repoKey, issue, o.action)
            }
        }
    }
}

