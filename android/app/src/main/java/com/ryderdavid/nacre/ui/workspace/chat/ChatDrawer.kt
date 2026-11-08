package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.Forum
import androidx.compose.material.icons.rounded.Insights
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialShapes
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.ToggleButton
import androidx.compose.material3.toShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.data.chat.DefaultAgent
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.NewChatButton
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.time.Duration
import java.time.Instant

/**
 * The chat drawer: Chats and Jobs as a connected button group. In Chats every chat of the project is a
 * card showing what it works on; tapping one unfolds it into the conversation, which takes the whole
 * drawer (the switch and the other chats fold away) until its chevron folds it back. New chat floats at
 * the bottom of the list. Jobs shows the same work from its side, one card per bead, epic, change or
 * round, with the chats on it.
 *
 * With no [chat] (the phone's sheet) only the lists show, and picking a chat hands it to [onThread].
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ChatDrawer(
    repoKey: String,
    thread: String?,
    jobs: Boolean,
    onJobs: (Boolean) -> Unit,
    unfolded: Boolean,
    onUnfold: (Boolean) -> Unit,
    onThread: (String) -> Unit,
    onInspect: (WorkSummary) -> Unit,
    chat: (@Composable (Modifier) -> Unit)?,
    modifier: Modifier = Modifier,
) {
    val container = LocalContainer.current
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val decisions by repo.decisions.collectAsState()
    val work by repo.work.collectAsState()
    val board by repo.board(repoKey).collectAsState()
    val siblings = repo.siblingKeys(repoKey)
    val open = siblings.plus(repoKey).flatMap { decisions[it].orEmpty() }.filter { it.pending && !it.chatId.isNullOrBlank() }
        .groupingBy { it.chatId.orEmpty() }.eachCount()
    val cards = remember(listing.chats, work[repoKey], board.data, open) {
        ChatCards.build(listing.chats, repoKey, siblings, work[repoKey].orEmpty(), board.data, open)
    }
    val workJobs = remember(listing.chats, work[repoKey], board.data, open) {
        WorkJobsModel.build(listing.chats, repoKey, siblings, work[repoKey].orEmpty(), board.data, open)
    }
    val scope = container.scope
    val host = Keys.host(repoKey)
    val defaultAgent by DefaultAgent.state(LocalContext.current)
    val show: (String) -> Unit = { key -> onJobs(false); onThread(key); onUnfold(true) }
    val start: (String, ChatReference?) -> Unit = { agent, about ->
        scope.launch {
            runCatching { container.chats.link.createChat(host, CreateChat(agent, Keys.id(repoKey), null)).copy(hostId = host).key }
                .onSuccess { key -> about?.let { container.chats.references.add(key, it) }; show(key) }
                .onFailure { repo.notify("Couldn't start the chat: ${it.message}") }
        }
    }
    // A minute clock for "4m ago".
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) { while (true) { delay(60_000); now = System.currentTimeMillis() } }

    val showOpen = !jobs && unfolded && thread != null && chat != null
    Column(modifier) {
        // An unfolded chat has the whole column: the switch folds away with the other chats.
        AnimatedVisibility(!showOpen) { DrawerSegments(cards.size, workJobs.now.size, workJobs.runningCount > 0, jobs, onJobs) }
        Box(Modifier.weight(1f).fillMaxWidth()) {
            when {
                jobs -> JobsList(workJobs, onOpenChat = show, onStartAbout = { ref -> start(defaultAgent, ref) }, onInspect = onInspect)
                showOpen -> OpenChat(
                    cards, thread!!, listing.chat(thread)?.let { c -> ChatCard(c.key, c.title.ifBlank { "New chat" }, c.agent, "", 0, c.active, c.updatedAt, null) },
                    now, onFold = { onUnfold(false) }, chat = chat!!,
                )
                else -> CardsList(cards, thread, now, onOpen = show)
            }
            if (!showOpen) NewChatButton(onStart = { agent -> start(agent, null) }, modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp))
        }
    }
}

/** Chats and Jobs as a connected button group: the selected one rounds into a filled pill. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun DrawerSegments(chats: Int, jobs: Int, running: Boolean, onJobsTab: Boolean, onJobs: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 12.dp, bottom = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween),
    ) {
        ToggleButton(
            checked = !onJobsTab, onCheckedChange = { onJobs(false) }, shapes = ButtonGroupDefaults.connectedLeadingButtonShapes(),
            modifier = Modifier.weight(1f).heightIn(min = 48.dp).semantics { contentDescription = "Chats, $chats" },
        ) {
            Text("Chats", style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.width(8.dp))
            CountBadge(chats)
        }
        ToggleButton(
            checked = onJobsTab, onCheckedChange = { onJobs(true) }, shapes = ButtonGroupDefaults.connectedTrailingButtonShapes(),
            modifier = Modifier.weight(1f).heightIn(min = 48.dp).semantics { contentDescription = "Jobs, $jobs" + if (running) ", work running" else "" },
        ) {
            Text("Jobs", style = MaterialTheme.typography.titleSmall)
            if (running) { Spacer(Modifier.width(8.dp)); LiveDot() }
            Spacer(Modifier.width(8.dp))
            CountBadge(jobs)
        }
    }
}

@Composable
private fun CountBadge(n: Int) {
    Box(
        Modifier.heightIn(min = 22.dp).background(androidx.compose.material3.LocalContentColor.current.copy(alpha = 0.16f), RoundedCornerShape(11.dp)).padding(horizontal = 7.dp),
        contentAlignment = Alignment.Center,
    ) { Text("$n", style = MaterialTheme.typography.labelMedium) }
}

@Composable
private fun LiveDot() {
    val pulse = rememberInfiniteTransition(label = "live")
    val a by pulse.animateFloat(1f, 0.3f, infiniteRepeatable(tween(700), androidx.compose.animation.core.RepeatMode.Reverse), label = "alpha")
    Box(Modifier.size(8.dp).graphicsLayer { alpha = a }.background(androidx.compose.material3.LocalContentColor.current, CircleShape))
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 6.dp, top = 10.dp, bottom = 2.dp).semantics { heading() },
    )
}

@Composable
private fun CardsList(cards: List<ChatCard>, current: String?, now: Long, onOpen: (String) -> Unit) {
    val active = cards.filter { it.active }
    val recent = cards.filterNot { it.active }
    LazyColumn(
        Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 96.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (cards.isEmpty()) item {
            Text(
                "No chats in this project yet. Start one below, or tap Send to chat on a card.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(12.dp),
            )
        }
        if (active.isNotEmpty()) {
            item(key = "l:active") { SectionLabel("Working and waiting on you") }
            items(active, key = { it.key }) { c -> ChatCardView(c, selected = c.key == current, now = now, onClick = { onOpen(c.key) }) }
        }
        if (recent.isNotEmpty()) {
            item(key = "l:recent") { SectionLabel("Recent") }
            items(recent, key = { it.key }) { c -> ChatCardView(c, selected = c.key == current, now = now, onClick = { onOpen(c.key) }) }
        }
    }
}

/** A chat as a card; the same header tops the unfolded chat. */
@Composable
private fun ChatCardView(card: ChatCard, selected: Boolean, now: Long, onClick: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = if (selected) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else null,
        modifier = Modifier.fillMaxWidth(),
    ) { ChatCardHeader(card, expanded = false, now = now, onToggle = onClick) }
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun ChatCardHeader(card: ChatCard, expanded: Boolean, now: Long, onToggle: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var confirming by remember { mutableStateOf(false) }
    val turn by animateFloatAsState(if (expanded) 180f else 0f, label = "chevron")
    Box {
        Row(
            Modifier.fillMaxWidth()
                .combinedClickable(
                    onClick = onToggle, onClickLabel = if (expanded) "Fold the chat" else "Open the chat",
                    onLongClick = { menu = true }, onLongClickLabel = "Chat options",
                )
                .padding(start = 14.dp, end = 10.dp, top = if (expanded) 10.dp else 14.dp, bottom = if (expanded) 8.dp else 14.dp)
                .semantics { stateDescription = if (expanded) "Open" else "Folded" },
            horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top,
        ) {
            AgentMark(card.agent, card.running, size = if (expanded) 36.dp else 44.dp)
            // Open, the card is the conversation's header: one line for the job and where it stands.
            if (expanded) Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(card.title, style = MaterialTheme.typography.titleMediumEmphasized, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        card.job?.let { j -> listOf(j.label.ifBlank { j.subject }, j.status).filter { it.isNotBlank() }.joinToString(" · ") } ?: "Conversation",
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false),
                    )
                    StatePill(card, now)
                }
                card.job?.takeIf { it.total > 0 }?.let { j -> LinearWavyProgressIndicator(progress = { j.done.toFloat() / j.total }, modifier = Modifier.fillMaxWidth()) }
            } else Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(card.title, style = MaterialTheme.typography.titleMediumEmphasized, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (!expanded && card.lastLine.isNotBlank()) Text(
                    card.lastLine, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2, overflow = TextOverflow.Ellipsis,
                )
                card.job?.let { JobBox(it) } ?: Text(
                    "Conversation · not working on anything yet", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.outline,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(DefaultAgent.label(card.agent), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    StatePill(card, now)
                }
            }
            FilledTonalIconButton(onClick = onToggle, modifier = Modifier.size(40.dp)) {
                Icon(Icons.Rounded.ExpandMore, if (expanded) "Fold" else "Open", Modifier.rotate(turn))
            }
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(
                text = { Text("Delete chat") }, leadingIcon = { Icon(Icons.Rounded.Delete, null) },
                onClick = { menu = false; confirming = true },
            )
        }
    }
    if (confirming) DeleteChatDialog(card.key, card.title) { confirming = false }
}

