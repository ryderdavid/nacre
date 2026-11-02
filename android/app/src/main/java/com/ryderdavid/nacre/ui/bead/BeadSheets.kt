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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.AgentChoice
import com.ryderdavid.nacre.data.model.CLAUDE_AGENT
import com.ryderdavid.nacre.data.model.CODEX_AGENT
import com.ryderdavid.nacre.data.model.EFFORT_LABELS
import com.ryderdavid.nacre.data.model.RUN_AGENTS
import com.ryderdavid.nacre.data.model.runAgent
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
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
                    labels.filterNot(::isMachineLabel).forEach { l ->
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

val RUN_MODES = listOf(
    Triple("plan", "Plan", "Read-only. Claude investigates and proposes a plan."),
    Triple("acceptEdits", "Edits", "File edits allowed; commands that need approval are refused."),
    Triple("auto", "Auto", "Claude Code's auto mode decides which actions are safe."),
    Triple("bypassPermissions", "Bypass", "Everything allowed, no checks. Only for trusted repos."),
)

/** What each [RUN_MODES] mode means for a Codex run (nacred maps them onto Codex's sandbox). */
val CODEX_MODE_NOTES = mapOf(
    "plan" to "Read-only sandbox. Codex investigates and proposes a plan.",
    "acceptEdits" to "Edits inside the repository; no network, nothing outside it.",
    "auto" to "Edits inside the repository, with network access.",
    "bypassPermissions" to "No sandbox at all. Only for trusted repos.",
)

/**
 * Configure and send a bead to Claude Code or Codex. The switch at the top picks the agent first;
 * the model and effort below it come from that agent only, and flipping it starts them over at
 * the agent's defaults. [targets] are the computers that have this repository; with more than
 * one, the user picks where it runs. Codex can be picked anywhere, but a run starts only where the
 * computer reports it can run Codex; otherwise the sheet says why.
 */
@OptIn(ExperimentalLayoutApi::class)
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
    // Saved as plain strings; AgentChoice keeps model and effort within the chosen agent.
    var agentWire by rememberSaveable { mutableStateOf(CLAUDE_AGENT.wire) }
    var modelId by rememberSaveable { mutableStateOf(CLAUDE_AGENT.defaultModel) }
    var effortLevel by rememberSaveable { mutableStateOf(CLAUDE_AGENT.defaultEffort) }
    val agent = runAgent(agentWire)
    val choice = AgentChoice(agent, modelId, effortLevel).withModel(modelId)
    fun choose(c: AgentChoice) { agentWire = c.agent.wire; modelId = c.model; effortLevel = c.effort }
    var worktree by rememberSaveable { mutableStateOf(prefs.runWorktree) }
    var instructions by rememberSaveable { mutableStateOf("") }
    val permission = com.ryderdavid.nacre.ui.common.rememberNotificationPermission()
    val hostName = targetRepo?.repo?.hostName?.ifBlank { null } ?: "your computer"

    HandleDismissSheet(onDismiss) {
        Column(Modifier.fillMaxWidth().navigationBarsPadding().imePadding().padding(horizontal = 20.dp).padding(bottom = 16.dp).verticalScroll(rememberScrollState())) {
            Text("Run an agent manually", style = MaterialTheme.typography.titleLarge)
            Text("${issue.id} · ${issue.title}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth().padding(top = 16.dp)) {
                RUN_AGENTS.forEachIndexed { i, a ->
                    SegmentedButton(
                        selected = agent == a,
                        onClick = { if (agent != a) choose(AgentChoice.defaults(a)) },
                        shape = SegmentedButtonDefaults.itemShape(i, RUN_AGENTS.size),
                    ) { Text(a.label) }
                }
            }
            val codexBlocker = targetRepo?.codexBlocker.takeIf { agent == CODEX_AGENT }
            Text(
                codexBlocker ?: "Runs ${agent.label} on $hostName with the options below.",
                style = MaterialTheme.typography.bodySmall,
                color = if (codexBlocker != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp),
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
                CODEX_MODE_NOTES[mode].takeIf { agent == CODEX_AGENT } ?: RUN_MODES.first { it.first == mode }.third,
                style = MaterialTheme.typography.bodySmall,
                color = if (mode == "bypassPermissions") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 6.dp),
            )

            Text("Model", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp, bottom = 4.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                agent.models.forEach { m -> FilterChip(selected = choice.model == m.id, onClick = { choose(choice.withModel(m.id)) }, label = { Text(m.label) }) }
            }

            Text(if (agent == CODEX_AGENT) "Reasoning effort" else "Effort", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                agent.model(choice.model).efforts.forEach { e ->
                    FilterChip(selected = choice.effort == e, onClick = { choose(choice.withEffort(e)) }, label = { Text(EFFORT_LABELS[e] ?: e) })
                }
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
                    onStart(target, choice.request(mode, instructions.ifBlank { null }, worktree))
                },
                enabled = !sending && targetRepo != null && codexBlocker == null,
                modifier = Modifier.fillMaxWidth().padding(top = 20.dp).height(52.dp),
            ) {
                Text(if (sending) "Sending to $hostName…" else "Run with ${agent.label}")
            }
        }
    }
}

/** Only the destination is needed for automatic work. */
@Composable
fun StartWorkDialog(targets: List<RunTarget>, sending: Boolean, onDismiss: () -> Unit, onStart: (String) -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Start work on…") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                targets.forEach { target ->
                    Button(onClick = { onStart(target.repo.key) }, enabled = !sending, modifier = Modifier.fillMaxWidth()) {
                        Text(target.repo.hostName.ifBlank { "This computer" })
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
