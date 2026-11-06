package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Chat
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.ViewKanban
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ViewKanban
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ShortNavigationBar
import androidx.compose.material3.ShortNavigationBarItem
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.WideNavigationRail
import androidx.compose.material3.WideNavigationRailItem
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/** What the workspace hosts in each place; sections 5–9 fill these in. */
class WorkspaceSlots(
    val chat: @Composable (Modifier) -> Unit,
    val spec: @Composable (Modifier) -> Unit,
    val beads: @Composable (Modifier) -> Unit,
    /** The project-wide status strip (work summary and Needs-you count). */
    val strip: @Composable (Modifier) -> Unit,
    /** The project switcher on wide layouts. */
    val switcher: @Composable () -> Unit,
)

private val Destination.selectedIcon: ImageVector
    get() = when (this) {
        Destination.CHAT -> Icons.AutoMirrored.Rounded.Chat
        Destination.SPEC -> Icons.Rounded.Description
        Destination.BEADS -> Icons.Rounded.ViewKanban
    }

private val Destination.icon: ImageVector
    get() = when (this) {
        Destination.CHAT -> Icons.AutoMirrored.Outlined.Chat
        Destination.SPEC -> Icons.Outlined.Description
        Destination.BEADS -> Icons.Outlined.ViewKanban
    }

/**
 * The workspace for one project checkout. On the cover screen one destination shows with the short
 * navigation bar; wider, the rail chooses Chat alone or Chat beside Spec or Beads.
 */
