package com.ryderdavid.nacre.ui.common

import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.ryderdavid.nacre.AppContainer

val LocalContainer = staticCompositionLocalOf<AppContainer> { error("AppContainer not provided") }

/** A ViewModel built from the app container, optionally keyed (e.g. per repo or bead). */
@Composable
inline fun <reified VM : ViewModel> nacreViewModel(key: String? = null, crossinline create: (AppContainer) -> VM): VM {
    val container = LocalContainer.current
    return viewModel(key = key, factory = viewModelFactory { initializer { create(container) } })
}
