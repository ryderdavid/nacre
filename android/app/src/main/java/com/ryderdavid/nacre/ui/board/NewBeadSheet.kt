package com.ryderdavid.nacre.ui.board

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.NewBeadForm
import com.ryderdavid.nacre.data.board.newBeadDraftKey
import com.ryderdavid.nacre.data.claude.BeadDraft
import com.ryderdavid.nacre.data.claude.DraftTurn
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.claude.ClaudeUnavailableException
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.IntakeDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.NewEffortPrefill
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.bead.TYPES
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun NewBeadSheet(
    repoKey: String, repoName: String, index: BoardIndex?, defaultParent: String?, onDismiss: () -> Unit, onCreated: (Issue) -> Unit,
    /** New effort: intake decides bead, new change, amendment or covered before anything is written. */
    effort: Boolean = false,
    /** Out-of-scope found work or a follow-on change, opened from Needs you or Found something. */
    prefill: NewEffortPrefill? = null,
) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    // A new effort keeps its own draft apart from a plain new bead, and a prefilled sheet (found
    // work, a follow-on) keeps one per thing it came from (NAC-cjm).
    val draftKey = newBeadDraftKey(repoKey, effort, prefill)
    /** The form as it opens with nothing kept: blank, or the prefill. */
    val start = remember { NewBeadForm(title = prefill?.title.orEmpty(), description = prefill?.description.orEmpty(), parent = defaultParent) }
    val saved = remember { container.newBeadDrafts.get(draftKey) ?: start }
    var title by rememberSaveable { mutableStateOf(saved.title) }
    var description by rememberSaveable { mutableStateOf(saved.description) }
    var verdict by remember { mutableStateOf<IntakeVerdict?>(null) }
    var route by remember { mutableStateOf<EffortRoute?>(null) }
    var checking by remember { mutableStateOf(false) }
    var intakeError by remember { mutableStateOf<String?>(null) }
    var created by remember { mutableStateOf<Issue?>(null) }
    var acceptance by rememberSaveable { mutableStateOf(saved.acceptance) }
    var design by rememberSaveable { mutableStateOf(saved.design) }
    var type by rememberSaveable { mutableStateOf(saved.type) }
    var priority by rememberSaveable { mutableStateOf(saved.priority) }
    var labels by rememberSaveable { mutableStateOf(saved.labels) }
    var parent by rememberSaveable { mutableStateOf(saved.parent) }
    var questions by rememberSaveable { mutableStateOf(saved.questions) }
    var chat by rememberSaveable { mutableStateOf(saved.chat) }
    var message by rememberSaveable { mutableStateOf(saved.message) }
    var drafted by rememberSaveable { mutableStateOf(saved.drafted) }
    var drafting by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var offerApiKey by remember { mutableStateOf(false) }
    /** Set once the bead exists, so the form stops being kept as a draft. */
    var written by remember { mutableStateOf(false) }

    fun form() = NewBeadForm(title, description, acceptance, design, type, priority, labels, parent, questions, chat, message, drafted)

    // Keep the form as it stands when the sheet goes away, so any way of dismissing it (tap
    // outside, back, the handle) leaves the input to come back on the next open (NAC-cjm). This
    // runs on leaving composition rather than as a SideEffect: the fields are read only inside the
    // sheet's content, so typing never recomposes this scope and a SideEffect would miss the edits.
    DisposableEffect(draftKey) {
        onDispose { if (!written) container.newBeadDrafts.put(draftKey, form(), start) }
    }

    /** Any edit to the draft makes intake's verdict out of date. */
    fun edited() { verdict = null; route = null; intakeError = null }

    fun discard() {
        val blank = start
        title = blank.title; description = blank.description; acceptance = blank.acceptance; design = blank.design
        type = blank.type; priority = blank.priority; labels = blank.labels; parent = blank.parent
        questions = blank.questions; chat = blank.chat; message = blank.message; drafted = blank.drafted
        error = null; offerApiKey = false
        edited()
        container.newBeadDrafts.clear(draftKey)
    }

    fun parentTitle() = parent?.let { p -> index?.byId?.get(p)?.title }
    fun existingTitles() = index?.issues?.filterNot { it.isClosed }?.map { "${it.id} ${it.title}" }.orEmpty()

    fun apply(d: BeadDraft) {
        title = d.title; description = d.description; acceptance = d.acceptance_criteria; design = d.design
        type = d.issue_type.takeIf { it in TYPES } ?: "task"; priority = d.priority.coerceIn(0, 4); labels = d.labels
        questions = d.open_questions; drafted = true
    }

    fun draft() = scope.launch {
        drafting = true; error = null; offerApiKey = false
        try {
            val s = container.settings.current()
            val rough = listOf(title, description).filter { it.isNotBlank() }.joinToString("\n\n")
            apply(container.backends.of(s).draft(repoKey, repoName, rough, parentTitle(), existingTitles()))
            chat = emptyList()
        } catch (e: ClaudeUnavailableException) {
            error = e.message
            offerApiKey = e.offerApiKey && container.settings.current().apiKeyConfigured
        } catch (e: Exception) {
            error = e.message
        } finally {
            drafting = false
        }
    }

    /** Send a chat message with the draft as it stands now, including the owner's own edits. */
    fun revise() = scope.launch {
        val text = message.trim()
        drafting = true; error = null; offerApiKey = false
        try {
            val s = container.settings.current()
            val current = BeadDraft(title, description, design, acceptance, type, priority, labels, questions)
            val d = container.backends.of(s).reviseDraft(repoKey, repoName, current, chat, text, parentTitle(), existingTitles())
            apply(d)
            chat = chat + DraftTurn(true, text) + DraftTurn(false, d.reply.ifBlank { "Updated the draft." })
            message = ""
        } catch (e: ClaudeUnavailableException) {
            error = e.message
            offerApiKey = e.offerApiKey && container.settings.current().apiKeyConfigured
        } catch (e: Exception) {
            error = e.message
        } finally {
            drafting = false
        }
    }

    fun check() = scope.launch {
        checking = true; intakeError = null; error = null
        try {
            val v = container.repository.intake(
                repoKey, IntakeDraft(title.trim(), description, design, acceptance, type, parent.orEmpty()),
            )
            verdict = v; route = EffortRoute.of(v)
        } catch (e: Exception) {
            intakeError = e.friendly()
            route = EffortRoute.NewChange
        } finally {
            checking = false
        }
    }

    fun create(develop: EffortRoute? = null) = scope.launch {
        saving = true; error = null
        try {
            val issue = container.repository.create(
                repoKey,
                CreateIssue(
                    title = title.trim(), description = description.ifBlank { null }, type = type, priority = priority,
                    parent = parent, labels = labels.ifEmpty { null }, design = design.ifBlank { null }, acceptance = acceptance.ifBlank { null },
                    related = prefill?.related?.ifEmpty { null }, followOnOf = prefill?.followOnOf?.ifBlank { null }, foundComment = prefill?.foundComment,
                ),
            )
            written = true
            container.newBeadDrafts.clear(draftKey)
            if (develop == null) {
                container.repository.notify("Created ${issue.id}")
                onCreated(issue)
                return@launch
            }
            try {
                container.repository.changeAction(repoKey, issue.id, ChangeAction("develop", feedback = develop.feedback(verdict), intake = develop.hint()))
                container.repository.notify("Created ${issue.id}; its proposal is being developed")
                onCreated(issue)
            } catch (e: Exception) {
                // The bead stays; say why development didn't start.
                created = issue
                error = "Created ${issue.id}, but its proposal didn't start: ${e.friendly()}"
            }
        } catch (e: Exception) {
            error = e.friendly()
        } finally {
            saving = false
        }
    }

    HandleDismissSheet(onDismiss) {
        Column(
            Modifier.fillMaxWidth().navigationBarsPadding().imePadding().padding(horizontal = 20.dp).padding(bottom = 16.dp).verticalScroll(rememberScrollState()),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(if (effort) "New effort" else "New bead", style = MaterialTheme.typography.titleLarge)
                    Text("in $repoName", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (created == null && form().hasContent && form() != start) {
                    TextButton(onClick = { discard() }, enabled = !drafting && !saving && !checking) { Text("Discard draft") }
                }
            }
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                title, { title = it; edited() }, label = { Text("Title") }, singleLine = true,
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences, imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(description, { description = it; edited() }, label = { Text("Description or a rough note") }, minLines = 3, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            if (acceptance.isNotBlank() || design.isNotBlank()) {
                OutlinedTextField(acceptance, { acceptance = it }, label = { Text("Acceptance criteria") }, minLines = 2, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
                OutlinedTextField(design, { design = it }, label = { Text("Design") }, minLines = 2, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            }
            FilledTonalButton(
                onClick = { draft() },
                enabled = !drafting && (title.isNotBlank() || description.isNotBlank()),
                modifier = Modifier.padding(top = 12.dp),
            ) {
                if (drafting) LoadingIndicator(Modifier.size(20.dp)) else Icon(AiIcon, null, Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(if (drafting) "Claude is drafting…" else "Draft with Claude")
            }
            if (offerApiKey) TextButton(
                onClick = { scope.launch { container.settings.update { it.copy(claudeRunner = AppSettings.RUNNER_API) } } },
            ) { Text("Use my Claude API key instead") }

            Text("Type", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 16.dp, bottom = 4.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TYPES.take(7).forEach { t -> FilterChip(selected = type == t, onClick = { type = t }, label = { Text(t) }) }
            }
            Text("Priority", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                (0..4).forEach { p -> FilterChip(selected = priority == p, onClick = { priority = p }, label = { Text("P$p") }) }
            }
            val epics = index?.issues?.filter { it.isEpic && !it.isClosed }.orEmpty()
            if (epics.isNotEmpty()) {
                Text("Parent", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(selected = parent == null, onClick = { parent = null }, label = { Text("None") })
                    epics.take(12).forEach { e -> FilterChip(selected = parent == e.id, onClick = { parent = e.id }, label = { Text(e.title, maxLines = 1) }) }
                }
            }
            if (labels.isNotEmpty()) Text("Labels: ${labels.joinToString()}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp))
            if (drafted) DraftChat(
                questions = questions, chat = chat, message = message, onMessage = { message = it },
                busy = drafting, onSend = { if (message.isNotBlank() && !drafting) revise() },
            )
            error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp)) }
            val done = created
            when {
                done != null -> Button(onClick = { onCreated(done) }, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).height(52.dp)) { Text("Done") }
                !effort -> Button(onClick = { create() }, enabled = title.isNotBlank() && !saving, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).height(52.dp)) {
                    Text(if (saving) "Creating…" else "Create bead")
                }
                route == null -> Button(onClick = { check() }, enabled = title.isNotBlank() && !checking, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).height(52.dp)) {
                    if (checking) LoadingIndicator(Modifier.size(20.dp))
                    Text(if (checking) "Checking against the specs…" else "Continue")
                }
                else -> IntakeChoice(
                    verdict = verdict, error = intakeError, route = route!!, saving = saving,
                    onRoute = { route = it },
                    onConfirm = { r -> create(develop = r.takeIf { it.develops }) },
                    onLater = { create() },
                )
            }
        }
    }
}

