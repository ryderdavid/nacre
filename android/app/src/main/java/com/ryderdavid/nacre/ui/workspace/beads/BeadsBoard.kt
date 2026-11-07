package com.ryderdavid.nacre.ui.workspace.beads

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.Column
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.board.LaneItem
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.board.BoardUi
import com.ryderdavid.nacre.ui.board.DragController
import com.ryderdavid.nacre.ui.board.DraftHandlers
import com.ryderdavid.nacre.ui.board.DraftPlaceholderCard
import com.ryderdavid.nacre.ui.board.dragSource
import com.ryderdavid.nacre.ui.board.dropTarget
import com.ryderdavid.nacre.ui.board.laneWithMatches
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/** What a lane's cards can do; shared by the compact board and the columns. */
class CardHandlers(
    val selectedId: String?,
    val onOpen: (Issue) -> Unit,
    val onMoveTo: (Issue, Lane) -> Unit,
    val onAsk: (Issue) -> Unit,
    val onCloseCovered: (Issue) -> Unit,
    val onShowMoreDone: () -> Unit,
    val drafts: DraftHandlers,
)

/** A lane chip: the selected lane is a filled secondary pill; each carries its count. */
@Composable
private fun LaneChip(col: Column, selected: Boolean, onClick: () -> Unit) {
    val radius by animateDpAsState(if (selected) 20.dp else 12.dp, label = "lane")
    val colors = MaterialTheme.colorScheme
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(radius),
        color = if (selected) colors.secondary else colors.surfaceContainerHigh,
        contentColor = if (selected) colors.onSecondary else colors.onSurfaceVariant,
        modifier = Modifier
            .minimumInteractiveComponentSize()
            .heightIn(min = 40.dp)
            .semantics {
                role = Role.Tab
                this.selected = selected
                contentDescription = "${col.title}, ${col.total} ${if (col.total == 1) "bead" else "beads"}"
            },
    ) {
        Row(Modifier.padding(horizontal = 14.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(col.title, style = MaterialTheme.typography.labelLarge, maxLines = 1)
            Text("${col.total}", style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold))
        }
    }
}

/** Lane chips with counts, and a menu of every lane for when the row scrolls. */
@Composable
fun LaneChips(columns: List<Column>, selectedKey: String, onSelect: (String) -> Unit, modifier: Modifier = Modifier) {
    val row = rememberLazyListState()
    val selected = columns.indexOfFirst { it.key == selectedKey }
    LaunchedEffect(selected) { if (selected >= 0) row.animateScrollToItem((selected - 1).coerceAtLeast(0)) }
    Row(modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        LazyRow(
            state = row, modifier = Modifier.weight(1f).semantics { contentDescription = "Lanes" },
            contentPadding = PaddingValues(horizontal = 12.dp), horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            items(columns, key = { it.key }) { col -> LaneChip(col, col.key == selectedKey) { onSelect(col.key) } }
        }
        var all by remember { mutableStateOf(false) }
        Box(Modifier.padding(end = 8.dp)) {
            FilledTonalIconButton(onClick = { all = true }, shape = RoundedCornerShape(16.dp)) { Icon(Icons.Rounded.ExpandMore, "All lanes") }
            DropdownMenu(expanded = all, onDismissRequest = { all = false }) {
                columns.forEach { col ->
                    DropdownMenuItem(text = { Text("${col.title} · ${col.total}") }, onClick = { all = false; onSelect(col.key) })
                }
            }
        }
    }
}

/** One lane's cards. While a card is dragged its slot stays, as a tonal shape where it was. */
@Composable
private fun LaneCards(
    col: Column,
    ui: BoardUi,
    drag: DragController,
    h: CardHandlers,
    inColumn: Boolean,
    contentPadding: PaddingValues,
    modifier: Modifier = Modifier,
    /** Other lanes with matches, offered when this one is empty under a search. */
    elsewhere: List<Column> = emptyList(),
    onReveal: (Column) -> Unit = {},
) {
    val lane = BeadMoves.laneByKey(col.key) ?: Lane.READY
    if (col.items.isEmpty()) {
        Column(modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                if (lane == Lane.PROPOSED) "No changes being planned" else "Nothing in ${col.title}",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
            )
            elsewhere.forEach { other -> TextButton(onClick = { onReveal(other) }) { Text("${other.total} in ${other.title}") } }
        }
        return
    }
    val index = ui.snapshot.index
    LazyColumn(modifier, contentPadding = contentPadding, verticalArrangement = Arrangement.spacedBy(if (inColumn) 8.dp else 10.dp)) {
        items(col.items, key = { it.key }) { item ->
            if (item is LaneItem.DraftPlaceholder) {
                DraftPlaceholderCard(item, h.drafts, Modifier.animateItem())
                return@items
            }
            val issue = (item as LaneItem.Card).issue
            val movable = item.draggable && BeadMoves.canMove(lane)
            val dragged = drag.dragging?.id == issue.id
            Column(Modifier.animateItem()) {
                Box(
                    if (dragged) Modifier.background(MaterialTheme.colorScheme.surfaceContainerHighest.copy(alpha = 0.7f), RoundedCornerShape(24.dp)) else Modifier,
                ) {
                    BeadTile(
                        issue, index, running = issue.id in ui.running, selected = issue.id == h.selectedId, inColumn = inColumn, movable = movable,
                        onOpen = { h.onOpen(issue) }, onMoveTo = { h.onMoveTo(issue, lane) }, onAsk = { h.onAsk(issue) },
                        modifier = Modifier.alpha(if (dragged) 0f else 1f).then(if (movable) Modifier.dragSource(drag, issue, col.key) else Modifier),
                    )
                }
                // A proposal judged already covered is usually closed; offer that right under its card.
                if (lane == Lane.PROPOSED && index.proposalOf(issue, ui.snapshot.changes)?.phase == "covered") {
                    TextButton(onClick = { h.onCloseCovered(issue) }) { Text("Close ${issue.id} as covered") }
                }
            }
        }
        if (lane == Lane.DONE && col.total > col.issues.size) item {
            TextButton(onClick = h.onShowMoreDone, modifier = Modifier.fillMaxWidth()) { Text("Show ${col.total - col.issues.size} more") }
        }
    }
}

