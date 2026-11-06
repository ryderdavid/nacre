package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.WarningAmber
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update

/** The most recent chat in a checkout, or null when it has none. */
fun mostRecentThread(chats: List<ChatRecord>, repoKey: String): String? =
    chats.filter { it.repo != null && Keys.of(it.hostId, it.repo) == repoKey }.maxByOrNull { it.updatedAt.ifBlank { it.createdAt } }?.key

/**
 * The project-wide status strip: the work summary (opens the work inspector) and the queue entry (always
 * opens the queue) as two separate targets. The entry is "N need you" while something waits on Ryder, and a
 * quieter "N in queue" while only deferred, running or unresolved outcomes are left, so those stay
 * reachable without counting as needing him.
 *
 * [repoKey] names the project; without it the strip uses the project whose workspace is on screen (the one
 * [projectStrip] was last read for). It also opens the queue when a notification asked for it.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun StatusStrip(projectName: String, work: WorkSummary?, needsYou: Int, onWork: (WorkSummary) -> Unit, onQueue: () -> Unit, modifier: Modifier = Modifier, repoKey: String? = null) {
    val repo = LocalContainer.current.repository
    val key = repoKey
    val decisions by repo.decisions.collectAsState()
    val inQueue = key?.let { DecisionModel.counts(decisions[it].orEmpty()).inQueue } ?: 0
    val focus by repo.queueFocus.collectAsState()
    LaunchedEffect(focus, key) {
        val f = focus ?: return@LaunchedEffect
        if (key != null && f.open && f.repoKey == key) {
            repo.queueOpened()
            if (System.currentTimeMillis() - f.at < QUEUE_REQUEST_MS) onQueue()
        }
    }
    // Rounds and runs are the project's work; a chat that is replying is shown too, so "Nothing running" is true.
    val listing by repo.chats.collectAsState()
    val replying = key?.let { k -> listing.chats.count { c -> c.repo != null && Keys.of(c.hostId, c.repo) == k && c.state == com.ryderdavid.nacre.data.model.ChatWire.RUNNING } } ?: 0
    if (work == null && needsYou == 0 && inQueue == 0 && replying == 0) return
    Surface(shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = modifier.fillMaxWidth()) {
        Row(Modifier.heightIn(min = 52.dp).padding(start = 6.dp, end = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            if (work != null) {
                Surface(
                    onClick = { onWork(work) }, shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
                    modifier = Modifier.weight(1f).heightIn(min = 48.dp).semantics { contentDescription = "$projectName, project-wide: ${ChatFirstData.statusText(work)}. Open the work inspector" },
                ) {
                    Row(Modifier.padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        when {
                            work.running && !work.stalled -> LoadingIndicator(Modifier.size(28.dp))
                            else -> Icon(Icons.Rounded.WarningAmber, null, tint = if (work.stopped || work.stalled) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Text(
                            ChatFirstData.statusText(work) + if (work.stalled) " · may be stalled" else "",
                            style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
            } else {
                Text(
                    when (replying) { 0 -> "Nothing running"; 1 -> "A chat is replying"; else -> "$replying chats replying" },
                    Modifier.weight(1f).padding(start = 10.dp), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (needsYou > 0) {
                Surface(
                    onClick = onQueue, shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.errorContainer,
                    contentColor = MaterialTheme.colorScheme.onErrorContainer,
                    modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "$needsYou need you: open the queue" },
                ) {
                    Box(Modifier.padding(horizontal = 14.dp), contentAlignment = Alignment.Center) {
                        Text("$needsYou need${if (needsYou == 1) "s" else ""} you", style = MaterialTheme.typography.labelLarge)
                    }
                }
            } else if (inQueue > 0) {
                Surface(
                    onClick = onQueue, shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest,
                    contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "$inQueue in the queue, none waiting on you: open the queue" },
                ) {
                    Box(Modifier.padding(horizontal = 14.dp), contentAlignment = Alignment.Center) {
                        Text("$inQueue in queue", style = MaterialTheme.typography.labelLarge)
                    }
                }
            }
        }
    }
}

/** Keeps a project's work and decisions fresh while its workspace is shown. */
@Composable
fun KeepFresh(repoKey: String) {
    val repo = LocalContainer.current.repository
    LaunchedEffect(repoKey) {
        while (true) {
            repo.refreshWork(repoKey)
            repo.refreshDecisions(repoKey)
            delay(15_000)
        }
    }
}

/** The open work shown in a project's strip: running first, then stopped or ready. */
fun stripWork(work: List<WorkSummary>): WorkSummary? =
    work.filter { it.open }.sortedWith(compareByDescending<WorkSummary> { it.stopped }.thenByDescending { it.running }).firstOrNull()

/** A notification's request to open the queue is honoured this long after it was made. */
private const val QUEUE_REQUEST_MS = 60_000L

/** A project's strip state: its open work and how many decisions wait on Ryder (pending or held; deferred doesn't count). */
@Composable
fun projectStrip(repoKey: String): Pair<WorkSummary?, Int> {
    val repo = LocalContainer.current.repository
    val work by repo.work.collectAsState()
    val decisions by repo.decisions.collectAsState()
    return stripWork(work[repoKey].orEmpty()) to DecisionModel.counts(decisions[repoKey].orEmpty()).needsYou
}

/** Something Ryder points Chat at: a bead, a change, a document, a requirement or a selection. */
data class ChatReference(
    /** bead, change, document, requirement, selection or round. */
    val kind: String,
    val id: String,
    /** What the chip shows, e.g. "NAC-axe.3" or "offline-chat-resume". */
    val label: String,
    /** Quoted text for a selection or requirement; empty otherwise. */
    val text: String = "",
)

/**
 * Ask about this: attaches a reference to the current thread's composer and shows Chat. Provided by the
 * workspace; Spec and Beads call it from their own actions.
 */
val LocalAskAbout = androidx.compose.runtime.staticCompositionLocalOf<(ChatReference) -> Unit> { {} }

/** What the Spec or Beads pane is showing, published as the next message's Viewing context. */
val LocalViewing = androidx.compose.runtime.staticCompositionLocalOf<(ChatReference?) -> Unit> { {} }

/** Opens a decision's evidence: review and changes in Spec, a bead in Beads, a round, run or gates in the work inspector. */
val LocalOpenEvidence = androidx.compose.runtime.staticCompositionLocalOf<(com.ryderdavid.nacre.data.model.DecisionEvidence) -> Unit> { {} }

/**
 * Opens the project's Needs-you queue (from a card that points at a newer request or held work). To open it
 * at one decision, point it there first with `repository.focusQueue(repoKey, decisionId)`.
 */
val LocalOpenQueue = androidx.compose.runtime.staticCompositionLocalOf<() -> Unit> { {} }
