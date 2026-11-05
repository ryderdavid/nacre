package com.ryderdavid.nacre.data

import java.io.File
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** The retired bead chat's files are deleted at the first launch after upgrading, and retried if that fails. */
class RetiredChatsTest {
    private fun filesDir(): File = Files.createTempDirectory("nacre-files").toFile().also { dir ->
        File(dir, RetiredChats.THREADS).writeText("""{"threads":[]}""")
        File(dir, RetiredChats.ATTACHMENTS).mkdirs()
        File(dir, "${RetiredChats.ATTACHMENTS}/a.jpg").writeBytes(byteArrayOf(1, 2, 3))
        File(dir, "settings.preferences_pb").writeText("kept")
    }

    @Test fun upgradeCleanupDeletesTheThreadsAndTheirAttachments() {
        val dir = filesDir()
        assertTrue(RetiredChats.cleanUp(dir))
        assertFalse(File(dir, RetiredChats.THREADS).exists())
        assertFalse(File(dir, RetiredChats.ATTACHMENTS).exists())
        // Nothing else in the app's files is touched.
        assertEquals("kept", File(dir, "settings.preferences_pb").readText())
    }

    @Test fun aFailedDeleteIsRetriedAtTheNextLaunch() {
        val dir = filesDir()
        // First launch: the delete fails (and even throws); the app isn't blocked and the files stay.
        val failed = RetiredChats.cleanUp(dir) { f -> if (f.name == RetiredChats.ATTACHMENTS) throw SecurityException("locked") else false }
        assertFalse(failed)
        assertEquals(2, RetiredChats.leftovers(dir).size)
        // Next launch: tried again, and this time both go.
        assertTrue(RetiredChats.cleanUp(dir))
        assertEquals(emptyList(), RetiredChats.leftovers(dir))
    }

    @Test fun laterLaunchesHaveNothingToDo() {
        val dir = filesDir()
        RetiredChats.cleanUp(dir)
        var calls = 0
        assertTrue(RetiredChats.cleanUp(dir) { calls++; true })
        assertEquals(0, calls)
    }
}
