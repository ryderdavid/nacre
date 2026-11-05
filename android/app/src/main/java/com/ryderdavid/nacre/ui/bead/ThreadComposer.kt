package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.sendOnHardwareEnter

/**
 * The bead's comment field: plain comments for the thread every agent on the
 * bead reads. Its Chat pill is the bead's chat entry, which opens the one
 * current chat (beside or over this screen); there is no chat of the bead's own.
 */
@Composable
fun ThreadComposer(
    issue: Issue,
    onComment: (String) -> Unit,
    /** The bead's chat entry: opens the one current chat. */
    onOpenChat: () -> Unit,
) {
    var text by rememberSaveable(issue.id) { mutableStateOf("") }
    val canSend = text.isNotBlank()
    val send = {
        onComment(text.trim())
        text = ""
    }
    val scheme = MaterialTheme.colorScheme
    Column(Modifier.fillMaxWidth().padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Surface(color = scheme.surfaceContainerHigh, shape = RoundedCornerShape(30.dp), modifier = Modifier.fillMaxWidth()) {
            Row(Modifier.heightIn(min = 60.dp).padding(6.dp), verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    onClick = onOpenChat, color = scheme.surfaceContainerHighest, contentColor = scheme.onSurface,
                    shape = RoundedCornerShape(20.dp), modifier = Modifier.heightIn(min = 40.dp),
                ) {
                    Row(Modifier.padding(horizontal = 10.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(AiIcon, "Open chat", Modifier.size(18.dp))
                        Text("Chat", style = MaterialTheme.typography.labelLarge)
                    }
                }
                TextField(
                    value = text, onValueChange = { text = it },
                    placeholder = { Text("Add a comment") },
                    modifier = Modifier.weight(1f).sendOnHardwareEnter(canSend, send),
                    maxLines = 5,
                    colors = TextFieldDefaults.colors(
                        focusedContainerColor = Color.Transparent, unfocusedContainerColor = Color.Transparent,
                        focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                    ),
                )
                FilledIconButton(onClick = send, enabled = canSend, modifier = Modifier.size(48.dp)) {
                    Icon(Icons.AutoMirrored.Rounded.Send, "Post comment")
                }
            }
        }
    }
}
