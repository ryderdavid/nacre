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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
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
import com.ryderdavid.nacre.data.claude.BeadDraft
import com.ryderdavid.nacre.data.claude.DraftTurn
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.bead.TYPES
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class, ExperimentalLayoutApi::class)
@Composable
fun NewBeadSheet(repoKey: String, repoName: String, index: BoardIndex?, defaultParent: String?, onDismiss: () -> Unit, onCreated: (Issue) -> Unit) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    var title by rememberSaveable { mutableStateOf("") }
    var description by rememberSaveable { mutableStateOf("") }
    var acceptance by rememberSaveable { mutableStateOf("") }
    var design by rememberSaveable { mutableStateOf("") }
    var type by rememberSaveable { mutableStateOf("task") }
    var priority by rememberSaveable { mutableStateOf(2) }
    var labels by rememberSaveable { mutableStateOf(listOf<String>()) }
    var parent by rememberSaveable { mutableStateOf(defaultParent) }
    var questions by rememberSaveable { mutableStateOf(listOf<String>()) }
    var chat by rememberSaveable { mutableStateOf(listOf<DraftTurn>()) }
    var message by rememberSaveable { mutableStateOf("") }
    var drafted by rememberSaveable { mutableStateOf(false) }
    var drafting by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    fun parentTitle() = parent?.let { p -> index?.byId?.get(p)?.title }
    fun existingTitles() = index?.issues?.filterNot { it.isClosed }?.map { "${it.id} ${it.title}" }.orEmpty()

    fun apply(d: BeadDraft) {
        title = d.title; description = d.description; acceptance = d.acceptance_criteria; design = d.design
        type = d.issue_type.takeIf { it in TYPES } ?: "task"; priority = d.priority.coerceIn(0, 4); labels = d.labels
        questions = d.open_questions; drafted = true
    }

    fun draft() = scope.launch {
        drafting = true; error = null
        try {
            val s = container.settings.current()
            val rough = listOf(title, description).filter { it.isNotBlank() }.joinToString("\n\n")
            apply(container.claude.draft(s, repoName, rough, parentTitle(), existingTitles()))
            chat = emptyList()
        } catch (e: Exception) {
            error = e.message
        } finally {
            drafting = false
        }
    }

    /** Send a chat message with the draft as it stands now, including the owner's own edits. */
    fun revise() = scope.launch {
        val text = message.trim()
        drafting = true; error = null
        try {
            val s = container.settings.current()
            val current = BeadDraft(title, description, design, acceptance, type, priority, labels, questions)
            val d = container.claude.reviseDraft(s, repoName, current, chat, text, parentTitle(), existingTitles())
            apply(d)
            chat = chat + DraftTurn(true, text) + DraftTurn(false, d.reply.ifBlank { "Updated the draft." })
            message = ""
        } catch (e: Exception) {
            error = e.message
        } finally {
            drafting = false
        }
    }

    fun create() = scope.launch {
        saving = true; error = null
        try {
            val created = container.repository.create(
                repoKey,
                CreateIssue(
                    title = title.trim(), description = description.ifBlank { null }, type = type, priority = priority,
                    parent = parent, labels = labels.ifEmpty { null }, design = design.ifBlank { null }, acceptance = acceptance.ifBlank { null },
                ),
            )
            container.repository.notify("Created ${created.id}")
            onCreated(created)
        } catch (e: Exception) {
            error = e.friendly()
        } finally {
            saving = false
        }
    }

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(
            Modifier.fillMaxWidth().navigationBarsPadding().imePadding().padding(horizontal = 20.dp).padding(bottom = 16.dp).verticalScroll(rememberScrollState()),
        ) {
            Text("New bead", style = MaterialTheme.typography.titleLarge)
            Text("in $repoName", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                title, { title = it }, label = { Text("Title") }, singleLine = true,
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences, imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(description, { description = it }, label = { Text("Description or a rough note") }, minLines = 3, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
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
            Button(onClick = { create() }, enabled = title.isNotBlank() && !saving, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).height(52.dp)) {
                Text(if (saving) "Creating…" else "Create bead")
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
