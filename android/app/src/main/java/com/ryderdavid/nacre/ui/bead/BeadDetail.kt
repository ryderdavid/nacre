package com.ryderdavid.nacre.ui.bead

import androidx.compose.runtime.LaunchedEffect
import android.content.ClipData
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Flag
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Devices
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.material3.ScaffoldDefaults
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.ui.platform.ClipEntry
import androidx.compose.ui.platform.LocalClipboard
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.ACTIVITY_IDLE
import com.ryderdavid.nacre.data.model.ActiveElsewhere
import com.ryderdavid.nacre.data.model.elsewhere
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.UpdateIssue
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.Dot
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.NeedsYouTag
import com.ryderdavid.nacre.ui.common.PriorityPill
import com.ryderdavid.nacre.ui.common.SectionHeader
import com.ryderdavid.nacre.ui.common.TypeLabel
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.common.typeIcon
import com.ryderdavid.nacre.ui.runs.AgentChip
import com.ryderdavid.nacre.ui.runs.RunStatusIcon
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.launch

val STATUSES = listOf("open", "in_progress", "blocked", "deferred", "pinned", "closed")
val TYPES = listOf("task", "bug", "feature", "chore", "epic", "decision", "spike", "story")

fun statusLabel(s: String) = when (s) {
    "in_progress" -> "In progress"
    "closed" -> "Done"
    else -> s.replaceFirstChar { it.uppercase() }
}

