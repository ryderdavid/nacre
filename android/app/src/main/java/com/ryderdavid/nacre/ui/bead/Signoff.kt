package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * A bead whose work is built and merged, with only Ryder's own check left:
 * what to look at, then Sign off (closes it) or Not right (says what, and the
 * bead goes back to ordinary work).
 */
@Composable
fun SignoffCard(check: String, busy: Boolean, onSignOff: (approve: Boolean, note: String) -> Unit) {
    var sendingBack by rememberSaveable { mutableStateOf(false) }
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = RoundedCornerShape(20.dp),
        modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(Icons.Rounded.Visibility, null)
                Text("Only your check is left", style = MaterialTheme.typography.titleMedium)
            }
            Text("The work is built, verified and merged. Look it over, then sign off to close the bead.", style = MaterialTheme.typography.bodyMedium)
            if (check.isNotBlank()) Text(check, style = MaterialTheme.typography.bodyLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End), modifier = Modifier.fillMaxWidth()) {
                OutlinedButton(onClick = { sendingBack = true }, enabled = !busy) { Text("Not right") }
                Button(onClick = { onSignOff(true, "") }, enabled = !busy) { Text("Sign off") }
            }
        }
    }
    if (sendingBack) NotRightDialog(onDismiss = { sendingBack = false }, onSend = { sendingBack = false; onSignOff(false, it) })
}

@Composable
private fun NotRightDialog(onDismiss: () -> Unit, onSend: (String) -> Unit) {
    var note by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("What isn't right?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("It goes on the bead as a comment, and the bead goes back to ordinary work.", style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(note, { note = it }, minLines = 3, modifier = Modifier.fillMaxWidth(), placeholder = { Text("The spinner overlaps the text…") })
            }
        },
        confirmButton = { TextButton(onClick = { onSend(note) }, enabled = note.isNotBlank()) { Text("Send back") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
