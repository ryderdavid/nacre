package com.ryderdavid.nacre.ui.board

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Inbox
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PrimaryScrollableTabRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Column
import com.ryderdavid.nacre.data.board.GroupBy
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.common.Dot
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

@Composable
fun columnColor(key: String) =
    Lane.entries.firstOrNull { it.key == key }?.let { Nacre.colors.lane(it) } ?: MaterialTheme.colorScheme.outline

/** "All work", one chip per open epic with its progress, and "Not in an epic". */
@Composable
fun EpicChips(ui: BoardUi, filter: BoardFilter, onEpic: (String) -> Unit, modifier: Modifier = Modifier) {
    LazyRow(modifier, contentPadding = PaddingValues(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        item {
            val total = ui.snapshot.index.issues.size
            val done = ui.snapshot.index.issues.count { it.isClosed }
            EpicChip("All work", done, total, selected = filter.epic.isEmpty()) { onEpic("") }
        }
        items(ui.epics, key = { it.id }) { e ->
            EpicChip(e.title, e.done, e.total, selected = filter.epic == e.id) { onEpic(e.id) }
        }
        if (ui.epics.isNotEmpty() && ui.looseCount > 0) item {
            FilterChip(selected = filter.epic == BoardFilter.NO_EPIC, onClick = { onEpic(BoardFilter.NO_EPIC) }, label = { Text("Not in an epic") })
        }
    }
}

@Composable
private fun EpicChip(title: String, done: Int, total: Int, selected: Boolean, onClick: () -> Unit) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.widthIn(max = 220.dp).padding(end = 8.dp))
                LinearProgressIndicator(
                    progress = { if (total == 0) 0f else done.toFloat() / total },
                    strokeCap = StrokeCap.Round,
                    gapSize = 0.dp,
                    drawStopIndicator = {},
                    modifier = Modifier.width(32.dp).height(5.dp),
                )
            }
        },
    )
}

