package com.ryderdavid.nacre.ui.chat

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.chat.newChatLabel
import com.ryderdavid.nacre.data.claude.ChatPresentation
import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.ui.chat.ConversationFrame
import com.ryderdavid.nacre.ui.chat.ConversationScreen
import com.ryderdavid.nacre.ui.chat.NewChatRequest
import com.ryderdavid.nacre.ui.chat.NewChatSheet
import com.ryderdavid.nacre.ui.common.*
import kotlinx.coroutines.launch

/** What ✨ and the bead's chat entry do: open the current chat, or New when there is none. */
enum class ChatEntry { OpenCurrent, New }

/** Opens the current chat in the pane or the sheet; with none, the caller offers New. */
fun openChatEntry(stack: ChatStack): ChatEntry = if (stack.openCurrent()) ChatEntry.OpenCurrent else ChatEntry.New

/** One row of the ✨ menu. */
data class ChatMenuRow(val key: String?, val label: String, val current: Boolean = false)

/** The ✨ menu: New · <label> for the screen shown, then the recent chats, newest first. */
fun chatMenu(screenLabel: String, chats: List<ChatRecord>, current: String?, recent: Int = 5): List<ChatMenuRow> =
    listOf(ChatMenuRow(null, newChatLabel(screenLabel))) +
        chats.take(recent).map { ChatMenuRow(it.key, it.title.ifBlank { "Chat" }, it.key == current) }

/**
 * Opens the current chat (or New when there is none), hosting the New chat sheet. ✨ and the
 * bead's chat entry both use it, so they open the same chat.
 */
@Composable
fun rememberChatOpener(): (NewChatRequest?) -> Unit {
    val stack = LocalContainer.current.chats.stack
    var newChat by remember { mutableStateOf<NewChatRequest?>(null) }
    newChat?.let { req ->
        NewChatSheet(req, onDismiss = { newChat = null }, onCreated = { key -> newChat = null; stack.open(key) })
    }
    return remember(stack) {
        { req -> if (req != null) newChat = req else if (openChatEntry(stack) == ChatEntry.New) newChat = NewChatRequest() }
    }
}

/**
 * The ✨ header action. A tap opens the current chat, or New when there is none; a long press,
 * or a tap while the chat is already open, shows the menu of New · <label> and the recent chats.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ChatHeaderAction() {
    val container = LocalContainer.current
    val stack = container.chats.stack
    val current by stack.current.collectAsState()
    val presentation by stack.presentation.collectAsState()
    val shown by container.chats.screens.current.collectAsState()
    val listing by container.repository.chats.collectAsState()
    val opener = rememberChatOpener()
    var menu by remember { mutableStateOf(false) }
    val expanded = current != null && presentation.shown && !presentation.collapsed
    Box {
        Box(
            Modifier.size(48.dp).clip(CircleShape)
                .combinedClickable(role = Role.Button, onLongClick = { menu = true }) { if (expanded) menu = true else opener(null) }
                .semantics { contentDescription = "Chat: ${shown.label}" },
            contentAlignment = Alignment.Center,
        ) { Icon(AiIcon, null) }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            chatMenu(shown.label, listing.chats, current).forEach { row ->
                DropdownMenuItem(
                    text = { Text(row.label, maxLines = 1) },
                    leadingIcon = { Icon(if (row.key == null) Icons.Rounded.Add else AiIcon, null) },
                    trailingIcon = { if (row.current) Icon(Icons.Rounded.Check, "Current chat") },
                    onClick = {
                        menu = false
                        val key = row.key
                        if (key == null) opener(NewChatRequest()) else stack.open(key)
                    },
                )
            }
        }
    }
}

/**
 * Shows the current chat beside [content]: docked on wide layouts (the existing 760.dp check,
 * keeping 240.dp of body beside the pane), collapsed to an edge tab, or as a sheet on phones.
 * On a board, the chat and the bead detail take turns in one right pane slot. With [enabled]
 * false the content is shown alone, in the same place, so toggling it keeps the content's state.
 */
@Composable
fun ChatHost(
    onOpenBead: (String, String) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    beadPaneOpen: Boolean = false,
    onCloseBead: () -> Unit = {},
    beadPane: (@Composable () -> Unit)? = null,
    content: @Composable () -> Unit,
) {
    val container = LocalContainer.current
    val stack = container.chats.stack
    val current by stack.current.collectAsState()
    val presented by stack.presentation.collectAsState()
    val presentation = if (enabled && current != null) presented else ChatPresentation()
    val prefs by container.settings.settings.collectAsState(initial = null)
    val opener = rememberChatOpener()
    var root by remember { mutableStateOf(Rect.Zero) }
    BoxWithConstraints(modifier.fillMaxSize().onGloballyPositioned { root = it.boundsInWindow() }) {
        val wide = isPaneWide(maxWidth)
        val width = rememberPaneWidth(root, maxWidth, prefs?.detailPaneDp ?: 440)
        val layout = chatLayout(wide, maxWidth - 16.dp, width.width, presentation = presentation, beadPaneOpen = beadPaneOpen)
        val slot = boardSlot(layout, wide && beadPaneOpen)
        val back = chatBack(layout, wide && beadPaneOpen)
        BackHandler(enabled && (layout == ChatLayout.Docked || (wide && beadPaneOpen && layout != ChatLayout.Sheet))) {
            when (back) {
                ChatBack.CollapseChat -> stack.collapse()
                ChatBack.CloseBead -> onCloseBead()
                else -> Unit
            }
        }
        // Following a link closes the sheet on phones; the docked pane stays beside the new screen.
        val follow: (String, String) -> Unit = { repo, bead -> stack.followedLink(layout == ChatLayout.Docked); onOpenBead(repo, bead) }
        val key = current
        Row(Modifier.fillMaxSize()) {
            Box(Modifier.weight(1f).fillMaxHeight()) { content() }
            if (slot != BoardSlot.None) {
                PaneDivider(onDrag = { width.resize(-it) }, onDragEnd = {
                    val dp = width.width.value.toInt()
                    container.scope.launch { container.settings.update { it.copy(detailPaneDp = dp) } }
                })
                Surface(Modifier.width(width.width).fillMaxHeight(), color = MaterialTheme.colorScheme.surfaceContainerLow) {
                    if (slot == BoardSlot.Chat && key != null) {
                        ConversationScreen(
                            key, onBack = stack::close, onNew = { opener(it) }, frame = ConversationFrame.Pane,
                            onCollapse = stack::collapse, onOpenBead = follow,
                        )
                    } else beadPane?.invoke()
                }
            }
            if (layout == ChatLayout.EdgeTab) Surface(
                onClick = stack::expand,
                modifier = Modifier.width(40.dp).fillMaxHeight(),
                color = MaterialTheme.colorScheme.surfaceContainerLow,
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
                    Icon(AiIcon, "Expand chat")
                    "Chat".forEach { Text(it.toString(), style = MaterialTheme.typography.labelMedium) }
                }
            }
        }
        if (layout == ChatLayout.Sheet && key != null) HandleDismissSheet(stack::close) {
            Box(Modifier.fillMaxWidth().fillMaxHeight(0.92f)) {
                ConversationScreen(key, onBack = stack::close, onNew = { opener(it) }, frame = ConversationFrame.Sheet, onOpenBead = follow)
            }
        }
    }
}