@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun Workspace(slots: WorkspaceSlots, destination: Destination, onDestination: (Destination) -> Unit, modifier: Modifier = Modifier) {
    var chatDp by rememberSaveable { mutableStateOf<Int?>(null) }
    var focus by rememberSaveable { mutableStateOf(false) }
    BoxWithConstraints(modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface)) {
        val w = maxWidth.value.toInt()
        val h = maxHeight.value.toInt()
        val layout = WorkspaceRules.layout(w, h, destination, chatAlone = focus, chatDp = chatDp)
        when (layout) {
            is WorkspaceLayout.Single -> if (!layout.rail) {
                Column(Modifier.fillMaxSize()) {
                    slots.strip(Modifier.padding(horizontal = 12.dp, vertical = 4.dp))
                    Box(Modifier.weight(1f).fillMaxWidth()) { content(slots, destination, Modifier.fillMaxSize()) }
                    if (!WindowInsets.isImeVisible) {
                        ShortNavigationBar {
                            Destination.entries.forEach { d ->
                                ShortNavigationBarItem(
                                    selected = d == destination, onClick = { onDestination(d) },
                                    icon = { Icon(if (d == destination) d.selectedIcon else d.icon, null) },
                                    label = { Text(d.label) },
                                )
                            }
                        }
                    }
                }
            } else {
                Row(Modifier.fillMaxSize()) {
                    Rail(slots, destination, focused = focus) { d -> if (d == destination && d != Destination.CHAT) focus = !focus else { focus = false; onDestination(d) } }
                    Column(Modifier.weight(1f).fillMaxHeight().padding(end = 8.dp, top = 8.dp, bottom = 8.dp)) {
                        slots.strip(Modifier.padding(bottom = 8.dp))
                        Box(Modifier.weight(1f).fillMaxWidth()) { content(slots, if (focus || destination == Destination.CHAT) destination else Destination.CHAT, Modifier.fillMaxSize()) }
                        if (focus && destination != Destination.CHAT) ChatBar(onExpand = { focus = false })
                    }
                }
            }
            is WorkspaceLayout.Split -> Row(Modifier.fillMaxSize()) {
                Rail(slots, destination, focused = false) { d -> if (d == destination && d != Destination.CHAT) focus = true else onDestination(d) }
                Column(Modifier.weight(1f).fillMaxHeight().padding(top = 8.dp, bottom = 8.dp)) {
                    slots.strip(Modifier.padding(end = 8.dp, bottom = 8.dp))
                    Row(Modifier.weight(1f)) {
                        Surface(Modifier.width(layout.chatDp.dp).fillMaxHeight(), shape = RoundedCornerShape(28.dp, 0.dp, 0.dp, 28.dp), color = MaterialTheme.colorScheme.surfaceContainerLow) {
                            slots.chat(Modifier.fillMaxSize())
                        }
                        PaneDivider(WorkspaceRules.usable(w), layout.chatDp, onChat = { chatDp = it }, onReset = { chatDp = null })
                        Surface(Modifier.width(layout.paneDp.dp).fillMaxHeight(), shape = RoundedCornerShape(0.dp, 28.dp, 28.dp, 0.dp), color = MaterialTheme.colorScheme.surfaceContainerLow) {
                            content(slots, destination, Modifier.fillMaxSize())
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun content(slots: WorkspaceSlots, d: Destination, modifier: Modifier) = when (d) {
    Destination.CHAT -> slots.chat(modifier)
    Destination.SPEC -> slots.spec(modifier)
    Destination.BEADS -> slots.beads(modifier)
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun Rail(slots: WorkspaceSlots, destination: Destination, focused: Boolean, onPick: (Destination) -> Unit) {
    WideNavigationRail(header = { slots.switcher() }) {
        Destination.entries.forEach { d ->
            WideNavigationRailItem(
                selected = d == destination, onClick = { onPick(d) },
                icon = { Icon(if (d == destination) d.selectedIcon else d.icon, null) },
                label = { Text(d.label) },
                railExpanded = false,
                modifier = Modifier.semantics {
                    if (d == destination && d != Destination.CHAT) contentDescription = if (focused) "${d.label}, focused. Tap to show chat beside it" else "${d.label}. Tap again to focus"
                },
            )
        }
    }
}

/** The divider between Chat and the pane: drag (clamped), and presets as accessibility actions. */
@Composable
private fun PaneDivider(usable: Int, chatDp: Int, onChat: (Int) -> Unit, onReset: () -> Unit) {
    val density = LocalDensity.current
    var drag by rememberSaveable { mutableStateOf(0f) }
    val state = rememberDraggableState { delta ->
        drag += with(density) { delta.toDp().value }
        onChat(WorkspaceRules.clamp(chatDp + drag.toInt(), usable)); drag = 0f
    }
    val actions = WorkspaceRules.offered(usable).map { p ->
        CustomAccessibilityAction(p.label) { WorkspaceRules.preset(p, usable)?.let(onChat); true }
    } + CustomAccessibilityAction("Reset") { onReset(); true }
    Box(
        Modifier.width(WorkspaceRules.DIVIDER.dp).fillMaxHeight()
            .draggable(state, Orientation.Horizontal)
            .semantics { contentDescription = "Resize panes"; customActions = actions },
        contentAlignment = Alignment.Center,
    ) {
        Box(Modifier.width(4.dp).height(48.dp).background(MaterialTheme.colorScheme.outline, RoundedCornerShape(2.dp)))
    }
}

/** Chat collapsed to one line while Spec or Beads has the whole content area. */
@Composable
private fun ChatBar(onExpand: () -> Unit) {
    Surface(
        onClick = onExpand, shape = RoundedCornerShape(32.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        shadowElevation = 3.dp, modifier = Modifier.fillMaxWidth().padding(top = 8.dp).heightIn(min = 64.dp),
    ) {
        Row(Modifier.padding(start = 20.dp, end = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Icon(Icons.AutoMirrored.Rounded.Chat, null)
            Text("Message the agent", Modifier.weight(1f), color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            FilledTonalIconButton(onClick = onExpand, modifier = Modifier.size(48.dp)) { Icon(Icons.Rounded.ExpandLess, "Expand chat") }
        }
        Spacer(Modifier.height(0.dp))
    }
}
