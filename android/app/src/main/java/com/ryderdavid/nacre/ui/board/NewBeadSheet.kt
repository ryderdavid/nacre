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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
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
    var drafting by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    fun draft() = scope.launch {
        drafting = true; error = null
        try {
            val s = container.settings.current()
            val rough = listOf(title, description).filter { it.isNotBlank() }.joinToString("\n\n")
            val d = container.claude.draft(
                s, repoName, rough,
                parentTitle = parent?.let { p -> index?.byId?.get(p)?.title },
                existingTitles = index?.issues?.filterNot { it.isClosed }?.map { "${it.id} ${it.title}" }.orEmpty(),
            )
            title = d.title; description = d.description; acceptance = d.acceptance_criteria; design = d.design
            type = d.issue_type.takeIf { it in TYPES } ?: "task"; priority = d.priority.coerceIn(0, 4); labels = d.labels
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
            error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp)) }
            Button(onClick = { create() }, enabled = title.isNotBlank() && !saving, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).height(52.dp)) {
                Text(if (saving) "Creating…" else "Create bead")
            }
        }
    }
}
