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
 * The project-wide status strip: the project's running work, which opens the work inspector. What waits on
 * Ryder is not here: it is the "N need you" badge ([NeedsYouBadge]), which starts the walk-through in Chat.
 * In a medium window the strip is the top bar, so the badge sits at its end.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun StatusStrip(projectName: String, work: WorkSummary?, needsYou: Int, onWork: (WorkSummary) -> Unit, onNeedsYou: () -> Unit, modifier: Modifier = Modifier, repoKey: String? = null, compact: Boolean = false) {
    val repo = LocalContainer.current.repository
    val key = repoKey
    // Rounds and runs are the project's work; a chat that is replying is shown too, so "Nothing running" is true.
    val listing by repo.chats.collectAsState()
    val replying = key?.let { k -> listing.chats.count { c -> c.repo != null && Keys.of(c.hostId, c.repo) == k && c.state == com.ryderdavid.nacre.data.model.ChatWire.RUNNING } } ?: 0
    if (work == null && needsYou == 0 && replying == 0) return
    if (compact) {
        CompactStrip(projectName, work, needsYou, onWork, onNeedsYou, modifier)
        return
    }
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
            NeedsYouBadge(needsYou, onNeedsYou)
        }
    }
}

/**
 * What waits on Ryder, as one badge: "N need you" (skipped decisions don't count), hidden at zero.
 * Tapping it opens the project's Needs you chat and starts the walk-through there.
 */
@Composable
fun NeedsYouBadge(needsYou: Int, onClick: () -> Unit, modifier: Modifier = Modifier, short: Boolean = false) {
    if (needsYou <= 0) return
    val label = if (short) "$needsYou" else "$needsYou need${if (needsYou == 1) "s" else ""} you"
    Surface(
        onClick = onClick, shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.errorContainer,
        contentColor = MaterialTheme.colorScheme.onErrorContainer,
        modifier = modifier.heightIn(min = 48.dp).semantics { contentDescription = "$needsYou need you: go through them in Chat" },
    ) {
        Box(Modifier.padding(horizontal = 14.dp), contentAlignment = Alignment.Center) {
            Text(label, style = if (short) MaterialTheme.typography.titleMedium else MaterialTheme.typography.labelLarge)
        }
    }
}

/**
 * The strip folded into the rail on short windows: the work's state as an icon button and the Needs you
 * badge as a number, stacked under the chat switcher, each a 48dp target with the strip's own labels.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun CompactStrip(projectName: String, work: WorkSummary?, needsYou: Int, onWork: (WorkSummary) -> Unit, onNeedsYou: () -> Unit, modifier: Modifier) {
    androidx.compose.foundation.layout.Column(modifier, horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        if (work != null) {
            androidx.compose.material3.IconButton(
                onClick = { onWork(work) },
                modifier = Modifier.semantics { contentDescription = "$projectName, project-wide: ${ChatFirstData.statusText(work)}. Open the work inspector" },
            ) {
                when {
                    work.running && !work.stalled -> LoadingIndicator(Modifier.size(24.dp))
                    else -> Icon(Icons.Rounded.WarningAmber, null, tint = if (work.stopped || work.stalled) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        NeedsYouBadge(needsYou, onNeedsYou, short = true)
    }
}

/**
 * Starts the walk-through when a notification asked for it (recently, for this project), at the
 * decision it named.
 */
@Composable
fun QueueWatcher(repoKey: String?, onNeedsYou: (String?) -> Unit) {
    val repo = LocalContainer.current.repository
    val focus by repo.queueFocus.collectAsState()
    LaunchedEffect(focus, repoKey) {
        val f = focus ?: return@LaunchedEffect
        if (repoKey != null && f.open && f.repoKey == repoKey) {
            repo.queueOpened()
            if (System.currentTimeMillis() - f.at < QUEUE_REQUEST_MS) onNeedsYou(f.decisionId)
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

/**
 * Ask the chat again: opens the chat that raised an expired intake or found card, with the card attached
 * and the composer prefilled asking it to raise the card again; Ryder sends it. Null outside a workspace,
 * where the card doesn't offer it.
 */
val LocalAskChatAgain = androidx.compose.runtime.staticCompositionLocalOf<((com.ryderdavid.nacre.data.model.Decision) -> Unit)?> { null }

/** What the Spec or Beads pane is showing, published as the next message's Viewing context. */
val LocalViewing = androidx.compose.runtime.staticCompositionLocalOf<(ChatReference?) -> Unit> { {} }

/** Opens a decision's evidence: review and changes in Spec, a bead in Beads, a round, run or gates in the work inspector. */
val LocalOpenEvidence = androidx.compose.runtime.staticCompositionLocalOf<(com.ryderdavid.nacre.data.model.DecisionEvidence) -> Unit> { {} }

/**
 * Opens the project's Needs you chat and starts the walk-through there, at a decision when one is given
 * ("Talk it through" on a banner, a notification).
 */
val LocalOpenNeedsYou = androidx.compose.runtime.staticCompositionLocalOf<(String?) -> Unit> { {} }
