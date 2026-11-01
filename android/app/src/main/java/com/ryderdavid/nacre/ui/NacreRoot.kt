package com.ryderdavid.nacre.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material.icons.rounded.ViewKanban
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Icon
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffold
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.ryderdavid.nacre.AppContainer
import com.ryderdavid.nacre.ui.assistant.AssistantPane
import com.ryderdavid.nacre.ui.assistant.AssistantTarget
import com.ryderdavid.nacre.ui.bead.BeadDetail
import com.ryderdavid.nacre.ui.board.BoardScreen
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.repos.ReposScreen
import com.ryderdavid.nacre.ui.rounds.RoundDetail
import com.ryderdavid.nacre.ui.runs.RunDetail
import com.ryderdavid.nacre.ui.runs.RunsScreen
import com.ryderdavid.nacre.ui.settings.PairRequest
import com.ryderdavid.nacre.ui.settings.SettingsScreen
import com.ryderdavid.nacre.ui.theme.NacreTheme
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable

@Serializable data object BoardsRoute
@Serializable data class BoardRoute(val repoKey: String)
/** [openChange] opens the bead's change view straight away (from Needs you). */
@Serializable data class BeadRoute(val repoKey: String, val beadId: String, val openChange: Boolean = false)
@Serializable data class AssistantRoute(val repoKey: String)
@Serializable data object OrchestratorRoute
@Serializable data object RunsRoute
@Serializable data class RunRoute(val runKey: String)
@Serializable data class RoundRoute(val roundKey: String)
@Serializable data object SettingsRoute

/** Something the activity was opened with: a pairing link or a run notification. */
sealed interface DeepLink {
    data class Pair(val request: PairRequest) : DeepLink
    data class OpenRun(val runKey: String) : DeepLink
    data class OpenRound(val roundKey: String) : DeepLink
    /** A change notification: the bead whose change view to open. */
    data class OpenChange(val repoKey: String, val beadId: String) : DeepLink
}

private enum class Tab { BOARDS, RUNS, SETTINGS }

@Composable
fun NacreRoot(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    val prefs by container.settings.settings.collectAsState(initial = null)
    NacreTheme(themeMode = prefs?.themeMode ?: com.ryderdavid.nacre.data.settings.ThemeMode.SYSTEM, dynamicColor = prefs?.dynamicColor ?: true) {
        androidx.compose.runtime.CompositionLocalProvider(LocalContainer provides container) {
            AppShell(container, deepLink, onDeepLinkHandled)
        }
    }
}

