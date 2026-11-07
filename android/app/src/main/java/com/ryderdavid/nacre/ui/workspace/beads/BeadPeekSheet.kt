package com.ryderdavid.nacre.ui.workspace.beads

import android.content.ClipData
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.CheckBoxOutlineBlank
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.PanTool
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Sync
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.ClipEntry
import androidx.compose.ui.platform.LocalClipboard
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.board.BoardUi
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.rounds.needsGateSetup
import com.ryderdavid.nacre.ui.workspace.LocalAskAbout
import kotlinx.coroutines.launch

/**
 * The bead peek: a bottom sheet that opens part way and expands to full height. Read-only (editing a bead is
 * asked for in chat), with Ask about this, Start work and Open change. It follows the board live.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun BeadPeekSheet(
    repoKey: String,
    issue: Issue,
    ui: BoardUi,
    onDismiss: () -> Unit,
    onMoveTo: (Issue, Lane) -> Unit,
    onOpenBead: (String) -> Unit,
    /** Opens the work inspector for a round or a run, by id, with a label. */
    onInspect: (PeekRun.Kind, String, String) -> Unit,
) {
    val sheet = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    val scope = rememberCoroutineScope()
    val dismiss: () -> Unit = { scope.launch { sheet.hide() }.invokeOnCompletion { onDismiss() } }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheet, containerColor = MaterialTheme.colorScheme.surfaceContainerLow) {
        BeadPeekBody(repoKey, issue, ui, dismiss, onMoveTo, onOpenBead, onInspect, Modifier.fillMaxWidth())
    }
}