@Composable
private fun ColumnHeader(col: Column, groupBy: GroupBy) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        Dot(columnColor(col.key), 10)
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(col.title, style = MaterialTheme.typography.titleSmall, maxLines = 1)
            if (col.note.isNotBlank()) Text(col.note, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
        }
        Text("${col.total}", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun CardList(
    col: Column,
    index: BoardIndex,
    ui: BoardUi,
    selectedId: String?,
    drag: DragController,
    onOpen: (Issue) -> Unit,
    onShowMore: (() -> Unit)?,
    contentPadding: PaddingValues,
    modifier: Modifier = Modifier,
    onClose: (Issue) -> Unit = {},
    /** Other lanes that have matches, offered when this one is empty under a search. */
    elsewhere: List<Column> = emptyList(),
    onReveal: (Column) -> Unit = {},
) {
    if (col.issues.isEmpty()) {
        Column(modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Icon(Icons.Rounded.Inbox, null, tint = MaterialTheme.colorScheme.outlineVariant)
            Spacer(Modifier.height(6.dp))
            Text(
                if (col.key == Lane.PROPOSED.key) "No changes being planned. Start a New effort." else "Nothing here for this filter",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            elsewhere.forEach { other ->
                TextButton(onClick = { onReveal(other) }) { Text("${other.total} in ${other.title}") }
            }
        }
        return
    }
    LazyColumn(modifier, contentPadding = contentPadding, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        items(col.issues, key = { it.id }) { issue ->
            val isDragged = drag.dragging?.id == issue.id
            Column(Modifier.animateItem()) {
                BeadCard(
                    issue = issue,
                    index = index,
                    running = issue.id in ui.running,
                    selected = issue.id == selectedId,
                    changes = ui.snapshot.changes,
                    modifier = Modifier
                        .alpha(if (isDragged) 0.3f else 1f)
                        // Proposals move by approval, not by dragging.
                        .then(if (col.key == Lane.PROPOSED.key) Modifier else Modifier.dragSource(drag, issue, col.key))
                        .clickable { onOpen(issue) },
                )
                // A proposal judged already covered is usually closed; offer that right on the card.
                if (col.key == Lane.PROPOSED.key && index.proposalOf(issue, ui.snapshot.changes)?.phase == "covered") {
                    TextButton(onClick = { onClose(issue) }) { Text("Close ${issue.id} as covered") }
                }
            }
        }
        if (onShowMore != null && col.total > col.issues.size) item {
            TextButton(onClick = onShowMore, modifier = Modifier.fillMaxWidth()) { Text("Show ${col.total - col.issues.size} more") }
        }
    }
}

/** Unfolded / tablet: columns side by side, scrolling horizontally, each a drop target. */
@Composable
fun WideBoard(
    ui: BoardUi,
    groupBy: GroupBy,
    selectedId: String?,
    drag: DragController,
    scrollToLane: Lane?,
    onScrolled: () -> Unit,
    onOpen: (Issue) -> Unit,
    onShowMoreDone: () -> Unit,
    modifier: Modifier = Modifier,
    onClose: (Issue) -> Unit = {},
) {
    val rowState = rememberLazyListState()
    var viewport by remember { mutableStateOf(Rect.Zero) }
    val index = ui.snapshot.index

    LaunchedEffect(scrollToLane) {
        if (scrollToLane != null) {
            val i = ui.columns.indexOfFirst { it.key == scrollToLane.key }
            if (i >= 0) rowState.animateScrollToItem(i)
            onScrolled()
        }
    }
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
                rowState.scrollBy(step)
                drag.drag(Offset.Zero)
            }
            delay(16)
        }
    }

    BoxWithConstraints(modifier.fillMaxSize().onGloballyPositioned { viewport = it.boundsInWindow() }) {
        val n = ui.columns.size.coerceAtLeast(1)
        val colWidth = ((maxWidth - 16.dp * 2 - 12.dp * (n - 1)) / n).coerceIn(272.dp, 340.dp)
        LazyRow(
            state = rowState,
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier.fillMaxSize(),
        ) {
            items(ui.columns, key = { it.key }) { col ->
                val hovered = drag.isDragging && drag.hoverKey == col.key && drag.sourceKey != col.key
                Surface(
                    shape = RoundedCornerShape(24.dp),
                    color = if (hovered) MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.55f) else MaterialTheme.colorScheme.surfaceContainer,
                    border = if (hovered) BorderStroke(2.dp, columnColor(col.key)) else null,
                    modifier = Modifier.width(colWidth).fillMaxHeight().then(if (col.key == Lane.PROPOSED.key || col.key == Lane.CHECK.key) Modifier else Modifier.dropTarget(drag, col.key)),
                ) {
                    Column {
                        ColumnHeader(col, groupBy)
                        CardList(
                            col, index, ui, selectedId, drag, onOpen,
                            onShowMore = if (col.key == Lane.DONE.key) onShowMoreDone else null,
                            contentPadding = PaddingValues(start = 10.dp, end = 10.dp, bottom = 96.dp),
                            onClose = onClose,
                        )
                    }
                }
            }
        }
    }
}

/** Cover screen: one lane at a time with tabs; dragging a card reveals a dock of lanes. */
@Composable
fun CompactBoard(
    ui: BoardUi,
    groupBy: GroupBy,
    drag: DragController,
    scrollToLane: Lane?,
    onScrolled: () -> Unit,
    onOpen: (Issue) -> Unit,
    onShowMoreDone: () -> Unit,
    modifier: Modifier = Modifier,
    onClose: (Issue) -> Unit = {},
    /** The board's search; a new search that misses this lane moves to a lane it matches (NAC-3mc). */
    query: String = "",
) {
    // The pager opens on Ready even when Proposals comes first.
    val pager = rememberPagerState(initialPage = ui.columns.indexOfFirst { it.key == Lane.READY.key }.coerceAtLeast(0)) { ui.columns.size }
    val scope = rememberCoroutineScope()
    val index = ui.snapshot.index

    // Keyed on the totals too: the filtered columns arrive a moment after the query changes.
    LaunchedEffect(query, ui.columns.map { it.total }) {
        if (query.isBlank()) return@LaunchedEffect
        // Launched outside the effect: a restart as the totals settle must not stop the pager between lanes.
        laneWithMatches(ui.columns, pager.targetPage)?.let { scope.launch { pager.animateScrollToPage(it) } }
    }

    LaunchedEffect(scrollToLane) {
        if (scrollToLane != null) {
            val i = ui.columns.indexOfFirst { it.key == scrollToLane.key }
            if (i >= 0) pager.animateScrollToPage(i)
            onScrolled()
        }
    }

    Box(modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            PrimaryScrollableTabRow(selectedTabIndex = pager.currentPage.coerceAtMost(ui.columns.lastIndex.coerceAtLeast(0)), edgePadding = 12.dp, divider = {}) {
                ui.columns.forEachIndexed { i, col ->
                    Tab(
                        selected = pager.currentPage == i,
                        onClick = { scope.launch { pager.animateScrollToPage(i) } },
                        text = {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Dot(columnColor(col.key))
                                Spacer(Modifier.width(6.dp))
                                Text(col.title, maxLines = 1)
                                Spacer(Modifier.width(6.dp))
                                Text("${col.total}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        },
                    )
                }
            }
            HorizontalPager(state = pager, modifier = Modifier.fillMaxSize(), beyondViewportPageCount = 1, key = { ui.columns[it].key }) { page ->
                val col = ui.columns[page]
                CardList(
                    col, index, ui, null, drag, onOpen,
                    onShowMore = if (col.key == Lane.DONE.key) onShowMoreDone else null,
                    contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 120.dp),
                    modifier = Modifier.fillMaxSize(),
                    onClose = onClose,
                    elsewhere = if (query.isBlank()) emptyList() else ui.columns.filter { it.key != col.key && it.total > 0 },
                    onReveal = { other -> ui.columns.indexOfFirst { it.key == other.key }.takeIf { it >= 0 }?.let { scope.launch { pager.animateScrollToPage(it) } } },
                )
            }
        }
        AnimatedVisibility(
            visible = drag.isDragging,
            enter = slideInVertically { it } + fadeIn(),
            exit = slideOutVertically { it } + fadeOut(),
            modifier = Modifier.align(Alignment.BottomCenter),
        ) {
            LaneDock(ui.columns.filter { it.key != Lane.PROPOSED.key && it.key != Lane.CHECK.key }, drag, groupBy)
        }
    }
}

