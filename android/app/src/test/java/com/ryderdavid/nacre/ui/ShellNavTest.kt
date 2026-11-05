package com.ryderdavid.nacre.ui

import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import com.ryderdavid.nacre.ui.chat.newChatLabel
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class ShellNavTest {
    private val bar = NavigationSuiteType.NavigationBar
    private val rail = NavigationSuiteType.NavigationRail

    // Every route the app shows, details included: none of them hides the bar any more.
    private val routes: List<Any> = listOf(
        BoardsRoute, InboxRoute, RunsRoute, ChatsRoute, SettingsRoute,
        BoardRoute("mac~nacre"), BeadRoute("mac~nacre", "NAC-3"), BeadRoute("mac~nacre", "NAC-3", openChange = true),
        RunRoute("mac~r1"), RoundRoute("mac~r-12"),
    )

    @Test fun barShowsOnEveryRouteIncludingBeadRunAndRound() {
        for (route in routes) {
            assertEquals(bar, shellLayoutType(bar, imeVisible = false), "bar on $route")
        }
    }

    @Test fun barHidesOnlyWhileTheKeyboardIsOpen() {
        assertEquals(NavigationSuiteType.None, shellLayoutType(bar, imeVisible = true))
        assertEquals(bar, shellLayoutType(bar, imeVisible = false))
    }

    @Test fun railNeverHides() {
        assertEquals(rail, shellLayoutType(rail, imeVisible = true))
        assertEquals(rail, shellLayoutType(rail, imeVisible = false))
    }

    @Test fun fourItemsInOrder() {
        assertEquals(listOf("Boards", "Inbox", "Activity", "Chat"), Tab.entries.map { it.label })
        assertEquals(listOf<Any>(BoardsRoute, InboxRoute, RunsRoute, ChatsRoute), Tab.entries.map { it.route })
    }

    @Test fun chatIsATopLevelTabSwitch() {
        assertEquals(NavStep.SwitchTab(ChatsRoute), ShellNav.tab(Tab.CHAT))
        assertEquals(Tab.CHAT, tabFor(ChatsRoute))
        assertEquals(Tab.ACTIVITY, tabFor(RoundRoute("mac~r-12")))
        assertEquals(Tab.ACTIVITY, tabFor(RunRoute("mac~r1")))
        assertEquals(Tab.BOARDS, tabFor(BeadRoute("mac~nacre", "NAC-3")))
    }

    private fun originAfter(vararg shown: Any): ScreenOrigin? =
        shown.fold(null as ScreenOrigin?) { acc, r -> rememberedOrigin(acc, r) { key -> if (key == "mac~nacre") "Nacre" else null } }

    @Test fun rememberedScreenAndLabelForEachOrigin() {
        val cases: List<Pair<Any, String>> = listOf(
            BoardRoute("mac~nacre") to "Nacre",
            BeadRoute("mac~nacre", "NAC-48p.5") to "NAC-48p.5",
            BeadRoute("mac~nacre", "NAC-lpu", openChange = true) to "change · NAC-lpu",
            RoundRoute("mac~r-12") to "round r-12",
            RunRoute("mac~run-7") to "run run-7",
            BoardsRoute to "all boards",
            InboxRoute to "all boards",
            RunsRoute to "all boards",
        )
        for ((route, label) in cases) {
            // Switching to Chat keeps the screen shown before it.
            val origin = originAfter(route, ChatsRoute)
            assertEquals(ScreenOrigin.of(route) { if (it == "mac~nacre") "Nacre" else null }, origin, "origin for $route")
            assertEquals(label, chatOriginLabel(origin), "label for $route")
            assertEquals("New · $label", newChatLabel(chatOriginLabel(origin)))
        }
    }

    @Test fun originKindsAreDistinct() {
        assertEquals(ScreenOrigin.AllBoards("inbox"), originAfter(InboxRoute, ChatsRoute))
        assertEquals(ScreenOrigin.AllBoards("activity"), originAfter(RunsRoute, ChatsRoute))
        assertEquals(ScreenOrigin.Change("mac~nacre", "NAC-lpu"), originAfter(BeadRoute("mac~nacre", "NAC-lpu", openChange = true), ChatsRoute))
    }

    @Test fun chatAndSettingsKeepTheLastScreen() {
        assertEquals(ScreenOrigin.Round("mac~r-12"), originAfter(BeadRoute("mac~nacre", "NAC-3"), RoundRoute("mac~r-12"), SettingsRoute, ChatsRoute, ChatsRoute))
    }

    @Test fun noScreenYetMeansAllBoards() {
        assertNull(originAfter(ChatsRoute))
        assertEquals("all boards", chatOriginLabel(null))
        assertEquals("New · all boards", newChatLabel(chatOriginLabel(null)))
    }

    @Test fun boardWithoutAKnownNameUsesItsId() {
        assertEquals("other", chatOriginLabel(originAfter(BoardRoute("mac~other"))))
    }

    /** The back stack NavController keeps: [NavStep.Push] adds on top; tabs pop to the start destination and save state. */
    private class RouteStack {
        val stack = mutableListOf<Any>(BoardsRoute)
        private val saved = mutableMapOf<Any, List<Any>>()
        val current get() = stack.last()
        fun go(step: NavStep) {
            when (step) {
                is NavStep.Push -> stack += step.route
                is NavStep.SwitchTab -> {
                    if (current == step.route) return
                    // The tab shown now, by its root: other tabs sit on top of the Boards start destination.
                    val roots = Tab.entries.map { it.route }
                    val shownRoot = stack.getOrNull(1)?.takeIf { it in roots } ?: BoardsRoute
                    saved[shownRoot] = stack.drop(1)
                    stack.subList(1, stack.size).clear()
                    stack.addAll(saved[step.route] ?: if (step.route == BoardsRoute) emptyList() else listOf(step.route))
                }
            }
        }
        fun back() { if (stack.size > 1) stack.removeAt(stack.lastIndex) }
    }

    @Test fun backFromABeadReturnsToTheBoard() {
        val nav = RouteStack()
        nav.go(ShellNav.openBoard("mac~nacre"))
        nav.go(ShellNav.openBead("mac~nacre", "NAC-3"))
        assertEquals(BeadRoute("mac~nacre", "NAC-3"), nav.current)
        nav.back()
        assertEquals(BoardRoute("mac~nacre"), nav.current)
    }

    @Test fun chatAndBackKeepTheBoardsStack() {
        val nav = RouteStack()
        nav.go(ShellNav.openBoard("mac~nacre"))
        nav.go(ShellNav.openBead("mac~nacre", "NAC-3"))
        nav.go(ShellNav.tab(Tab.CHAT))
        assertEquals(ChatsRoute, nav.current)
        nav.go(ShellNav.tab(Tab.BOARDS))
        assertEquals(BeadRoute("mac~nacre", "NAC-3"), nav.current)
        nav.back()
        assertEquals(BoardRoute("mac~nacre"), nav.current)
    }
}
