package com.ryderdavid.nacre.ui.runs

import kotlin.test.Test
import kotlin.test.assertEquals

class PaneWidthTest {
    @Test fun aWidthThatLeavesBothPanesRoomIsKept() {
        assertEquals(400f, clampListPaneWidth(400f, 1000f))
        assertEquals(680f, clampListPaneWidth(680f, 1000f), "the detail pane may shrink to its minimum")
    }

    @Test fun draggingPastEitherEndStopsAtThePaneMinimum() {
        assertEquals(280f, clampListPaneWidth(40f, 1000f))
        assertEquals(680f, clampListPaneWidth(2000f, 1000f))
    }

    @Test fun aWindowTooNarrowForBothMinimumsKeepsTheListMinimum() {
        assertEquals(280f, clampListPaneWidth(400f, 400f))
    }
}
