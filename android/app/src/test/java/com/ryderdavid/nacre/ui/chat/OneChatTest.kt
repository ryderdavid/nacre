package com.ryderdavid.nacre.ui.chat

import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.ChatHub
import com.ryderdavid.nacre.data.chat.FakeChatLink
import com.ryderdavid.nacre.data.chat.RecordingNotices
import com.ryderdavid.nacre.data.claude.ChatImage
import com.ryderdavid.nacre.data.claude.ChatPresentation
import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.claude.ChatViewState
import com.ryderdavid.nacre.data.claude.ImagePayload
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.ui.BeadRoute
import com.ryderdavid.nacre.ui.BoardRoute
import com.ryderdavid.nacre.ui.BoardsRoute
import com.ryderdavid.nacre.ui.ChatsRoute
import com.ryderdavid.nacre.ui.InboxRoute
import com.ryderdavid.nacre.ui.RoundRoute
import com.ryderdavid.nacre.ui.RunRoute
import com.ryderdavid.nacre.ui.SettingsRoute
import com.ryderdavid.nacre.ui.chat.beadMentions
import com.ryderdavid.nacre.ui.shellHostsChat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Section 11: one chat behind ✨, the bead entry, the pane and the sheet. */
class OneChatTest {
    private val persisted = mutableListOf<String?>()
    private val stack = ChatStack(persist = { persisted += it })
    private val chat = "mac~c1"
    private val other = "mac~c2"

    // The unfolded Fold beside the rail, and a phone.
    private fun wideLayout(p: ChatPresentation = stack.presentation.value, beadPaneOpen: Boolean = false) =
        chatLayout(true, 1000.dp, 440.dp, presentation = p, beadPaneOpen = beadPaneOpen)
    private fun phoneLayout(p: ChatPresentation = stack.presentation.value) = chatLayout(false, 400.dp, 440.dp, presentation = p)

    // ---- 11.1 one app-wide current chat, persisted, with view state per chat id ----

    @Test fun theCurrentChatIsAppWideAndRemembered() {
        assertNull(stack.current.value)
        stack.open(chat)
        assertEquals(chat, stack.current.value)
        assertEquals(listOf<String?>(chat), persisted, "the current chat id is written to settings")
        stack.open(chat)
        assertEquals(listOf<String?>(chat), persisted, "reopening the same chat writes nothing new")

        // At the next launch the remembered chat is current again, shown nowhere until opened.
        val next = ChatStack()
        next.restore(chat)
        assertEquals(chat, next.current.value)
        assertFalse(next.presentation.value.shown)
        next.select(other)
        next.restore(chat)
        assertEquals(other, next.current.value, "a chat chosen before settings load isn't overwritten")
    }

    @Test fun viewStateIsKeptPerChat() {
        stack.updateView(chat) { it.copy(draft = "first", firstVisibleIndex = 4, firstVisibleOffset = 9) }
        stack.updateView(other) { it.copy(draft = "second") }
        assertEquals(ChatViewState("first", 4, 9), stack.view(chat))
        assertEquals("second", stack.view(other).draft)
    }

    // ---- 11.2 ✨ and the bead entry open the same chat id ----

    @Test fun headerAndBeadEntryOpenTheSameChat() {
        assertEquals(ChatEntry.New, openChatEntry(stack), "with no chat yet, ✨ offers New")
        assertFalse(stack.presentation.value.shown)

        // New from ✨ on a board makes the new chat current; Ryder starts typing.
        stack.open(chat)
        stack.updateView(chat) { it.copy(draft = "half a thought") }
        stack.close()

        // ✨ on the board, then the bead's chat entry: both open the current chat.
        assertEquals(ChatEntry.OpenCurrent, openChatEntry(stack))
        val fromHeader = stack.shownChat()
        stack.close()
        assertEquals(ChatEntry.OpenCurrent, openChatEntry(stack))
        val fromBead = stack.shownChat()
        assertEquals(chat, fromHeader)
        assertEquals(fromHeader, fromBead)
        assertEquals("half a thought", stack.view(fromBead!!).draft)
    }

