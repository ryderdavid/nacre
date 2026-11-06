package com.ryderdavid.nacre.ui.workspace.beads

import androidx.compose.runtime.staticCompositionLocalOf
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.Run
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * Open change: shows a change in the Spec pane. Provided by the workspace; the default does nothing, so the
 * Beads panel works on its own.
 */
val LocalOpenChange = staticCompositionLocalOf<(changeId: String) -> Unit> { {} }

/** The bead peek's live data beyond the board: comments, and the runs and rounds that worked on the bead. */
class BeadPeekViewModel(private val repo: NacreRepository, val repoKey: String, val beadId: String) : ViewModel() {
    val comments = MutableStateFlow<List<Comment>>(emptyList())
    val busy = MutableStateFlow(false)

    /** Runs of this bead on any computer that has this repository. */
    val runs: StateFlow<List<Run>> = combine(repo.runs, repo.groups) { list, _ ->
        val keys = repo.siblingKeys(repoKey)
        list.filter { it.repoKey in keys && it.issueId == beadId }
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    val rounds: StateFlow<List<Round>> = combine(repo.rounds, repo.groups) { list, _ ->
        val keys = repo.siblingKeys(repoKey)
        list.filter { r -> r.repoKey in keys && r.beads.any { it.id == beadId } }
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    fun load() = viewModelScope.launch {
        runCatching { repo.detail(repoKey, beadId) }
            .onSuccess { comments.value = it.comments }
            .onFailure { repo.notify("Couldn't load the comments on $beadId: ${it.friendly()}") }
    }

    /** A one-bead round that stops before landing: Ryder lands it once he has looked. */
    fun startWork(onStarted: (Round) -> Unit) {
        if (busy.value) return
        busy.value = true
        viewModelScope.launch {
            try {
                onStarted(repo.startRound(repoKey, listOf(beadId), landWhenReady = false))
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                repo.notify("Couldn't start work on $beadId: ${e.friendly()}")
            } finally {
                busy.value = false
            }
        }
    }
}
