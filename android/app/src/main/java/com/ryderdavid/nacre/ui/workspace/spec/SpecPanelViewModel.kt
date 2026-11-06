package com.ryderdavid.nacre.ui.workspace.spec

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.Loadable
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.Round
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Where the Spec pane is: the list, a change's overview, or one of its documents. */
sealed interface SpecScreen {
    data object List : SpecScreen
    data class Change(val changeId: String) : SpecScreen
    data class Document(val changeId: String, val doc: DocKind, val anchor: String = "") : SpecScreen
}

/** A change's documents, in the selector's order. */
enum class DocKind(val label: String) {
    PROPOSAL("Proposal"),
    DESIGN("Design"),
    TASKS("Tasks"),
    SPECS("Specs");

    companion object {
        /** Which document an artifact path belongs to. */
        fun of(path: String): DocKind? = when {
            path.endsWith("/proposal.md") || path == "proposal.md" -> PROPOSAL
            path.endsWith("/design.md") || path == "design.md" -> DESIGN
            path.endsWith("/tasks.md") || path == "tasks.md" -> TASKS
            SpecList.capabilityOf(path) != null -> SPECS
            else -> null
        }
    }
}

/** Beads parked in a round that hasn't landed or been cancelled: their sections show Failed. */
fun failedBeads(rounds: List<Round>, repoKey: String): Set<String> =
    rounds.filter { it.repoKey == repoKey && it.isActive }.flatMap { r -> r.beads.filter { it.state == BEAD_PARKED }.map { it.id } }.toSet()

/**
 * The Spec pane's state for one project: where it is (list → change → document, so Back walks back),
 * the search, and the changes loaded so far. Scoped to the workspace, so switching destinations on the
 * cover screen keeps the place.
 */
class SpecPanelViewModel(private val repository: NacreRepository, val repoKey: String) : ViewModel() {
    val stack = MutableStateFlow<List<SpecScreen>>(listOf(SpecScreen.List))
    val query = MutableStateFlow("")
    val searching = MutableStateFlow(false)
    val envelopes = MutableStateFlow<Map<String, Loadable<ChangeEnvelope>>>(emptyMap())

    /** The summary's updated_at each change was last loaded at, so a newer one loads again. */
    private val loadedAt = mutableMapOf<String, String>()

    val top get() = stack.value.last()

    fun open(screen: SpecScreen) = stack.update { s -> if (s.last() == screen) s else s + screen }

    /** Swaps the document shown without growing the back stack. */
    fun switchTo(screen: SpecScreen) = stack.update { s -> if (s.size > 1 && s.last() is SpecScreen.Document) s.dropLast(1) + screen else s + screen }

    /** From a document, the change's overview: back to it when it is underneath, else in the document's place. */
    fun showOverview(changeId: String) = stack.update { s ->
        val below = s.dropLast(1)
        if (below.lastOrNull() == SpecScreen.Change(changeId)) below else below + SpecScreen.Change(changeId)
    }

    /** Opens [changeId]'s overview on top of the list, as an Open change link from elsewhere asks. */
    fun openChange(changeId: String) = stack.update { listOf(SpecScreen.List, SpecScreen.Change(changeId)) }

    /** Back: document → change → list. False when already on the list. */
    fun back(): Boolean {
        if (stack.value.size <= 1) return false
        stack.update { it.dropLast(1) }
        return true
    }

    fun setQuery(q: String) { query.value = q }

    fun setSearching(on: Boolean) {
        searching.value = on
        if (!on) query.value = ""
    }

    /** Loads [summary]'s change, unless the same version is loaded or loading. */
    fun load(summary: ChangeSummary, force: Boolean = false) {
        val id = summary.id
        val current = envelopes.value[id]
        if (!force && current != null && (current.loading || (loadedAt[id] == summary.updatedAt && current.error == null))) return
        loadedAt[id] = summary.updatedAt
        envelopes.update { it + (id to (current ?: Loadable()).copy(loading = true)) }
        viewModelScope.launch {
            try {
                val env = if (summary.untracked) repository.untrackedChange(repoKey, summary.changeName.ifBlank { summary.id })
                else repository.change(repoKey, SpecList.openId(summary))
                envelopes.update { it + (id to Loadable(env)) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // Keep the last loaded change readable through a connection drop.
                envelopes.update { it + (id to (it[id] ?: Loadable()).copy(loading = false, error = e.friendly())) }
            }
        }
    }

    /** Loads the listed changes in the background, so the specs they touch and search can be shown. */
    fun prefetch(changes: List<ChangeSummary>) {
        changes.filter { SpecList.group(it) != null }.take(MAX_PREFETCH).forEach { load(it) }
    }

    companion object {
        const val MAX_PREFETCH = 16
    }
}
