package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Undo
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.CardCheck
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel
import kotlinx.coroutines.launch

// Interim pane contents for the chat-first shell; sections 5 to 9 replace each of them.

@Composable
fun ChatPane(repoKey: String, thread: String, onThread: (String) -> Unit, onProjects: () -> Unit, showHeader: Boolean = true) {
    com.ryderdavid.nacre.ui.workspace.chat.ChatPaneM3(repoKey, thread, onThread, onProjects, onEvidence = LocalOpenEvidence.current, showHeader = showHeader)
}

/**
 * The project's Needs-you queue: every open decision, pending first, then held, unresolved outcomes, work
 * still executing and deferred. Opened at a decision (a notification, or a change waiting in Spec), that
 * one is listed first.
 *
 * Opening it starts nacred's card check: a read-only agent run that closes cards already handled
 * elsewhere (a desktop session fixed it, another chat filed it). Those are listed last, with what
 * handled them and Reopen.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun DecisionQueue(repoKey: String, onClose: () -> Unit) {
    val repo = LocalContainer.current.repository
    val decisions by repo.decisions.collectAsState()
    val focus by repo.queueFocus.collectAsState()
    val focusId = focus?.takeIf { it.repoKey == repoKey }?.decisionId
    val all = decisions[repoKey].orEmpty()
    val list = DecisionModel.queue(all, focusId)
    val handled = DecisionModel.handled(all, System.currentTimeMillis()).filter { it.id != focusId }
    androidx.compose.runtime.DisposableEffect(repoKey) { onDispose { repo.clearQueueFocus(repoKey) } }
    val checks by repo.cardChecks.collectAsState()
    val check = checks[repoKey]
    LaunchedEffect(repoKey) { repo.checkCards(repoKey) }
    val underCheck = check?.takeIf { it.running }?.cards.orEmpty().toSet()
    val open = LocalOpenEvidence.current
    // The title and what the check is doing stay put while the cards scroll under them.
    Column(Modifier.fillMaxWidth()) {
        Column(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, bottom = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Needs you", style = MaterialTheme.typography.headlineSmallEmphasized)
            check?.let { c -> checkLine(c)?.let { line -> CheckLine(c.running, line) } }
        }
    LazyColumn(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = PaddingValues(bottom = 32.dp)) {
        if (list.isEmpty()) item { Text("Nothing needs you in this project.", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        items(list, key = { it.key }) { d ->
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                if (d.id in underCheck) CheckingChip()
                com.ryderdavid.nacre.ui.workspace.decisions.DecisionCard(d, onEvidence = { onClose(); open(it) })
            }
        }
        if (handled.isNotEmpty()) {
            item(key = "l:handled") {
                Text(
                    "Handled elsewhere", style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.padding(top = 12.dp).semantics { heading() },
                )
            }
            items(handled, key = { "h:" + it.key }) { d -> HandledCard(d) }
        }
    }
    }
}

/**
 * What Projects says about the card checks its Needs you filter started over [checks], or null: how
 * many projects are being checked, then how many cards were closed, or that nothing was.
 */
fun projectsCheckLine(checks: List<CardCheck>): String? {
    val running = checks.count { it.running }
    if (running > 0) return "Checking $running project${if (running == 1) "" else "s"} for cards already handled elsewhere…"
    val closed = checks.sumOf { it.handled.size }
    if (closed > 0) return "Closed $closed card${if (closed == 1) "" else "s"} already handled elsewhere"
    val done = checks.count { it.finishedAt.isNotBlank() && it.error.isBlank() && it.checking > 0 }
    return if (done > 0) "Checked $done project${if (done == 1) "" else "s"}: everything still needs you" else null
}

/** Marks a card the running check is looking at. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun CheckingChip() {
    Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.secondaryContainer, contentColor = MaterialTheme.colorScheme.onSecondaryContainer) {
        Row(Modifier.padding(start = 6.dp, end = 12.dp, top = 2.dp, bottom = 2.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            LoadingIndicator(Modifier.size(24.dp))
            Text("Checking whether this still needs you", style = MaterialTheme.typography.labelMedium)
        }
    }
}

/** What the queue says about its card check, or null when there is nothing worth saying. */
fun checkLine(c: CardCheck): String? = when {
    c.running && c.checking == 0 -> "Checking the cards for anything already handled elsewhere…"
    c.running -> "Checking ${c.checking} card${if (c.checking == 1) "" else "s"} for anything already handled elsewhere…"
    c.error.isNotBlank() -> "Couldn't check the cards for work handled elsewhere: ${c.error}"
    c.finishedAt.isBlank() -> null
    c.handled.isNotEmpty() -> "Closed ${c.handled.size} already handled elsewhere; they're listed at the end"
    else -> "Checked ${c.checking} card${if (c.checking == 1) "" else "s"}: ${if (c.checking == 1) "it still needs" else "they all still need"} you"
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
internal fun CheckLine(running: Boolean, text: String) {
    Row(
        Modifier.fillMaxWidth().semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
        horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically,
    ) {
        if (running) LoadingIndicator(Modifier.size(28.dp))
        else Icon(Icons.Rounded.TaskAlt, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.primary)
        Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** A card the check closed: what it asked, what took care of it, and Reopen. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun HandledCard(d: Decision) {
    val repo = LocalContainer.current.repository
    val scope = rememberCoroutineScope()
    var busy by remember(d.key) { mutableStateOf(false) }
    val h = d.handled ?: return
    Surface(shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(d.title, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            Text(h.reason, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    (h.evidence.take(3) + listOfNotNull(com.ryderdavid.nacre.ui.workspace.chat.agoText(h.at, System.currentTimeMillis()))).joinToString(" · "),
                    style = MaterialTheme.typography.labelMedium, fontFamily = FontFamily.Monospace, color = MaterialTheme.colorScheme.outline,
                    maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                )
                OutlinedButton(
                    onClick = {
                        busy = true
                        scope.launch {
                            runCatching { repo.reopenDecision(d) }.onFailure { repo.notify("Couldn't reopen it: ${it.message}") }
                            busy = false
                        }
                    },
                    enabled = !busy, shapes = ButtonDefaults.shapes(), modifier = Modifier.heightIn(min = 48.dp),
                    contentPadding = PaddingValues(start = 16.dp, end = 20.dp),
                ) {
                    Icon(Icons.AutoMirrored.Rounded.Undo, null, Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text("Reopen")
                }
            }
        }
    }
}
