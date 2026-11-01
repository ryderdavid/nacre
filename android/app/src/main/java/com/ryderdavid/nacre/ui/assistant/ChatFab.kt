package com.ryderdavid.nacre.ui.assistant

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.FloatingActionButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.ryderdavid.nacre.data.claude.ChatContext
import com.ryderdavid.nacre.ui.bead.BeadChatSheet
import com.ryderdavid.nacre.ui.bead.BeadDetailViewModel
import com.ryderdavid.nacre.ui.common.AiIcon
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.nacreViewModel

/** Shared entry point; place first in the screen's FAB column, above its primary action. */
@Composable
fun ChatFab(
    context: ChatContext,
    label: String,
    onOpenBead: (String, String) -> Unit,
    onOpenRun: (String) -> Unit,
    /** The current bead's "Develop a proposal instead…" action, shown in its own chat only. */
    onDevelop: (() -> Unit)? = null,
) {
    val container = LocalContainer.current
    val stack = container.chatStack
    val chats by stack.active.collectAsState()
    var expanded by remember(context) { mutableStateOf(false) }
    var selected by remember(context) { mutableStateOf<ChatContext?>(null) }
    var openedActive by remember(selected) { mutableStateOf(false) }
    val slots = stack.slots(context, label)

    // An expired conversation closes instead of silently sending into an expired backend session.
    LaunchedEffect(chats, selected) {
        selected?.let {
            if (stack.contains(it)) openedActive = true
            else if (openedActive) selected = null
        }
    }
    BackHandler(expanded) { expanded = false }

    fun open(target: ChatContext) {
        stack.expire()
        if (!stack.contains(target)) container.resetChat(target)
        expanded = false
        selected = target
    }

    Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        AnimatedVisibility(
            visible = expanded,
            enter = fadeIn() + expandVertically(expandFrom = Alignment.Bottom),
            exit = fadeOut() + shrinkVertically(shrinkTowards = Alignment.Bottom),
        ) {
            Surface(shape = RoundedCornerShape(24.dp), tonalElevation = 3.dp, shadowElevation = 6.dp) {
                LazyColumn(
                    modifier = Modifier.width(320.dp).heightIn(max = 360.dp),
                    contentPadding = PaddingValues(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    // Lazy list keys must be Bundle-saveable on Android.
                    items(slots, key = { it.context.toString() }) { slot ->
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            FilledTonalButton(onClick = { open(slot.context) }, modifier = Modifier.weight(1f)) {
                                Icon(if (slot.isNew) Icons.Rounded.Add else AiIcon, null, Modifier.size(20.dp))
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    if (slot.isNew) "New · ${slot.label}" else slot.label,
                                    maxLines = 2, overflow = TextOverflow.Ellipsis,
                                )
                            }
                            if (!slot.isNew) IconButton(onClick = { stack.dismiss(slot.context) }) {
                                Icon(Icons.Rounded.Close, "Dismiss chat: ${slot.label}")
                            }
                        }
                    }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (chats.isNotEmpty()) Surface(shape = RoundedCornerShape(12.dp), tonalElevation = 3.dp) {
                Text(
                    if (slots.first().isNew) "New · $label" else slots.first().label,
                    style = MaterialTheme.typography.labelMedium,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.widthIn(max = 180.dp).padding(horizontal = 12.dp, vertical = 6.dp),
                )
            }
            Box(Modifier.size(64.dp), contentAlignment = Alignment.BottomEnd) {
                // Offset Material surfaces expose the chats behind the current screen's top bubble.
                if (chats.size >= 2) Surface(
                    modifier = Modifier.size(56.dp).offset(x = (-8).dp, y = (-8).dp),
                    shape = FloatingActionButtonDefaults.shape,
                    color = MaterialTheme.colorScheme.tertiaryContainer, shadowElevation = 2.dp,
                ) {}
                if (chats.size >= 3) Surface(
                    modifier = Modifier.size(56.dp).offset(x = (-4).dp, y = (-4).dp),
                    shape = FloatingActionButtonDefaults.shape,
                    color = MaterialTheme.colorScheme.secondaryContainer, shadowElevation = 2.dp,
                ) {}
                FloatingActionButton(
                    onClick = {
                        stack.expire()
                        if (expanded) expanded = false
                        else if (stack.shouldExpand(context)) expanded = true
                        else open(context)
                    },
                    modifier = Modifier.semantics {
                        contentDescription = "AI chat: ${slots.first().label}, ${chats.size} active chats"
                    },
                ) {
                    Icon(if (expanded) Icons.Rounded.Close else AiIcon, if (expanded) "Close chat stack" else "Open chat: $label")
                }
            }
        }
    }
    selected?.let { target ->
        key(target) {
            ChatWindow(target, onClose = { selected = null }, onOpenBead = { repo, bead ->
                selected = null
                onOpenBead(repo, bead)
            }, onOpenRun = {
                selected = null
                onOpenRun(it)
            }, onDevelop = onDevelop?.takeIf { target == context }?.let { develop -> { selected = null; develop() } })
        }
    }
}

@Composable
private fun ChatWindow(
    context: ChatContext,
    onClose: () -> Unit,
    onOpenBead: (String, String) -> Unit,
    onOpenRun: (String) -> Unit,
    onDevelop: (() -> Unit)? = null,
) {
    val container = LocalContainer.current
    val dismiss = { container.chatStack.dismiss(context); onClose() }
    if (context is ChatContext.Bead) {
        val vm = nacreViewModel(key = "chat:${context.repoKey}:${context.beadId}") {
            BeadDetailViewModel(it.repository, it.backends, it.settings, it.beadChats, context.repoKey, context.beadId)
        }
        val issue by vm.issue.collectAsState()
        val prefs by vm.prefs.collectAsState(initial = null)
        val bead = issue
        if (bead != null) BeadChatSheet(vm, bead, prefs?.claudeConfigured == true, onClose, onDismissChat = dismiss, onDevelop = onDevelop)
        else Dialog(onDismissRequest = onClose) { Surface { LoadingState("Loading ${context.beadId}…") } }
    } else {
        val target = if (context is ChatContext.Board) AssistantTarget.Board(context.repoKey, container.chatLabel(context))
            else AssistantTarget.AllBoards
        Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
            Surface(Modifier.fillMaxSize()) {
                AssistantPane(
                    target = target,
                    inPane = false, onClose = onClose, onOpenBead = onOpenBead, onOpenRun = onOpenRun,
                    onDismissChat = dismiss,
                )
            }
        }
    }
}
