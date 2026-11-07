// Lives with the Spec panel; declared in the workspace package so the shell's slot keeps its call.
package com.ryderdavid.nacre.ui.workspace

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.workspace.spec.SpecChangeList
import com.ryderdavid.nacre.ui.workspace.spec.SpecPanelViewModel
import com.ryderdavid.nacre.ui.workspace.spec.SpecScreen
import com.ryderdavid.nacre.ui.workspace.spec.ChangeOverviewPane
import com.ryderdavid.nacre.ui.workspace.spec.DocumentReaderPane
import kotlinx.coroutines.delay

/**
 * The Spec destination: a read-only OpenSpec viewer for one checkout. List → change → document inside
 * the pane, and Back walks back. It never approves, revises, runs or archives: those are asked for in
 * chat or answered on a decision card.
 */
@Composable
fun SpecSlot(
    repoKey: String,
    modifier: Modifier,
    /** On the board the Specs column is the list: Back past a change leaves the viewer instead of showing it. */
    onList: (() -> Unit)? = null,
) {
    val vm = nacreViewModel(key = "spec:$repoKey") { SpecPanelViewModel(it.repository, repoKey) }
    val container = LocalContainer.current
    val repo = container.repository
    val stack by vm.stack.collectAsState()
    val viewing = LocalViewing.current
    val screen = stack.last()

    // Open change (from a bead peek, a card or chat) names a change for this pane: open its overview.
    val targets by container.chats.references.specTarget.collectAsState()
    val target = targets[repoKey]
    val board by repo.board(repoKey).collectAsState()
    val boardLoaded = board.data != null
    LaunchedEffect(target, boardLoaded) {
        if (onList != null || target == null || (!boardLoaded && board.loading)) return@LaunchedEffect
        // A cited file: a change's document opens in the reader; a current spec opens at its row in the list.
        vm.show(com.ryderdavid.nacre.ui.workspace.spec.SpecList.opening(target.ref, target.requirement, board.data?.changes?.forSpec.orEmpty()))
        container.chats.references.specOpened(repoKey)
    }

    // Live progress: the board (section evidence) and rounds (failed beads) while the pane is shown.
    LaunchedEffect(repoKey) {
        while (true) {
            repo.refreshBoard(repoKey, quiet = true)
            repo.refreshRounds()
            delay(REFRESH_MS)
        }
    }
    BackHandler(enabled = stack.size > 1) { vm.back() }
    if (screen == SpecScreen.List) LaunchedEffect(Unit) { if (onList != null) onList() else viewing(null) }

    Box(modifier) {
        when (screen) {
            SpecScreen.List -> if (onList == null) SpecChangeList(vm, Modifier)
            is SpecScreen.Change -> ChangeOverviewPane(vm, screen, Modifier)
            is SpecScreen.Document -> DocumentReaderPane(vm, screen, Modifier)
        }
    }
}

private const val REFRESH_MS = 20_000L
