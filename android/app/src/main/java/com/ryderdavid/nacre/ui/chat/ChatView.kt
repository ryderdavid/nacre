package com.ryderdavid.nacre.ui.chat

import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.*
import com.ryderdavid.nacre.ui.common.LocalContainer
import kotlinx.coroutines.flow.distinctUntilChanged

/** The pane, the sheet and the Chat tab share one draft and scroll position per chat key. */
@Composable
fun rememberChatList(key: String): LazyListState {
    val stack = LocalContainer.current.chatStack
    val initial = remember(key) { stack.view(key) }
    val list = rememberLazyListState(initial.firstVisibleIndex, initial.firstVisibleOffset)
    LaunchedEffect(key, list) {
        snapshotFlow { list.firstVisibleItemIndex to list.firstVisibleItemScrollOffset }
            .distinctUntilChanged().collect { (index, offset) ->
                stack.updateView(key) { it.copy(firstVisibleIndex = index, firstVisibleOffset = offset) }
            }
    }
    return list
}
