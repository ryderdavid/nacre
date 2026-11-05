package com.ryderdavid.nacre.ui

import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import com.ryderdavid.nacre.data.model.Keys

/** The four top-level destinations, in the order the bar and the rail show them. */
internal enum class Tab(val label: String, val route: Any) {
    BOARDS("Boards", BoardsRoute),
    INBOX("Inbox", InboxRoute),
    ACTIVITY("Activity", RunsRoute),
    CHAT("Chat", ChatsRoute),
}

/** The tab a shown route belongs to. Details live under the tab that lists them. */
internal fun tabFor(route: Any?): Tab = when (route) {
    is RunsRoute, is RunRoute, is RoundRoute -> Tab.ACTIVITY
    is InboxRoute -> Tab.INBOX
    is ChatsRoute -> Tab.CHAT
    else -> Tab.BOARDS
}

/**
 * The navigation suite for the window. The bar shows on every screen, detail screens included,
 * and hides only while the keyboard is open, so text fields keep their room. The rail never hides.
 */
internal fun shellLayoutType(adaptive: NavigationSuiteType, imeVisible: Boolean): NavigationSuiteType =
    if (adaptive == NavigationSuiteType.NavigationBar && imeVisible) NavigationSuiteType.None else adaptive

/** One navigation step: a detail pushed on the current tab, or a switch to a top-level tab. */
internal sealed interface NavStep {
    val route: Any
    /** Pushed on top, so Back returns to the screen it was opened from. */
    data class Push(override val route: Any) : NavStep
    /** A top-level destination: one copy of each tab, with its state kept. */
    data class SwitchTab(override val route: Any) : NavStep
}

internal object ShellNav {
    fun tab(tab: Tab): NavStep = NavStep.SwitchTab(tab.route)
    fun openBoard(repoKey: String): NavStep = NavStep.Push(BoardRoute(repoKey))
    fun openBead(repoKey: String, beadId: String): NavStep = NavStep.Push(BeadRoute(repoKey, beadId))
    fun openChange(repoKey: String, beadId: String): NavStep = NavStep.Push(BeadRoute(repoKey, beadId, openChange = true))
    fun openRun(runKey: String): NavStep = NavStep.Push(RunRoute(runKey))
    fun openRound(roundKey: String): NavStep = NavStep.Push(RoundRoute(roundKey))
}

/** The screen Ryder was looking at, as the Chat tab talks about it. */
sealed interface ScreenOrigin {
    /** Short name for "About: <label>" and "New · <label>". */
    val label: String

    /** Boards, Inbox and Activity are about every board on the computer. */
    data class AllBoards(val screen: String) : ScreenOrigin {
        override val label get() = ALL_BOARDS
    }
    data class Board(val repoKey: String, val name: String) : ScreenOrigin {
        override val label get() = name.ifBlank { Keys.id(repoKey) }
    }
    data class Bead(val repoKey: String, val beadId: String) : ScreenOrigin {
        override val label get() = beadId
    }
    /** The change view of a bead (its seed or epic). */
    data class Change(val repoKey: String, val beadId: String) : ScreenOrigin {
        override val label get() = "change · $beadId"
    }
    data class Round(val roundKey: String) : ScreenOrigin {
        override val label get() = "round ${Keys.id(roundKey)}"
    }
    data class Run(val runKey: String) : ScreenOrigin {
        override val label get() = "run ${Keys.id(runKey)}"
    }

    companion object {
        const val ALL_BOARDS = "all boards"

        /** The origin a shown route stands for; null for Chat and Settings, which aren't about a screen. */
        fun of(route: Any?, boardName: (String) -> String? = { null }): ScreenOrigin? = when (route) {
            is BoardsRoute -> AllBoards("boards")
            is InboxRoute -> AllBoards("inbox")
            is RunsRoute -> AllBoards("activity")
            is BoardRoute -> Board(route.repoKey, boardName(route.repoKey).orEmpty())
            is BeadRoute -> if (route.openChange) Change(route.repoKey, route.beadId) else Bead(route.repoKey, route.beadId)
            is RoundRoute -> Round(route.roundKey)
            is RunRoute -> Run(route.runKey)
            else -> null
        }
    }
}

/**
 * What the Chat tab remembers as "the screen last shown before Chat": every other screen replaces it,
 * while Chat and Settings keep it.
 */
internal fun rememberedOrigin(previous: ScreenOrigin?, shown: Any?, boardName: (String) -> String? = { null }): ScreenOrigin? =
    ScreenOrigin.of(shown, boardName) ?: previous

/** The label the Chat tab uses: the remembered screen's, or all boards when there was none. */
internal fun chatOriginLabel(origin: ScreenOrigin?): String = origin?.label ?: ScreenOrigin.ALL_BOARDS

/**
 * Whether the app shell shows the current chat beside (or over) [shown]. A board hosts the chat
 * itself, sharing its right pane slot with the bead detail; the Chat tab shows it full screen;
 * Settings has no chat. Every other screen keeps the pane docked, or collapsed, as Ryder moves.
 */
fun shellHostsChat(shown: Any?): Boolean = when (shown) {
    is BoardRoute, ChatsRoute, SettingsRoute, null -> false
    else -> true
}
