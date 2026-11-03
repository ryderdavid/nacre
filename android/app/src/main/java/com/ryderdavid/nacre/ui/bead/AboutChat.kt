package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.BeadChatItem
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.nacreViewModel

/**
 * A contained chat about one thing on a bead (a question, a change), in the
 * bead's own thread. [about] is what Claude is told about it; only the
 * exchange that began with it shows. What Ryder types can also be used as his
 * own answer or decision through [useLabel].
 */
@Composable
fun AboutChat(
    repoKey: String,
    beadId: String,
    about: String,
    claudeReady: Boolean,
    hint: String,
    placeholder: String,
    useLabel: String,
    busy: Boolean,
    onUse: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val vm = nacreViewModel(key = "bead:$repoKey:$beadId") { BeadDetailViewModel(it.repository, it.backends, it.settings, it.beadChats, repoKey, beadId) }
    val chat by vm.chat.collectAsState()
    val thinking by vm.chatThinking.collectAsState()
    val failure by vm.chatFailure.collectAsState()
    var text by rememberSaveable(beadId, about) { mutableStateOf("") }
    // Only this exchange: from its first message on.
    val start = chat.indexOfFirst { it is BeadChatItem.Mine && it.about == about }
    val shown = if (start < 0) emptyList() else chat.drop(start)
    val scroll = rememberScrollState()
    LaunchedEffect(shown.size, thinking) { scroll.animateScrollTo(scroll.maxValue) }
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (shown.isEmpty() && !thinking) Text(hint, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (shown.isNotEmpty() || thinking) Column(
                Modifier.heightIn(max = 320.dp).verticalScroll(scroll), verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                shown.forEach { m ->
                    when (m) {
                        is BeadChatItem.Mine -> Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                            Surface(color = MaterialTheme.colorScheme.primaryContainer, shape = RoundedCornerShape(14.dp), modifier = Modifier.widthIn(max = 280.dp)) {
                                Text(m.text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                            }
                        }
                        is BeadChatItem.Answer -> MarkdownText(m.text, Modifier.fillMaxWidth())
                        is BeadChatItem.Connection -> ConnectionNote(m)
                        else -> Unit
                    }
                }
                if (thinking) Text("Claude is thinking…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                failure?.let { Text(it.message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
            }
            if (!claudeReady) Text("Pair a computer, or add a Claude API key, in Settings to chat.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = text, onValueChange = { text = it }, modifier = Modifier.weight(1f),
                    placeholder = { Text(placeholder) }, shape = RoundedCornerShape(24.dp), maxLines = 4,
                )
                IconButton(
                    onClick = { vm.askAbout(about, text); text = "" },
                    enabled = claudeReady && !thinking && text.isNotBlank(),
                ) { Icon(Icons.AutoMirrored.Rounded.Send, "Ask Claude") }
            }
            if (text.isNotBlank()) TextButton(onClick = { onUse(text.trim()); text = "" }, enabled = !busy) { Text(useLabel) }
        }
    }
}
