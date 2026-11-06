package com.ryderdavid.nacre.ui.workspace

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WorkspaceRulesTest {
    @Test
    fun `unfolded at 840dp leaves 728dp and splits evenly`() {
        assertEquals(728, WorkspaceRules.usable(840))
        assertEquals(WorkspaceLayout.Split(364, 364), WorkspaceRules.layout(840, 740, Destination.BEADS, chatAlone = false, chatDp = null))
    }

    @Test
    fun `presets at 728dp`() {
        assertEquals(408, WorkspaceRules.preset(PanePreset.CHAT_WIDE, 728))
        assertEquals(364, WorkspaceRules.preset(PanePreset.EVEN, 728))
        assertEquals(360, WorkspaceRules.preset(PanePreset.PANE_WIDE, 728))
    }

    @Test
    fun `at 690dp even breaks chat's minimum and isn't offered`() {
        assertNull(WorkspaceRules.preset(PanePreset.EVEN, 690))
        assertEquals(listOf(PanePreset.CHAT_WIDE, PanePreset.PANE_WIDE), WorkspaceRules.offered(690))
        assertEquals(360, WorkspaceRules.reset(690))
    }

    @Test
    fun `reset at 680dp gives chat 360 and the pane 320`() {
        assertEquals(360, WorkspaceRules.reset(680))
        assertEquals(WorkspaceLayout.Split(360, 320), WorkspaceRules.layout(680 + 96 + 16, 740, Destination.SPEC, false, null))
    }

    @Test
    fun `below 680dp usable one destination shows`() {
        val layout = WorkspaceRules.layout(600 + 96 + 16 - 40, 900, Destination.BEADS, false, null)
        assertEquals(WorkspaceLayout.Single(rail = true), layout)
        assertTrue(WorkspaceRules.offered(600).isEmpty())
    }

    @Test
    fun `the cover screen gets the bottom bar, and compact height a rail`() {
        assertEquals(WorkspaceLayout.Single(rail = false), WorkspaceRules.layout(412, 915, Destination.CHAT, false, null))
        assertEquals(WorkspaceLayout.Single(rail = true), WorkspaceRules.layout(560, 400, Destination.CHAT, false, null))
    }

    @Test
    fun `chat alone shows one pane even when two would fit`() {
        assertEquals(WorkspaceLayout.Single(rail = true), WorkspaceRules.layout(840, 740, Destination.CHAT, chatAlone = true, chatDp = null))
    }

    @Test
    fun `dragging clamps to the minimums`() {
        assertEquals(360, WorkspaceRules.clamp(200, 728))
        assertEquals(408, WorkspaceRules.clamp(700, 728))
    }
}
