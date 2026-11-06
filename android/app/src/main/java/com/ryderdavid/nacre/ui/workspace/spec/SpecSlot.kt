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
fun SpecSlot(repoKey: String, modifier: Modifier) {
    val vm = nacreViewModel(key = "spec:$repoKey") { SpecPanelViewModel(it.repository, repoKey) }
    val repo = LocalContainer.current.repository
    val stack by vm.stack.collectAsState()
    val viewing = LocalViewing.current
    val screen = stack.last()

    // Live progress: the board (section evidence) and rounds (failed beads) while the pane is shown.
    LaunchedEffect(repoKey) {
        while (true) {
            repo.refreshBoard(repoKey, quiet = true)
            repo.refreshRounds()
            delay(REFRESH_MS)
        }
    }
    BackHandler(enabled = stack.size > 1) { vm.back() }
    if (screen == SpecScreen.List) LaunchedEffect(Unit) { viewing(null) }

    Box(modifier) {
        when (screen) {
            SpecScreen.List -> SpecChangeList(vm, Modifier)
            is SpecScreen.Change -> ChangeOverviewPane(vm, screen, Modifier)
            is SpecScreen.Document -> DocumentReaderPane(vm, screen, Modifier)
        }
    }
}

private const val REFRESH_MS = 20_000L
