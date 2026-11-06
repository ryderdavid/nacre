package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.model.ChatDraft
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.FoundComment
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.IntakeDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
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

private val TYPES = listOf("task", "bug", "feature", "chore", "epic", "decision", "spike", "story")

/** New effort drafts in Chat: the chat it opens, the Draft card's Continue, and what replaced DraftChat. */
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

    @Test fun continueRunsIntakeWithTheDraft() = runBlocking {
        val key = hub.startNewEffort("studio~nacre", prefill = null, context = board)
        val chat = ChatRecord("new", repo = "nacre", newEffort = true, hostId = "studio")
        val c = assertNotNull(hub.continueDraft(key, chat, draft))
        assertEquals("studio~nacre", c.repoKey)
        assertEquals(draft, c.draft)

        val form = draftForm(c.draft, parent = null, types = TYPES)
        val asked = mutableListOf<Pair<String, IntakeDraft>>()
        val verdict = runIntake(c.repoKey, form) { k, d -> asked += k to d; IntakeVerdict("new", rationale = "changes behaviour") }
        assertEquals("new", verdict.getOrThrow().verdict)
        assertEquals(
            listOf("studio~nacre" to IntakeDraft("Attach PDFs to chat", "Let Ryder send PDFs.", "Reuse the image path.", "- a PDF reaches the agent", "feature", "")),
            asked,
        )
        // An intake failure is reported, not thrown, so the sheet still offers just a bead and new change.
        assertTrue(runIntake(c.repoKey, form) { _, _ -> error("timed out") }.isFailure)
    }

    @Test fun theNewestDraftIsTheDraftCard() {
        val t = ChatTranscript()
        t.add(ev(1, ChatWire.TEXT, "Here's a first go."))
        t.add(ChatEvent(seq = 2, kind = ChatWire.DRAFT, text = "One", draft = ChatDraft(title = "One")))
        t.add(ev(3, ChatWire.ERROR, "The draft couldn't be read: it has no title"))
        t.add(ChatEvent(seq = 4, kind = ChatWire.DRAFT, text = "Two", draft = draft))
        val drafts = t.shown.filterIsInstance<ChatItem.Draft>()
        assertEquals(listOf(false, true), drafts.map { it.latest })
        assertEquals(draft, t.newestDraft)
        assertEquals(draft, ConversationState(items = t.shown).newestDraft)
        // The unreadable block is a note in the conversation, not a card.
        assertTrue(t.shown.any { it is ChatItem.Note && it.error && it.text.startsWith("The draft couldn't be read") })
    }

    @Test fun outOfScopePrefillKeepsTheEpicRelation() = runBlocking {
        val prefill = NewEffortPrefill(
            title = "Migrate the store", description = "Found under NAC-7: the store needs a migration",
            related = listOf("NAC-7"), foundComment = FoundComment("NAC-7.2", "c-1"),
        )
        val key = hub.startNewEffort("studio~nacre", prefill, board)
        val c = assertNotNull(hub.continueDraft(key, ChatRecord("new", repo = "nacre", hostId = "studio"), draft))
        assertEquals(prefill, c.prefill)
        val issue = createIssueOf(draftForm(c.draft, parent = null, types = TYPES), c.prefill)
        assertEquals(listOf("NAC-7"), issue.related)
        assertNull(issue.parent, "out of scope is related to the epic, not a child of it")
        assertEquals(FoundComment("NAC-7.2", "c-1"), issue.foundComment)
        assertEquals("Attach PDFs to chat", issue.title)
    }

    @Test fun aDraftInAnyChatContinuesOnItsOwnBoard() {
        val chat = ChatRecord("ch_1", repo = "nacre", hostId = "studio")
        assertEquals(DraftContinuation("studio~nacre", draft, null), draftContinuation(chat, "studio~ch_1", draft, origin = null))
        // An all-boards chat has no board to continue on.
        assertNull(draftContinuation(ChatRecord("ch_2", repo = null, hostId = "studio"), "studio~ch_2", draft, origin = null))
    }

    @Test fun nonOpenSpecFormAndOlderBridgeSheet() {
        val withChat = Health(hostName = "Studio", features = listOf(Features.CLAUDE, Features.AGENT_CHAT))
        val older = Health(hostName = "Studio", features = listOf(Features.CLAUDE))
        assertEquals(NewEffortStart.Chat, newEffortStart(openspec = true, health = withChat))
        // An older nacred without agent_chat: the New effort sheet, with no draft step.
        assertEquals(NewEffortStart.Sheet, newEffortStart(openspec = true, health = older))
        assertEquals(NewEffortStart.Sheet, newEffortStart(openspec = true, health = null))
        // No OpenSpec: the New bead form directly, with or without Chat.
        assertEquals(NewEffortStart.Form, newEffortStart(openspec = false, health = withChat))
        assertEquals(NewEffortStart.Form, newEffortStart(openspec = false, health = older))
        // A draft's unknown type falls back to task.
        assertEquals("task", draftForm(ChatDraft(title = "x", type = "Wish"), types = TYPES).type)
    }

    @Test fun noDraftChatRemains() {
        val src = File("src/main/java/com/ryderdavid/nacre")
        assertTrue(src.isDirectory, "run from the app module")
        val offenders = src.walkTopDown().filter { it.isFile && it.extension == "kt" && Regex("""\bDraftChat\b""").containsMatchIn(it.readText()) }.toList()
        assertEquals(emptyList(), offenders)
    }
}
