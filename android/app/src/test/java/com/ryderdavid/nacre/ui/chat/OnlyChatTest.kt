package com.ryderdavid.nacre.ui.chat

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * No screen offers an AI chat other than Chat: the board assistant (one board and across
 * boards), the bead chat (Ask and Refine), its store and the draft step are gone, nothing in
 * the app calls nacred's assist endpoints any more, and no screen reaches Claude except
 * through Chat's records on nacred.
 */
class OnlyChatTest {
    private val main = File("src/main/java/com/ryderdavid/nacre")

    private fun sources(): Map<String, String> =
        main.walkTopDown().filter { it.isFile && it.extension == "kt" }
            .associate { it.relativeTo(main).invariantSeparatorsPath to it.readText() }

    @Test fun theRetiredChatsAreGone() {
        for (name in listOf(
            "com.ryderdavid.nacre.data.claude.BoardAssistant",
            "com.ryderdavid.nacre.data.claude.AllBoardsRouter",
            "com.ryderdavid.nacre.data.claude.RepoBoardTools",
            "com.ryderdavid.nacre.data.claude.BeadChats",
            "com.ryderdavid.nacre.data.claude.MacBackend",
            "com.ryderdavid.nacre.data.claude.MacSession",
            "com.ryderdavid.nacre.data.claude.ChatAttachments",
            "com.ryderdavid.nacre.data.claude.ApiSession",
            "com.ryderdavid.nacre.ui.bead.BeadChatKt",
            "com.ryderdavid.nacre.ui.bead.AboutChatKt",
            "com.ryderdavid.nacre.ui.assistant.AssistantPaneKt",
            "com.ryderdavid.nacre.ui.board.DraftChatKt",
        )) assertFailsWith<ClassNotFoundException>(name) { Class.forName(name) }
        assertTrue(main.isDirectory, "run from the app module")
        assertTrue(!File(main, "ui/assistant").exists(), "ui/assistant/ is removed")
    }

    @Test fun theAppNoLongerCallsTheAssistEndpoints() {
        val callers = sources().filter { (_, text) ->
            listOf("/v1/assist", "/claude/stream", "/claude/json").any { it in text }
        }.keys
        assertEquals(emptySet(), callers)
    }

    @Test fun noScreenReachesClaudeExceptThroughChat() {
        // The API-key backend stays in data/claude (with the Settings key check), but no
        // screen or view model constructs or calls it: Chat is the only AI chat in the app.
        val outside = sources().filterKeys { !it.startsWith("data/claude/") }
        val reaching = outside.filter { (_, text) ->
            listOf("ClaudeBackend", "ApiBackend", "BeadChatItem", "ChatMode.", "DraftChat", "AssistantSession").any { it in text }
        }.keys
        assertEquals(emptySet(), reaching)
        // The bead composer, the change box and the round-question box hand questions to Chat.
        val handOff = listOf("ui/bead/ThreadComposer.kt", "ui/bead/OwnAnswer.kt")
        for (f in handOff) assertTrue(outside.getValue(f).let { "onOpenChat" in it || "rememberChatOpener" in it }, f)
        assertTrue("OwnAnswer(" in outside.getValue("ui/bead/ChangeGuide.kt"))
        assertTrue("OwnAnswer(" in outside.getValue("ui/repos/RoundQuestions.kt"))
    }
}