    @Test fun headerMenuOffersNewThenRecentChats() {
        val chats = listOf(ChatRecord("c2", title = "Why did r-12 stop?", hostId = "mac"), ChatRecord("c1", title = "", hostId = "mac"))
        val menu = chatMenu("NAC-48p.5", chats, current = "mac~c1")
        assertEquals(listOf("New · NAC-48p.5", "Why did r-12 stop?", "Chat"), menu.map { it.label })
        assertEquals(listOf(null, "mac~c2", "mac~c1"), menu.map { it.key })
        assertEquals(listOf(false, false, true), menu.map { it.current })

        // Picking a recent chat makes it current and opens it.
        stack.open(menu[1].key!!)
        assertEquals("mac~c2", stack.shownChat())
    }

    // ---- 11.3 pane and sheet ----

    @Test fun thePaneStaysDockedAndCollapsedAcrossABeadToRoundMove() {
        val bead = BeadRoute("mac~nacre", "NAC-3")
        val round = RoundRoute("mac~r-12")
        assertTrue(shellHostsChat(bead))
        assertTrue(shellHostsChat(round))

        stack.open(chat)
        stack.updateView(chat) { it.copy(draft = "and this?") }
        val besideBead = wideLayout()
        assertEquals(ChatLayout.Docked, besideBead)
        // Navigating doesn't touch the app-wide presentation, so the round shows the same pane.
        val besideRound = wideLayout()
        assertEquals(besideBead, besideRound)
        assertEquals(chat, stack.shownChat())
        assertEquals("and this?", stack.view(chat).draft)

        stack.collapse()
        assertEquals(ChatLayout.EdgeTab, wideLayout(), "collapsed beside the bead")
        assertEquals(ChatLayout.EdgeTab, wideLayout(), "and still collapsed beside the round")
        assertEquals(chat, stack.current.value)
    }

    @Test fun theShellHostsChatEverywhereButTheBoardTheChatTabAndSettings() {
        listOf(BoardsRoute, InboxRoute, BeadRoute("k", "b"), RoundRoute("r"), RunRoute("x")).forEach { assertTrue(shellHostsChat(it), "$it") }
        // The board hosts it in its own right pane slot, so there is never a second pane.
        listOf(BoardRoute("k"), ChatsRoute, SettingsRoute, null).forEach { assertFalse(shellHostsChat(it), "$it") }
    }

    @Test fun aPhoneLinkClosesTheSheetAndHeaderReopensItWithItsDraft() {
        stack.open(chat)
        val shot = ImagePayload(ChatImage.create("image/png", "shot.png"), ByteArray(4))
        stack.updateView(chat) { it.copy(draft = "build this", images = listOf(shot)) }
        assertEquals(ChatLayout.Sheet, phoneLayout())

        stack.followedLink(wide = false)
        assertEquals(ChatLayout.None, phoneLayout(), "the sheet closed for the bead the link opened")
        assertEquals(chat, stack.current.value)

        assertEquals(ChatEntry.OpenCurrent, openChatEntry(stack))
        assertEquals(ChatLayout.Sheet, phoneLayout())
        assertEquals(ChatViewState(draft = "build this", images = listOf(shot)), stack.view(chat))
    }

    @Test fun aWideLinkKeepsThePaneDocked() {
        stack.open(chat)
        stack.followedLink(wide = true)
        assertEquals(ChatLayout.Docked, wideLayout())
    }

    @Test fun backCollapsesThenLeaves() {
        stack.open(chat)
        assertEquals(ChatBack.CollapseChat, chatBack(wideLayout(), beadPaneOpen = false))
        stack.collapse()
        val collapsed = wideLayout()
        assertEquals(ChatLayout.EdgeTab, collapsed)
        assertEquals(ChatBack.Navigate, chatBack(collapsed, beadPaneOpen = false), "the second Back follows the screen's order")

        // On a board with a bead in the pane: collapse, then close the bead, then leave.
        stack.expand()
        assertEquals(ChatBack.CollapseChat, chatBack(wideLayout(beadPaneOpen = true), true))
        stack.collapse()
        assertEquals(ChatBack.CloseBead, chatBack(wideLayout(beadPaneOpen = true), true))
        assertEquals(ChatBack.Navigate, chatBack(wideLayout(beadPaneOpen = false), false))

        // On phones Back closes the sheet.
        stack.expand()
        assertEquals(ChatBack.CloseSheet, chatBack(phoneLayout(), false))
    }

