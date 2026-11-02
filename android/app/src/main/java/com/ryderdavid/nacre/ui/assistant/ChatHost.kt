package com.ryderdavid.nacre.ui.assistant

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ChevronRight
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.ChatContext
import com.ryderdavid.nacre.data.claude.ChatPresentation
import com.ryderdavid.nacre.ui.bead.BeadChatContent
import com.ryderdavid.nacre.ui.bead.BeadDetailViewModel
import com.ryderdavid.nacre.ui.common.*
import kotlinx.coroutines.launch

/** One header entry, including the screen's new chat and all active conversations. */
@Composable
fun ChatHeaderAction(
    screenContext: ChatContext,
    label: String,
    onSelect: ((ChatContext) -> Unit)? = null,
) {
    val container = LocalContainer.current
    val stack = container.chatStack
    val active by stack.active.collectAsState()
    var menu by remember(screenContext) { mutableStateOf(false) }
    Box {
        IconButton(onClick = { stack.expire(); menu = true }) {
            BadgedBox(badge = { if (active.isNotEmpty()) Badge { Text("${active.size}") } }) {
                Icon(AiIcon, "AI chat: $label")
            }
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            stack.slots(screenContext, label).forEach { slot ->
                DropdownMenuItem(
                    text = { Text(if (slot.isNew) "New · ${slot.label}" else slot.label) },
                    leadingIcon = { Icon(if (slot.isNew) Icons.Rounded.Add else AiIcon, null) },
                    trailingIcon = {
                        if (!slot.isNew) IconButton(onClick = { stack.dismiss(slot.context) }) {
                            Icon(Icons.Rounded.Close, "Dismiss chat: ${slot.label}")
                        }
                    },
                    onClick = {
                        stack.expire()
                        if (!stack.contains(slot.context)) container.resetChat(slot.context)
                        menu = false
                        if (onSelect != null) onSelect(slot.context) else stack.select(screenContext, slot.context)
                    },
                )
            }
        }
    }
}

/** A screen-local pane slot. Embedded bead detail supplies a header callback instead of a host. */
@Composable
fun ChatHost(
    screenContext: ChatContext,
    onOpenBead: (String, String) -> Unit,
    onOpenRun: (String) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    beadPaneOpen: Boolean = false,
    onCloseBead: () -> Unit = {},
    beadPane: (@Composable () -> Unit)? = null,
    onDevelop: (() -> Unit)? = null,
    content: @Composable () -> Unit,
) {
    if (!enabled) { content(); return }
    val container = LocalContainer.current
    val stack = container.chatStack
    val presentations by stack.presentations.collectAsState()
    val presentation = presentations[screenContext] ?: ChatPresentation()
    val prefs by container.settings.settings.collectAsState(initial = null)
    var root by remember { mutableStateOf(Rect.Zero) }
    BoxWithConstraints(modifier.fillMaxSize().onGloballyPositioned { root = it.boundsInWindow() }) {
        val wide = isPaneWide(maxWidth)
        val width = rememberPaneWidth(root, maxWidth, prefs?.detailPaneDp ?: 440)
        val layout = chatLayout(wide, maxWidth, width.width, presentation = presentation, beadPaneOpen = beadPaneOpen)
        val slot = boardSlot(layout, wide && beadPaneOpen)
        val back = chatBack(layout, wide && beadPaneOpen)
        BackHandler(layout == ChatLayout.Docked || (wide && beadPaneOpen && layout != ChatLayout.Sheet)) {
            when (back) {
                ChatBack.CollapseChat -> stack.collapse(screenContext)
                ChatBack.CloseBead -> onCloseBead()
                else -> Unit
            }
        }
        val close = { stack.close(screenContext) }
        val selected = presentation.selected
        Row(Modifier.fillMaxSize()) {
            Box(Modifier.weight(1f).fillMaxHeight()) { content() }
            if (slot != BoardSlot.None) {
                PaneDivider(onDrag = { width.resize(-it) }, onDragEnd = {
                    val dp = width.width.value.toInt()
                    container.scope.launch { container.settings.update { it.copy(detailPaneDp = dp) } }
                })
                Surface(Modifier.width(width.width).fillMaxHeight(), color = MaterialTheme.colorScheme.surfaceContainerLow) {
                    if (slot == BoardSlot.Chat && selected != null) Column(Modifier.fillMaxSize()) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                            IconButton(onClick = { stack.collapse(screenContext) }) {
                                Icon(Icons.Rounded.ChevronRight, "Collapse chat")
                            }
                        }
                        Box(Modifier.weight(1f)) {
                            ChatContent(selected, close, onOpenBead, onOpenRun, onDevelop?.takeIf { selected == screenContext })
                        }
                    } else beadPane?.invoke()
                }
            }
            if (layout == ChatLayout.EdgeTab) Surface(
                onClick = { stack.expand(screenContext) },
                modifier = Modifier.width(40.dp).fillMaxHeight(),
                color = MaterialTheme.colorScheme.surfaceContainerLow,
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
                    Icon(AiIcon, "Expand chat")
                    "Chat".forEach { Text(it.toString(), style = MaterialTheme.typography.labelMedium) }
                }
            }
        }
        if (layout == ChatLayout.Sheet && selected != null) HandleDismissSheet(close) {
            Box(Modifier.fillMaxWidth().fillMaxHeight(0.92f)) {
                ChatContent(selected, close, onOpenBead, onOpenRun, onDevelop?.takeIf { selected == screenContext })
            }
        }
    }
}

@Composable
fun ChatContent(
    selected: ChatContext,
    onClose: () -> Unit,
    onOpenBead: (String, String) -> Unit,
    onOpenRun: (String) -> Unit,
    onDevelop: (() -> Unit)? = null,
) {
    val container = LocalContainer.current
    val dismiss = { container.chatStack.dismiss(selected); onClose() }
    key(selected) {
        if (selected is ChatContext.Bead) {
            val vm = nacreViewModel(key = "chat:${selected.repoKey}:${selected.beadId}") {
                BeadDetailViewModel(it.repository, it.backends, it.settings, it.beadChats, selected.repoKey, selected.beadId)
            }
            val issue by vm.issue.collectAsState()
            val prefs by vm.prefs.collectAsState(initial = null)
            val bead = issue
            if (bead == null) LoadingState("Loading ${selected.beadId}…")
            else BeadChatContent(vm, bead, prefs?.claudeConfigured == true, onClose, dismiss, onDevelop, context = selected)
        } else {
            val target = if (selected is ChatContext.Board) AssistantTarget.Board(selected.repoKey, container.chatLabel(selected)) else AssistantTarget.AllBoards
            AssistantPane(target, inPane = true, onClose, onOpenBead, onOpenRun, dismiss)
        }
    }
}
