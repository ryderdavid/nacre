package com.ryderdavid.nacre.ui.workspace

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
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
                        onOpenProject = { key -> nav.navigate(WorkspaceRoute(key)) { popUpTo(ProjectsRoute) } },
                    )
                }
                composable<ChatFirstSettingsRoute> {
                    SettingsScreen(pending = pendingPair, onPendingHandled = { pendingPair = null }, onBack = { nav.popBackStack() })
                }
            }
        }
        overlay.content?.invoke()
        SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 80.dp))
    }
}

/** One project's workspace with its slots filled. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ProjectWorkspace(repoKey: String, startChat: String?, startDestination: Destination, onProjects: () -> Unit, onOpenProject: (String) -> Unit) {
    val container = LocalContainerOrThrow()
    val repo = container.repository
    val listing by repo.chats.collectAsState()
    val repos by repo.repos.collectAsState()
    val project = repos.data?.firstOrNull { it.key == repoKey }
    var destination by rememberSaveable(repoKey) { mutableStateOf(startDestination) }
    var thread by rememberSaveable(repoKey) { mutableStateOf(startChat) }
    if (thread == null) mostRecentThread(listing.chats, repoKey)?.let { thread = it }
    var inspecting by remember { mutableStateOf<WorkSummary?>(null) }
    var queue by remember { mutableStateOf(false) }
    val drawer = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    KeepFresh(repoKey)
    LaunchedEffect(repoKey) { repo.refreshBoard(repoKey, quiet = true) }
    val (work, needsYou) = projectStrip(repoKey)
    val name = project?.name ?: Keys.id(repoKey)

    val slots = WorkspaceSlots(
        chat = { m -> ChatSlot(repoKey, thread, onThread = { thread = it }, onProjects = onProjects, modifier = m) },
        spec = { m -> SpecSlot(repoKey, m) },
        beads = { m -> BeadsSlot(repoKey, m) },
        strip = { m -> StatusStrip(name, work, needsYou, onWork = { inspecting = it }, onQueue = { queue = true }, modifier = m) },
        switcher = {
            Surface(
                onClick = { scope.launch { drawer.open() } }, shape = projectAvatarShape(name),
                color = MaterialTheme.colorScheme.primary, contentColor = MaterialTheme.colorScheme.onPrimary,
                modifier = Modifier.padding(top = 8.dp, bottom = 16.dp).size(52.dp).semantics { contentDescription = "Switch project. $name" },
            ) { Box(contentAlignment = Alignment.Center) { Text(name.take(1).uppercase(), style = MaterialTheme.typography.titleLargeEmphasized) } }
        },
    )

    ModalNavigationDrawer(
        drawerState = drawer, gesturesEnabled = drawer.isOpen,
        drawerContent = {
            ModalDrawerSheet {
                Column(Modifier.statusBarsPadding().padding(horizontal = 12.dp)) {
                    Text("Projects", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.padding(16.dp))
                    ProjectsDrawerList(current = repoKey) { key -> scope.launch { drawer.close() }; if (key != repoKey) onOpenProject(key) }
                    OutlinedButton(onClick = { scope.launch { drawer.close() }; onProjects() }, modifier = Modifier.padding(16.dp)) { Text("All projects") }
                }
            }
        },
    ) {
        CompositionLocalProvider(
            LocalAskAbout provides { ref -> container.chats.references.add(repoKey, ref); destination = Destination.CHAT },
            LocalViewing provides { ref -> container.chats.references.view(repoKey, ref) },
        ) {
            Workspace(slots, destination, onDestination = { destination = it }, modifier = Modifier.statusBarsPadding())
        }
    }

    inspecting?.let { w ->
        ModalBottomSheet(onDismissRequest = { inspecting = null }) {
            WorkInspector(repoKey, w, onClose = { inspecting = null })
        }
    }
    if (queue) ModalBottomSheet(onDismissRequest = { queue = false }) {
        DecisionQueue(repoKey, onClose = { queue = false })
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