/**
 * The compact board: lane chips with counts and one lane at a time (swipe or pick a chip). Dragging a card
 * raises the drop toolbar, which names the lanes it can go to.
 */
@Composable
fun CompactBeads(ui: BoardUi, laneKey: String, onLane: (String) -> Unit, drag: DragController, query: String, h: CardHandlers, modifier: Modifier = Modifier) {
    val cols = ui.columns
    val start = cols.indexOfFirst { it.key == laneKey }.coerceAtLeast(0)
    val pager = rememberPagerState(initialPage = start) { cols.size }
    val scope = rememberCoroutineScope()

    // The chips and the pager follow each other.
    LaunchedEffect(laneKey, cols.size) {
        val i = cols.indexOfFirst { it.key == laneKey }
        if (i >= 0 && i != pager.currentPage) pager.animateScrollToPage(i)
    }
    LaunchedEffect(pager, cols) {
        snapshotFlow { pager.settledPage }.collect { p -> cols.getOrNull(p)?.let { if (it.key != laneKey) onLane(it.key) } }
    }
    // A search that misses this lane moves to the nearest lane it matches.
    LaunchedEffect(query, cols.map { it.total }) {
        if (query.isBlank()) return@LaunchedEffect
        laneWithMatches(cols, pager.targetPage)?.let { i -> cols.getOrNull(i)?.let { scope.launch { onLane(it.key) } } }
    }

    Box(modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            LaneChips(cols, laneKey, onLane, Modifier.padding(vertical = 4.dp))
            HorizontalPager(state = pager, modifier = Modifier.fillMaxSize(), beyondViewportPageCount = 1, key = { cols[it].key }) { page ->
                val col = cols[page]
                LaneCards(
                    col, ui, drag, h, inColumn = false,
                    contentPadding = PaddingValues(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 160.dp),
                    modifier = Modifier.fillMaxSize(),
                    elsewhere = if (query.isBlank()) emptyList() else cols.filter { it.key != col.key && it.total > 0 },
                    onReveal = { onLane(it.key) },
                )
            }
        }
        AnimatedVisibility(
            visible = drag.isDragging,
            enter = slideInVertically { it } + fadeIn(),
            exit = slideOutVertically { it } + fadeOut(),
            modifier = Modifier.align(Alignment.BottomCenter),
        ) {
            DropToolbar(drag)
        }
    }
}

/**
 * The drop targets in a vibrant floating toolbar (primary container). The target under the finger becomes a
 * filled primary pill. Proposals and To check aren't offered.
 */
@Composable
private fun DropToolbar(drag: DragController) {
    val issue = drag.dragging
    val from = BeadMoves.laneByKey(drag.sourceKey)
    val colors = MaterialTheme.colorScheme
    Surface(
        color = colors.primaryContainer, contentColor = colors.onPrimaryContainer,
        shape = RoundedCornerShape(32.dp), shadowElevation = 6.dp,
        modifier = Modifier.navigationBarsPadding().padding(start = 12.dp, end = 12.dp, bottom = 12.dp).fillMaxWidth()
            .semantics { contentDescription = "Drop to move ${issue?.id.orEmpty()}" },
    ) {
        Column(Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Drop to move ${issue?.id.orEmpty()}", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(start = 8.dp))
            val targets = BeadMoves.targets(from)
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.fillMaxWidth()) {
                targets.forEach { lane ->
                    val over = drag.hoverKey == lane.key
                    val radius by animateDpAsState(if (over) 32.dp else 20.dp, label = "drop")
                    Surface(
                        shape = RoundedCornerShape(radius),
                        color = if (over) colors.primary else colors.surface,
                        contentColor = if (over) colors.onPrimary else colors.onSurface,
                        modifier = Modifier.weight(1f).heightIn(min = 64.dp).dropTarget(drag, lane.key),
                    ) {
                        Column(Modifier.padding(horizontal = 4.dp, vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
                            Text(
                                lane.title, style = MaterialTheme.typography.labelLarge.copy(fontWeight = if (over) FontWeight.ExtraBold else FontWeight.SemiBold),
                                textAlign = TextAlign.Center, maxLines = 2, overflow = TextOverflow.Ellipsis,
                            )
                            if (lane == Lane.DONE) Text("closes", style = MaterialTheme.typography.labelSmall)
                        }
                    }
                }
            }
        }
    }
}

