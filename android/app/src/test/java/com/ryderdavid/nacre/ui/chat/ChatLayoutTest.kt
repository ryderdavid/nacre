package com.ryderdavid.nacre.ui.chat

import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.ChatPresentation
import com.ryderdavid.nacre.ui.common.isPaneWide
import kotlin.test.*

class ChatLayoutTest {
    private val selected = ChatPresentation(shown = true)
    @Test fun existingWideBoundary() {
        assertFalse(isPaneWide(759.dp))
        assertTrue(isPaneWide(760.dp))
    }
    @Test fun noSelectionNeverShowsChat() {
        assertEquals(ChatLayout.None, chatLayout(true, 1000.dp, 440.dp, presentation = ChatPresentation()))
    }
    @Test fun wideDocksAndNarrowUsesSheet() {
        assertEquals(ChatLayout.Docked, chatLayout(true, 1000.dp, 440.dp, presentation = selected))
        assertEquals(ChatLayout.Sheet, chatLayout(false, 500.dp, 440.dp, presentation = selected))
    }
    @Test fun preservesMinimumBodyAtExactBoundary() {
        assertEquals(ChatLayout.Sheet, chatLayout(true, 679.dp, 440.dp, presentation = selected))
        assertEquals(ChatLayout.Docked, chatLayout(true, 680.dp, 440.dp, presentation = selected))
    }
    @Test fun unfoldedFoldDocksAtTheHinge() {
        // Unfolded Fold: ~700dp beside the rail, pane snapped to the hinge, like the bead pane.
        assertEquals(ChatLayout.Docked, chatLayout(true, 700.dp, 395.dp, presentation = selected))
    }
    @Test fun collapsedChatHasAnEdgeOnlyOnWide() {
        assertEquals(ChatLayout.EdgeTab, chatLayout(true, 1000.dp, 440.dp, presentation = selected.copy(collapsed = true)))
        assertEquals(ChatLayout.None, chatLayout(false, 500.dp, 440.dp, presentation = selected.copy(collapsed = true)))
    }
    @Test fun openingChatTakesTheSingleBoardSlotAndClosingRestoresBead() {
        val layout = chatLayout(true, 1000.dp, 440.dp, presentation = selected, beadPaneOpen = true)
        assertEquals(BoardSlot.Chat, boardSlot(layout, true))
        assertEquals(BoardSlot.Bead, boardSlot(ChatLayout.None, true))
        assertEquals(BoardSlot.None, boardSlot(ChatLayout.None, false))
    }
    @Test fun tappingBeadCollapsesChatAndRestoresBeadAlongsideEdgeTab() {
        assertEquals("NAC-1", boardBeadAfterTap("NAC-1", "NAC-1", selected))
        assertEquals("NAC-2", boardBeadAfterTap("NAC-1", "NAC-2", selected))
        assertEquals(null, boardBeadAfterTap("NAC-1", "NAC-1", selected.copy(collapsed = true)))
        val stack = com.ryderdavid.nacre.data.claude.ChatStack()
        stack.open("h1~c1")
        stack.collapse()
        val layout = chatLayout(true, 1000.dp, 440.dp, presentation = stack.presentation.value, beadPaneOpen = true)
        assertEquals(ChatLayout.EdgeTab, layout)
        assertEquals(BoardSlot.Bead, boardSlot(layout, true))
        stack.expand()
        assertEquals(BoardSlot.Chat, boardSlot(chatLayout(true, 1000.dp, 440.dp, presentation = stack.presentation.value), true))
    }
    @Test fun backCollapsesChatThenClosesBeadThenNavigates() {
        assertEquals(ChatBack.CollapseChat, chatBack(ChatLayout.Docked, true))
        assertEquals(ChatBack.CloseBead, chatBack(ChatLayout.EdgeTab, true))
        assertEquals(ChatBack.Navigate, chatBack(ChatLayout.EdgeTab, false))
        assertEquals(ChatBack.CloseSheet, chatBack(ChatLayout.Sheet, true))
    }
}
