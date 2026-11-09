package com.ryderdavid.nacre.ui.workspace.beads

import androidx.compose.ui.semantics.contentDescription

import androidx.compose.material.icons.rounded.Comment

import androidx.compose.material.icons.rounded.AddComment

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Devices
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.PanTool
import androidx.compose.material.icons.rounded.Sync
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.elsewhere
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.theme.Nacre

/** Tonal fills: no outlines, so cards read by tone against the lane or pane behind them. */
@Composable
internal fun cardColor(inColumn: Boolean): Color {
    val dark = MaterialTheme.colorScheme.surface.luminance() < 0.5f
    return when {
        inColumn && dark -> MaterialTheme.colorScheme.surfaceContainerHighest
        inColumn -> MaterialTheme.colorScheme.surfaceContainerLowest
        dark -> MaterialTheme.colorScheme.surfaceContainerHigh
        else -> MaterialTheme.colorScheme.surfaceContainer
    }
}

/** One line under the title that says the bead's state in words, with an icon: never color alone. */
private data class TileStatus(val icon: ImageVector, val text: String, val color: @Composable () -> Color)

private fun tileStatus(issue: Issue, index: BoardIndex, running: Boolean): TileStatus? {
    val elsewhere = issue.elsewhere(liveRun = running)
    val blockers = if (issue.isClosed) emptyList() else index.openBlockers(issue)
    return when {
        running -> TileStatus(Icons.Rounded.Sync, "Working now", { MaterialTheme.colorScheme.primary })
        issue.awaitsSignoff -> TileStatus(Icons.Rounded.TaskAlt, "Waiting for your sign-off", { MaterialTheme.colorScheme.tertiary })
        issue.needsYou -> TileStatus(Icons.Rounded.PanTool, "Needs you", { MaterialTheme.colorScheme.error })
        elsewhere != null -> TileStatus(Icons.Rounded.Devices, elsewhere.title, { Nacre.colors.doing })
        blockers.isNotEmpty() -> TileStatus(
            Icons.Rounded.HourglassTop,
            "Waits on " + blockers.take(2).joinToString(", ") + if (blockers.size > 2) " +${blockers.size - 2}" else "",
            { MaterialTheme.colorScheme.onSurfaceVariant },
        )
        issue.status == "blocked" -> TileStatus(Icons.Rounded.Error, "Marked blocked", { MaterialTheme.colorScheme.onSurfaceVariant })
        else -> null
    }
}

@Composable
internal fun PriorityTag(priority: Int) {
    val urgent = priority <= 1
    Surface(
        shape = RoundedCornerShape(10.dp),
        color = if (urgent) MaterialTheme.colorScheme.tertiaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
        contentColor = if (urgent) MaterialTheme.colorScheme.onTertiaryContainer else MaterialTheme.colorScheme.onSurfaceVariant,
    ) {
        Text("P$priority", style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 7.dp, vertical = 1.dp))
    }
}

/**
 * The line above a child bead's title naming its parent: the parent's title when the board has it,
 * otherwise its id. Null for a bead without a parent.
 */
fun parentLine(issue: Issue, index: BoardIndex): String? {
    if (issue.parent.isBlank()) return null
    val title = index.byId[issue.parent]?.title?.trim().orEmpty()
    return if (title.isNotEmpty()) "In $title" else "In ${issue.parent}"
}

/**
 * A bead card: tonal, 24dp corners, no outline. Tapping opens the peek; long-press drags (wired by the
 * caller); the ⋮ menu and the TalkBack actions offer Move to… and Ask about this.
 */
