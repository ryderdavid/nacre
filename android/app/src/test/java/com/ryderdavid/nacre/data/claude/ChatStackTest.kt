package com.ryderdavid.nacre.data.claude

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class ChatStackTest {
    private var clock = 1L
    private val removed = mutableListOf<ChatContext>()
    private val stack = ChatStack(now = { clock }, onRemoved = { removed += it })
    private val board = ChatContext.Board("mac~nacre")
    private val bead = ChatContext.Bead("mac~nacre", "NAC-32n")

    @Test fun browsingAndOpeningAnUnstartedChatAddsNothing() {
        repeat(3) {
            assertEquals(listOf(ChatSlot(board, "Nacre", true)), stack.slots(board, "Nacre"))
            assertFalse(stack.shouldExpand(board))
            stack.slots(bead, "Floating chat")
            stack.slots(ChatContext.Multi, "multi")
        }
        assertTrue(stack.active.value.isEmpty())
    }

    @Test fun noChatsAndOnlyTheCurrentChatOpenDirectly() {
        assertFalse(stack.shouldExpand(board))
        stack.started(board, "Nacre")
        assertFalse(stack.shouldExpand(board))
        assertEquals(listOf(ChatSlot(board, "Nacre", false)), stack.slots(board, "Nacre"))
        assertTrue(stack.shouldExpand(bead))
        assertEquals(listOf(ChatSlot(bead, "Floating chat", true), ChatSlot(board, "Nacre", false)), stack.slots(bead, "Floating chat"))
    }

    @Test fun theCurrentScreenIsFirstWithoutDiscardingOrTouchingOtherChats() {
        stack.started(board, "Nacre")
        clock++
        stack.started(ChatContext.Multi, "multi")
        clock++
        stack.started(bead, "Floating chat")
        val before = stack.active.value
        assertEquals(listOf(board, bead, ChatContext.Multi), stack.slots(board, "Nacre").map { it.context })
        assertEquals(listOf(ChatContext.Multi, bead, board), stack.slots(ChatContext.Multi, "multi").map { it.context })
        assertTrue(stack.shouldExpand(board))
        assertFalse(stack.slots(board, "Nacre").any { it.isNew })
        assertEquals(before, stack.active.value, "navigation does not count as activity")
    }

    @Test fun sendingAgainAndRenamingCannotCreateASecondChatForAContext() {
        stack.started(bead, "Old name")
        clock++
        stack.started(bead, "New name")
        assertEquals(1, stack.active.value.size)
        assertEquals("New name", stack.slots(bead, "New name").single().label)
        val sameIdOnAnotherBoard = ChatContext.Bead("other~project", "NAC-32n")
        stack.started(sameIdOnAnotherBoard, "Other bead")
        assertEquals(2, stack.active.value.size)
    }

    @Test fun dismissalOffersNewAndLateActivityCannotResurrectTheChat() {
        stack.started(board, "Nacre")
        stack.started(bead, "Floating chat")
        stack.dismiss(board)
        stack.touch(board)
        assertEquals<List<ChatContext>>(listOf(board), removed)
        assertEquals(listOf(ChatSlot(board, "Nacre", true), ChatSlot(bead, "Floating chat", false)), stack.slots(board, "Nacre"))
        stack.started(board, "Nacre")
        assertEquals(2, stack.active.value.size)
    }

    @Test fun expiresAtExactlyThirtyIdleMinutesAndOffersNewAgain() {
        stack.started(board, "Nacre")
        clock += ChatStack.IDLE_MS - 1
        stack.expire()
        assertTrue(stack.contains(board))
        stack.slots(board, "Nacre")
        clock++
        stack.expire()
        assertFalse(stack.contains(board))
        assertEquals<List<ChatContext>>(listOf(board), removed)
        assertTrue(stack.slots(board, "Nacre").single().isNew)
        stack.expire()
        assertEquals(1, removed.size)
    }

    @Test fun repliesAndUserDecisionsRefreshOnlyTheirConversation() {
        stack.started(board, "Nacre")
        stack.started(bead, "Floating chat")
        clock += ChatStack.IDLE_MS - 1
        stack.touch(board)
        clock++
        stack.expire()
        assertTrue(stack.contains(board))
        assertFalse(stack.contains(bead))
        clock += ChatStack.IDLE_MS - 1
        stack.expire()
        assertTrue(stack.active.value.isEmpty())
    }
}