/** What the chat works on: the item, a wavy bar of its progress and where it stands. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun JobBox(job: ChatJob) {
    Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainer, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (job.label.isNotBlank() && job.label != job.subject) Text(
                    job.label, style = MaterialTheme.typography.labelSmall, fontFamily = FontFamily.Monospace, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1,
                )
                Text(job.subject, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            }
            if (job.total > 0) LinearWavyProgressIndicator(progress = { job.done.toFloat() / job.total }, modifier = Modifier.fillMaxWidth())
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    job.status.ifBlank { "Not started" }, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                )
                if (job.total > 0) Text("${job.done}/${job.total}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun StatePill(card: ChatCard, now: Long) {
    val c = MaterialTheme.colorScheme
    when {
        card.waiting > 0 -> Pill(if (card.waiting == 1) "1 needs you" else "${card.waiting} need you", c.errorContainer, c.onErrorContainer)
        card.running -> Pill("Working", c.primaryContainer, c.onPrimaryContainer, live = true)
        else -> agoText(card.updatedAt, now)?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = c.outline) }
    }
}

@Composable
private fun Pill(text: String, container: Color, content: Color, live: Boolean = false) {
    Surface(shape = RoundedCornerShape(10.dp), color = container, contentColor = content) {
        Row(Modifier.padding(horizontal = 9.dp, vertical = 3.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            if (live) LiveDot()
            Text(text, style = MaterialTheme.typography.labelMedium)
        }
    }
}

/** "Just now", "4m ago", "3h ago", "2d ago"; null without a time. */
fun agoText(at: String, now: Long): String? {
    val t = runCatching { Instant.parse(at) }.getOrNull() ?: return null
    val d = Duration.between(t, Instant.ofEpochMilli(now)).coerceAtLeast(Duration.ZERO)
    return when {
        d.toMinutes() < 1 -> "Just now"
        d.toHours() < 1 -> "${d.toMinutes()}m ago"
        d.toDays() < 1 -> "${d.toHours()}h ago"
        else -> "${d.toDays()}d ago"
    }
}

