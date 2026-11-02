package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.Round
import kotlin.test.Test
import kotlin.test.assertEquals

class StartWorkLabelTest {
    @Test fun saysWhatTheRoundIsDoing() {
        assertEquals("Start work", startWorkLabel(null, busy = false))
        assertEquals("Starting…", startWorkLabel(null, busy = true))
        assertEquals("Running", startWorkLabel(Round(id = "r", state = "implementing"), busy = false))
        assertEquals("Needs you", startWorkLabel(Round(id = "r", state = ROUND_NEEDS_DECISION), busy = false))
    }
}
