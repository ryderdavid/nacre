package com.ryderdavid.nacre.ui.common

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.OpenInNew
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

/** What a computer says when its Claude needs signing in again. */
fun signedOutOn(message: String): String? =
    Regex("""Claude is signed out on (.+?)\.""").find(message)?.groupValues?.get(1)

/**
 * Signs a computer's Claude in from the phone: the computer hands over a
 * sign-in page, which opens here, and the code that page shows goes back to
 * the computer, which keeps a token that lasts a year.
 */
@Composable
fun ClaudeSignInDialog(hostId: String, onDismiss: () -> Unit) {
    val container = LocalContainer.current
    val repo = container.repository
    val uri = LocalUriHandler.current
    val scope = rememberCoroutineScope()
    val name = repo.hostName(hostId).ifBlank { "this computer" }
    var link by remember { mutableStateOf<String?>(null) }
    var code by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(true) }
    var problem by remember { mutableStateOf<String?>(null) }
    var done by remember { mutableStateOf(false) }

    fun start() {
        busy = true; problem = null; code = ""
        scope.launch {
            runCatching { repo.startClaudeSignIn(hostId) }
                .onSuccess { link = it; uri.openUri(it) }
                .onFailure { problem = it.message ?: "Couldn't start signing in" }
            busy = false
        }
    }
    LaunchedEffect(hostId) { start() }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (done) "Claude is signed in" else "Sign in to Claude on $name") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                when {
                    done -> Text("$name now runs Claude with a sign-in that lasts a year, so it won't need signing in at the computer again.")
                    else -> {
                        Text("Sign in on the page that opened, then copy the code it shows and paste it here.", style = MaterialTheme.typography.bodyMedium)
                        link?.let { l ->
                            FilledTonalButton(onClick = { uri.openUri(l) }) {
                                Icon(Icons.AutoMirrored.Rounded.OpenInNew, null)
                                Text("Open the sign-in page again", Modifier.padding(start = 8.dp))
                            }
                        }
                        OutlinedTextField(
                            value = code, onValueChange = { code = it }, singleLine = true,
                            label = { Text("Code from the sign-in page") }, enabled = link != null && !busy,
                            modifier = Modifier.fillMaxWidth(),
                        )
                        problem?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
                    }
                }
            }
        },
        confirmButton = {
            when {
                done -> TextButton(onClick = onDismiss) { Text("Done") }
                problem != null && link == null -> TextButton(onClick = ::start, enabled = !busy) { Text("Try again") }
                else -> TextButton(
                    enabled = !busy && link != null && code.isNotBlank(),
                    onClick = {
                        busy = true; problem = null
                        scope.launch {
                            runCatching { repo.finishClaudeSignIn(hostId, code.trim()) }
                                .onSuccess { done = true }
                                .onFailure { problem = it.message ?: "Signing in failed"; link = null }
                            busy = false
                        }
                    },
                ) { Text(if (busy && link != null) "Signing in…" else "Sign in") }
            }
        },
        dismissButton = { if (!done) TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