/** Chat box under the drafted fields: answer Claude's open questions or tell it what to fix. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun DraftChat(questions: List<String>, chat: List<DraftTurn>, message: String, onMessage: (String) -> Unit, busy: Boolean, onSend: () -> Unit) {
    Column(Modifier.fillMaxWidth().padding(top = 16.dp)) {
        Text("Steer the draft", style = MaterialTheme.typography.titleSmall)
        if (questions.isNotEmpty()) {
            Text("Claude asks:", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
            questions.forEach { q -> Text("• $q", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(start = 4.dp, top = 2.dp)) }
        }
        chat.forEach { t ->
            Box(Modifier.fillMaxWidth().padding(top = 6.dp), contentAlignment = if (t.fromUser) Alignment.CenterEnd else Alignment.CenterStart) {
                Surface(
                    color = if (t.fromUser) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHigh,
                    shape = RoundedCornerShape(16.dp),
                ) {
                    Text(
                        t.text, style = MaterialTheme.typography.bodyMedium,
                        color = if (t.fromUser) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                    )
                }
            }
        }
        Row(Modifier.fillMaxWidth().padding(top = 8.dp), verticalAlignment = Alignment.Bottom) {
            OutlinedTextField(
                value = message,
                onValueChange = onMessage,
                placeholder = { Text(if (questions.isNotEmpty()) "Answer a question or say what to change" else "Tell Claude what to change") },
                maxLines = 5,
                shape = RoundedCornerShape(24.dp),
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences, imeAction = ImeAction.Send),
                keyboardActions = KeyboardActions(onSend = { onSend() }),
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(8.dp))
            FilledIconButton(onClick = onSend, enabled = message.isNotBlank() && !busy, modifier = Modifier.size(52.dp)) {
                if (busy) LoadingIndicator(Modifier.size(24.dp)) else Icon(Icons.AutoMirrored.Rounded.Send, "Send to Claude")
            }
        }
    }
}

/**
 * Intake's verdict with its reason and citations, and the routes Ryder can
 * pick instead. Without a verdict (intake failed) only just a bead and a new
 * change are offered.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun IntakeChoice(
    verdict: IntakeVerdict?, error: String?, route: EffortRoute, saving: Boolean,
    onRoute: (EffortRoute) -> Unit, onConfirm: (EffortRoute) -> Unit, onLater: () -> Unit,
) {
    Column(Modifier.fillMaxWidth().padding(top = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (verdict != null) Surface(color = MaterialTheme.colorScheme.secondaryContainer.copy(alpha = 0.55f), shape = RoundedCornerShape(16.dp)) {
            Column(Modifier.fillMaxWidth().padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    "Intake · \$" + "%.2f".format(verdict.costUsd), style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSecondaryContainer,
                )
                Text(EffortRoute.of(verdict).headline(), style = MaterialTheme.typography.titleMedium)
                if (verdict.rationale.isNotBlank()) Text(verdict.rationale, style = MaterialTheme.typography.bodyMedium)
                verdict.references.forEach { r ->
                    Text(
                        r.path + if (r.requirement.isNotBlank()) " · Requirement: ${r.requirement}" else "",
                        style = MaterialTheme.typography.labelSmall.copy(fontFamily = com.ryderdavid.nacre.ui.theme.MonoFamily),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (verdict.fallback.isNotBlank()) Text(verdict.fallback, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (verdict.verdict == "amend" && !verdict.targetTracked) Text(
                    "Nacre isn't tracking work on this change, so it can't tell whether a round is using it.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        if (error != null) Text("Intake couldn't decide: $error", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
        Text("Route it as", style = MaterialTheme.typography.titleSmall)
        val changes = verdict?.activeChanges.orEmpty()
        val amend = (route as? EffortRoute.Amend)?.change ?: verdict?.changeName?.takeIf { it.isNotBlank() } ?: changes.firstOrNull()
        val options = buildList {
            if (amend != null) add(EffortRoute.Amend(amend))
            add(EffortRoute.NewChange)
            add(EffortRoute.Bead)
            if (verdict?.verdict == "covered") add(EffortRoute.Covered)
        }
        options.forEach { r ->
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                androidx.compose.material3.RadioButton(selected = r == route, onClick = { onRoute(r) })
                Column(Modifier.weight(1f)) {
                    Text(r.headline(), style = MaterialTheme.typography.bodyLarge)
                    Text(
                        when (r) {
                            is EffortRoute.Amend -> "A reviewed revision of $amend follows; your approval is needed."
                            EffortRoute.NewChange -> "Write a separate OpenSpec change, reviewed before any beads."
                            EffortRoute.Bead -> "No proposal. It goes to Ready, like today."
                            EffortRoute.Covered -> "Already asked for. Create it as a plain bead only if you still want one."
                        },
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        if (route is EffortRoute.Amend && changes.size > 1) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            changes.forEach { c -> FilterChip(selected = c == route.change, onClick = { onRoute(EffortRoute.Amend(c)) }, label = { Text(c) }) }
        }
        if (route.develops) Text(
            "The full review compares the effort with every spec and active change and may choose differently; the change's history will say why.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        val blocked = route.blockedBy(verdict)
        if (blocked != null) {
            Text(blocked, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            Button(onClick = onLater, enabled = !saving, modifier = Modifier.fillMaxWidth().height(52.dp)) { Text(if (saving) "Creating…" else "Create the bead; develop later") }
        } else {
            Button(onClick = { onConfirm(route) }, enabled = !saving, modifier = Modifier.fillMaxWidth().height(52.dp)) {
                Text(if (saving) "Creating…" else route.confirmLabel())
            }
        }
    }
}
