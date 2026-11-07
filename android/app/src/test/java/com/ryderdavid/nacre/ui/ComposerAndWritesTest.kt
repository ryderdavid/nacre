package com.ryderdavid.nacre.ui

import java.io.File
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * Device-check regressions from 0.38.0: an answer or a message must not die with the composable that sent
 * it, text outside a Surface must not fall back to black, and the composer keeps its room while typing.
 */
class ComposerAndWritesTest {
    private val main = File("src/main/java/com/ryderdavid/nacre")
    private fun src(path: String) = File(main, path).readText()

    /** "Not sent: rememberCoroutineScope left the composition": writes run in the app's scope. */
    @Test fun writesOutliveTheirComposable() {
        val writes = Regex("""answerDecision\(|sendDraftNow\(|createChat\(""")
        val offenders = File(main, "ui").walkTopDown().filter { it.isFile && it.extension == "kt" }.filter { f ->
            val t = f.readText()
            writes.containsMatchIn(t) && t.contains("rememberCoroutineScope()") &&
                // Screens that keep a composition scope for scrolling and sheets, but launch writes on container.scope.
                f.name !in setOf("ProjectsScreen.kt", "ChatFirstShell.kt")
        }.map { it.name }.toList()
        assertTrue(offenders.isEmpty(), "writes launched from a composition-bound scope in $offenders")
        assertTrue(src("ui/workspace/decisions/DecisionCard.kt").contains("val scope = container.scope"))
        assertTrue(src("ui/workspace/chat/ChatPane.kt").contains("val scope = container.scope"))
    }

    /** Titles outside any Surface were drawn in the default black on the dark theme. */
    @Test fun theThemeRootSetsTheContentColour() {
        assertTrue(src("ui/theme/Theme.kt").contains("LocalContentColor provides scheme.onSurface"))
    }

    /** With the keyboard up the strip, suggestions and tall chips give way to the composer. */
    @Test fun typingCompactsTheChrome() {
        val pane = src("ui/workspace/chat/ChatPane.kt")
        assertTrue(pane.contains("WindowInsets.isImeVisible"))
        assertTrue(pane.contains("draft.isEmpty() && !state.turnRunning && pending.isEmpty()"), "suggestions show only in an empty box: typing or an image clears them")
        val workspace = src("ui/workspace/Workspace.kt")
        assertTrue(workspace.contains("slots.queueWatcher()"), "the phone has no strip, but a notification still opens the queue")
        assertTrue(!workspace.contains("slots.strip(Modifier.padding(horizontal = 12.dp, vertical = 4.dp), false)"), "no strip above the phone's chat")
    }
}

class ChatDraftsTest {
    @Test fun draftsRoundTripAndBadDataIsIgnored() {
        val d = mapOf("h1~c1" to "half a message", "h1~c2" to "another")
        kotlin.test.assertEquals(d, com.ryderdavid.nacre.data.chat.ChatDrafts.decode(com.ryderdavid.nacre.data.chat.ChatDrafts.encode(d)))
        kotlin.test.assertEquals(emptyMap(), com.ryderdavid.nacre.data.chat.ChatDrafts.decode("not json"))
        kotlin.test.assertEquals(emptyMap(), com.ryderdavid.nacre.data.chat.ChatDrafts.decode(null))
    }
}
