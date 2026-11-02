package com.ryderdavid.nacre.ui.assistant

import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.*
import com.ryderdavid.nacre.data.claude.ChatContext
import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.flow.distinctUntilChanged

/** Both renderers use the same process-owned draft and scroll position. */
@Composable
fun rememberChatList(context: ChatContext): LazyListState {
    val stack = LocalContainer.current.chatStack
    val initial = remember(context) { stack.view(context) }
    val list = rememberLazyListState(initial.firstVisibleIndex, initial.firstVisibleOffset)
    LaunchedEffect(context, list) {
        snapshotFlow { list.firstVisibleItemIndex to list.firstVisibleItemScrollOffset }
            .distinctUntilChanged().collect { (index, offset) ->
                stack.updateView(context) { it.copy(firstVisibleIndex = index, firstVisibleOffset = offset) }
            }
    }
    return list
}

@Composable
fun chatDraft(context: ChatContext): String {
    val views by LocalContainer.current.chatStack.views.collectAsState()
    return views[context]?.draft.orEmpty()
}
