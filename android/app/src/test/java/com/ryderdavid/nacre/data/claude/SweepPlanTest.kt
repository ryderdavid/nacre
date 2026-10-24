package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.model.Repo
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** Which boards a board-less list_beads reads, decided from the scan's counts alone. */
class SweepPlanTest {
    private fun group(name: String, updated: String, open: Int = 1, closed: Int = 0, error: String = "") =
        Repo(
            id = name, name = name, path = "/Users/r/Code/$name", lastUpdated = updated, error = error,
            counts = Repo.Counts(open = open, closed = closed, total = open + closed),
            hostId = "c0", hostName = "Mac-Studio",
        ).let { RepoGroup(it.groupId, it, listOf(it)) }

    /** Board01 is the stalest, Board09 the most recently updated. */
    private val nine = (1..9).map { group("Board%02d".format(it), "2026-10-%02dT00:00:00Z".format(it)) }

    private fun names(gs: List<RepoGroup>) = gs.map { it.home.name }

    @Test fun aSweepReadsAtMostTheFiveMostRecentlyUpdatedBoards() {
        val plan = AllBoardsRouter.planSweep(nine, emptySet(), lane = null)
        assertEquals(5, AllBoardsRouter.MAX_BOARDS_SWEPT)
        assertEquals(listOf("Board09", "Board08", "Board07", "Board06", "Board05"), names(plan.targets))
        assertEquals(listOf("Board04", "Board03", "Board02", "Board01"), names(plan.notRead))
    }

    @Test fun aCutShortSweepSaysSoAndNamesWhatItSkipped() {
        val note = AllBoardsRouter.sweepNote(AllBoardsRouter.planSweep(nine, emptySet(), lane = null))
        assertTrue("Sweep cut short" in note, note)
        assertTrue("5 most recently updated of 9" in note, note)
        assertTrue("Board04, Board03, Board02, Board01" in note, note)
        assertFalse("hidden" in note, note)
    }

    @Test fun aSweepThatReadEverythingAddsNoNote() {
        val plan = AllBoardsRouter.planSweep(nine.take(3), emptySet(), lane = null)
        assertEquals(3, plan.targets.size)
        assertEquals("", AllBoardsRouter.sweepNote(plan))
    }

    @Test fun hiddenBoardsAreSkippedButNamedInTheNote() {
        val boards = nine.take(3) + group("Secret", "2026-10-30T00:00:00Z") + group("Attic", "2026-10-29T00:00:00Z")
        val plan = AllBoardsRouter.planSweep(boards, setOf("c0~Secret", "c0~Attic"), lane = null)
        assertFalse("Secret" in names(plan.targets), "a hidden board is never swept, however recent")
        assertEquals(listOf("Attic", "Secret"), names(plan.hidden))
        val note = AllBoardsRouter.sweepNote(plan)
        assertTrue("Plus 2 hidden: Attic, Secret" in note, note)
        assertFalse("cut short" in note, note)
    }

    @Test fun boardsWithNothingToShowDoNotTakeASweepSlot() {
        val boards = listOf(
            group("Broken", "2026-10-30T00:00:00Z", error = "repo moved"),
            group("Empty", "2026-10-29T00:00:00Z", open = 0),
            group("AllDone", "2026-10-28T00:00:00Z", open = 0, closed = 4),
            group("Busy", "2026-10-01T00:00:00Z"),
        )
        assertEquals(listOf("AllDone", "Busy"), names(AllBoardsRouter.planSweep(boards, emptySet(), lane = null).targets))
        assertEquals(listOf("Busy"), names(AllBoardsRouter.planSweep(boards, emptySet(), lane = "ready").targets), "open lanes skip all-done boards")
    }
}