/** Which long-text field is being edited. */
private enum class TextField(val key: String, val title: String) {
    TITLE("title", "Title"),
    DESCRIPTION("description", "Description"),
    DESIGN("design", "Design"),
    ACCEPTANCE("acceptance_criteria", "Acceptance criteria"),
    NOTES("notes", "Notes"),
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun BeadDetail(
    repoKey: String,
    beadId: String,
    inPane: Boolean,
    onClose: () -> Unit,
    onOpenBead: (String) -> Unit,
    onOpenRun: (String) -> Unit,
) {
    val vm = nacreViewModel(key = "bead:$repoKey:$beadId") { BeadDetailViewModel(it.repository, it.backends, it.settings, it.beadChats, repoKey, beadId) }
    val issue by vm.issue.collectAsState()
    val index by vm.index.collectAsState()
    val comments by vm.comments.collectAsState()
    val runs by vm.runs.collectAsState()
    val busy by vm.busy.collectAsState()
    val prefs by vm.prefs.collectAsState(initial = null)

    var editing by remember { mutableStateOf<TextField?>(null) }
    var showRefine by rememberSaveable { mutableStateOf(false) }
    var showRun by rememberSaveable { mutableStateOf(false) }
    var showLabels by remember { mutableStateOf(false) }
    var showAssignee by remember { mutableStateOf(false) }
    var showDepPicker by remember { mutableStateOf(false) }
    var showClose by remember { mutableStateOf(false) }
    var menu by remember { mutableStateOf(false) }
    val clipboard = LocalClipboard.current
    val scope = rememberCoroutineScope()

    Scaffold(
        containerColor = if (inPane) MaterialTheme.colorScheme.surfaceContainerLow else MaterialTheme.colorScheme.surface,
        contentWindowInsets = if (inPane) WindowInsets(0) else ScaffoldDefaults.contentWindowInsets,
        topBar = {
            TopAppBar(
                windowInsets = if (inPane) WindowInsets(0) else TopAppBarDefaults.windowInsets,
                colors = if (inPane) TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow) else TopAppBarDefaults.topAppBarColors(),
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Icon(if (inPane) Icons.Rounded.Close else Icons.AutoMirrored.Rounded.ArrowBack, if (inPane) "Close" else "Back")
                    }
                },
                title = { BeadId(beadId, color = MaterialTheme.colorScheme.onSurface) },
                actions = {
                    IconButton(onClick = { showRefine = true }, enabled = issue != null) { Icon(AiIcon, "Refine with Claude") }
                    IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text("Copy id") }, leadingIcon = { Icon(Icons.Rounded.ContentCopy, null) }, onClick = {
                            menu = false
                            scope.launch { clipboard.setClipEntry(ClipEntry(ClipData.newPlainText("bead id", beadId))) }
                        })
                        val i = issue
                        if (i != null) {
                            DropdownMenuItem(
                                text = { Text(if ("human" in i.labels) "Clear “needs you”" else "Mark as needing you") },
                                leadingIcon = { Icon(Icons.Rounded.Flag, null) },
                                onClick = { menu = false; vm.toggleNeedsYou() },
                            )
                            if (i.isClosed) DropdownMenuItem(text = { Text("Reopen") }, leadingIcon = { Icon(Icons.Rounded.Refresh, null) }, onClick = { menu = false; vm.setStatus("open") })
                            else DropdownMenuItem(text = { Text("Close…") }, leadingIcon = { Icon(Icons.Rounded.TaskAlt, null) }, onClick = { menu = false; showClose = true })
                        }
                        DropdownMenuItem(text = { Text("Reload") }, leadingIcon = { Icon(Icons.Rounded.Refresh, null) }, onClick = { menu = false; vm.load() })
                    }
                },
            )
        },
        floatingActionButton = {
            val i = issue
            if (i != null && !i.isClosed) {
                // Prefer our own run: an agent's session can't be steered from here, so it must not
                // take the place of the button that starts one.
                val active = runs.firstOrNull { it.isNacreWork } ?: runs.firstOrNull { it.isActive }
                ExtendedFloatingActionButton(
                    onClick = { if (active != null) onOpenRun(active.key) else showRun = true },
                    icon = { Icon(if (active != null) Icons.Rounded.Terminal else Icons.Rounded.PlayArrow, null) },
                    text = {
                        Text(
                            when {
                                active == null -> "Run with Claude Code"
                                active.isAdopted && active.activity == ACTIVITY_IDLE -> "An agent's session is idle"
                                active.isAdopted -> "An agent is working…"
                                else -> "Claude is working…"
                            }
                        )
                    },
                )
            }
        },
    ) { pad ->
        val i = issue
        val ix = index
        if (i == null || ix == null) {
            LoadingState(modifier = Modifier.padding(pad))
            return@Scaffold
        }
        Column(Modifier.padding(pad).fillMaxSize()) {
            if (busy) LinearWavyProgressIndicator(Modifier.fillMaxWidth())
            DetailBody(
                i, ix, comments, runs,
                onEdit = { editing = it },
                onStatus = vm::setStatus,
                onPriority = { vm.save(UpdateIssue(priority = it)) },
                onType = { vm.save(UpdateIssue(type = it)) },
                onLabels = { showLabels = true },
                onAssignee = { showAssignee = true },
                onAddBlocker = { showDepPicker = true },
                onRemoveBlocker = vm::removeBlocker,
                onOpenBead = onOpenBead,
                onOpenRun = onOpenRun,
                onComment = vm::addComment,
            )
        }

        editing?.let { f ->
            val initial = when (f) {
                TextField.TITLE -> i.title
                TextField.DESCRIPTION -> i.description
                TextField.DESIGN -> i.design
                TextField.ACCEPTANCE -> i.acceptance
                TextField.NOTES -> i.notes
            }
            TextEditDialog(
                title = "${f.title} · ${i.id}",
                initial = initial,
                singleLine = f == TextField.TITLE,
                onDismiss = { editing = null },
                onSave = { v ->
                    editing = null
                    vm.save(
                        when (f) {
                            TextField.TITLE -> UpdateIssue(title = v)
                            TextField.DESCRIPTION -> UpdateIssue(description = v)
                            TextField.DESIGN -> UpdateIssue(design = v)
                            TextField.ACCEPTANCE -> UpdateIssue(acceptance = v)
                            TextField.NOTES -> UpdateIssue(notes = v)
                        }
                    )
                },
            )
        }
        if (showLabels) LabelsDialog(i.labels, onDismiss = { showLabels = false }) { showLabels = false; vm.save(UpdateIssue(labels = it)) }
        if (showAssignee) TextEditDialog("Assignee · ${i.id}", i.assignee, singleLine = true, onDismiss = { showAssignee = false }) { showAssignee = false; vm.save(UpdateIssue(assignee = it)) }
        if (showDepPicker) BeadPicker(
            title = "${i.id} waits on…",
            candidates = ix.issues.filter { it.id != i.id && !it.isClosed && it.id !in i.blockerIds },
            onDismiss = { showDepPicker = false },
        ) { showDepPicker = false; vm.addBlocker(it.id) }
        if (showClose) CloseDialog(i.id, onDismiss = { showClose = false }) { showClose = false; vm.close(it) }
        if (showRefine) BeadChatSheet(vm, i, claudeReady = prefs?.claudeConfigured == true, onDismiss = { showRefine = false })
        if (showRun) prefs?.let { p ->
            val targets by vm.runTargets.collectAsState()
            var initial by remember { mutableStateOf<String?>(null) }
            LaunchedEffect(Unit) { initial = vm.preferredRunKey() }
            initial?.let { start ->
                RunLaunchSheet(i, p, targets, start, sending = busy, onDismiss = { showRun = false }) { target, req ->
                    vm.startRun(target, req) { run -> showRun = false; onOpenRun(run.key) }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun DetailBody(
    i: Issue,
    ix: BoardIndex,
    comments: List<com.ryderdavid.nacre.data.model.Comment>,
    runs: List<Run>,
    onEdit: (TextField) -> Unit,
    onStatus: (String) -> Unit,
    onPriority: (Int) -> Unit,
    onType: (String) -> Unit,
    onLabels: () -> Unit,
    onAssignee: () -> Unit,
    onAddBlocker: () -> Unit,
    onRemoveBlocker: (String) -> Unit,
    onOpenBead: (String) -> Unit,
    onOpenRun: (String) -> Unit,
    onComment: (String) -> Unit,
) {
    val lane = ix.laneOf(i)
    LazyColumn(contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 112.dp), modifier = Modifier.fillMaxSize().imePadding()) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Dot(Nacre.colors.lane(lane), 10)
                Text(lane.title, style = MaterialTheme.typography.labelLarge, color = Nacre.colors.lane(lane))
                if (i.needsYou) NeedsYouTag()
            }
            Spacer(Modifier.height(8.dp))
            Text(
                i.title,
                style = MaterialTheme.typography.headlineSmall,
                modifier = Modifier.fillMaxWidth().clickable { onEdit(TextField.TITLE) },
            )
            val crumb = ix.breadcrumb(i)
            if (crumb.isNotEmpty()) Text(crumb, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
            Spacer(Modifier.height(12.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                MenuChip(label = { Text(statusLabel(i.status)) }, leading = { Dot(Nacre.colors.lane(lane)) }, options = STATUSES.map { it to statusLabel(it) }, onPick = onStatus)
                MenuChip(label = { Text("Priority") }, leading = { PriorityPill(i.priority) }, options = (0..4).map { "$it" to "P$it" }, onPick = { onPriority(it.toInt()) })
                MenuChip(label = { TypeLabel(i.type) }, leading = null, options = TYPES.map { it to it.replaceFirstChar { c -> c.uppercase() } }, onPick = onType)
                AssistChip(onClick = onAssignee, label = { Text(i.assignee.ifBlank { "Unassigned" }, maxLines = 1) }, leadingIcon = { Icon(Icons.Rounded.Person, null, Modifier.size(18.dp)) })
            }
            i.elsewhere(liveRun = runs.any { it.isActive && it.inRunsList })?.let { ElsewhereNote(it) }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(top = 4.dp)) {
                i.labels.forEach { l -> InputChip(selected = false, onClick = onLabels, label = { Text(l) }) }
                AssistChip(onClick = onLabels, label = { Text(if (i.labels.isEmpty()) "Add labels" else "Edit") }, leadingIcon = { Icon(Icons.Rounded.Add, null, Modifier.size(18.dp)) })
            }
        }

        val blockers = i.blockerIds
        item {
            SectionHeader("Waits on", trailing = { TextButton(onClick = onAddBlocker) { Text("Add") } })
            if (blockers.isEmpty()) Text("Nothing. This bead isn't waiting on anything.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        items(blockers, key = { "b:$it" }) { id ->
            RelatedRow(id, ix.byId[id], ix, onOpenBead, trailing = {
                IconButton(onClick = { onRemoveBlocker(id) }) { Icon(Icons.Rounded.Close, "Remove dependency on $id") }
            })
        }

        item { TextSection("Description", i.description, "Add a description") { onEdit(TextField.DESCRIPTION) } }
        item { TextSection("Acceptance criteria", i.acceptance, "Add acceptance criteria") { onEdit(TextField.ACCEPTANCE) } }
        item { TextSection("Design", i.design, "Add design notes") { onEdit(TextField.DESIGN) } }
        item { TextSection("Notes", i.notes, "Add notes") { onEdit(TextField.NOTES) } }
        if (i.isClosed && i.closeReason.isNotBlank()) item { TextSection("Close reason", i.closeReason, "", onEdit = null) }

        val parent = ix.byId[i.parent]
        val children = ix.childrenOf(i.id).sortedWith(compareBy<Issue> { it.isClosed }.thenBy { it.priority }.then(BoardIndex.idOrder))
        val dependents = ix.dependentsOf(i.id)
        if (parent != null) {
            item { SectionHeader("Parent") }
            item { RelatedRow(parent.id, parent, ix, onOpenBead) }
        }
        if (children.isNotEmpty()) {
            item { SectionHeader("Children · ${children.count { it.isClosed }}/${children.size} done") }
            items(children, key = { "c:${it.id}" }) { RelatedRow(it.id, it, ix, onOpenBead) }
        }
        if (dependents.isNotEmpty()) {
            item { SectionHeader("Blocks") }
            items(dependents, key = { "d:${it.id}" }) { RelatedRow(it.id, it, ix, onOpenBead) }
        }

        if (runs.isNotEmpty()) {
            item { SectionHeader("Claude Code runs") }
            items(runs, key = { "r:${it.id}" }) { r ->
                Row(
                    Modifier.fillMaxWidth().clickable { onOpenRun(r.key) }.padding(vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RunStatusIcon(r.status, stalled = r.stalled)
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) {
                        Text(r.lastText.ifBlank { r.status }, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        Text(
                            listOfNotNull(relativeTime(r.createdAt), r.mode.ifBlank { null }).joinToString(" · "),
                            style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (r.isAdopted) AgentChip(r, Modifier.padding(start = 8.dp))
                }
            }
        }

        item { SectionHeader("Comments · ${comments.size}") }
        items(comments, key = { "m:${it.id}" }) { c ->
            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer), modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                Column(Modifier.padding(12.dp)) {
                    Text("${c.author} · ${relativeTime(c.createdAt)}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(4.dp))
                    MarkdownText(c.text)
                }
            }
        }
        item { CommentComposer(onComment) }
        item {
            HorizontalDivider(Modifier.padding(vertical = 16.dp))
            Text(
                listOfNotNull(
                    "Created ${relativeTime(i.createdAt)}",
                    "updated ${relativeTime(i.updatedAt)}",
                    i.owner.takeIf { it.isNotBlank() }?.let { "owner $it" },
                    i.specId.takeIf { it.isNotBlank() }?.let { "spec $it" },
                ).joinToString(" · "),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.outline,
            )
        }
    }
}

/** bd shows the bead claimed while no run in Nacre works on it. A marker, not a run: nothing to open. */
@Composable
private fun ElsewhereNote(e: ActiveElsewhere) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
        Row(Modifier.padding(12.dp)) {
            Icon(Icons.Rounded.Devices, null, Modifier.size(20.dp), tint = Nacre.colors.doing)
            Spacer(Modifier.width(12.dp))
            Column {
                Text(e.title, style = MaterialTheme.typography.titleSmall)
                Text(
                    e.detail(relativeTime(e.since)),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun MenuChip(label: @Composable () -> Unit, leading: (@Composable () -> Unit)?, options: List<Pair<String, String>>, onPick: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        AssistChip(onClick = { open = true }, label = label, leadingIcon = leading)
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (value, text) -> DropdownMenuItem(text = { Text(text) }, onClick = { open = false; onPick(value) }) }
        }
    }
}

@Composable
private fun TextSection(title: String, text: String, emptyHint: String, onEdit: (() -> Unit)?) {
    SectionHeader(title, trailing = onEdit?.let { { IconButton(onClick = it) { Icon(Icons.Rounded.Edit, "Edit $title") } } })
    if (text.isBlank()) {
        if (onEdit != null) Surface(
            onClick = onEdit,
            shape = RoundedCornerShape(12.dp),
            color = MaterialTheme.colorScheme.surfaceContainer,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(emptyHint, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(14.dp))
        }
    } else {
        MarkdownText(text, Modifier.fillMaxWidth().padding(bottom = 4.dp))
    }
}

@Composable
fun RelatedRow(id: String, issue: Issue?, ix: BoardIndex, onOpen: (String) -> Unit, trailing: (@Composable () -> Unit)? = null) {
    Row(
        Modifier.fillMaxWidth().clickable(enabled = issue != null) { onOpen(id) }.padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val lane = issue?.let { ix.laneOf(it) }
        Icon(
            if (lane == Lane.BLOCKED) Icons.Rounded.HourglassTop else typeIcon(issue?.type ?: "task"),
            null,
            tint = lane?.let { Nacre.colors.lane(it) } ?: MaterialTheme.colorScheme.outline,
            modifier = Modifier.size(18.dp),
        )
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            BeadId(id)
            Text(
                issue?.title ?: "Not on this board",
                style = MaterialTheme.typography.bodyMedium.copy(fontWeight = if (issue?.isClosed == true) FontWeight.Normal else FontWeight.Medium),
                color = if (issue?.isClosed == true) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                maxLines = 2, overflow = TextOverflow.Ellipsis,
            )
        }
        if (issue != null) PriorityPill(issue.priority)
        trailing?.invoke()
    }
}

@Composable
private fun CommentComposer(onSend: (String) -> Unit) {
    var text by rememberSaveable { mutableStateOf("") }
    OutlinedTextField(
        value = text,
        onValueChange = { text = it },
        placeholder = { Text("Add a comment") },
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
        shape = RoundedCornerShape(16.dp),
        trailingIcon = {
            IconButton(onClick = { onSend(text.trim()); text = "" }, enabled = text.isNotBlank()) {
                Icon(Icons.AutoMirrored.Rounded.Send, "Send comment")
            }
        },
    )
}
