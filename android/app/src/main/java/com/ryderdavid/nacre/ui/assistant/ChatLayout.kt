package com.ryderdavid.nacre.ui.assistant

import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.ChatPresentation

enum class ChatLayout { None, Docked, EdgeTab, Sheet }
enum class BoardSlot { None, Bead, Chat }
enum class ChatBack { CollapseChat, CloseSheet, CloseBead, Navigate }

fun chatLayout(
    isWide: Boolean,
    available: Dp,
    paneWidth: Dp,
    minBody: Dp = 360.dp,
    presentation: ChatPresentation,
    beadPaneOpen: Boolean = false,
): ChatLayout = when {
    presentation.selected == null -> ChatLayout.None
    presentation.collapsed -> if (isWide) ChatLayout.EdgeTab else ChatLayout.None
    !isWide || available - paneWidth < minBody -> ChatLayout.Sheet
    else -> ChatLayout.Docked
}

fun boardSlot(layout: ChatLayout, beadPaneOpen: Boolean): BoardSlot = when {
    layout == ChatLayout.Docked -> BoardSlot.Chat
    beadPaneOpen -> BoardSlot.Bead
    else -> BoardSlot.None
}

fun chatBack(layout: ChatLayout, beadPaneOpen: Boolean): ChatBack = when {
    layout == ChatLayout.Docked -> ChatBack.CollapseChat
    layout == ChatLayout.Sheet -> ChatBack.CloseSheet
    beadPaneOpen -> ChatBack.CloseBead
    else -> ChatBack.Navigate
}
