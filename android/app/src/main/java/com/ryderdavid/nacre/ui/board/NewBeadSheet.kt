package com.ryderdavid.nacre.ui.board

import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
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
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.chat.createIssueOf
import com.ryderdavid.nacre.data.chat.draftForm
import com.ryderdavid.nacre.data.chat.runIntake
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.ChatDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.NewEffortPrefill
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.bead.TYPES
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
    /** A draft from Chat (Continue on a Draft card): the sheet opens with it at its intake step. */
    draft: ChatDraft? = null,
) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    // A new effort keeps its own draft apart from a plain new bead, and a prefilled sheet (found
    // work, a follow-on) keeps one per thing it came from (NAC-cjm).
    val draftKey = newBeadDraftKey(repoKey, effort, prefill)
    /** The form as it opens with nothing kept: blank, or the prefill. */
    val start = remember {
        draft?.let { draftForm(it, defaultParent, TYPES) }
            ?: NewBeadForm(title = prefill?.title.orEmpty(), description = prefill?.description.orEmpty(), parent = defaultParent)
    }
    // A draft from Chat is what Ryder just continued with; it wins over a form left behind earlier.
    val saved = remember { if (draft != null) start else container.newBeadDrafts.get(draftKey) ?: start }
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
    /** Acceptance and design fields opened by hand before either has text. */
    var addingDetails by rememberSaveable { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    /** Set once the bead exists, so the form stops being kept as a draft. */
    var written by remember { mutableStateOf(false) }

    fun form() = NewBeadForm(title, description, acceptance, design, type, priority, labels, parent)

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
        error = null; addingDetails = false
        edited()
        container.newBeadDrafts.clear(draftKey)
    }

    fun check() = scope.launch {
        checking = true; intakeError = null; error = null
        runIntake(repoKey, form(), container.repository::intake)
            .onSuccess { v -> verdict = v; route = EffortRoute.of(v) }
            .onFailure { e -> intakeError = e.friendly(); route = EffortRoute.NewChange }
        checking = false
    }

    // Continue on a Draft card opens the sheet at its intake step: intake runs on the draft at once.
    LaunchedEffect(Unit) { if (effort && draft != null && title.isNotBlank() && route == null) check() }

    fun create(develop: EffortRoute? = null) = scope.launch {
        saving = true; error = null
        try {
            val issue = container.repository.create(repoKey, createIssueOf(form(), prefill))
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
                    TextButton(onClick = { discard() }, enabled = !saving && !checking) { Text("Discard draft") }
                }
            }
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                title, { title = it; edited() }, label = { Text("Title") }, singleLine = true,
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences, imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(description, { description = it; edited() }, label = { Text("Description or a rough note") }, minLines = 3, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            if (showDetailFields(addingDetails, acceptance, design)) {
                OutlinedTextField(acceptance, { acceptance = it; edited() }, label = { Text("Acceptance criteria") }, minLines = 2, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
                OutlinedTextField(design, { design = it; edited() }, label = { Text("Design") }, minLines = 2, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            } else {
                // Written by hand, or filled from a draft in Chat (NAC-3mc).
                TextButton(onClick = { addingDetails = true }, modifier = Modifier.padding(top = 4.dp)) { Text("Add acceptance criteria and design") }
            }
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

/** Acceptance and design show once either has text, or when the owner asks for them. */
fun showDetailFields(adding: Boolean, acceptance: String, design: String): Boolean =
    adding || acceptance.isNotBlank() || design.isNotBlank()
