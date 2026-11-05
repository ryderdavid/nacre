package com.ryderdavid.nacre.ui.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.selectable
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.agentOptions
import com.ryderdavid.nacre.data.chat.contextForChat
import com.ryderdavid.nacre.data.chat.preselectedAgent
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.ui.common.HandleDismissSheet
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.launch

/**
 * The New chat sheet: Claude or Codex (remembered), and the computer, which comes from the screen.
 * There is no model choice.
 */
@Composable
fun NewChatSheet(req: NewChatRequest, onDismiss: () -> Unit, onCreated: (String) -> Unit) {
    val container = LocalContainer.current
    val target = rememberNewChatTarget(req)
    val prefs by container.settings.settings.collectAsState(initial = null)
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val options = target?.let(::agentOptions).orEmpty()
    var picked by remember(prefs?.chatAgent, options.map { it.enabled }) {
        mutableStateOf(preselectedAgent(prefs?.chatAgent ?: "claude", options))
    }
    HandleDismissSheet(onDismiss) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 24.dp).padding(bottom = 24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("New chat", style = MaterialTheme.typography.titleLarge)
            if (target == null) {
                Text("Pair a computer in Settings to chat.", style = MaterialTheme.typography.bodyMedium)
                return@Column
            }
            Text("Runs on ${target.computer} · ${target.label}", style = MaterialTheme.typography.bodyMedium)
            options.forEach { o ->
                Row(
                    Modifier.fillMaxWidth().selectable(selected = picked == o.agent, enabled = o.enabled, role = Role.RadioButton) { picked = o.agent },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RadioButton(selected = picked == o.agent, onClick = null, enabled = o.enabled)
                    Column(Modifier.padding(start = 12.dp)) {
                        Text(o.label, style = MaterialTheme.typography.bodyLarge)
                        o.reason?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
                        o.note?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                    }
                }
            }
            target.blocker?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
            error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onDismiss) { Text("Cancel") }
                Button(
                    enabled = !busy && target.blocker == null,
                    onClick = {
                        busy = true
                        error = null
                        scope.launch {
                            try {
                                container.settings.setChatAgent(picked)
                                val shown = container.chats.screens.atSend()
                                val ctx = contextForChat(shown, target.hostId, container.repository.groups.value, container.repository::hostName).context
                                val rec = container.repository.createChat(target.hostId, CreateChat(picked, target.repoId, ctx))
                                req.carry?.let { container.chats.prefill(rec.key, it) }
                                onCreated(rec.key)
                            } catch (e: Exception) {
                                if (e is kotlinx.coroutines.CancellationException) throw e
                                error = e.friendly()
                            } finally {
                                busy = false
                            }
                        }
                    },
                ) { Text("Start chat") }
            }
        }
    }
}
