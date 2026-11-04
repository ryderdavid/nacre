package com.ryderdavid.nacre.ui.rounds

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

/** True when [repo] can't take a round only because it has no nacre.yaml yet. */
fun needsGateSetup(repo: Repo?): Boolean = repo != null && repo.gates.isEmpty() && repo.gatesError.isBlank()

/**
 * Sets up a repository's gates from the phone: Claude on the computer reads the repository
 * and drafts nacre.yaml, Ryder reviews or edits it, and the computer commits it alone on the
 * checkout's branch. [onSaved] runs after the commit, so the action that needed gates can go on.
 */
@Composable
fun GatesSetupSheet(repo: Repo, onDismiss: () -> Unit, onSaved: () -> Unit) {
    val container = LocalContainer.current
    val scope = rememberCoroutineScope()
    var yaml by rememberSaveable { mutableStateOf<String?>(null) }
    var notes by rememberSaveable { mutableStateOf("") }
    var problem by rememberSaveable { mutableStateOf("") }
    var drafting by rememberSaveable { mutableStateOf(false) }
    var saving by rememberSaveable { mutableStateOf(false) }
    var error by rememberSaveable { mutableStateOf<String?>(null) }
    val host = repo.hostName.ifBlank { "the computer" }

    fun draft() {
        drafting = true
        error = null
        scope.launch {
            try {
                val d = container.repository.draftRepoConfig(repo.key)
                yaml = d.yaml
                notes = d.notes
                problem = d.problem
            } catch (e: Exception) {
                error = e.friendly()
            } finally {
                drafting = false
            }
        }
    }

    HandleDismissSheet(onDismiss = { if (!saving) onDismiss() }) {
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).navigationBarsPadding().imePadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("Set up gates", style = MaterialTheme.typography.headlineSmall)
            Text(
                "Rounds in ${repo.name} need a nacre.yaml: the tests and build that decide when work is done. " +
                    "Claude on $host reads the repository and drafts one; you check it, and it's committed on its own.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val current = yaml
            when {
                drafting -> Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    Text("Reading ${repo.name}… this can take a couple of minutes.", style = MaterialTheme.typography.bodyMedium)
                }
                current == null -> Button(onClick = ::draft, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                    Text("Draft it")
                }
                else -> {
                    if (notes.isNotBlank()) Text(notes, style = MaterialTheme.typography.bodyMedium)
                    if (problem.isNotBlank()) Text(
                        "Needs a fix before it can be saved: $problem",
                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error,
                    )
                    OutlinedTextField(
                        value = current, onValueChange = { yaml = it; problem = "" },
                        label = { Text("nacre.yaml") },
                        textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                        shape = RoundedCornerShape(12.dp),
                        minLines = 8,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
                        TextButton(onClick = ::draft, enabled = !saving) { Text("Draft again") }
                        Button(
                            onClick = {
                                saving = true
                                error = null
                                scope.launch {
                                    try {
                                        container.repository.saveRepoConfig(repo.key, current)
                                        onSaved()
                                    } catch (e: Exception) {
                                        error = e.friendly()
                                    } finally {
                                        saving = false
                                    }
                                }
                            },
                            enabled = !saving && current.isNotBlank(),
                        ) { Text(if (saving) "Saving…" else "Save and commit") }
                    }
                }
            }
            error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
        }
    }
}
