package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.ryderdavid.nacre.AppContainer
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.DeepLink
import com.ryderdavid.nacre.ui.common.LocalScreenOverlay
import com.ryderdavid.nacre.ui.common.ScreenOverlay
import com.ryderdavid.nacre.ui.settings.PairRequest
import com.ryderdavid.nacre.ui.settings.SettingsScreen
import com.ryderdavid.nacre.ui.workspace.beads.BeadsSlot
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable

@Serializable data object ProjectsRoute
@Serializable data class WorkspaceRoute(val repoKey: String, val chat: String? = null, val dest: String = "CHAT")
@Serializable data object ChatFirstSettingsRoute

/** The chat-first app: Projects, then one workspace per project checkout, and Settings. */
@Composable
fun ChatFirstShell(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    val nav = rememberNavController()
    val snackbar = remember { SnackbarHostState() }
    var pendingPair by remember { mutableStateOf<PairRequest?>(null) }
    val overlay = remember { ScreenOverlay() }

    LaunchedEffect(Unit) {
        container.repository.notices.collect { n ->
            launch {
                val r = snackbar.showSnackbar(n.text, actionLabel = if (n.undo != null) "Undo" else null, withDismissAction = n.undo == null, duration = SnackbarDuration.Short)
                if (r == SnackbarResult.ActionPerformed) n.undo?.let { undo ->
                    container.scope.launch { runCatching { undo() }.onFailure { container.repository.notify("Undo failed: ${it.message}") } }
                }
            }
        }
    }
    LaunchedEffect(deepLink) {
        when (deepLink) {
            is DeepLink.Pair -> { pendingPair = deepLink.request; nav.navigate(ChatFirstSettingsRoute) }
            is DeepLink.OpenChat -> {
                container.chats.openChat(deepLink.chatKey, deepLink.requestId)
                val rec = container.repository.chats.value.chat(deepLink.chatKey)
                val repoKey = rec?.repo?.let { Keys.of(Keys.host(deepLink.chatKey), it) }
                if (repoKey != null) nav.navigate(WorkspaceRoute(repoKey, deepLink.chatKey))
            }
            is DeepLink.OpenChange -> nav.navigate(WorkspaceRoute(deepLink.repoKey, dest = Destination.SPEC.name))
            // The queue focus is already set; the strip opens the queue (or the chat shows the card).
            is DeepLink.OpenDecision -> {
                deepLink.chatKey?.let { container.chats.openChat(it, null) }
                nav.navigate(WorkspaceRoute(deepLink.repoKey, deepLink.chatKey, dest = Destination.CHAT.name))
            }
            is DeepLink.OpenRound -> container.repository.rounds.value.firstOrNull { Keys.of(it.hostId, it.id) == deepLink.roundKey }?.let {
                nav.navigate(WorkspaceRoute(Keys.of(it.hostId, it.repoId)))
            }
            is DeepLink.OpenRun -> container.repository.runs.value.firstOrNull { it.key == deepLink.runKey }?.let { run ->
                container.repository.repos.value.data?.firstOrNull { r -> r.hostId == run.hostId && (r.path == run.repoPath || r.id == run.repoId) }?.let {
                    nav.navigate(WorkspaceRoute(it.key))
                }
            }
            null -> Unit
        }
        if (deepLink != null) onDeepLinkHandled()
    }

    Box(Modifier.fillMaxSize()) {
        CompositionLocalProvider(LocalScreenOverlay provides overlay) {
            NavHost(nav, startDestination = ProjectsRoute) {
                composable<ProjectsRoute> {
                    ProjectsScreen(
                        onOpen = { repoKey, chat -> nav.navigate(WorkspaceRoute(repoKey, chat)) },
                        onSettings = { nav.navigate(ChatFirstSettingsRoute) },
                    )
                }
                composable<WorkspaceRoute> { e ->
                    val r = e.toRoute<WorkspaceRoute>()
                    ProjectWorkspace(
                        repoKey = r.repoKey, startChat = r.chat,
                        startDestination = runCatching { Destination.valueOf(r.dest) }.getOrDefault(Destination.CHAT),
                        onProjects = { nav.popBackStack(ProjectsRoute, inclusive = false) },
                        onOpenProject = { key, chat -> nav.navigate(WorkspaceRoute(key, chat)) { popUpTo(ProjectsRoute) } },
                    )
                }
                composable<ChatFirstSettingsRoute> {
                    SettingsScreen(pending = pendingPair, onPendingHandled = { pendingPair = null }, onBack = { nav.popBackStack() })
                }
            }
        }
        overlay.content?.invoke()
        // Above the navigation bar and the chat composer, and above the keyboard when it is up.
        SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().imePadding().padding(bottom = 152.dp))
    }
}