@Composable
private fun AppShell(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    val nav = rememberNavController()
    val entry by nav.currentBackStackEntryAsState()
    val dest = entry?.destination
    val tab = when {
        dest?.hasRoute(RunsRoute::class) == true || dest?.hasRoute(RunRoute::class) == true || dest?.hasRoute(RoundRoute::class) == true -> Tab.RUNS
        dest?.hasRoute(SettingsRoute::class) == true -> Tab.SETTINGS
        else -> Tab.BOARDS
    }
    val runs by container.repository.runs.collectAsState()
    // Counts agent-owned sessions too: the badge stands for what the Runs tab is showing.
    val rounds by container.repository.rounds.collectAsState()
    // Rounds waiting on a decision or a landing count too: they need a look.
    val activeRuns = runs.count { it.isActive && it.inRunsList } + rounds.count { it.needsYou }
    // Boards' own badge counts Needs you; the Runs badge keeps its meaning.
    val needsYou = container.repository.attention.collectAsState().value.size
    val snackbar = remember { SnackbarHostState() }
    var pendingPair by remember { mutableStateOf<PairRequest?>(null) }

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
            is DeepLink.Pair -> { pendingPair = deepLink.request; nav.switchTab(SettingsRoute) }
            is DeepLink.OpenRun -> nav.navigate(RunRoute(deepLink.runKey))
            is DeepLink.OpenRound -> nav.navigate(RoundRoute(deepLink.roundKey))
            is DeepLink.OpenChange -> nav.navigate(BeadRoute(deepLink.repoKey, deepLink.beadId, openChange = true))
            null -> Unit
        }
        if (deepLink != null) onDeepLinkHandled()
    }

    val adaptive = currentWindowAdaptiveInfo()
    val detailRoute = dest?.let { d ->
        d.hasRoute(BeadRoute::class) || d.hasRoute(RunRoute::class) || d.hasRoute(RoundRoute::class) || d.hasRoute(AssistantRoute::class) || d.hasRoute(OrchestratorRoute::class)
    } == true
    val baseType = NavigationSuiteScaffoldDefaults.calculateFromAdaptiveInfo(adaptive)
    val layoutType = if (detailRoute && baseType == NavigationSuiteType.NavigationBar) NavigationSuiteType.None else baseType

    Box(Modifier.fillMaxSize()) {
        NavigationSuiteScaffold(
            layoutType = layoutType,
            navigationSuiteItems = {
                item(
                    selected = tab == Tab.BOARDS, onClick = { nav.switchTab(BoardsRoute) },
                    icon = { BadgedBox(badge = { if (needsYou > 0) Badge { Text("$needsYou") } }) { Icon(Icons.Rounded.ViewKanban, null) } },
                    label = { Text("Boards") },
                )
                item(
                    selected = tab == Tab.RUNS, onClick = { nav.switchTab(RunsRoute) },
                    icon = {
                        BadgedBox(badge = { if (activeRuns > 0) Badge { Text("$activeRuns") } }) { Icon(Icons.Rounded.Terminal, null) }
                    },
                    label = { Text("Runs") },
                )
                item(
                    selected = tab == Tab.SETTINGS, onClick = { nav.switchTab(SettingsRoute) },
                    icon = { Icon(Icons.Rounded.Settings, null) }, label = { Text("Settings") },
                )
            },
        ) {
            NavHost(nav, startDestination = BoardsRoute) {
                composable<BoardsRoute> {
                    ReposScreen(
                        onOpen = { nav.navigate(BoardRoute(it.key)) },
                        onPair = { nav.switchTab(SettingsRoute) },
                        onOpenAssistant = { nav.navigate(OrchestratorRoute) },
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onOpenChange = { repo, seed -> nav.navigate(BeadRoute(repo, seed, openChange = true)) },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
                    )
                }
                composable<BoardRoute> { e ->
                    val r = e.toRoute<BoardRoute>()
                    BoardScreen(
                        r.repoKey,
                        onBack = { nav.popBackStack() },
                        onOpenBead = { nav.navigate(BeadRoute(r.repoKey, it)) },
                        onOpenAssistant = { nav.navigate(AssistantRoute(r.repoKey)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onSwitchBoard = { key -> nav.navigate(BoardRoute(key)) { popUpTo<BoardRoute> { inclusive = true } } },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
                    )
                }
                composable<BeadRoute> { e ->
                    val r = e.toRoute<BeadRoute>()
                    BeadDetail(
                        r.repoKey, r.beadId, inPane = false, openChange = r.openChange,
                        onClose = { nav.popBackStack() },
                        onOpenBead = { nav.navigate(BeadRoute(r.repoKey, it)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
                    )
                }
                composable<AssistantRoute> { e ->
                    val r = e.toRoute<AssistantRoute>()
                    AssistantPane(
                        AssistantTarget.Board(r.repoKey, container.repository.repo(r.repoKey)?.name.orEmpty()), inPane = false,
                        onClose = { nav.popBackStack() },
                        onOpenBead = { _, bead -> nav.navigate(BeadRoute(r.repoKey, bead)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                    )
                }
                composable<OrchestratorRoute> {
                    AssistantPane(
                        AssistantTarget.AllBoards, inPane = false,
                        onClose = { nav.popBackStack() },
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                    )
                }
                composable<RunsRoute> {
                    RunsScreen(
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                    )
                }
                composable<RunRoute> { e ->
                    val r = e.toRoute<RunRoute>()
                    RunDetail(r.runKey, inPane = false, onBack = { nav.popBackStack() }, onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) })
                }
                composable<RoundRoute> { e ->
                    val r = e.toRoute<RoundRoute>()
                    RoundDetail(
                        r.roundKey, inPane = false, onBack = { nav.popBackStack() },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                    )
                }
                composable<SettingsRoute> {
                    SettingsScreen(pending = pendingPair, onPendingHandled = { pendingPair = null })
                }
            }
        }
        SnackbarHost(
            snackbar,
            Modifier.align(Alignment.BottomCenter).navigationBarsPadding()
                .padding(bottom = if (layoutType == NavigationSuiteType.NavigationBar) 88.dp else 16.dp),
        )
    }
}

/** Top-level navigation that keeps one copy of each tab and restores its state. */
private fun NavHostController.switchTab(route: Any) {
    navigate(route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}