@Composable
private fun LaneDock(columns: List<Column>, drag: DragController, groupBy: GroupBy) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHighest,
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
        shadowElevation = 8.dp,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.navigationBarsPadding().padding(12.dp)) {
            Text(
                "Drop to move",
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 4.dp, bottom = 8.dp),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.fillMaxWidth()) {
                columns.forEach { col ->
                    val hovered = drag.hoverKey == col.key && drag.sourceKey != col.key
                    val isSource = drag.sourceKey == col.key
                    val scale by animateFloatAsState(if (hovered) 1.08f else 1f, label = "dock")
                    Surface(
                        shape = RoundedCornerShape(18.dp),
                        color = if (hovered) columnColor(col.key) else MaterialTheme.colorScheme.surfaceContainerLow,
                        contentColor = if (hovered) MaterialTheme.colorScheme.surface else MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.weight(1f).height(64.dp).graphicsLayer { scaleX = scale; scaleY = scale }
                            .alpha(if (isSource) 0.4f else 1f).dropTarget(drag, col.key),
                    ) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
                            if (!hovered) Dot(columnColor(col.key))
                            Spacer(Modifier.height(4.dp))
                            Text(col.title.substringBefore(" ·"), style = MaterialTheme.typography.labelSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        }
                    }
                }
            }
        }
    }
}

/** The card that follows the finger. [rootOffset] is the window position of the overlay's parent. */
@Composable
fun DragOverlay(drag: DragController, index: BoardIndex?, rootOffset: Offset) {
    val issue = drag.dragging ?: return
    index ?: return
    val density = LocalDensity.current
    val topLeft = drag.pointer - drag.grabOffset - rootOffset
    Box(
        Modifier
            .offset { IntOffset(topLeft.x.roundToInt(), topLeft.y.roundToInt()) }
            .width(with(density) { drag.cardSize.width.toDp() })
            .graphicsLayer { rotationZ = 2.5f; scaleX = 1.03f; scaleY = 1.03f },
    ) {
        BeadCard(issue, index, running = false, selected = false, lifted = true)
    }
}

@Composable
fun FilterSummary(filter: BoardFilter, onClear: () -> Unit) {
    if (!filter.isActive) return
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
        Text("Filtered", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
        TextButton(onClick = onClear) { Text("Clear filters") }
    }
}

/**
 * The lane to show when the lane at [current] has nothing for the filter but another lane does:
 * the nearest one with matches, so a search on the cover screen doesn't look empty. Null when the
 * current lane has matches or no lane does.
 */
fun laneWithMatches(columns: List<Column>, current: Int): Int? {
    if (columns.getOrNull(current)?.let { it.total > 0 } == true) return null
    return columns.indices.filter { columns[it].total > 0 }.minByOrNull { kotlin.math.abs(it - current) }
}
