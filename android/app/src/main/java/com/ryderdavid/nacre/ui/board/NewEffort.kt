package com.ryderdavid.nacre.ui.board

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.chat.DraftContinuation
import com.ryderdavid.nacre.data.chat.NewEffortStart
import com.ryderdavid.nacre.data.chat.contextForChat
import com.ryderdavid.nacre.data.chat.newEffortStart
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.NewEffortPrefill
import com.ryderdavid.nacre.ui.common.LocalContainer

/**
 * The board's create action, and New effort opened prefilled (found work out of scope, a
 * follow-on change):
 * - in a repository with OpenSpec on a computer with Chat, a New effort chat on the board's
 *   computer, opened in the pane, the sheet or the Chat tab as the layout chooses;
 * - on an older nacred without `agent_chat`, the New effort sheet without a draft step;
 * - in a repository without OpenSpec, the New bead form.
 */
@Composable
fun NewEffortEntry(
    repoKey: String, repoName: String, index: BoardIndex?, defaultParent: String?, openspec: Boolean,
    onDismiss: () -> Unit, onCreated: (Issue) -> Unit, prefill: NewEffortPrefill? = null,
) {
    val container = LocalContainer.current
    val hosts by container.repository.hosts.collectAsState()
    val health = (hosts.firstOrNull { it.host.id == Keys.host(repoKey) }?.connection as? Connection.Online)?.health
    // Decided once: a health refresh mid-way doesn't swap the chat for the sheet.
    var start by remember(repoKey) { mutableStateOf(newEffortStart(openspec, health)) }
    when (start) {
        NewEffortStart.Chat -> LaunchedEffect(repoKey) {
            try {
                val hub = container.chats
                val ctx = contextForChat(hub.screens.atSend(), Keys.host(repoKey), container.repository.groups.value, container.repository::hostName).context
                hub.startNewEffort(repoKey, prefill, ctx)
                onDismiss()
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                // The chat couldn't start: New effort still works, without the draft step.
                container.repository.notify("Couldn't start a New effort chat: ${e.friendly()}")
                start = NewEffortStart.Sheet
            }
        }
        NewEffortStart.Sheet -> NewBeadSheet(repoKey, repoName, index, defaultParent, onDismiss, onCreated, effort = true, prefill = prefill)
        NewEffortStart.Form -> NewBeadSheet(repoKey, repoName, index, defaultParent, onDismiss, onCreated, effort = false, prefill = prefill)
    }
}

/**
 * Continue on a Draft card: the New effort sheet at its intake step with the draft and any
 * relation the prefill carried. In a repository without OpenSpec the draft continues into the
 * New bead form.
 */
@Composable
fun DraftContinueSheet(c: DraftContinuation, onDone: () -> Unit) {
    val container = LocalContainer.current
    val board by container.repository.board(c.repoKey).collectAsState()
    val s = board.data
    if (s == null) {
        board.error?.let { e -> LaunchedEffect(e) { container.repository.notify("Couldn't open the board: $e"); onDone() } }
        return
    }
    NewBeadSheet(
        c.repoKey, s.repo.name, s.index, defaultParent = null, onDismiss = onDone, onCreated = { onDone() },
        effort = s.changes?.openspec == true, prefill = c.prefill, draft = c.draft,
    )
}
