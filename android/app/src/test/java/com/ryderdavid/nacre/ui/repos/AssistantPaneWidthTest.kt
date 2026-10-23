package com.ryderdavid.nacre.ui.repos

import kotlin.test.Test
import kotlin.test.assertEquals

class AssistantPaneWidthTest {
    @Test fun aWidthThatLeavesTheBoardsListRoomIsKept() {
        assertEquals(420f, clampAssistantPaneWidth(420f, 1000f))
        assertEquals(640f, clampAssistantPaneWidth(640f, 1000f), "the boards list may shrink to its minimum")
    }

    @Test fun draggingPastEitherEndStopsAtThePaneMinimum() {
        assertEquals(320f, clampAssistantPaneWidth(40f, 1000f))
        assertEquals(640f, clampAssistantPaneWidth(2000f, 1000f))
    }

    @Test fun aWindowTooNarrowForBothMinimumsKeepsTheAssistantMinimum() {
        assertEquals(320f, clampAssistantPaneWidth(500f, 500f))
    }
}
