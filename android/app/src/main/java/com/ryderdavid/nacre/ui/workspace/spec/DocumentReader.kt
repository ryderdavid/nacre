package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/** A change's document reader (task 8.3). */
@Composable
internal fun DocumentReaderPane(vm: SpecPanelViewModel, screen: SpecScreen.Document, modifier: Modifier) {
    Column(modifier.fillMaxSize()) { PaneTopBar(screen.doc.label, screen.changeId, onBack = { vm.back() }, backLabel = "Back to the change") }
}
