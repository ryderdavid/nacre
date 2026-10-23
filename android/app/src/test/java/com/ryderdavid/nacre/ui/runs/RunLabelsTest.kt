package com.ryderdavid.nacre.ui.runs

import com.ryderdavid.nacre.data.model.ACTIVITY_IDLE
import com.ryderdavid.nacre.data.model.ACTIVITY_WORKING
import com.ryderdavid.nacre.data.model.ORIGIN_ADOPTED
import com.ryderdavid.nacre.data.model.Run
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** What the Runs tab calls a session nobody started from here. */
class RunLabelsTest {
    @Test fun activityIsSpelledOutOnlyForAnAdoptedSession() {
        assertEquals("working", activityLabel(ACTIVITY_WORKING))
        assertEquals("waiting for input", activityLabel(ACTIVITY_IDLE))
        assertNull(activityLabel(""), "our own runs say everything in their status")
    }

    @Test fun anAdoptedSessionSaysWhoIsWorkingAndWhether() {
        val adopted = Run(id = "r_1", status = "running", origin = ORIGIN_ADOPTED, activity = ACTIVITY_WORKING)
        assertEquals("An agent is working in this session…", workingLabel(adopted))
        assertEquals("The agent's session is waiting for input on the Mac…", workingLabel(adopted.copy(activity = ACTIVITY_IDLE)))
        assertEquals("Claude is working…", workingLabel(Run(id = "r_2", status = "running")))
        assertEquals("Waiting for a free slot on the Mac…", workingLabel(Run(id = "r_3", status = "queued")))
    }

    @Test fun aSessionThatMovedNoBeadFallsBackToItsOwnName() {
        val adopted = Run(id = "r_4", status = "running", origin = ORIGIN_ADOPTED, sessionName = "scratch")
        assertEquals("scratch", runTitle(adopted))
        assertEquals("Agent session", runTitle(adopted.copy(sessionName = "")))
        assertEquals("Fix the parser", runTitle(adopted.copy(issueTitle = "Fix the parser")))
    }
}
