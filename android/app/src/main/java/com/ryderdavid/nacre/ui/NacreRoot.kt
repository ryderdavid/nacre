package com.ryderdavid.nacre.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.vector.ImageVector
import com.ryderdavid.nacre.ui.chat.ChatsScreen
import androidx.compose.material.icons.automirrored.rounded.Chat
import com.ryderdavid.nacre.ui.common.LocalScreenOverlay
import com.ryderdavid.nacre.ui.common.ScreenOverlay
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Inbox
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
import com.ryderdavid.nacre.data.model.activityBadgeCount
import com.ryderdavid.nacre.ui.bead.BeadDetail
import com.ryderdavid.nacre.ui.board.BoardScreen
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.repos.ReposScreen
import com.ryderdavid.nacre.ui.repos.InboxScreen
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
@Serializable data object InboxRoute
@Serializable data object RunsRoute
@Serializable data class RunRoute(val runKey: String)
@Serializable data class RoundRoute(val roundKey: String)
@Serializable data object SettingsRoute
@Serializable data object ChatsRoute

/** Something the activity was opened with: a pairing link or a run notification. */
sealed interface DeepLink {
    data class Pair(val request: PairRequest) : DeepLink
    data class OpenRun(val runKey: String) : DeepLink
    data class OpenRound(val roundKey: String) : DeepLink
    /** A change notification: the bead whose change view to open. */
    data class OpenChange(val repoKey: String, val beadId: String) : DeepLink
}

@Composable
fun NacreRoot(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    val prefs by container.settings.settings.collectAsState(initial = null)
    NacreTheme(themeMode = prefs?.themeMode ?: com.ryderdavid.nacre.data.settings.ThemeMode.SYSTEM) {
        androidx.compose.runtime.CompositionLocalProvider(LocalContainer provides container) {
            AppShell(container, deepLink, onDeepLinkHandled)
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AppShell(container: AppContainer, deepLink: DeepLink?, onDeepLinkHandled: () -> Unit) {
    LaunchedEffect(container) {
        while (true) {
            container.chatStack.expire()
            kotlinx.coroutines.delay(1_000)
        }
    }
    val nav = rememberNavController()
    val entry by nav.currentBackStackEntryAsState()
    val shown: Any? = entry?.let { e ->
        val d = e.destination
        when {
            d.hasRoute(BoardsRoute::class) -> BoardsRoute
            d.hasRoute(InboxRoute::class) -> InboxRoute
            d.hasRoute(RunsRoute::class) -> RunsRoute
            d.hasRoute(ChatsRoute::class) -> ChatsRoute
            d.hasRoute(SettingsRoute::class) -> SettingsRoute
            d.hasRoute(BoardRoute::class) -> e.toRoute<BoardRoute>()
            d.hasRoute(BeadRoute::class) -> e.toRoute<BeadRoute>()
            d.hasRoute(RunRoute::class) -> e.toRoute<RunRoute>()
            d.hasRoute(RoundRoute::class) -> e.toRoute<RoundRoute>()
            else -> null
        }
    }
    val tab = tabFor(shown)
    // The screen last shown before Chat: what the Chat tab is about and what New · <label> starts from.
    var origin by remember { mutableStateOf<ScreenOrigin?>(null) }
    LaunchedEffect(shown) { origin = rememberedOrigin(origin, shown) { container.repository.repo(it)?.name } }
    val overlay = remember { ScreenOverlay() }
    val runs by container.repository.runs.collectAsState()
    // Counts agent-owned sessions too: the badge stands for what the Runs tab is showing.
    val rounds by container.repository.rounds.collectAsState()
    // Only work that is going on: a round waiting on the user is Inbox's to count.
    val activeRuns = activityBadgeCount(runs, rounds)
    // Inbox owns the attention badge; Activity retains the running-session badge.
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
    val layoutType = shellLayoutType(NavigationSuiteScaffoldDefaults.calculateFromAdaptiveInfo(adaptive), WindowInsets.isImeVisible)

    NavigationSuiteScaffold(
        layoutType = layoutType,
        navigationSuiteItems = {
            Tab.entries.forEach { t ->
                val badge = when (t) {
                    Tab.INBOX -> needsYou
                    Tab.ACTIVITY -> activeRuns
                    else -> 0
                }
                item(
                    selected = tab == t, onClick = { nav.go(ShellNav.tab(t)) },
                    icon = { BadgedBox(badge = { if (badge > 0) Badge { Text("$badge") } }) { Icon(t.icon, null) } },
                    label = { Text(t.label) },
                )
            }
        },
    ) {
        // The content area ends above the bar and beside the rail: everything drawn here sits above the bar.
        Box(Modifier.fillMaxSize()) {
          CompositionLocalProvider(LocalScreenOverlay provides overlay) {
            NavHost(nav, startDestination = BoardsRoute) {
                composable<BoardsRoute> {
                    ReposScreen(
                        onOpen = { nav.navigate(BoardRoute(it.key)) },
                        onPair = { nav.switchTab(SettingsRoute) },
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                    )
                }
                composable<InboxRoute> {
                    InboxScreen(
                        onOpenBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                        onOpenChange = { repo, seed -> nav.navigate(BeadRoute(repo, seed, openChange = true)) },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
                    )
                }
                composable<BoardRoute> { e ->
                    val r = e.toRoute<BoardRoute>()
                    BoardScreen(
                        r.repoKey,
                        onBack = { nav.popBackStack() },
                        onOpenBead = { nav.go(ShellNav.openBead(r.repoKey, it)) },
                        onOpenChatBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
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
                        onOpenChatBead = { repo, bead -> nav.navigate(BeadRoute(repo, bead)) },
                        onOpenRun = { nav.navigate(RunRoute(it)) },
                        onOpenRound = { nav.navigate(RoundRoute(it)) },
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
                    SettingsScreen(pending = pendingPair, onPendingHandled = { pendingPair = null }, onBack = { nav.popBackStack() })
                }
                composable<ChatsRoute> {
                    ChatsScreen(chatOriginLabel(origin))
                }
            }
          }
            overlay.content?.invoke()
            // The scaffold has already taken the bar's insets, so only the system bars still showing are padded.
            SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 16.dp))
        }
    }
}

private val Tab.icon: ImageVector
    get() = when (this) {
        Tab.BOARDS -> Icons.Rounded.ViewKanban
        Tab.INBOX -> Icons.Rounded.Inbox
        Tab.ACTIVITY -> Icons.Rounded.Terminal
        Tab.CHAT -> Icons.AutoMirrored.Rounded.Chat
    }

private fun NavHostController.go(step: NavStep) = when (step) {
    is NavStep.Push -> navigate(step.route)
    is NavStep.SwitchTab -> switchTab(step.route)
}

/** Top-level navigation that keeps one copy of each tab and restores its state. */
private fun NavHostController.switchTab(route: Any) {
    navigate(route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}