/** Claude's mark is a cookie, Codex's a rounded square; the shape turns slowly while the agent works. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun AgentMark(agent: String, running: Boolean, size: Dp = 44.dp) {
    val c = MaterialTheme.colorScheme
    val rotation = if (running) {
        val spin = rememberInfiniteTransition(label = "mark")
        spin.animateFloat(0f, 360f, infiniteRepeatable(tween(6000, easing = LinearEasing)), label = "turn").value
    } else 0f
    val shape: Shape = (if (agent == ChatWire.AGENT_CODEX) MaterialShapes.Square else MaterialShapes.Cookie9Sided).toShape()
    Box(Modifier.size(size), contentAlignment = Alignment.Center) {
        Box(Modifier.size(size).graphicsLayer { rotationZ = rotation }.background(if (running) c.primaryContainer else c.secondaryContainer, shape))
        Text(
            if (agent == ChatWire.AGENT_CODEX) "X" else "C",
            style = if (size >= 40.dp) MaterialTheme.typography.titleMedium else MaterialTheme.typography.labelMedium,
            color = if (running) c.onPrimaryContainer else c.onSecondaryContainer,
        )
    }
}

/**
 * One chat unfolded: its card takes the whole drawer, the conversation under the card's header. Folding
 * it brings back the switch and the other chats.
 */
