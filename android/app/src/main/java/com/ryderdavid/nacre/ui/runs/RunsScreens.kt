package com.ryderdavid.nacre.ui.runs

import android.content.ClipData
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Block
import androidx.compose.material.icons.rounded.Checklist
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.HourglassDisabled
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.Hub
import androidx.compose.material.icons.rounded.Public
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.SmartToy
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material.icons.rounded.Build
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.VerticalDivider
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.ClipEntry
import androidx.compose.ui.platform.LocalClipboard
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.ACTIVITY_ENDED
import com.ryderdavid.nacre.data.model.ACTIVITY_IDLE
import com.ryderdavid.nacre.data.model.ACTIVITY_WORKING
import com.ryderdavid.nacre.data.model.EFFORT_LABELS
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.RunEvent
import com.ryderdavid.nacre.notify.RunNotifier
import com.ryderdavid.nacre.ui.bead.RUN_MODES
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.formatDuration
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.common.sendOnHardwareEnter
import com.ryderdavid.nacre.ui.rounds.RoundRow
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

@Composable
fun RunStatusIcon(status: String, modifier: Modifier = Modifier, stalled: Boolean = false) {
    if (status == "running" && stalled) {
        Icon(Icons.Rounded.HourglassDisabled, "May be stuck", tint = Nacre.colors.needsYou, modifier = modifier)
        return
    }
    when (status) {
        "running" -> {
            val t = rememberInfiniteTransition(label = "run")
            val a by t.animateFloat(0.4f, 1f, infiniteRepeatable(tween(800), RepeatMode.Reverse), label = "a")
            Icon(Icons.Rounded.Terminal, "Running", tint = Nacre.colors.running, modifier = modifier.alpha(a))
        }
        "queued" -> Icon(Icons.Rounded.Schedule, "Queued", tint = MaterialTheme.colorScheme.outline, modifier = modifier)
        "succeeded" -> Icon(Icons.Rounded.CheckCircle, "Succeeded", tint = Nacre.colors.doing, modifier = modifier)
        "cancelled" -> Icon(Icons.Rounded.Block, "Cancelled", tint = MaterialTheme.colorScheme.outline, modifier = modifier)
        else -> Icon(Icons.Rounded.Error, "Failed", tint = MaterialTheme.colorScheme.error, modifier = modifier)
    }
}

/** How an adopted session is doing, in the app's words; null for a run Nacre dispatched. */
internal fun activityLabel(activity: String): String? = when (activity) {
    ACTIVITY_WORKING -> "working"
    ACTIVITY_IDLE -> "waiting for input"
    ACTIVITY_ENDED -> "session ended"
    else -> null
}

/** The line shown under a transcript that is still going. */
internal fun workingLabel(r: Run): String = when {
    r.status == "queued" -> "Waiting for a free slot on the Mac…"
    r.stalled -> "${r.agentName} hasn't printed anything since ${relativeTime(r.stalledSince).ifBlank { "a while ago" }}, so it may be stuck. Wait, or cancel it and send a follow-up to carry on."
    r.isAdopted && r.activity == ACTIVITY_IDLE -> "The agent's session is waiting for input on the Mac…"
    r.isAdopted -> "An agent is working in this session…"
    else -> "${r.agentName} is working…"
}

/** What to call a run: an adopted session may have moved no bead, and then has only its own name. */
internal fun runTitle(r: Run): String = r.issueTitle.ifBlank { r.sessionName.ifBlank { if (r.isAdopted) "Agent session" else "Run" } }

/** Marks a run nobody started from Nacre: an agent's own session, which the Mac only watches. */
@Composable
fun AgentChip(r: Run, modifier: Modifier = Modifier) {
    Surface(color = MaterialTheme.colorScheme.tertiaryContainer, shape = RoundedCornerShape(8.dp), modifier = modifier) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 3.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.SmartToy, null, Modifier.size(14.dp))
            Spacer(Modifier.width(4.dp))
            Text(listOfNotNull("Agent", activityLabel(r.activity)).joinToString(" · "), style = MaterialTheme.typography.labelSmall)
        }
    }
}

