package com.ryderdavid.nacre.data.model

import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlin.test.assertNull

/** The bridge's bd-only marker for a bead someone holds while no run here works on it. */
class ActiveElsewhereTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test fun decodesFromTheIssueAndIsAbsentByDefault() {
        val marked = json.decodeFromString(Issue.serializer(),
            """{"id":"NAC-1","status":"in_progress","assignee":"codex","active_elsewhere":{"assignee":"codex","since":"2026-10-24T10:00:00Z"}}""")
        assertEquals(ActiveElsewhere("codex", "2026-10-24T10:00:00Z"), marked.activeElsewhere)
        assertNull(json.decodeFromString(Issue.serializer(), """{"id":"NAC-2"}""").activeElsewhere)
    }

    @Test fun aLiveRunsMarkerWins() {
        val i = Issue(id = "NAC-1", status = "in_progress", activeElsewhere = ActiveElsewhere("codex"))
        assertEquals("codex", i.elsewhere(liveRun = false)?.assignee)
        assertNull(i.elsewhere(liveRun = true))
        assertNull(i.copy(status = "closed").elsewhere(liveRun = false), "a stale copy of a closed bead")
    }

    @Test fun theUsersOwnClaimIsAboutTheSessionNotThePerson() {
        val mine = ActiveElsewhere("Ryder Cobean", self = true)
        assertEquals("Active in another session", mine.title)
        assertTrue(mine.detail("3 min. ago").startsWith("Claimed 3 min. ago from a session Nacre isn't running"))
        assertFalse("Ryder" in mine.detail(""), "one person: naming them says nothing")
        val theirs = ActiveElsewhere("codex")
        assertEquals("Active elsewhere · codex", theirs.title)
        assertTrue(theirs.detail("").startsWith("Claimed by codex. "))
    }

    @Test fun aStaleAgentSessionIsNotALiveRun() {
        val keys = setOf(Keys.of("h", "repo"))
        val working = Run(id = "r_1", repoId = "repo", issueId = "NAC-1", status = "running", origin = ORIGIN_ADOPTED, hostId = "h")
        val parked = working.copy(id = "r_2", issueId = "NAC-2", stale = true)
        val done = working.copy(id = "r_3", issueId = "NAC-3", status = "succeeded")
        val otherRepo = working.copy(id = "r_4", issueId = "NAC-4", repoId = "other")
        assertEquals(setOf("NAC-1"), liveRunIssueIds(listOf(working, parked, done, otherRepo), keys))
    }
}
