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
import kotlinx.coroutines.delay

/** The most recent chat in a checkout, or null when it has none. */
fun mostRecentThread(chats: List<ChatRecord>, repoKey: String): String? =
    chats.filter { it.repo != null && Keys.of(it.hostId, it.repo) == repoKey }.maxByOrNull { it.updatedAt.ifBlank { it.createdAt } }?.key

/**
 * The project-wide status strip: the work summary (opens the work inspector) and the Needs-you count (always
 * opens the queue) as two separate targets.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun StatusStrip(projectName: String, work: WorkSummary?, needsYou: Int, onWork: (WorkSummary) -> Unit, onQueue: () -> Unit, modifier: Modifier = Modifier) {
    if (work == null && needsYou == 0) return
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
                Text("$projectName", Modifier.weight(1f).padding(start = 10.dp), style = MaterialTheme.typography.labelLarge)
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

@Composable
fun projectStrip(repoKey: String): Pair<WorkSummary?, Int> {
    val repo = LocalContainer.current.repository
    val work by repo.work.collectAsState()
    val decisions by repo.decisions.collectAsState()
    return stripWork(work[repoKey].orEmpty()) to decisions[repoKey].orEmpty().count { it.pending || it.state == "held" }
}
