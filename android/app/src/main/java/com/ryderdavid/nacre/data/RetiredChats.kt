package com.ryderdavid.nacre.data

import java.io.File

/**
 * Removes what the retired bead chat left in the app's files: its threads in
 * `bead-chats.json` and their images under `chat-attachments/`.
 *
 * It runs at every launch, off the main thread, and does nothing once both are
 * gone. So the first launch after upgrading deletes them, and a delete that
 * failed (a locked or unreadable file) is simply tried again at the next
 * launch. A failure is never thrown: the app opens normally either way.
 */
object RetiredChats {
    const val THREADS = "bead-chats.json"
    const val ATTACHMENTS = "chat-attachments"

    /** The leftovers under [filesDir] that are still there. */
    fun leftovers(filesDir: File): List<File> =
        listOf(File(filesDir, THREADS), File(filesDir, ATTACHMENTS)).filter { it.exists() }

    /**
     * Deletes the leftovers under [filesDir] with [delete], and returns true when
     * none remain. [delete] is replaceable so a failure can be tested.
     */
    fun cleanUp(filesDir: File, delete: (File) -> Boolean = { it.deleteRecursively() }): Boolean {
        for (f in leftovers(filesDir)) runCatching { delete(f) }
        return runCatching { leftovers(filesDir).isEmpty() }.getOrDefault(false)
    }
}
