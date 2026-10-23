package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.model.Repo
import kotlin.test.Test
import kotlin.test.assertEquals

/** How the cross-board assistant turns the board name it wrote back into a board. */
class MatchBoardsTest {
    private fun group(name: String, path: String = "/Users/r/Code/$name") =
        Repo(id = name, name = name, path = path, hostId = "c0", hostName = "Mac-Studio")
            .let { RepoGroup(it.groupId, it, listOf(it)) }

    private val boards = listOf(group("Nacre"), group("nacred-docs"), group("Beads", "/Users/r/Beads"))

    private fun match(name: String) = AllBoardsRouter.matchBoards(name, boards).map { it.home.name }

    @Test fun anExactNameWinsEvenWhenItIsAPrefixOfAnother() {
        assertEquals(listOf("Nacre"), match("Nacre"))
        assertEquals(listOf("Nacre"), match("nacre"), "names are matched case-insensitively")
    }

    @Test fun aPartialNameOrPathStillFindsTheBoard() {
        assertEquals(listOf("nacred-docs"), match("docs"))
        assertEquals(listOf("Beads"), match("/Users/r/Beads"))
    }

    @Test fun anAmbiguousNameReturnsEveryCandidateSoTheAssistantCanBeAsked() {
        assertEquals(listOf("Nacre", "nacred-docs"), match("nacr"))
    }

    @Test fun anUnknownNameMatchesNothing() {
        assertEquals(emptyList(), match("widgets"))
    }
}