@Composable
fun BeadTile(
    issue: Issue,
    index: BoardIndex,
    running: Boolean,
    selected: Boolean,
    inColumn: Boolean,
    /** Move to… is offered: not for proposals, which move by approval. */
    movable: Boolean,
    onOpen: () -> Unit,
    onMoveTo: () -> Unit,
    onAsk: () -> Unit,
    modifier: Modifier = Modifier,
    lifted: Boolean = false,
) {
    val status = tileStatus(issue, index, running)
    val shape = RoundedCornerShape(if (lifted) 28.dp else 24.dp)
    val actions = buildList {
        if (movable) add(CustomAccessibilityAction("Move to…") { onMoveTo(); true })
        add(CustomAccessibilityAction("Ask about this") { onAsk(); true })
    }
    Surface(
        shape = shape,
        color = when {
            selected -> MaterialTheme.colorScheme.secondaryContainer
            lifted -> MaterialTheme.colorScheme.surfaceContainerLowest
            else -> cardColor(inColumn)
        },
        shadowElevation = if (lifted) 12.dp else 0.dp,
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .clickable(onClickLabel = "Open", role = Role.Button, onClick = onOpen)
            .semantics { customActions = actions },
    ) {
        Column(Modifier.padding(start = 18.dp, end = 4.dp, top = 6.dp, bottom = 14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                BeadId(issue.id)
                PriorityTag(issue.priority)
                Text(issue.type, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, modifier = Modifier.weight(1f))
                if (lifted) Box(Modifier.size(48.dp)) else {
                    LocalCardAttach.current?.let { AttachButton(beadReference(issue.id), it) }
                    TileMenu(issue.id, movable, onMoveTo, onAsk)
                }
            }
            Column(Modifier.padding(end = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                // The parent says what a child like "12. Device checks (human)" is part of.
                parentLine(issue, index)?.let { line ->
                    Text(line, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                Text(
                    issue.title,
                    style = if (inColumn) MaterialTheme.typography.titleSmall else MaterialTheme.typography.titleMedium,
                    color = if (issue.isClosed) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                    maxLines = 4, overflow = TextOverflow.Ellipsis,
                )
                if (status != null) {
                    val c = status.color()
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(status.icon, null, Modifier.size(16.dp), tint = c)
                        Text(status.text, style = MaterialTheme.typography.labelMedium, color = c, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    }
                }
                if (issue.isEpic) {
                    val kids = index.descendants(issue.id)
                    if (kids.isNotEmpty()) Text(
                        "Epic · ${kids.count { it.isClosed }} of ${kids.size} done",
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun TileMenu(id: String, movable: Boolean, onMoveTo: () -> Unit, onAsk: () -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }) { Icon(Icons.Rounded.MoreVert, "More for $id") }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            if (movable) DropdownMenuItem(text = { Text("Move to…") }, onClick = { open = false; onMoveTo() })
            DropdownMenuItem(text = { Text("Ask about this") }, onClick = { open = false; onAsk() })
        }
    }
}

/** Move to…: the same moves as dragging, as a list of lanes. Done says that it closes the bead. */
@Composable
fun MoveToDialog(issue: Issue, from: Lane, onDismiss: () -> Unit, onPick: (Lane) -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Move ${issue.id} to") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(issue.title, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
                BeadMoves.targets(from).forEach { lane ->
                    MoveRow(lane, onClick = { onPick(lane) })
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun MoveRow(lane: Lane, onClick: () -> Unit, minHeight: Dp = 56.dp) {
    Surface(
        onClick = onClick, shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier.fillMaxWidth().heightIn(min = minHeight),
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 10.dp), verticalArrangement = Arrangement.Center) {
            Text(lane.title, style = MaterialTheme.typography.titleSmall)
            if (lane == Lane.DONE) Text("Closes the bead. It doesn't mean verified or signed off.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** On the board every card carries a send-to-chat button; null elsewhere, where cards keep Ask about this in their menu. */
class CardAttach(
    /** The `kind:id` of each reference attached to the next message. */
    val attached: Set<String>,
    /** Attaches the reference to the next message, or takes it off again. */
    val toggle: (com.ryderdavid.nacre.ui.workspace.ChatReference) -> Unit,
)

val LocalCardAttach = androidx.compose.runtime.compositionLocalOf<CardAttach?> { null }

/** Send to chat: attaches the card to the next message; filled while it is attached. */
@Composable
fun AttachButton(ref: com.ryderdavid.nacre.ui.workspace.ChatReference, attach: CardAttach, modifier: Modifier = Modifier) {
    val on = "${ref.kind}:${ref.id}" in attach.attached
    androidx.compose.material3.FilledIconToggleButton(
        checked = on, onCheckedChange = { attach.toggle(ref) },
        colors = androidx.compose.material3.IconButtonDefaults.filledIconToggleButtonColors(
            containerColor = Color.Transparent, contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        ),
        modifier = modifier.size(48.dp).semantics { contentDescription = if (on) "Take ${ref.label} off the next message" else "Send ${ref.label} to chat" },
    ) {
        Icon(if (on) Icons.Rounded.Comment else Icons.Rounded.AddComment, null)
    }
}
