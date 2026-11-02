package com.ryderdavid.nacre.ui.repos

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Inbox
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.BoardSnapshot
import com.ryderdavid.nacre.data.model.*
import com.ryderdavid.nacre.ui.bead.FoundSomethingSheet
import com.ryderdavid.nacre.ui.board.NewBeadSheet
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.nacreViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InboxScreen(
    onOpenBead: (String, String) -> Unit,
    onOpenChange: (String, String) -> Unit,
    onOpenRound: (String) -> Unit,
) {
    val vm = nacreViewModel { ReposViewModel(it.repository, it.settings) }
    val hosts by vm.hosts.collectAsState()
    val attention by vm.attention.collectAsState()
    val outdated by vm.outdated.collectAsState()
    val busy by vm.busy.collectAsState()
    var foundTarget by remember { mutableStateOf<Pair<String, AttentionTarget>?>(null) }
    var effort by remember { mutableStateOf<Pair<String, NewEffortPrefill>?>(null) }
    val onOption: (AttentionItem, AttentionOption) -> Unit = { item, o ->
        val t = o.target
        val key = Keys.of(item.hostId, t?.repo?.ifBlank { null } ?: item.repo)
        when {
            o.type == "action" -> if (o.disabled.isBlank()) vm.perform(item, o)
            t == null -> Unit
            t.kind == "bead" -> onOpenBead(key, t.id)
            t.kind == "change" -> onOpenChange(key, t.id)
            t.kind == "round" -> onOpenRound(Keys.of(item.hostId, t.id))
            t.kind == "found_something" -> foundTarget = key to t
            t.kind == "new_effort" -> effort = key to NewEffortPrefill(
                title = t.note.lineSequence().firstOrNull().orEmpty().take(80), description = t.note, related = t.related,
                followOnOf = t.followOnOf, foundComment = if (t.commentId.isNotBlank()) FoundComment(t.foundIn, t.commentId) else null,
            )
        }
    }
    Scaffold(topBar = { LargeTopAppBar(title = { Text("Inbox") }) }) { pad ->
        if (attention.isEmpty() && outdated.isEmpty()) MessageState(
            Icons.Rounded.Inbox, "You're all caught up", "Decisions and work needing your attention appear here.",
            modifier = Modifier.padding(pad),
        ) else Column(Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (outdated.isNotEmpty()) OutdatedNacredNote(outdated)
            NeedsYouSection(attention, busy, showComputers = hosts.size > 1, onOption = onOption, initiallyAll = true)
        }
    }
    foundTarget?.let { (key, t) -> FoundFromNeedsYou(key, t, vm, onDismiss = { foundTarget = null }, onOutOfScope = { foundTarget = null; effort = key to it }) }
    effort?.let { (key, prefill) -> EffortFromNeedsYou(key, prefill, vm, onDone = { effort = null }) }
}

/** Found something opened from Needs you: needs the change from the board's summary. */
@Composable
private fun FoundFromNeedsYou(key: String, t: AttentionTarget, vm: ReposViewModel, onDismiss: () -> Unit, onOutOfScope: (NewEffortPrefill) -> Unit) {
    val snapshot by produceState<BoardSnapshot?>(null, key) { value = runCatching { vm.board(key) }.getOrNull() }
    val change = snapshot?.changesById?.get(t.change) ?: return
    FoundSomethingSheet(key, change, foundIn = t.foundIn.ifBlank { null }, onDismiss = onDismiss, onOutOfScope = onOutOfScope, commentId = t.commentId, note = t.note)
}

/** New effort opened from Needs you or an out-of-scope answer, prefilled. */
@Composable
private fun EffortFromNeedsYou(key: String, prefill: NewEffortPrefill, vm: ReposViewModel, onDone: () -> Unit) {
    val snapshot by produceState<BoardSnapshot?>(null, key) { value = runCatching { vm.board(key) }.getOrNull() }
    val s = snapshot ?: return
    NewBeadSheet(
        key, s.repo.name, s.index, defaultParent = null, onDismiss = onDone, onCreated = { onDone() },
        effort = s.changes?.openspec == true, prefill = prefill,
    )
}