/**
 * Wide or focused panes: the lanes as columns at least 272dp wide, scrolling sideways. Each column that takes
 * beads is a drop target; the one under the finger fills with primary container.
 */
@Composable
fun ColumnBeads(ui: BoardUi, widthDp: Int, row: LazyListState, drag: DragController, h: CardHandlers, modifier: Modifier = Modifier, leading: (@Composable () -> Unit)? = null) {
    var viewport by remember { mutableStateOf(Rect.Zero) }
    // Edge auto-scroll while a card is dragged near the left or right edge.
    LaunchedEffect(drag.isDragging) {
        while (drag.isDragging) {
            val x = drag.pointer.x
            val edge = 96f
            val step = when {
                x < viewport.left + edge -> -24f
                x > viewport.right - edge -> 24f
                else -> 0f
            }
            if (step != 0f) {
                row.scrollBy(step)
                drag.drag(Offset.Zero)
            }
            delay(16)
        }
    }
    val colWidth = BeadsLayout.columnWidth(widthDp, ui.columns.size).dp
    LazyRow(
        state = row,
        contentPadding = PaddingValues(horizontal = BeadsLayout.EDGE.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(BeadsLayout.GAP.dp),
        modifier = modifier.fillMaxSize().onGloballyPositioned { viewport = it.boundsInWindow() }.semantics { contentDescription = "Lanes, scrolls sideways" },
    ) {
        if (leading != null) item(key = "leading") { leading() }
        itemsIndexed(ui.columns, key = { _, c -> c.key }) { _, col ->
            val lane = BeadMoves.laneByKey(col.key) ?: Lane.READY
            val accepts = BeadMoves.accepts(lane)
            val over = drag.isDragging && drag.hoverKey == col.key && drag.sourceKey != col.key
            val colors = MaterialTheme.colorScheme
            Surface(
                shape = RoundedCornerShape(28.dp),
                color = if (over) colors.primaryContainer else colors.surfaceContainer,
                modifier = Modifier.width(colWidth).fillMaxHeight()
                    .alpha(if (drag.isDragging && !accepts) 0.5f else 1f)
                    .then(if (accepts) Modifier.dropTarget(drag, col.key) else Modifier),
            ) {
                Column {
                    Row(Modifier.fillMaxWidth().padding(start = 18.dp, end = 12.dp, top = 14.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(col.title, style = MaterialTheme.typography.titleMediumEmphasized, modifier = Modifier.semantics { heading() })
                            if (col.note.isNotBlank()) Text(col.note, style = MaterialTheme.typography.labelSmall, color = colors.onSurfaceVariant)
                            if (over) Text(if (lane == Lane.DONE) "Drop to close" else "Drop to move here", style = MaterialTheme.typography.labelMedium)
                        }
                        Surface(shape = RoundedCornerShape(12.dp), color = colors.secondaryContainer, contentColor = colors.onSecondaryContainer) {
                            Text("${col.total}", style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 2.dp))
                        }
                    }
                    LaneCards(col, ui, drag, h, inColumn = true, contentPadding = PaddingValues(start = 8.dp, end = 8.dp, bottom = 96.dp))
                }
            }
        }
    }
}

/** The lifted card that follows the finger: scaled up with a shadow, no tilt. */
@Composable
fun LiftedCard(drag: DragController, ui: BoardUi?, rootOffset: Offset, inColumn: Boolean) {
    val issue = drag.dragging ?: return
    ui ?: return
    val density = LocalDensity.current
    val topLeft = drag.pointer - drag.grabOffset - rootOffset
    Box(
        Modifier
            .offset { IntOffset(topLeft.x.roundToInt(), topLeft.y.roundToInt()) }
            .width(with(density) { drag.cardSize.width.toDp() })
            .graphicsLayer { scaleX = 1.03f; scaleY = 1.03f },
    ) {
        BeadTile(issue, ui.snapshot.index, running = issue.id in ui.running, selected = false, inColumn = inColumn, movable = false, onOpen = {}, onMoveTo = {}, onAsk = {}, lifted = true)
    }
}
