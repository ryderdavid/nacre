package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/** A change's overview (task 8.2). */
@Composable
internal fun ChangeOverviewPane(vm: SpecPanelViewModel, screen: SpecScreen.Change, modifier: Modifier) {
    Column(modifier.fillMaxSize()) { PaneTopBar(screen.changeId, "", onBack = { vm.back() }, backLabel = "Back to changes") }
}
