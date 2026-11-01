package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.unreadableBoards
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Keys
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** The board assistant across every board, run on the Mac. */
class AcrossBoardsTest {
    private fun board(name: String, vararg hosts: String) = SpanBoard(name, hosts.map { Keys.of(it, "${name.lowercase()}-id") })

    private val boards = listOf(
        board("Nacre", "studio", "laptop"),
        board("Home", "studio"),
        board("Notes", "laptop"),
        board("Garden", "studio"),
    )

    @Test fun runsOnTheComputerHoldingTheMostBoardsAndNamesTheRest() {
        val plan = planAcross(boards, hostOrder = listOf("laptop", "studio"))!!
        assertEquals("studio", plan.hostId)
        assertEquals(listOf("Nacre" to "studio~nacre-id", "Home" to "studio~home-id", "Garden" to "studio~garden-id"), plan.read)
        assertEquals(listOf("Notes"), plan.elsewhere.map { it.name })
    }

    @Test fun aTieGoesToTheFirstPairedComputer() {
        val two = listOf(board("A", "studio"), board("B", "laptop"))
        assertEquals("laptop", planAcross(two, listOf("laptop", "studio"))!!.hostId)
        assertEquals("studio", planAcross(two, listOf("studio", "laptop"))!!.hostId)
    }

    @Test fun aConversationStaysOnItsComputer() {
        // Its session lives there, so later turns don't move even when another holds more.
        assertEquals("laptop", planAcross(boards, listOf("studio"), pinned = "laptop")!!.hostId)
        // Unless that computer no longer holds any board.
        assertEquals("studio", planAcross(boards, listOf("studio"), pinned = "gone")!!.hostId)
    }

    @Test fun noBoardsMeansNoPlan() {
        assertNull(planAcross(emptyList(), listOf("studio")))
    }

    @Test fun thePromptStandsInForListBoardsAndSaysWhatItCantRead() {
        val tools = assistantTools(everyBoard = true)
        val p = Prompts.macAcrossBoards("BASE", tools, "Nacre — 3 open", "Mac Studio", listOf("Notes: only on Laptop, not on Mac Studio"))
        assertTrue(p.startsWith("BASE"))
        assertTrue("Nacre — 3 open" in p)
        assertTrue("Notes: only on Laptop" in p)
        assertTrue("tell Ryder they weren't read" in p)
        assertTrue("`board` set to the board's name" in p)
        // Proposals name their board: the catalogue's write tools all take one.
        assertTrue(tools.filter { it.write }.all { "board" in it.required })
        assertFalse("in the repository this board belongs to" in p)
    }

    @Test fun theTurnMessageCarriesTheBoardsNacredCouldntOpen() {
        val turn = Json.parseToJsonElement("""{"turn_id":"t_1","unreadable":[{"name":"Bare","reason":"its .beads folder is missing"},{"name":"Gone"}]}""") as JsonObject
        assertEquals(listOf("Bare: its .beads folder is missing", "Gone"), unreadableBoards(turn))
        assertEquals(emptyList(), unreadableBoards(JsonObject(mapOf("turn_id" to JsonPrimitive("t_2")))))
    }

    @Test fun followTurnPassesThoseBoardsOn() = runBlocking {
        val noted = mutableListOf<String>()
        followTurn(
            first = flow {
                emit(AssistMessage.Turn("t_1", listOf("Gone: not found on this computer")))
                emit(AssistMessage.Done(AssistResult(sessionId = "s-1")))
            },
            progress = TurnProgress("hi"),
            resume = { _, _ -> flow {} },
            relay = {},
            dropped = object : DropListener {
                override fun lost(attempt: Int) = Unit
                override fun back() = Unit
            },
            noted = { noted += it },
        )
        assertEquals(listOf("Gone: not found on this computer"), noted)
    }
}