    @Test fun theEmbeddedBeadDetailOpensTheChatInTheBoardPane() {
        // A bead is open in the board's right pane, and the shell hosts no chat on the board.
        assertFalse(shellHostsChat(BoardRoute("mac~nacre")))
        stack.select(chat)
        assertEquals(BoardSlot.Bead, boardSlot(wideLayout(beadPaneOpen = true), true))

        // The chat entry in the embedded bead detail opens the current chat in that one slot.
        assertEquals(ChatEntry.OpenCurrent, openChatEntry(stack))
        val layout = wideLayout(beadPaneOpen = true)
        assertEquals(ChatLayout.Docked, layout)
        assertEquals(BoardSlot.Chat, boardSlot(layout, true))

        // Collapsing the chat shows the bead in the pane again, beside the edge tab.
        stack.collapse()
        val back = wideLayout(beadPaneOpen = true)
        assertEquals(ChatLayout.EdgeTab, back)
        assertEquals(BoardSlot.Bead, boardSlot(back, true))
    }

    // ---- 11.4 deleting ----

    @Test fun deletingClosesTheViewAndDropsTheDraft() {
        stack.open(chat)
        stack.updateView(chat) { it.copy(draft = "unsent", images = listOf(ImagePayload(ChatImage.create("image/png", "a.png"), ByteArray(2)))) }
        stack.updateView(other) { it.copy(draft = "keep") }

        stack.deleted(chat)
        assertNull(stack.current.value)
        assertEquals(ChatLayout.None, wideLayout())
        assertEquals(ChatLayout.None, phoneLayout())
        assertFalse(chat in stack.views.value)
        assertEquals(ChatViewState(), stack.view(chat))
        assertNull(persisted.last(), "the remembered current chat is forgotten")
        assertEquals(ChatEntry.New, openChatEntry(stack), "✨ offers New")

        // A late update from the closing composer can't bring the draft back.
        stack.updateView(chat) { it.copy(draft = "late") }
        assertEquals(ChatViewState(), stack.view(chat))
        stack.restore(chat)
        assertNull(stack.current.value, "a deleted chat isn't restored")
        assertEquals("keep", stack.view(other).draft)
    }

    @Test fun deletingAnotherChatLeavesTheCurrentOneOpen() {
        stack.open(chat)
        stack.deleted(other)
        assertEquals(chat, stack.shownChat())
    }

    @Test fun closingTheViewDoesNotDeleteTheChat() {
        stack.open(chat)
        stack.updateView(chat) { it.copy(draft = "keep me") }
        stack.close()
        assertEquals(chat, stack.current.value)
        assertEquals("keep me", stack.view(chat).draft)
    }

    @Test fun hubDeleteClosesTheViewAndDropsTheDraft() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val link = FakeChatLink()
            val hub = ChatHub(link, scope, RecordingNotices(), inBackground = { false }, stack = stack)
            hub.openChat(chat)
            stack.open(chat)
            hub.prefill(chat, "carrying on")
            hub.delete(chat)
            assertNull(hub.current.value)
            assertNull(stack.shownChat())
            assertEquals(ChatViewState(), stack.view(chat))
        } finally {
            scope.cancel()
        }
    }

    // ---- links out of the chat ----

    @Test fun beadLinksNameOnlyBeadsOnTheChatsBoard() {
        val known = setOf("NAC-3", "NAC-48p.5")
        assertEquals(listOf("NAC-48p.5", "NAC-3"), beadMentions("NAC-48p.5 waits on NAC-3. NAC-48p.5 again, and NAC-999.", known))
        assertEquals(emptyList(), beadMentions("NAC-3", emptySet()))
    }
}