/** A round or run for the work inspector, with the computer it runs on. */
data class InspectTarget(val hostId: String, val kind: String, val id: String, val label: String, val review: Boolean = false) {
    val key get() = Keys.of(hostId, id)
}

/** One project's workspace with its slots filled. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ProjectWorkspace(repoKey: String, startChat: String?, startDestination: Destination, onProjects: () -> Unit, onOpenProject: (String, String?) -> Unit) {
    val container = LocalContainerOrThrow()
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val repos by repo.repos.collectAsState()
    val project = repos.data?.firstOrNull { it.key == repoKey }
    var destination by rememberSaveable(repoKey) { mutableStateOf(startDestination) }
    var thread by rememberSaveable(repoKey) { mutableStateOf(startChat) }
    if (thread == null) mostRecentThread(listing.chats, repoKey)?.let { thread = it }
    // Each thread keeps the panel shown beside it: switching back to a thread restores it. Opening the
    // project itself starts on the destination it was opened with (Chat unless a link said otherwise).
    var shownThread by rememberSaveable(repoKey) { mutableStateOf<String?>(null) }
    LaunchedEffect(thread) {
        val t = thread ?: return@LaunchedEffect
        val switched = shownThread != null && shownThread != t
        shownThread = t
        if (switched) container.chatStack.view(t).panel?.let { p -> Destination.entries.firstOrNull { it.name == p } }?.let { destination = it }
    }
    // Folding to one pane while a message is being written shows Chat with the message (spec "Fold while typing").
    val oneWide = androidx.compose.ui.platform.LocalConfiguration.current.screenWidthDp < 600
    LaunchedEffect(oneWide) {
        val t = thread
        if (oneWide && t != null && container.chatStack.view(t).draft.isNotBlank()) destination = Destination.CHAT
    }
    LaunchedEffect(thread, destination) {
        thread?.let { t -> container.chatStack.updateView(t) { it.copy(panel = destination.name) } }
    }
    var inspecting by remember { mutableStateOf<InspectTarget?>(null) }
    var queue by remember { mutableStateOf(false) }
    val drawer = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    KeepFresh(repoKey)
    LaunchedEffect(repoKey) { repo.refreshBoard(repoKey, quiet = true) }
    val (work, needsYou) = projectStrip(repoKey)
    val name = project?.name ?: Keys.id(repoKey)
    val host = Keys.host(repoKey)

    // A thread of the same project on another computer moves the whole workspace to that checkout.
    val pickThread: (String) -> Unit = { key ->
        val rec = listing.chat(key)
        val targetRepo = rec?.repo?.let { Keys.of(Keys.host(key), it) }
        if (targetRepo != null && targetRepo != repoKey) onOpenProject(targetRepo, key) else thread = key
    }
    val inspect: (WorkSummary) -> Unit = { w -> inspecting = InspectTarget(host, w.kind, w.id, w.title) }

    val slots = WorkspaceSlots(
        chat = { m -> ChatSlot(repoKey, thread, onThread = pickThread, onProjects = onProjects, modifier = m) },
        spec = { m -> SpecSlot(repoKey, m) },
        beads = { m -> BeadsSlot(repoKey, m) },
        strip = { m -> StatusStrip(name, work, needsYou, onWork = inspect, onQueue = { queue = true }, modifier = m, repoKey = repoKey) },
        switcher = {
            Surface(
                onClick = { scope.launch { drawer.open() } }, shape = projectAvatarShape(name),
                color = MaterialTheme.colorScheme.primary, contentColor = MaterialTheme.colorScheme.onPrimary,
                modifier = Modifier.padding(top = 8.dp, bottom = 16.dp).size(52.dp).semantics { contentDescription = "Switch project. $name" },
            ) { Box(contentAlignment = Alignment.Center) { Text(name.take(1).uppercase(), style = MaterialTheme.typography.titleLargeEmphasized) } }
        },
    )

    // Ask about this attaches to the current thread; with none yet, a Claude chat starts in this checkout.
    val askAbout: (ChatReference) -> Unit = { ref ->
        // One pane: go to Chat. Side by side, Chat is already showing beside the pane, so the split stays.
        if (oneWide) destination = Destination.CHAT
        queue = false; inspecting = null
        val t = thread
        val viewing = container.chats.references.viewingAll.value[repoKey]?.first
        // Asking about what the pane already shows sends it as Viewing rather than a second chip.
        if (t != null && viewing != null && viewing.kind == ref.kind && viewing.id == ref.id && ref.text.isEmpty()) container.chats.references.dropViewing(repoKey, false)
        else if (t != null) container.chats.references.add(t, ref) else scope.launch {
            runCatching { container.chats.link.createChat(host, CreateChat(ChatWire.AGENT_CLAUDE, Keys.id(repoKey), null)).copy(hostId = host).key }
                .onSuccess { k -> thread = k; container.chats.references.add(k, ref) }
                .onFailure { repo.notify("Couldn't start a chat: ${it.message}") }
        }
    }

    CompositionLocalProvider(
        LocalAskAbout provides askAbout,
        LocalOpenQueue provides { inspecting = null; queue = true },
        LocalViewing provides { ref -> container.chats.references.view(repoKey, ref) },
        com.ryderdavid.nacre.ui.workspace.beads.LocalOpenChange provides { changeId -> container.chats.references.openSpec(repoKey, changeId); destination = Destination.SPEC },
        LocalOpenEvidence provides { e ->
            when (e.kind) {
                // A round's changes are reviewed in its inspector; a change's review in the Spec panel.
                "changes" -> {
                    val h = if (e.ref.contains('~')) Keys.host(e.ref) else host
                    val id = Keys.id(e.ref).ifBlank { work?.id.orEmpty() }
                    if (id.isNotBlank()) { queue = false; inspecting = InspectTarget(h, "round", id, e.label, review = true) }
                }
                "review" -> { if (e.ref.isNotBlank()) container.chats.references.openSpec(repoKey, e.ref); destination = Destination.SPEC; queue = false; inspecting = null }
                // A requirement or spec file by path: the change's document in the reader, or a current spec in the list.
                "spec" -> {
                    if (e.ref.isNotBlank()) container.chats.references.openSpec(repoKey, e.ref)
                    destination = Destination.SPEC; queue = false; inspecting = null
                }
                "bead" -> { if (e.ref.isNotBlank()) container.chats.references.openBead(repoKey, Keys.id(e.ref)); destination = Destination.BEADS; queue = false; inspecting = null }
                else -> {
                    val h = if (e.ref.contains('~')) Keys.host(e.ref) else host
                    val id = Keys.id(e.ref).ifBlank { work?.id.orEmpty() }
                    if (id.isNotBlank()) inspecting = InspectTarget(h, if (e.kind == "run") "run" else "round", id, e.label)
                }
            }
        },
    ) {
        // Back closes the project drawer before it leaves the workspace.
        androidx.activity.compose.BackHandler(enabled = drawer.isOpen) { scope.launch { drawer.close() } }
        ModalNavigationDrawer(
            drawerState = drawer, gesturesEnabled = drawer.isOpen,
            drawerContent = {
                ModalDrawerSheet {
                    Column(Modifier.statusBarsPadding().navigationBarsPadding().padding(horizontal = 12.dp)) {
                        Text("Projects", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.padding(16.dp))
                        ProjectsDrawerList(current = repoKey) { key -> scope.launch { drawer.close() }; if (key != repoKey) onOpenProject(key, null) }
                        OutlinedButton(onClick = { scope.launch { drawer.close() }; onProjects() }, modifier = Modifier.padding(16.dp)) { Text("All projects") }
                    }
                }
            },
        ) {
            Workspace(slots, destination, onDestination = { destination = it }, modifier = Modifier.statusBarsPadding())
        }

        inspecting?.let { t ->
            ModalBottomSheet(onDismissRequest = { inspecting = null }) {
                com.ryderdavid.nacre.ui.workspace.work.WorkInspector(repoKey, t, onClose = { inspecting = null }, onQueue = { inspecting = null; queue = true })
            }
        }
        if (queue) ModalBottomSheet(onDismissRequest = { queue = false }) {
            DecisionQueue(repoKey, onClose = { queue = false })
        }
    }
}

@Composable
private fun LocalContainerOrThrow() = com.ryderdavid.nacre.ui.common.LocalContainer.current

/** The projects list inside the wide layout's drawer. */
@Composable
private fun ProjectsDrawerList(current: String, onPick: (String) -> Unit) {
    val repo = LocalContainerOrThrow().repository
    val hosts by repo.hosts.collectAsState()
    val repos by repo.repos.collectAsState()
    val reported by repo.projectsByHost.collectAsState()
    val attention by repo.attention.collectAsState()
    val work by repo.work.collectAsState()
    val lastSeen by repo.lastSeen.collectAsState()
    val sections = ProjectsModel.sections(hosts.map { ComputerInfo(it.host.id, it.host.name, it.online, lastSeen[it.host.id]) }, repos.data.orEmpty(), reported, attention, work, ProjectFilter.ALL, "", System.currentTimeMillis())
    LazyColumn(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        sections.forEach { s ->
            item { Text(s.name + " · " + s.checked, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 16.dp, top = 12.dp, bottom = 6.dp)) }
            items(s.rows.size) { i ->
                val row = s.rows[i]
                Surface(
                    onClick = { onPick(row.key) }, shape = segmentShape(i, s.rows.size),
                    color = if (row.key == current) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainer,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Row(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(row.name, style = MaterialTheme.typography.titleMedium)
                            Text(row.statusText.ifBlank { "Idle" }, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                        }
                        val n = row.needsYou ?: 0
                        if (n > 0) StatusPill("$n", MaterialTheme.colorScheme.errorContainer, MaterialTheme.colorScheme.onErrorContainer)
                    }
                }
            }
        }
    }
}

