package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.FoundRequest
import com.ryderdavid.nacre.data.model.NewEffortPrefill
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

/** The three answers to discovered work, by whether it changes what "done" means. */
enum class FoundAnswer(val wire: String, val label: String) {
    HOW_ORDER("how_order", "No, only how or in what order"),
    AMEND("amend", "Yes, it changes what done means"),
    OUT_OF_SCOPE("out_of_scope", "It's out of scope"),
}

/** What each answer does, said before Ryder picks it. */
fun FoundAnswer.explain(change: ChangeSummary, landed: Int) = when (this) {
    FoundAnswer.HOW_ORDER -> "Add a bead under ${change.epicId}. The approved plan stays as it is."
    FoundAnswer.AMEND -> if (landed > 0) "Files it and asks whether to revise ${change.name} now, which reopens $landed landed section${if (landed == 1) "" else "s"}, or make it a follow-on change."
    else "Files it and asks whether to revise ${change.name} now (a new review and your approval, up to \$5) or make it a follow-on change."
    FoundAnswer.OUT_OF_SCOPE -> "Start a New effort, linked to ${change.epicId} as related."
}

/**
 * Found something: report discovered work on a spec epic, section, found bead
 * or spec round, and answer it by the boundary rule. From a round, [roundBeads]
 * asks which bead it was found in. An out-of-scope answer hands a prefill back
 * for New effort.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun FoundSomethingSheet(
    repoKey: String,
    change: ChangeSummary,
    foundIn: String?,
    onDismiss: () -> Unit,
    onOutOfScope: (NewEffortPrefill) -> Unit,
    roundBeads: List<String> = emptyList(),
    commentId: String = "",
    note: String = "",
) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    var text by rememberSaveable { mutableStateOf(note) }
    var answer by rememberSaveable { mutableStateOf(FoundAnswer.HOW_ORDER) }
    var where by rememberSaveable { mutableStateOf(foundIn ?: roundBeads.firstOrNull().orEmpty()) }
    var before by rememberSaveable { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val open = change.sections.filter { it.evidence != "landed" }

    fun submit() = scope.launch {
        saving = true; error = null
        try {
            val res = container.repository.found(
                repoKey, change.id,
                FoundRequest(answer.wire, note = text.trim(), foundIn = where, beforeSection = before.takeIf { answer == FoundAnswer.HOW_ORDER }.orEmpty(), commentId = commentId),
            )
            when {
                res.prefill != null -> onOutOfScope(res.prefill)
                else -> {
                    container.repository.notify(if (answer == FoundAnswer.AMEND) "Filed ${res.beadId}; it's waiting in Needs you" else "Added ${res.beadId} under ${change.epicId}")
                    onDismiss()
                }
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
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text("Found something", style = MaterialTheme.typography.titleLarge)
            Text(
                if (where.isNotBlank()) "While working $where on ${change.name}" else "In ${change.name}",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (roundBeads.size > 1) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                roundBeads.forEach { b -> FilterChip(selected = where == b, onClick = { where = b }, label = { Text(b) }) }
            }
            OutlinedTextField(text, { text = it }, label = { Text("What did you find?") }, minLines = 3, modifier = Modifier.fillMaxWidth())
            Text("Does it change what “done” means?", style = MaterialTheme.typography.titleSmall)
            FoundAnswer.entries.forEach { a ->
                val selected = a == answer
                Surface(
                    shape = RoundedCornerShape(12.dp),
                    color = if (selected) MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.4f) else MaterialTheme.colorScheme.surface,
                    border = BorderStroke(1.dp, if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant),
                    modifier = Modifier.fillMaxWidth().clickable(role = Role.RadioButton) { answer = a },
                ) {
                    Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp)) {
                        Text(a.label, style = MaterialTheme.typography.titleSmall)
                        Text(a.explain(change, change.landed), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                if (selected && a == FoundAnswer.HOW_ORDER && open.isNotEmpty()) {
                    Text("Must happen before (optional)", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(selected = before.isBlank(), onClick = { before = "" }, label = { Text("No order") })
                        open.forEachIndexed { i, s -> FilterChip(selected = before == s.id, onClick = { before = s.id }, label = { Text("§ ${change.sections.indexOf(s) + 1} ${s.title}", maxLines = 1) }) }
                    }
                }
            }
            error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            Spacer(Modifier.height(4.dp))
            Button(onClick = { submit() }, enabled = !saving && (text.isNotBlank() || commentId.isNotBlank()), modifier = Modifier.fillMaxWidth().height(52.dp)) {
                Text(
                    when {
                        saving -> "Saving…"
                        answer == FoundAnswer.OUT_OF_SCOPE -> "Start a New effort"
                        answer == FoundAnswer.AMEND -> "File it"
                        else -> "Add bead"
                    },
                )
            }
        }
    }
}
