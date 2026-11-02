package com.ryderdavid.nacre.data.claude

import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.assistant.ChatLayout
import com.ryderdavid.nacre.ui.assistant.chatLayout
import kotlin.test.*

class ChatPresentationTest {
    private var clock = 1L
    private val stack = ChatStack(now = { clock })
    private val board = ChatContext.Board("repo")
    private val bead = ChatContext.Bead("repo", "NAC-1")

    @Test fun screensOwnSelectionAndReturningRestoresIt() {
        stack.select(board, bead)
        assertEquals(ChatPresentation(), stack.presentation(bead))
        stack.select(bead, board)
        stack.collapse(board)
        assertEquals(ChatPresentation(bead, true), stack.presentation(board))
        assertEquals(ChatPresentation(board), stack.presentation(bead))
    }
    @Test fun collapseExpandAndLayoutChangesPreserveDraftAndPosition() {
        stack.select(board, bead)
        stack.updateView(bead) { ChatViewState("half written", 7, 29) }
        stack.collapse(board)
        assertEquals(bead, stack.presentation(board).selected)
        stack.expand(board)
        assertEquals(ChatLayout.Docked, chatLayout(true, 1000.dp, 440.dp, presentation = stack.presentation(board)))
        assertEquals(ChatLayout.Sheet, chatLayout(false, 400.dp, 440.dp, presentation = stack.presentation(board)))
        assertEquals(ChatViewState("half written", 7, 29), stack.view(bead))
    }
    @Test fun closeKeepsConversationDraftAndPositionForReopening() {
        stack.started(bead, "Bead")
        stack.select(board, bead)
        stack.updateView(bead) { ChatViewState("draft", 3, 4) }
        stack.close(board)
        assertEquals(ChatPresentation(), stack.presentation(board))
        assertTrue(stack.contains(bead))
        stack.select(board, bead)
        assertEquals(ChatViewState("draft", 3, 4), stack.view(bead))
    }
    @Test fun expiryClearsEveryScreenSelectingChatAndItsViewState() {
        stack.started(bead, "Bead")
        stack.select(board, bead)
        stack.select(bead, bead)
        stack.updateView(bead) { ChatViewState("draft", 3, 4) }
        clock += ChatStack.IDLE_MS
        stack.expire()
        assertEquals(ChatPresentation(), stack.presentation(board))
        assertEquals(ChatPresentation(), stack.presentation(bead))
        assertEquals(ChatViewState(), stack.view(bead))
        assertFalse(stack.contains(bead))
    }
    @Test fun dismissAlsoClearsAnUnsentChatAndLeavesOtherChatsIntact() {
        stack.select(board, bead)
        stack.select(ChatContext.Multi, ChatContext.Multi)
        stack.updateView(bead) { it.copy(draft = "unsent") }
        stack.updateView(ChatContext.Multi) { it.copy(draft = "keep") }
        stack.dismiss(bead)
        assertEquals(ChatPresentation(), stack.presentation(board))
        assertEquals(ChatViewState(), stack.view(bead))
        assertEquals("keep", stack.view(ChatContext.Multi).draft)
    }
}
