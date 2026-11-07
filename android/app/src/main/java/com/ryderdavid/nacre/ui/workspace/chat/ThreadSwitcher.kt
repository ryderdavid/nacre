package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

/**
 * The project's chats and jobs in a sheet, for the one-pane layouts: the same cards as the board's drawer.
 * Picking a chat (or Open chat on a job) shows it and closes the sheet.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ThreadSwitcher(repoKey: String, current: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val openEvidence = com.ryderdavid.nacre.ui.workspace.LocalOpenEvidence.current
    var jobs by rememberSaveable { mutableStateOf(false) }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        ChatDrawer(
            repoKey, current, jobs = jobs, onJobs = { jobs = it }, unfolded = false, onUnfold = {},
            onThread = onPick,
            onInspect = { w -> onDismiss(); openEvidence(com.ryderdavid.nacre.data.model.DecisionEvidence(label = w.title, kind = w.kind, ref = w.id)) },
            chat = null,
            modifier = Modifier.fillMaxWidth().heightIn(max = (LocalConfiguration.current.screenHeightDp * 0.85f).dp).navigationBarsPadding(),
        )
    }
}

/**
 * Confirms deleting a chat. Deleting removes the conversation, its log and images from its computer; beads,
 * specs and work it started stay.
 */
@Composable
fun DeleteChatDialog(key: String, title: String, onDismiss: () -> Unit) {
    val container = LocalContainer.current
    val name = title.ifBlank { "this chat" }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Delete “$name”?") },
        text = { Text("Its messages and images are removed from the computer, and it can't be brought back. Beads, specs and work it started stay.") },
        confirmButton = {
            TextButton(onClick = {
                onDismiss()
                container.scope.launch {
                    runCatching { container.chats.delete(key) }.onFailure { container.repository.notify("Couldn't delete the chat: ${it.message}") }
                }
            }) { Text("Delete", color = MaterialTheme.colorScheme.error) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