/** Live transcript of one run, by Claude Code or Codex. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun RunDetail(runKey: String, inPane: Boolean, onBack: () -> Unit, onOpenBead: (repoKey: String, beadId: String) -> Unit) {
    val vm = nacreViewModel(key = "run:$runKey") { RunViewModel(it.repository, runKey) }
    val run by vm.run.collectAsState()
    val entries by vm.transcript.collectAsState()
    run?.let { r -> if (!inPane) com.ryderdavid.nacre.ui.chat.PublishScreen(com.ryderdavid.nacre.ui.ScreenOrigin.Run(runKey), com.ryderdavid.nacre.data.chat.ScreenSummaries.run(r)) }
    val error by vm.error.collectAsState()
    val sending by vm.sending.collectAsState()
    val list = rememberLazyListState()
    var follow by remember { mutableStateOf(true) }
    val posture = currentWindowAdaptiveInfo().windowPosture
    val density = LocalDensity.current

    LaunchedEffect(entries.size) {
        if (follow && entries.isNotEmpty()) list.animateScrollToItem(entries.size)
    }
    LaunchedEffect(list.isScrollInProgress) {
        if (list.isScrollInProgress) follow = !list.canScrollForward || list.layoutInfo.visibleItemsInfo.lastOrNull()?.index == list.layoutInfo.totalItemsCount - 1
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = { if (!inPane) IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back") } },
                title = {
                    Column {
                        // An adopted session that moved no bead has no id to show, only its own name.
                        Text(run?.issueId?.ifBlank { null } ?: "Run", style = MaterialTheme.typography.titleMedium.copy(fontFamily = MonoFamily))
                        Text(
                            run?.let { it.issueTitle.ifBlank { it.sessionName } }.orEmpty(),
                            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1, overflow = TextOverflow.Ellipsis,
                        )
                    }
                },
                actions = {
                    run?.let { r ->
                        if (r.repoId.isNotBlank() && r.issueId.isNotBlank()) {
                            IconButton(onClick = { onOpenBead(r.repoKey, r.issueId) }) { Icon(Icons.Rounded.Description, "Open bead") }
                        }
                    }
                },
            )
        },
        bottomBar = {
            val r = run
            if (r != null) {
                // On a half-folded phone lying flat, keep the controls on the lower half.
                val tabletopMin = posture.hingeList.firstOrNull { !it.isVertical && it.isSeparating }?.let { with(density) { it.bounds.top.toDp() } }
                Column(Modifier.then(if (posture.isTabletop && tabletopMin != null) Modifier.heightIn(min = 240.dp) else Modifier)) {
                    if (posture.isTabletop) RunHeader(r, Modifier.padding(horizontal = 16.dp))
                    RunControls(r, sending, onCancel = vm::cancel, onFollowUp = vm::followUp)
                }
            }
        },
    ) { pad ->
        val r = run
        if (r == null && error == null) return@Scaffold LoadingState(modifier = Modifier.padding(pad))
        LazyColumn(
            state = list,
            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
            modifier = Modifier.padding(pad).fillMaxSize(),
        ) {
            if (r != null && !posture.isTabletop) item("header") { RunHeader(r) }
            if (error != null) item("err") {
                Text(error!!, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            items(entries, key = { it.key }) { e ->
                when (e) {
                    is Entry.Tool -> ToolEntry(e)
                    is Entry.Single -> EventEntry(e.event)
                }
            }
            if (r?.isActive == true) item("working") {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    // A parked agent session is alive but doing nothing; don't pretend it is thinking.
                    if (!(r.isAdopted && r.activity == ACTIVITY_IDLE) && !r.stalled) {
                        LoadingIndicator(Modifier.size(32.dp))
                        Spacer(Modifier.width(8.dp))
                    }
                    Text(workingLabel(r), color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

@Composable
private fun RunHeader(r: Run, modifier: Modifier = Modifier) {
    val clipboard = LocalClipboard.current
    val scope = rememberCoroutineScope()
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer), shape = RoundedCornerShape(20.dp), modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                RunStatusIcon(r.status, stalled = r.stalled)
                Spacer(Modifier.width(8.dp))
                Text(r.status.replaceFirstChar { it.uppercase() }, style = MaterialTheme.typography.titleMedium)
                if (r.isAdopted) AgentChip(r, Modifier.padding(start = 8.dp))
                Spacer(Modifier.weight(1f))
                Text(listOfNotNull(r.hostName.ifBlank { null }, r.repoName).joinToString(" · "), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text(
                listOfNotNull(
                    "Codex".takeIf { r.isCodex && !r.isAdopted },
                    RUN_MODES.firstOrNull { it.first == r.mode }?.second?.let { "$it mode" },
                    r.model.ifBlank { null },
                    EFFORT_LABELS[r.effort]?.let { "$it effort" },
                    if (r.worktree) "worktree" else null,
                    r.durationMs.takeIf { it > 0 }?.let { formatDuration(it) },
                    RunNotifier.cost(r),
                    r.toolCalls.takeIf { it > 0 }?.let { "$it tool calls" },
                    r.turns.takeIf { it > 1 }?.let { "$it prompts" },
                    r.sessionName.takeIf { r.isAdopted && it.isNotBlank() },
                    r.ownerPid.takeIf { it > 0 }?.let { "pid $it" },
                ).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp),
            )
            if (r.error.isNotBlank()) Text(r.error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 6.dp))
            if (r.isAdopted) Text(
                "An agent started this session on the Mac. Nacre is watching it, not running it, so it can't be steered or stopped from here.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 6.dp),
            )
            if (r.isAdopted) LiveMessages(r)
            r.takeoverCommand?.let { command ->
                val label = if (r.isAdopted) "Copy “take over on Mac” command" else "Copy “resume on Mac” command"
                FilledTonalButton(
                    onClick = { scope.launch { clipboard.setClipEntry(ClipEntry(ClipData.newPlainText(if (r.isAdopted) "take over" else "resume", command))) } },
                    modifier = Modifier.padding(top = 10.dp),
                ) {
                    Icon(Icons.Rounded.ContentCopy, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp)); Text(label)
                }
            }
        }
    }
}

@Composable
private fun RunControls(r: Run, sending: Boolean, onCancel: () -> Unit, onFollowUp: (String, String?) -> Unit) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.navigationBarsPadding().imePadding().padding(12.dp)) {
            if (r.isActive) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        r.lastText.ifBlank { workingLabel(r) }, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                    )
                    // Cancel is hidden, not disabled, when the Mac refuses it: there is no process of ours to stop.
                    if (r.showsCancel) {
                        Spacer(Modifier.width(12.dp))
                        OutlinedButton(onClick = onCancel) { Icon(Icons.Rounded.Stop, null, Modifier.size(18.dp)); Spacer(Modifier.width(6.dp)); Text("Cancel") }
                    }
                }
                if (r.isAdopted) ReadOnlyNote()
            } else if (r.showsFollowUp) {
                var text by rememberSaveable(r.key) { mutableStateOf("") }
                var mode by rememberSaveable(r.key) { mutableStateOf(r.mode.ifBlank { "auto" }) }
                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    RUN_MODES.forEach { (key, label, _) -> FilterChip(selected = mode == key, onClick = { mode = key }, label = { Text(label) }) }
                }
                Row(verticalAlignment = Alignment.Bottom, modifier = Modifier.padding(top = 6.dp)) {
                    val canSend = text.isNotBlank() && !sending
                    val send = { onFollowUp(text.trim(), mode); text = "" }
                    OutlinedTextField(
                        text, { text = it }, placeholder = { Text("Follow up with ${r.agentName}") }, maxLines = 5,
                        shape = RoundedCornerShape(24.dp), modifier = Modifier.weight(1f).sendOnHardwareEnter(canSend, send),
                    )
                    Spacer(Modifier.width(8.dp))
                    FilledIconButton(onClick = send, enabled = canSend, modifier = Modifier.size(52.dp)) {
                        Icon(Icons.AutoMirrored.Rounded.Send, "Send follow-up")
                    }
                }
            } else if (r.isAdopted) {
                ReadOnlyNote()
            }
        }
    }
}

/** The live messages sent to this session and where each one stands. */
@Composable
private fun LiveMessages(r: Run) {
    if (r.peerMessages.isEmpty()) return
    Column(Modifier.padding(top = 10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("Sent to the live session", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        r.peerMessages.takeLast(3).forEach { pm ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    when {
                        pm.confirmed -> Icons.Rounded.CheckCircle
                        pm.failed -> Icons.Rounded.Error
                        else -> Icons.AutoMirrored.Rounded.Send
                    },
                    null, Modifier.size(16.dp),
                    tint = if (pm.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
                )
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(pm.text, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text(
                        listOfNotNull(pm.label, pm.reason.ifBlank { null }).joinToString(" · "),
                        style = MaterialTheme.typography.labelSmall,
                        color = if (pm.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/** Why an adopted run has no controls, and what to do instead. */
@Composable
private fun ReadOnlyNote() {
    Text(
        "Read-only: copy the take-over command above to pick this session up on the Mac.",
        style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline,
        modifier = Modifier.fillMaxWidth().padding(top = 6.dp), textAlign = TextAlign.Center,
    )
}

private fun toolIcon(tool: String): ImageVector = when (tool) {
    "Bash", "BashOutput" -> Icons.Rounded.Terminal
    "Edit", "MultiEdit", "Write", "NotebookEdit" -> Icons.Rounded.Edit
    "Read" -> Icons.Rounded.Description
    "Grep", "Glob", "LS" -> Icons.Rounded.Search
    "Task", "Agent" -> Icons.Rounded.Hub
    "WebFetch", "WebSearch" -> Icons.Rounded.Public
    "TodoWrite", "TaskCreate", "TaskUpdate" -> Icons.Rounded.Checklist
    else -> Icons.Rounded.Build
}

private val pretty = Json { prettyPrint = true }

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun ToolEntry(e: Entry.Tool) {
    var open by remember { mutableStateOf(false) }
    val failed = e.result?.isError == true
    Surface(
        onClick = { open = !open },
        shape = RoundedCornerShape(14.dp),
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        modifier = Modifier.fillMaxWidth().animateContentSize(),
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(toolIcon(e.use.tool), null, Modifier.size(16.dp), tint = if (failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary)
                Spacer(Modifier.width(8.dp))
                Text(e.use.tool, style = MaterialTheme.typography.labelLarge)
                Spacer(Modifier.width(8.dp))
                Text(
                    e.use.text, style = MaterialTheme.typography.bodySmall.copy(fontFamily = MonoFamily),
                    color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = if (open) 6 else 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                )
                if (e.result == null) LoadingIndicator(Modifier.size(20.dp))
            }
            if (open) {
                e.use.input?.let { input ->
                    CodeBlock(runCatching { pretty.encodeToString(JsonElement.serializer(), input) }.getOrDefault(input.toString()), Modifier.padding(top = 8.dp))
                }
                e.result?.let { res ->
                    Text(if (res.isError) "Error" else "Result", style = MaterialTheme.typography.labelSmall, color = if (res.isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp))
                    CodeBlock(res.text.ifBlank { "(no output)" })
                }
            }
        }
    }
}

@Composable
private fun CodeBlock(text: String, modifier: Modifier = Modifier) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHighest, shape = RoundedCornerShape(10.dp), modifier = modifier.fillMaxWidth()) {
        Text(
            text.take(6000), style = MaterialTheme.typography.bodySmall.copy(fontFamily = MonoFamily),
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(10.dp),
        )
    }
}

@Composable
private fun EventEntry(e: RunEvent) {
    when (e.kind) {
        "user" -> {
            var open by remember { mutableStateOf(false) }
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterEnd) {
                Surface(
                    onClick = { open = !open },
                    color = MaterialTheme.colorScheme.primaryContainer,
                    shape = RoundedCornerShape(20.dp, 20.dp, 4.dp, 20.dp),
                    modifier = Modifier.widthIn(max = 560.dp).animateContentSize(),
                ) {
                    Text(
                        e.text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onPrimaryContainer,
                        maxLines = if (open) Int.MAX_VALUE else 6, overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                    )
                }
            }
        }
        "text" -> MarkdownText(e.text, Modifier.fillMaxWidth())
        "thinking" -> Text(e.text, style = MaterialTheme.typography.bodySmall.copy(fontStyle = FontStyle.Italic), color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 4, overflow = TextOverflow.Ellipsis)
        "system" -> Text(e.text, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth())
        "stderr" -> Text(e.text, style = MaterialTheme.typography.bodySmall.copy(fontFamily = MonoFamily), color = MaterialTheme.colorScheme.error, maxLines = 3, overflow = TextOverflow.Ellipsis)
        "result" -> Surface(
            color = if (e.isError) MaterialTheme.colorScheme.errorContainer else Nacre.colors.doing.copy(alpha = 0.12f),
            shape = RoundedCornerShape(16.dp),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Column(Modifier.padding(14.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(if (e.isError) Icons.Rounded.Error else Icons.Rounded.CheckCircle, null, tint = if (e.isError) MaterialTheme.colorScheme.error else Nacre.colors.doing)
                    Spacer(Modifier.width(8.dp))
                    Text(if (e.isError) "Turn ended with an error" else "Turn complete", style = MaterialTheme.typography.titleSmall)
                }
                if (e.text.isNotBlank()) MarkdownText(e.text, Modifier.padding(top = 8.dp))
            }
        }
        else -> Text(e.text, style = MaterialTheme.typography.bodySmall)
    }
}
