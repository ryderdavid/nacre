package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.model.ChatDraft
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.NewEffortPrefill
import com.ryderdavid.nacre.data.model.ScreenContext
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import java.io.File
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** New effort drafts in Chat: the chat it opens, the Draft card's "Where does this belong?", and what replaced DraftChat. */
class NewEffortTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val link = FakeChatLink()
    private val stack = ChatStack()
    private val hub = ChatHub(link, scope, RecordingNotices(), inBackground = { false }, stack = stack)
    private val board = ScreenContext(screen = ScreenContext.BOARD, repo = "nacre")
    private val draft = ChatDraft(
        title = "Attach PDFs to chat", type = "feature", description = "Let Ryder send PDFs.",
        design = "Reuse the image path.", acceptance = "- a PDF reaches the agent",
    )

    @AfterTest fun tearDown() = scope.cancel()

    @Test fun newEffortOpensAChatOnTheBoardsComputer() = runBlocking {
        val key = hub.startNewEffort("studio~nacre", prefill = null, context = board)
        assertEquals(listOf("studio" to CreateChat(ChatWire.AGENT_CLAUDE, "nacre", board, newEffort = true)), link.created)
        assertEquals("studio~new", key)
        // Opened as the layout chooses: current, and shown expanded in the pane or the sheet.
        assertEquals(key, hub.current.value)
        assertEquals(key, stack.shownChat())
        assertFalse(stack.presentation.value.collapsed)
        // Not opened prefilled: the composer is empty.
        assertEquals("", stack.view(key).draft)
        assertEquals(NewEffortOrigin("studio~nacre", null), hub.effortOrigin(key))
    }

    @Test fun prefilledNewEffortPrefillsTheComposer() = runBlocking {
        val prefill = NewEffortPrefill(title = "Flaky upload test", description = "Flaky upload test\nseen in NAC-9's round")
        val key = hub.startNewEffort("studio~nacre", prefill, board)
        assertEquals("Flaky upload test\nseen in NAC-9's round", stack.view(key).draft)
        assertEquals("Follow-on\n\nfrom change a", composerPrefill(NewEffortPrefill(title = "Follow-on", description = "from change a")))
        assertNull(composerPrefill(NewEffortPrefill()))
    }

    @Test fun whereDoesThisBelongRaisesIntakeFromTheRecordedDraft() = runBlocking {
        val key = hub.startNewEffort("studio~nacre", prefill = null, context = board)
        assertEquals(DraftIntakeResult.Raised(com.ryderdavid.nacre.data.model.Decision(id = "dc_1", kind = "intake")), hub.raiseDraftIntake(key, 4))
        assertEquals(listOf(key to 4L), link.draftRaises)
        link.draftRaise = { _, _ -> DraftIntakeResult.Gone }
        assertEquals(DraftIntakeResult.Gone, hub.raiseDraftIntake(key, 2))
        assertEquals("That draft is gone. Ask the chat for a new one.", DRAFT_GONE)
    }

    @Test fun theNewestDraftIsTheDraftCard() {
        val t = ChatTranscript()
        t.add(ev(1, ChatWire.TEXT, "Here's a first go."))
        t.add(ChatEvent(seq = 2, kind = ChatWire.DRAFT, text = "One", draft = ChatDraft(title = "One")))
        t.add(ev(3, ChatWire.ERROR, "The draft couldn't be read: it has no title"))
        t.add(ChatEvent(seq = 4, kind = ChatWire.DRAFT, text = "Two", draft = draft))
        val drafts = t.shown.filterIsInstance<ChatItem.Draft>()
        assertEquals(listOf(false, true), drafts.map { it.latest })
        // Each card knows the event it came from, so intake is raised on that exact draft.
        assertEquals(listOf(2L, 4L), drafts.map { it.seq })
        assertEquals(draft, t.newestDraft)
        assertEquals(draft, ConversationState(items = t.shown).newestDraft)
        // The unreadable block is a note in the conversation, not a card.
        assertTrue(t.shown.any { it is ChatItem.Note && it.error && it.text.startsWith("The draft couldn't be read") })
    }

    @Test fun aFiledDraftSaysWhatItFiled() {
        val done = com.ryderdavid.nacre.data.model.Decision(id = "dc_1", kind = "intake", state = "done",
            result = com.ryderdavid.nacre.data.model.DecisionResult(ok = true, message = "Created NAC-9"))
        assertEquals("Already filed: Created NAC-9", filedNote(done))
        assertNull(filedNote(done.copy(state = "pending")))
    }

    @Test fun newEffortIsOfferedOnlyWithOpenSpecAndADraftingComputer() {
        val drafting = Health(hostName = "Studio", features = listOf(Features.CLAUDE, Features.AGENT_CHAT, Features.DRAFT_INTAKE))
        val older = Health(hostName = "Studio", features = listOf(Features.CLAUDE, Features.AGENT_CHAT))
        assertTrue(offersNewEffort(openspec = true, health = drafting))
        // An older nacred can't draft or raise intake from a draft, and a repository without OpenSpec has no intake.
        assertFalse(offersNewEffort(openspec = true, health = older))
        assertFalse(offersNewEffort(openspec = true, health = null))
        assertFalse(offersNewEffort(openspec = false, health = drafting))
    }

    @Test fun theDraftCardSaysWhereItGoesNotContinue() {
        assertEquals("Where does this belong?", com.ryderdavid.nacre.ui.chat.DRAFT_ACTION)
        assertEquals("Or tell the chat what to change.", com.ryderdavid.nacre.ui.chat.DRAFT_HINT)
        val src = File("src/main/java/com/ryderdavid/nacre/ui/chat/Conversation.kt").readText()
        assertFalse(src.contains("Text(\"Continue\")"), "the Draft card still says Continue")
    }

    @Test fun noDraftChatRemains() {
        val src = File("src/main/java/com/ryderdavid/nacre")
        assertTrue(src.isDirectory, "run from the app module")
        val offenders = src.walkTopDown().filter { it.isFile && it.extension == "kt" && Regex("""\bDraftChat\b""").containsMatchIn(it.readText()) }.toList()
        assertEquals(emptyList(), offenders)
    }
}
