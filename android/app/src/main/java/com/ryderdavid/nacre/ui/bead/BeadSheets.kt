package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.ryderdavid.nacre.data.claude.RefineProposal
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.PriorityPill
import com.ryderdavid.nacre.ui.theme.Nacre

/** Full-screen editor for a text field (markdown for long fields). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TextEditDialog(title: String, initial: String, singleLine: Boolean, onDismiss: () -> Unit, onSave: (String) -> Unit) {
    var text by rememberSaveable { mutableStateOf(initial) }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Scaffold(
            topBar = {
                TopAppBar(
                    title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = { IconButton(onClick = onDismiss) { Icon(Icons.Rounded.Close, "Discard") } },
                    actions = { TextButton(onClick = { onSave(text.trim()) }, enabled = text != initial) { Text("Save") } },
                )
            },
        ) { pad ->
            OutlinedTextField(
                value = text,
                onValueChange = { text = it },
                singleLine = singleLine,
                textStyle = MaterialTheme.typography.bodyLarge,
                placeholder = { if (!singleLine) Text("Markdown supported") },
                modifier = Modifier.padding(pad).padding(16.dp).fillMaxWidth().let { if (singleLine) it else it.fillMaxSize() }.imePadding(),
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun LabelsDialog(initial: List<String>, onDismiss: () -> Unit, onSave: (List<String>) -> Unit) {
    var labels by remember { mutableStateOf(initial) }
    var input by remember { mutableStateOf("") }
    fun add() {
        val l = input.trim().lowercase().replace(' ', '-')
        if (l.isNotEmpty() && l !in labels) labels = labels + l
        input = ""
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Labels") },
        text = {
            Column {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    labels.forEach { l ->
                        InputChip(selected = false, onClick = { labels = labels - l }, label = { Text(l) }, trailingIcon = { Icon(Icons.Rounded.Close, "Remove $l") })
                    }
                }
                OutlinedTextField(
                    input, { input = it }, singleLine = true, placeholder = { Text("New label") },
                    trailingIcon = { TextButton(onClick = ::add, enabled = input.isNotBlank()) { Text("Add") } },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                )
                Text("The “human” label marks a bead as needing you.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp))
            }
        },
        confirmButton = { TextButton(onClick = { add(); onSave(labels) }) { Text("Save") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
fun BeadPicker(title: String, candidates: List<Issue>, onDismiss: () -> Unit, onPick: (Issue) -> Unit) {
    var q by remember { mutableStateOf("") }
    val shown = candidates.filter { q.isBlank() || it.id.contains(q, true) || it.title.contains(q, true) }.take(100)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column {
                OutlinedTextField(q, { q = it }, singleLine = true, placeholder = { Text("Search id or title") }, modifier = Modifier.fillMaxWidth())
                LazyColumn(Modifier.heightIn(max = 380.dp).padding(top = 8.dp)) {
                    items(shown, key = { it.id }) { i ->
                        Row(Modifier.fillMaxWidth().clickable { onPick(i) }.padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            PriorityPill(i.priority)
                            Spacer(Modifier.width(8.dp))
                            Column(Modifier.weight(1f)) {
                                BeadId(i.id)
                                Text(i.title, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
fun CloseDialog(id: String, onDismiss: () -> Unit, onClose: (String) -> Unit) {
    var reason by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Close $id") },
        text = { OutlinedTextField(reason, { reason = it }, label = { Text("Reason (optional)") }, modifier = Modifier.fillMaxWidth()) },
        confirmButton = { Button(onClick = { onClose(reason) }) { Text("Close bead") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

private val REFINE_SUGGESTIONS = listOf(
    "Make it agent-ready",
    "Write acceptance criteria",
    "Tighten the description",
    "Split into subtasks",
    "Add design notes",
    "Suggest priority and type",
)

/** Ask Claude to improve the bead, then review a field-by-field diff before applying. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun RefineSheet(vm: BeadDetailViewModel, issue: Issue, claudeReady: Boolean, onDismiss: () -> Unit) {
    val state by vm.refine.collectAsState()
    var instruction by rememberSaveable { mutableStateOf("") }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(Modifier.fillMaxWidth().navigationBarsPadding().imePadding().padding(horizontal = 20.dp).padding(bottom = 16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(AiIcon, null, tint = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.width(8.dp))
                Text("Refine with Claude", style = MaterialTheme.typography.titleLarge)
            }
            Spacer(Modifier.height(12.dp))
            when (val s = state) {
                RefineState.Idle, is RefineState.Failed -> {
                    if (!claudeReady) {
                        Text("Add your Claude API key in Settings to use this.", color = MaterialTheme.colorScheme.error)
                        return@Column
                    }
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        REFINE_SUGGESTIONS.forEach { sug -> SuggestionChip(onClick = { instruction = sug }, label = { Text(sug) }) }
                    }
                    OutlinedTextField(
                        instruction, { instruction = it },
                        placeholder = { Text("What should change? Leave empty to make it agent-ready.") },
                        minLines = 2,
                        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                    )
                    if (s is RefineState.Failed) Text(s.message, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
                    Button(onClick = { vm.requestRefine(instruction) }, modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
                        Icon(AiIcon, null); Spacer(Modifier.width(8.dp)); Text("Ask Claude")
                    }
                }
                RefineState.Thinking -> Column(Modifier.fillMaxWidth().padding(vertical = 32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    LoadingIndicator()
                    Spacer(Modifier.height(12.dp))
                    Text("Claude is reading ${issue.id}…", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                is RefineState.Ready -> ProposalReview(s.proposal, issue, onApply = { sel -> vm.applyRefine(s.proposal, sel, onDismiss) }, onRetry = vm::dismissRefine)
            }
        }
    }
}

@Composable
private fun ProposalReview(p: RefineProposal, issue: Issue, onApply: (RefineSelection) -> Unit, onRetry: () -> Unit) {
    val changes = buildList {
        if (p.title != null && p.title != issue.title) add(Triple("title", "Title", issue.title to p.title))
        if (p.description != null && p.description != issue.description) add(Triple("description", "Description", issue.description to p.description))
        if (p.acceptance_criteria != null && p.acceptance_criteria != issue.acceptance) add(Triple("acceptance_criteria", "Acceptance criteria", issue.acceptance to p.acceptance_criteria))
        if (p.design != null && p.design != issue.design) add(Triple("design", "Design", issue.design to p.design))
        if (p.notes != null && p.notes != issue.notes) add(Triple("notes", "Notes", issue.notes to p.notes))
        if (p.priority != null && p.priority != issue.priority) add(Triple("priority", "Priority", "P${issue.priority}" to "P${p.priority}"))
        if (p.issue_type != null && p.issue_type != issue.type) add(Triple("issue_type", "Type", issue.type to p.issue_type))
        if (p.labels != null && p.labels.toSet() != issue.labels.toSet()) add(Triple("labels", "Labels", issue.labels.joinToString() to p.labels.joinToString()))
    }
    var fields by remember { mutableStateOf(changes.map { it.first }.toSet()) }
    var children by remember { mutableStateOf(p.children.indices.toSet()) }

    Column(Modifier.verticalScroll(rememberScrollState())) {
        if (p.summary.isNotBlank()) Surface(color = MaterialTheme.colorScheme.secondaryContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
            Text(p.summary, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(14.dp))
        }
        if (changes.isEmpty() && p.children.isEmpty()) {
            Text("Claude didn't propose any changes.", modifier = Modifier.padding(vertical = 16.dp))
        }
        changes.forEach { (key, label, diff) ->
            val on = key in fields
            Surface(
                shape = RoundedCornerShape(16.dp),
                color = MaterialTheme.colorScheme.surfaceContainer,
                modifier = Modifier.fillMaxWidth().padding(top = 10.dp),
                onClick = { fields = if (on) fields - key else fields + key },
            ) {
                Column(Modifier.padding(12.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(checked = on, onCheckedChange = { fields = if (on) fields - key else fields + key })
                        Text(label, style = MaterialTheme.typography.titleSmall)
                    }
                    if (diff.first.isNotBlank()) Text(
                        diff.first.take(600), style = MaterialTheme.typography.bodySmall.copy(textDecoration = TextDecoration.LineThrough),
                        color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 6, overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(horizontal = 12.dp),
                    )
                    Surface(color = Nacre.colors.doing.copy(alpha = 0.12f), shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth().padding(8.dp)) {
                        MarkdownText(diff.second, Modifier.padding(10.dp))
                    }
                }
            }
        }
        if (p.children.isNotEmpty()) {
            Text("New child beads", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 16.dp, bottom = 4.dp))
            p.children.forEachIndexed { i, c ->
                val on = i in children
                Row(
                    Modifier.fillMaxWidth().clickable { children = if (on) children - i else children + i }.padding(vertical = 4.dp),
                    verticalAlignment = Alignment.Top,
                ) {
                    Checkbox(checked = on, onCheckedChange = { children = if (on) children - i else children + i })
                    Column(Modifier.weight(1f).padding(top = 12.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            PriorityPill(c.priority.coerceIn(0, 4)); Spacer(Modifier.width(6.dp))
                            Text(c.issue_type.uppercase(), style = MaterialTheme.typography.labelSmall, color = Nacre.colors.type(c.issue_type))
                        }
                        Text(c.title, style = MaterialTheme.typography.bodyLarge)
                        if (c.description.isNotBlank()) Text(c.description, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth().padding(top = 16.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            TextButton(onClick = onRetry, modifier = Modifier.weight(1f)) { Text("Ask again") }
            Button(onClick = { onApply(RefineSelection(fields, children)) }, enabled = fields.isNotEmpty() || children.isNotEmpty(), modifier = Modifier.weight(1f)) {
                Text("Apply ${fields.size + children.size}")
            }
        }
    }
}

val RUN_MODES = listOf(
    Triple("plan", "Plan", "Read-only. Claude investigates and proposes a plan."),
    Triple("acceptEdits", "Edits", "File edits allowed; commands that need approval are refused."),
    Triple("auto", "Auto", "Claude Code's auto mode decides which actions are safe."),
    Triple("bypassPermissions", "Bypass", "Everything allowed, no checks. Only for trusted repos."),
)

val RUN_MODELS = listOf("" to "Default", "opus" to "Opus", "sonnet" to "Sonnet", "haiku" to "Haiku", "fable" to "Fable")

/**
 * Configure and send a bead to Claude Code. [targets] are the computers that have this
 * repository; with more than one, the user picks where it runs.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun RunLaunchSheet(
    issue: Issue,
    prefs: AppSettings,
    targets: List<RunTarget>,
    initialTarget: String,
    sending: Boolean,
    onDismiss: () -> Unit,
    onStart: (targetKey: String, RunRequest) -> Unit,
) {
    var target by rememberSaveable { mutableStateOf(initialTarget) }
    val targetRepo = targets.firstOrNull { it.repo.key == target }
    var mode by rememberSaveable { mutableStateOf(prefs.runMode) }
    var model by rememberSaveable { mutableStateOf(prefs.runModel) }
    var worktree by rememberSaveable { mutableStateOf(prefs.runWorktree) }
    var instructions by rememberSaveable { mutableStateOf("") }
    val permission = com.ryderdavid.nacre.ui.common.rememberNotificationPermission()

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(Modifier.fillMaxWidth().navigationBarsPadding().imePadding().padding(horizontal = 20.dp).padding(bottom = 16.dp).verticalScroll(rememberScrollState())) {
            Text("Run with Claude Code", style = MaterialTheme.typography.titleLarge)
            Text("${issue.id} · ${issue.title}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            Text(
                "Runs on ${targetRepo?.repo?.hostName?.ifBlank { null } ?: "your computer"} with your Claude subscription. Claude claims the bead, does the work, verifies it and closes it when done.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp),
            )

            if (targets.size > 1) {
                Text("Run on", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp, bottom = 4.dp))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    targets.forEach { t ->
                        FilterChip(
                            selected = target == t.repo.key,
                            onClick = { target = t.repo.key },
                            label = { Text(t.repo.hostName) },
                            leadingIcon = { com.ryderdavid.nacre.ui.common.Dot(if (t.online) Nacre.colors.doing else MaterialTheme.colorScheme.outline) },
                        )
                    }
                }
                if (targetRepo?.online == false) Text(
                    "${targetRepo.repo.hostName} isn't reachable right now.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 4.dp),
                )
                if (targetRepo != null && targetRepo.repo.gitBranch.isNotBlank()) Text(
                    "Checkout on ${targetRepo.repo.gitBranch} · ${targetRepo.repo.path.replace(Regex("^/Users/[^/]+"), "~")}",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp),
                )
            }

            Text("Permissions", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp, bottom = 8.dp))
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                RUN_MODES.forEachIndexed { i, (key, label, _) ->
                    SegmentedButton(selected = mode == key, onClick = { mode = key }, shape = SegmentedButtonDefaults.itemShape(i, RUN_MODES.size)) { Text(label, maxLines = 1) }
                }
            }
            Text(
                RUN_MODES.first { it.first == mode }.third,
                style = MaterialTheme.typography.bodySmall,
                color = if (mode == "bypassPermissions") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 6.dp),
            )

            Text("Model", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp, bottom = 4.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                RUN_MODELS.forEach { (key, label) -> FilterChip(selected = model == key, onClick = { model = key }, label = { Text(label) }) }
            }

            Row(Modifier.fillMaxWidth().padding(top = 12.dp).clickable { worktree = !worktree }, verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("Use a git worktree", style = MaterialTheme.typography.bodyLarge)
                    Text("Work on an isolated branch so your checkout is untouched", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = worktree, onCheckedChange = { worktree = it })
            }

            OutlinedTextField(
                instructions, { instructions = it },
                label = { Text("Extra instructions (optional)") },
                minLines = 2,
                modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
            )

            Button(
                onClick = {
                    permission()
                    onStart(target, RunRequest(mode = mode, model = model.ifBlank { null }, instructions = instructions.ifBlank { null }, worktree = worktree))
                },
                enabled = !sending && targets.any { it.repo.key == target },
                modifier = Modifier.fillMaxWidth().padding(top = 20.dp).height(52.dp),
            ) {
                Text(if (sending) "Sending to ${targetRepo?.repo?.hostName?.ifBlank { null } ?: "your computer"}…" else "Start run")
            }
        }
    }
}
