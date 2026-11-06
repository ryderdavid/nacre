package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Badge
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.FloatingActionButtonMenu
import androidx.compose.material3.FloatingActionButtonMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialShapes
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.ToggleButton
import androidx.compose.material3.ToggleFloatingActionButton
import androidx.compose.material3.ToggleFloatingActionButtonDefaults.animateIcon
import androidx.compose.material3.animateFloatingActionButton
import androidx.compose.material3.toShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** The shapes project avatars take, picked by name so each project keeps its own. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
private val avatarShapes = listOf(
    MaterialShapes.Cookie9Sided, MaterialShapes.Clover4Leaf, MaterialShapes.Sunny,
    MaterialShapes.Cookie7Sided, MaterialShapes.Gem, MaterialShapes.SoftBurst,
)

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
internal fun projectAvatarShape(name: String): Shape = avatarShapes[(name.hashCode() and 0x7fffffff) % avatarShapes.size].toShape()

/** Segmented-list shape for row [i] of [n]: large outer corners, small inner ones. */
internal fun segmentShape(i: Int, n: Int): Shape = when {
    n == 1 -> RoundedCornerShape(24.dp)
    i == 0 -> RoundedCornerShape(24.dp, 24.dp, 6.dp, 6.dp)
    i == n - 1 -> RoundedCornerShape(6.dp, 6.dp, 24.dp, 24.dp)
    else -> RoundedCornerShape(6.dp)
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ProjectsScreen(onOpen: (repoKey: String, chatKey: String?) -> Unit, onSettings: () -> Unit) {
    val container = LocalContainer.current
    val repo = container.repository
    val hosts by repo.hosts.collectAsState()
    val repos by repo.repos.collectAsState()
    val reported by repo.projectsByHost.collectAsState()
    val attention by repo.attention.collectAsState()
    val work by repo.work.collectAsState()
    val lastSeen by repo.lastSeen.collectAsState()
    var filter by rememberSaveable { mutableStateOf(ProjectFilter.ALL) }
    var query by rememberSaveable { mutableStateOf("") }
    var now by remember { mutableStateOf(System.currentTimeMillis()) }
    var fabOpen by rememberSaveable { mutableStateOf(false) }
    var newChatAgent by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        repo.refreshProjects()
        while (true) { delay(30_000); now = System.currentTimeMillis(); repo.refreshProjects() }
    }

    val computers = hosts.map { ComputerInfo(it.host.id, it.host.name, it.online, lastSeen[it.host.id]) }
    val sections = ProjectsModel.sections(computers, repos.data.orEmpty(), reported, attention, work, filter, query, now)
    val waiting = ProjectsModel.needsYouTotal(ProjectsModel.sections(computers, repos.data.orEmpty(), reported, attention, work, ProjectFilter.ALL, "", now))

    Box(Modifier.fillMaxSize()) {
        LazyColumn(
            Modifier.fillMaxSize().statusBarsPadding(),
            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 120.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    TextField(
                        value = query, onValueChange = { query = it },
                        modifier = Modifier.weight(1f).heightIn(min = 56.dp),
                        placeholder = { Text("Search projects") },
                        leadingIcon = { Icon(Icons.Rounded.Search, null) },
                        singleLine = true,
                        shape = RoundedCornerShape(28.dp),
                        colors = TextFieldDefaults.colors(
                            focusedIndicatorColor = androidx.compose.ui.graphics.Color.Transparent,
                            unfocusedIndicatorColor = androidx.compose.ui.graphics.Color.Transparent,
                            unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                            focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                        ),
                        keyboardOptions = KeyboardOptions.Default,
                    )
                    FilledTonalIconButton(
                        onClick = onSettings, modifier = Modifier.size(56.dp),
                        colors = IconButtonDefaults.filledTonalIconButtonColors(
                            containerColor = MaterialTheme.colorScheme.tertiaryContainer,
                            contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
                        ),
                    ) { Icon(Icons.Rounded.Tune, "Settings") }
                }
            }
            item {
                Column(Modifier.padding(start = 8.dp, top = 24.dp, bottom = 12.dp)) {
                    Text("Projects", style = MaterialTheme.typography.displaySmallEmphasized)
                    Text(
                        "${computers.size} computer${if (computers.size == 1) "" else "s"} · ${repos.data.orEmpty().size} projects",
                        style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            item {
                Row(Modifier.padding(start = 4.dp, bottom = 16.dp), horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween)) {
                    val all = ProjectFilter.entries
                    all.forEachIndexed { i, f ->
                        ToggleButton(
                            checked = filter == f, onCheckedChange = { filter = f },
                            shapes = when (i) {
                                0 -> ButtonGroupDefaults.connectedLeadingButtonShapes()
                                all.lastIndex -> ButtonGroupDefaults.connectedTrailingButtonShapes()
                                else -> ButtonGroupDefaults.connectedMiddleButtonShapes()
                            },
                            modifier = Modifier.semantics { stateDescription = if (filter == f) "Selected" else "" },
                        ) {
                            Text(f.label)
                            if (f == ProjectFilter.NEEDS_YOU && waiting > 0) { Spacer(Modifier.width(6.dp)); Badge { Text("$waiting") } }
                        }
                    }
                }
            }
            if (computers.isEmpty()) item {
                Text("Pair a computer in Settings to see its projects.", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            sections.forEach { section ->
                item(key = "h:" + section.hostId) {
                    Row(Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, top = 12.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(section.name, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                        StatusPill(section.checked, if (section.online) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
                            if (section.online) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                if (section.rows.isEmpty()) item(key = "e:" + section.hostId) {
                    if (section.online && repos.data.isNullOrEmpty()) Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) {
                        androidx.compose.material3.LoadingIndicator(Modifier.semantics { contentDescription = "Loading projects" })
                    } else Text(if (section.online) "No projects found on this computer." else "Its projects show when it's back.", Modifier.padding(horizontal = 16.dp, vertical = 8.dp), color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                items(section.rows.size, key = { section.rows[it].key }) { i ->
                    ProjectRowView(section.rows[i], segmentShape(i, section.rows.size), online = section.online) {
                        onOpen(section.rows[i].key, null)
                    }
                }
            }
        }

        FloatingActionButtonMenu(
            expanded = fabOpen,
            modifier = Modifier.align(Alignment.BottomEnd).navigationBarsPadding(),
            button = {
                ToggleFloatingActionButton(
                    modifier = Modifier.semantics { contentDescription = if (fabOpen) "Close new chat menu" else "New chat" }.animateFloatingActionButton(visible = true, alignment = Alignment.BottomEnd),
                    checked = fabOpen, onCheckedChange = { fabOpen = it },
                ) {
                    val icon by remember { androidx.compose.runtime.derivedStateOf { if (checkedProgress > 0.5f) Icons.Rounded.Close else Icons.Rounded.Add } }
                    Icon(icon, null, Modifier.animateIcon({ checkedProgress }))
                }
            },
        ) {
            FloatingActionButtonMenuItem(onClick = { fabOpen = false; newChatAgent = ChatWire.AGENT_CLAUDE }, icon = { Icon(Icons.AutoMirrored.Rounded.Chat, null) }, text = { Text("Claude chat") })
            FloatingActionButtonMenuItem(onClick = { fabOpen = false; newChatAgent = ChatWire.AGENT_CODEX }, icon = { Icon(Icons.Rounded.Code, null) }, text = { Text("Codex chat") })
        }
    }

    newChatAgent?.let { agent ->
        val choices = sections.filter { it.online }.flatMap { s -> s.rows.map { s.name to it } }
        AlertDialog(
            onDismissRequest = { newChatAgent = null },
            title = { Text(if (agent == ChatWire.AGENT_CODEX) "New Codex chat in…" else "New Claude chat in…") },
            text = {
                LazyColumn(Modifier.heightIn(max = 420.dp)) {
                    items(choices) { (computer, row) ->
                        ListItem(
                            headlineContent = { Text(row.name) },
                            supportingContent = { Text(computer) },
                            colors = ListItemDefaults.colors(containerColor = androidx.compose.ui.graphics.Color.Transparent),
                            modifier = Modifier.clip(RoundedCornerShape(16.dp)).clickableRow {
                                newChatAgent = null
                                scope.launch {
                                    runCatching {
                                        val rec = container.chats.link.createChat(Keys.host(row.key), CreateChat(agent, Keys.id(row.key), null))
                                        rec.copy(hostId = Keys.host(row.key)).key
                                    }.onSuccess { onOpen(row.key, it) }.onFailure { container.repository.notify("Couldn't start the chat: ${it.message}") }
                                }
                            },
                        )
                    }
                }
            },
            confirmButton = {},
            dismissButton = { TextButton(onClick = { newChatAgent = null }) { Text("Cancel") } },
        )
    }
}

private fun Modifier.clickableRow(onClick: () -> Unit) = this.then(Modifier.clickable(onClick = onClick))

@Composable
internal fun StatusPill(text: String, container: androidx.compose.ui.graphics.Color, content: androidx.compose.ui.graphics.Color) {
    Surface(color = container, contentColor = content, shape = RoundedCornerShape(12.dp)) {
        Text(text, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp), maxLines = 1)
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun ProjectRowView(row: ProjectRow, shape: Shape, online: Boolean, onClick: () -> Unit) {
    val busy = row.work != null
    Surface(
        onClick = onClick, shape = shape,
        color = if (!online) MaterialTheme.colorScheme.surfaceContainerLow else if (busy) MaterialTheme.colorScheme.surfaceContainerHigh else MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 18.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                val avatar = if (busy) 52.dp else 44.dp
                Surface(
                    shape = projectAvatarShape(row.name),
                    color = if (!online) MaterialTheme.colorScheme.surfaceContainerHighest else if (busy) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.secondaryContainer,
                    contentColor = if (!online) MaterialTheme.colorScheme.onSurfaceVariant else if (busy) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSecondaryContainer,
                    modifier = Modifier.size(avatar),
                ) {
                    Box(contentAlignment = Alignment.Center) {
                        Text(row.name.take(1).uppercase(), style = MaterialTheme.typography.titleLargeEmphasized, fontWeight = FontWeight.Black)
                    }
                }
                Column(Modifier.weight(1f)) {
                    Text(row.name, style = if (busy) MaterialTheme.typography.titleLargeEmphasized else MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    val sub = row.statusText.ifBlank { row.lastLine }.ifBlank { "Idle" }
                    Text(sub, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                val n = row.needsYou ?: 0
                if (n > 0) StatusPill(if (busy) "$n need${if (n == 1) "s" else ""} you" else "$n", MaterialTheme.colorScheme.errorContainer, MaterialTheme.colorScheme.onErrorContainer)
                else if (row.needsYou == null && row.lastKnownNeedsYou > 0) StatusPill("${row.lastKnownNeedsYou} last known", MaterialTheme.colorScheme.surfaceContainerHighest, MaterialTheme.colorScheme.onSurfaceVariant)
            }
            val w = row.work
            if (w != null && w.total > 0 && online) {
                if (w.running) {
                    LinearWavyProgressIndicator(progress = { w.landed.toFloat() / w.total }, modifier = Modifier.fillMaxWidth().semantics { contentDescription = "${w.landed} of ${w.total} landed" })
                } else {
                    androidx.compose.material3.LinearProgressIndicator(progress = { w.landed.toFloat() / w.total }, modifier = Modifier.fillMaxWidth().height(6.dp).semantics { contentDescription = "${w.landed} of ${w.total} landed" })
                }
            }
            if (row.activeChange.isNotBlank() && online) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Icon(Icons.Rounded.Description, null, Modifier.size(16.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(row.activeChange, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}
