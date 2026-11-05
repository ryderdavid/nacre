package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
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
import com.ryderdavid.nacre.ui.chat.rememberChatOpener
import com.ryderdavid.nacre.ui.common.AiIcon

/**
 * Ryder's own answer or decision about one thing on a screen (a round's question,
 * a change), written in his words and used through [useLabel]. Questions about it
 * go to Chat, the one AI chat in the app: Ask in Chat opens the current chat,
 * whose next message carries this screen as its context.
 */
@Composable
fun OwnAnswer(
    /** Remembers the draft per thing, so two cards don't share one. */
    key: String,
    hint: String,
    placeholder: String,
    useLabel: String,
    busy: Boolean,
    onUse: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val openChat = rememberChatOpener()
    var text by rememberSaveable(key) { mutableStateOf("") }
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(hint, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            OutlinedTextField(
                value = text, onValueChange = { text = it }, modifier = Modifier.fillMaxWidth(),
                placeholder = { Text(placeholder) }, shape = RoundedCornerShape(24.dp), maxLines = 4,
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = { openChat(null) }) {
                    Icon(AiIcon, null, Modifier.size(18.dp))
                    Text("Ask in Chat", Modifier.padding(start = 6.dp))
                }
                Row(Modifier.weight(1f), horizontalArrangement = Arrangement.End) {
                    TextButton(onClick = { onUse(text.trim()); text = "" }, enabled = !busy && text.isNotBlank()) { Text(useLabel) }
                }
            }
        }
    }
}