/** Chat for the project: the current thread, or a start when the checkout has none yet. */
@Composable
private fun ChatSlot(repoKey: String, thread: String?, onThread: (String) -> Unit, onProjects: () -> Unit, modifier: Modifier) {
    val container = LocalContainerOrThrow()
    val scope = rememberCoroutineScope()
    Box(modifier) {
        if (thread != null) {
            ChatPane(repoKey, thread, onThread = onThread, onProjects = onProjects)
        } else {
            val name = container.repository.repos.collectAsState().value.data?.firstOrNull { it.key == repoKey }?.name ?: Keys.id(repoKey)
            Row(Modifier.fillMaxWidth().padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
                if (androidx.compose.ui.platform.LocalConfiguration.current.screenWidthDp < 600) androidx.compose.material3.IconButton(onClick = onProjects) {
                    androidx.compose.material3.Icon(androidx.compose.material.icons.Icons.AutoMirrored.Rounded.ArrowBack, "Back to projects")
                }
                Text(name, style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.padding(horizontal = 8.dp))
            }
            Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically), horizontalAlignment = Alignment.CenterHorizontally) {
                Text("No chats in this project yet", style = MaterialTheme.typography.titleLargeEmphasized)
                Text("Start one with Claude Code or Codex on this computer.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (agent in listOf(ChatWire.AGENT_CLAUDE, ChatWire.AGENT_CODEX)) {
                        val start = {
                            scope.launch {
                                runCatching { container.chats.link.createChat(Keys.host(repoKey), CreateChat(agent, Keys.id(repoKey), null)).copy(hostId = Keys.host(repoKey)).key }
                                    .onSuccess(onThread).onFailure { container.repository.notify("Couldn't start the chat: ${it.message}") }
                            }
                        }
                        if (agent == ChatWire.AGENT_CLAUDE) Button(onClick = { start() }) { Text("Claude chat") } else OutlinedButton(onClick = { start() }) { Text("Codex chat") }
                    }
                }
            }
        }
    }
}
