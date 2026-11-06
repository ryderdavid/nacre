package com.ryderdavid.nacre.ui

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * Chat-first replaced the old shell: the tabs, the round and run screens, the bead sheets and the
 * docked chat are gone, and nothing can turn the old layout back on.
 */
class RemovedFlowsTest {
    private val main = File("src/main/java/com/ryderdavid/nacre")

    @Test fun theReplacedScreensAreGone() {
        for (name in listOf(
            "com.ryderdavid.nacre.ui.repos.ReposScreenKt",
            "com.ryderdavid.nacre.ui.repos.InboxScreenKt",
            "com.ryderdavid.nacre.ui.board.BoardScreenKt",
            "com.ryderdavid.nacre.ui.board.NewBeadSheetKt",
            "com.ryderdavid.nacre.ui.board.IntakeKt",
            "com.ryderdavid.nacre.ui.rounds.StartRoundSheetKt",
            "com.ryderdavid.nacre.ui.bead.BeadDetailKt",
            "com.ryderdavid.nacre.ui.bead.BeadSheetsKt",
            "com.ryderdavid.nacre.ui.bead.FoundSomethingKt",
            "com.ryderdavid.nacre.ui.bead.OwnAnswerKt",
            "com.ryderdavid.nacre.ui.bead.SignoffKt",
            "com.ryderdavid.nacre.ui.bead.ChangeDetailKt",
            "com.ryderdavid.nacre.ui.chat.ChatHostKt",
            "com.ryderdavid.nacre.ui.chat.ChatsScreenKt",
            "com.ryderdavid.nacre.ui.ShellNavKt",
        )) assertFailsWith<ClassNotFoundException>(name) { Class.forName(name) }
        assertTrue(main.isDirectory, "run from the app module")
        assertTrue(!File(main, "ui/repos").exists(), "ui/repos/ is removed")
    }

    @Test fun noScreenOffersTheOldLayoutOrLiveSend() {
        val offenders = File(main, "ui").walkTopDown().filter { it.isFile && it.extension == "kt" }.filter { f ->
            val t = f.readText()
            listOf("chatFirstLayout", "AppShell(", "Send to live session", "RoundDetail(", "RunsScreen(").any { it in t }
        }.map { it.relativeTo(main).path }.toList()
        assertEquals(emptyList(), offenders)
    }
}