/**
 * The bead's details and actions: the peek sheet's content, and the bead viewer's on the board. [dismiss]
 * runs before an action that leaves the bead (a sheet hides itself; the viewer stays).
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun BeadPeekBody(
    repoKey: String,
    issue: Issue,
    ui: BoardUi,
    dismiss: () -> Unit,
    onMoveTo: (Issue, Lane) -> Unit,
    onOpenBead: (String) -> Unit,
    onInspect: (PeekRun.Kind, String, String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val vm = nacreViewModel(key = "beadpeek:$repoKey:${issue.id}") { BeadPeekViewModel(it.repository, repoKey, issue.id) }
    val comments by vm.comments.collectAsState()
    val runs by vm.runs.collectAsState()
    val rounds by vm.rounds.collectAsState()
    val busy by vm.busy.collectAsState()
    // Comments come from the bead's detail: load them again whenever the bead changes on the board.
    LaunchedEffect(vm, issue.updatedAt, issue.commentCount) { vm.load() }
    val peek = remember(issue, ui.snapshot, comments, runs, rounds) {
        BeadPeekModel.build(issue, ui.snapshot.index, ui.snapshot.changes, comments, runs, rounds)
    }
    val ask = LocalAskAbout.current
    val openChange = LocalOpenChange.current
    val repository = LocalContainer.current.repository
    // Starting work in a repository without gates sets them up first, then goes on.
    val start = { vm.startWork { r -> onInspect(PeekRun.Kind.ROUND, r.id, "Round ${r.id}") } }

    run {
        Column(modifier) {
            var showAllComments by androidx.compose.runtime.saveable.rememberSaveable(peek.id) { androidx.compose.runtime.mutableStateOf(false) }
            LazyColumn(
                Modifier.weight(1f, fill = false).fillMaxWidth(),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 20.dp, end = 8.dp, bottom = 12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                item { PeekHeader(peek, issue, onMoveTo) }
                item { PeekPills(peek, onOpenChange = { id -> dismiss(); openChange(id) }) }
                if (peek.description.isNotEmpty()) item { MarkdownText(peek.description, Modifier.padding(end = 12.dp)) }
                if (peek.acceptance.isNotEmpty()) item { Acceptance(peek.acceptance) }
                peek.latestRun?.let { run -> item { LatestRun(run) { onInspect(run.kind, run.id, run.title) } } }
                if (peek.waitsOn.isNotEmpty() || peek.blocks.isNotEmpty() || peek.parent != null) item { Dependencies(peek, onOpenBead) }
                item {
                    Text(
                        if (peek.commentCount == 0) "No comments" else "Comments · ${peek.commentCount}",
                        style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() },
                    )
                }
                if (peek.comments.size > 20 && !showAllComments) item(key = "c:earlier") {
                    androidx.compose.material3.TextButton(onClick = { showAllComments = true }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Show ${peek.comments.size - 20} earlier comments") }
                }
                items(if (showAllComments) peek.comments else peek.comments.takeLast(20), key = { "c:" + it.id }) { c ->
                    Surface(shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.surfaceContainerLowest, modifier = Modifier.fillMaxWidth().padding(end = 12.dp)) {
                        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(listOf(c.author, relativeTime(c.createdAt)).filter { it.isNotBlank() }.joinToString(" · "), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            MarkdownText(c.text)
                        }
                    }
                }
            }
            // The actions: they wrap and stack at large font sizes instead of truncating.
            FlowRow(
                Modifier.fillMaxWidth().navigationBarsPadding().padding(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FilledTonalButton(
                    onClick = { dismiss(); ask(peek.reference) },
                    modifier = Modifier.weight(1f).heightIn(min = 56.dp),
                ) {
                    Icon(Icons.AutoMirrored.Rounded.Chat, null)
                    Spacer(Modifier.size(8.dp))
                    Text("Ask about this")
                }
                peek.startWork?.let { start ->
                    OutlinedButton(
                        onClick = {
                            val round = start.opensRound
                            if (round != null) onInspect(PeekRun.Kind.ROUND, round, "Round $round")
                            // No gates yet: setting them up is a conversation, so it moves to Chat with this bead attached.
                            else if (needsGateSetup(repository.repo(repoKey))) {
                                dismiss()
                                ask(peek.reference.copy(text = GATES_FIRST.format(issue.id)))
                            }
                            else start()
                        },
                        enabled = !busy,
                        modifier = Modifier.heightIn(min = 56.dp).semantics { contentDescription = "${start.label}. ${start.note}" },
                    ) {
                        Icon(Icons.Rounded.PlayArrow, null)
                        Spacer(Modifier.size(8.dp))
                        Column {
                            Text(if (busy) "Starting…" else start.label)
                            Text(start.note, style = MaterialTheme.typography.labelSmall)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun PeekHeader(peek: BeadPeek, issue: Issue, onMoveTo: (Issue, Lane) -> Unit) {
    var menu by remember { mutableStateOf(false) }
    val clipboard = LocalClipboard.current
    val scope = rememberCoroutineScope()
    Row(verticalAlignment = Alignment.Top) {
        Column(Modifier.weight(1f).padding(top = 6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            BeadId(peek.id)
            Text(peek.title, style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
        }
        Box {
            IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "More for ${peek.id}") }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                if (BeadMoves.canMove(peek.lane)) DropdownMenuItem(text = { Text("Move to…") }, onClick = { menu = false; onMoveTo(issue, peek.lane) })
                DropdownMenuItem(text = { Text("Copy id") }, onClick = {
                    menu = false
                    scope.launch { clipboard.setClipEntry(ClipEntry(ClipData.newPlainText("bead id", peek.id))) }
                })
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun PeekPills(peek: BeadPeek, onOpenChange: (String) -> Unit) {
    val colors = MaterialTheme.colorScheme
    FlowRow(Modifier.padding(end = 12.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp), itemVerticalAlignment = Alignment.CenterVertically) {
        Pill(peek.lane.title, colors.primaryContainer, colors.onPrimaryContainer)
        if (peek.awaitsSignoff) Pill("Waiting for your sign-off", colors.tertiaryContainer, colors.onTertiaryContainer)
        else if (peek.needsYou) Pill("Needs you", colors.errorContainer, colors.onErrorContainer)
        PriorityTag(peek.priority)
        Pill(peek.type, colors.surfaceContainerHighest, colors.onSurfaceVariant)
        peek.change?.let { change ->
            Surface(
                onClick = { onOpenChange(change.id) }, shape = RoundedCornerShape(12.dp),
                color = colors.surfaceContainerHighest, contentColor = colors.onSurfaceVariant,
                modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "Open change ${change.label}" },
            ) {
                Row(Modifier.padding(horizontal = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    Icon(Icons.Rounded.Description, null, Modifier.size(16.dp))
                    Text(change.label, style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}

@Composable
private fun Pill(text: String, color: Color, content: Color) {
    Surface(shape = RoundedCornerShape(12.dp), color = color, contentColor = content) {
        Text(text, style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp))
    }
}

@Composable
private fun Acceptance(items: List<String>) {
    Column(Modifier.padding(end = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text("Acceptance", style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
        items.forEach { line ->
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(Icons.Rounded.CheckBoxOutlineBlank, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.outline)
                Text(line, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
private fun LatestRun(run: PeekRun, onOpen: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    val (icon, tint) = when (run.state) {
        PeekRunState.RUNNING -> Icons.Rounded.Sync to colors.primary
        PeekRunState.NEEDS_YOU -> Icons.Rounded.PanTool to colors.error
        PeekRunState.FAILED -> Icons.Rounded.Error to colors.error
        PeekRunState.DONE -> Icons.Rounded.TaskAlt to colors.onSurfaceVariant
    }
    Surface(
        onClick = onOpen, shape = RoundedCornerShape(20.dp), color = colors.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth().padding(end = 12.dp).heightIn(min = 56.dp)
            .semantics { contentDescription = "Latest run: ${run.title}. ${run.state.text}. ${run.detail}. Open the work inspector" },
    ) {
        Row(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Icon(icon, null, tint = tint)
            Column(Modifier.weight(1f)) {
                Text(run.title, style = MaterialTheme.typography.labelLarge)
                Text(
                    listOf(run.state.text, relativeTime(run.at), run.detail).filter { it.isNotBlank() }.joinToString(" · "),
                    style = MaterialTheme.typography.bodySmall, color = colors.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis,
                )
            }
            Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, tint = colors.onSurfaceVariant)
        }
    }
}

@Composable
private fun Dependencies(peek: BeadPeek, onOpenBead: (String) -> Unit) {
    Column(Modifier.padding(end = 12.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text("Dependencies", style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
        peek.parent?.let { DepRow("Part of", it, onOpenBead) }
        peek.waitsOn.forEach { DepRow("Waits on", it, onOpenBead) }
        peek.blocks.forEach { DepRow("Blocks", it, onOpenBead) }
    }
}

@Composable
private fun DepRow(relation: String, dep: PeekDependency, onOpenBead: (String) -> Unit) {
    Surface(
        onClick = { onOpenBead(dep.id) }, enabled = dep.known, color = Color.Transparent, shape = RoundedCornerShape(12.dp),
        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
    ) {
        Row(Modifier.padding(vertical = 6.dp, horizontal = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Column(Modifier.weight(1f)) {
                Text("$relation ${dep.id} · ${dep.stateText}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (dep.title.isNotBlank()) Text(dep.title, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/** What Chat is asked when work can't start because the project has no gates yet. */
internal const val GATES_FIRST =
    "This project has no gates in nacre.yaml yet, so %s can't start. Propose gates for it (build, tests, lint), and once I agree, set them up."