@Composable
private fun OpenChat(
    cards: List<ChatCard>,
    thread: String,
    fallback: ChatCard?,
    now: Long,
    onFold: () -> Unit,
    chat: @Composable (Modifier) -> Unit,
) {
    val card = cards.firstOrNull { it.key == thread } ?: fallback ?: ChatCard(thread, "Chat", ChatWire.AGENT_CLAUDE, "", 0, false, "", null)
    Surface(
        shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerLowest,
        modifier = Modifier.fillMaxSize().padding(8.dp),
    ) {
        Column(Modifier.fillMaxSize()) {
            ChatCardHeader(card, expanded = true, now = now, onToggle = onFold)
            chat(Modifier.weight(1f).fillMaxWidth())
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun JobsList(model: WorkJobs, onOpenChat: (String) -> Unit, onStartAbout: (ChatReference) -> Unit, onInspect: (WorkSummary) -> Unit) {
    var open by rememberSaveable { mutableStateOf<String?>(null) }
    LazyColumn(
        Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 96.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (model.now.isEmpty()) item {
            Text(
                "Nothing in progress. A chat's work shows here once it files beads or a change, or starts a round.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(12.dp),
            )
        } else item(key = "l:now") { SectionLabel("Working now") }
        items(model.now, key = { it.key }) { j ->
            WorkJobCard(j, expanded = open == j.key, onToggle = { open = if (open == j.key) null else j.key }, onOpenChat, onStartAbout, onInspect)
        }
        if (model.done.isNotEmpty()) {
            item(key = "l:done") { SectionLabel("Done · ${model.done.size}") }
            items(model.done, key = { "d:" + it.key }) { j ->
                WorkJobCard(j, expanded = open == j.key, onToggle = { open = if (open == j.key) null else j.key }, onOpenChat, onStartAbout, onInspect)
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun WorkJobCard(
    job: WorkJob,
    expanded: Boolean,
    onToggle: () -> Unit,
    onOpenChat: (String) -> Unit,
    onStartAbout: (ChatReference) -> Unit,
    onInspect: (WorkSummary) -> Unit,
) {
    val c = MaterialTheme.colorScheme
    Surface(
        onClick = onToggle, shape = RoundedCornerShape(if (expanded) 28.dp else 24.dp),
        color = if (job.finished) c.surfaceContainer else c.surfaceContainerLowest,
        border = if (expanded) BorderStroke(2.dp, c.primary) else null,
        modifier = Modifier.fillMaxWidth().semantics { stateDescription = if (expanded) "Open" else "Folded" },
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
                KindBadge(job.kind, job.finished)
                Column(Modifier.weight(1f)) {
                    if (job.label != job.title) Text(job.label, style = MaterialTheme.typography.labelSmall, fontFamily = FontFamily.Monospace, color = c.onSurfaceVariant)
                    Text(job.title, style = MaterialTheme.typography.titleMediumEmphasized, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
                when {
                    job.waiting > 0 -> Pill(if (job.waiting == 1) "1 needs you" else "${job.waiting} need you", c.errorContainer, c.onErrorContainer)
                    job.running -> Pill("Running", c.primaryContainer, c.onPrimaryContainer, live = true)
                }
            }
            if (job.total > 0 && !job.finished) LinearWavyProgressIndicator(progress = { job.done.toFloat() / job.total }, modifier = Modifier.fillMaxWidth())
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(job.status, style = MaterialTheme.typography.labelLarge, color = c.onSurfaceVariant, modifier = Modifier.weight(1f))
                if (job.total > 0) Text("${job.done}/${job.total}", style = MaterialTheme.typography.labelLarge, color = c.onSurfaceVariant)
            }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                job.chats.forEach { (key, title) ->
                    FilledTonalButton(onClick = { onOpenChat(key) }, modifier = Modifier.heightIn(min = 40.dp)) {
                        Icon(Icons.Rounded.Forum, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp))
                        Text("Open chat · $title", maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
                if (job.chats.isEmpty() && !job.finished) job.reference?.let { ref ->
                    OutlinedButton(onClick = { onStartAbout(ref) }, modifier = Modifier.heightIn(min = 40.dp)) {
                        Icon(Icons.Rounded.Add, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp)); Text("Start a chat about this")
                    }
                }
                job.work?.let { w ->
                    TextButton(onClick = { onInspect(w) }, modifier = Modifier.heightIn(min = 40.dp)) {
                        Icon(Icons.Rounded.Insights, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp)); Text("Inspect work")
                    }
                }
            }
            AnimatedVisibility(expanded && job.steps.isNotEmpty()) {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) { job.steps.forEach { StepRow(it) } }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun KindBadge(kind: String, finished: Boolean) {
    val c = MaterialTheme.colorScheme
    val (shape, container, content) = when {
        finished -> Triple(CircleShape, c.primaryContainer, c.onPrimaryContainer)
        kind == "spec" -> Triple(MaterialShapes.Cookie4Sided.toShape(), c.tertiaryContainer, c.onTertiaryContainer)
        kind == "bug" -> Triple(RoundedCornerShape(22.dp, 22.dp, 22.dp, 8.dp), c.errorContainer, c.onErrorContainer)
        kind == "epic" -> Triple(MaterialShapes.Clover4Leaf.toShape(), c.primaryContainer, c.onPrimaryContainer)
        kind == "round" -> Triple(MaterialShapes.Pill.toShape(), c.secondaryContainer, c.onSecondaryContainer)
        else -> Triple(RoundedCornerShape(14.dp), c.primaryContainer, c.onPrimaryContainer)
    }
    Box(Modifier.size(44.dp).background(container, shape), contentAlignment = Alignment.Center) {
        if (finished) Icon(Icons.Rounded.Check, "Done", tint = content)
        else Text(
            when (kind) { "spec" -> "S"; "bug" -> "B"; "epic" -> "E"; "round" -> "R"; else -> "T" },
            style = MaterialTheme.typography.titleMedium, color = content,
        )
    }
}

@Composable
private fun StepRow(step: JobStep) {
    val c = MaterialTheme.colorScheme
    Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier.size(20.dp).background(
                when (step.state) { StepState.DONE -> c.primary; StepState.ACTIVE -> c.primaryContainer; StepState.TODO -> c.surfaceContainerHighest },
                CircleShape,
            ),
            contentAlignment = Alignment.Center,
        ) {
            when (step.state) {
                StepState.DONE -> Icon(Icons.Rounded.Check, "Done", Modifier.size(14.dp), tint = c.onPrimary)
                StepState.ACTIVE -> Box(Modifier.size(8.dp).background(c.onPrimaryContainer, CircleShape))
                StepState.TODO -> Unit
            }
        }
        Text(
            step.text, style = MaterialTheme.typography.bodyMedium,
            color = if (step.state == StepState.TODO) c.onSurfaceVariant else c.onSurface, maxLines = 2, overflow = TextOverflow.Ellipsis,
        )
    }
}
