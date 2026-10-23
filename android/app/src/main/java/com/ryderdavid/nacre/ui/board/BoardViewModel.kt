package com.ryderdavid.nacre.ui.board

import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.RepoGroup
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.BoardSnapshot
import com.ryderdavid.nacre.data.Loadable
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.Column
import com.ryderdavid.nacre.data.board.EpicSummary
import com.ryderdavid.nacre.data.board.GroupBy
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.board.moveActionFor
import com.ryderdavid.nacre.data.model.Issue
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

data class BoardUi(
    val snapshot: BoardSnapshot,
    val columns: List<Column>,
    val stats: Map<Lane, Int>,
    val needsYou: Int,
    val epics: List<EpicSummary>,
    val looseCount: Int,
    val running: Set<String>,
)

class BoardViewModel(private val repo: NacreRepository, val repoKey: String) : ViewModel() {
    val filter = MutableStateFlow(BoardFilter())
    val groupBy = MutableStateFlow(GroupBy.WORKFLOW)
    val showAllDone = MutableStateFlow(false)

    private val source: StateFlow<Loadable<BoardSnapshot>> = repo.board(repoKey)

    val state: StateFlow<Loadable<BoardUi>> = combine(
        source, filter, groupBy, showAllDone,
        // A run of this repository on any computer marks the bead as running.
        combine(repo.runs, repo.groups) { runs, _ ->
            val keys = repo.siblingKeys(repoKey)
            runs.filter { it.isActive && it.repoKey in keys }.map { it.issueId }.toSet()
        },
    ) { src, f, g, all, running ->
        val snap = src.data ?: return@combine Loadable<BoardUi>(null, src.loading, src.error)
        val ix = snap.index
        val ui = BoardUi(
            snapshot = snap,
            columns = ix.columns(g, f, doneLimit = if (all) Int.MAX_VALUE else 25),
            stats = ix.stats(f),
            needsYou = ix.needsYouCount(f),
            epics = ix.epics,
            looseCount = ix.issues.count { ix.epicOf(it) == null },
            running = running,
        )
        Loadable(ui, src.loading, src.error)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), Loadable(loading = true))

    init {
        // Gentle polling while the board is on screen; live repo events refresh it sooner.
        viewModelScope.launch {
            while (isActive) {
                delay(30_000)
                repo.refreshBoard(repoKey, quiet = true)
            }
        }
    }

    fun refresh() = repo.refreshBoard(repoKey)

    /** The repository this board belongs to, with its copies on other computers. */
    val group: StateFlow<RepoGroup?> = repo.groups.map { gs -> gs.firstOrNull { g -> g.members.any { it.key == repoKey } } }
        .stateIn(viewModelScope, SharingStarted.Eagerly, repo.groupOf(repoKey))

    val hosts = repo.hosts

    /** Makes [member] this repository's home; the caller then shows that computer's board. */
    fun makeHome(member: Repo) = viewModelScope.launch { group.value?.let { repo.setHome(it, member.hostId) } }

    fun move(issue: Issue, columnKey: String) {
        val action = moveActionFor(columnKey) ?: return
        repo.move(repoKey, issue, action)
    }

    fun setEpic(id: String) = filter.value.let { filter.value = it.copy(epic = if (it.epic == id) "" else id) }
    fun toggleNeedsYou() = filter.value.let { filter.value = it.copy(needsYou = !it.needsYou) }
    fun setQuery(q: String) = filter.value.let { filter.value = it.copy(query = q) }
    fun toggleType(t: String) = filter.value.let { filter.value = it.copy(types = if (t in it.types) it.types - t else it.types + t) }
    fun clearFilters() { filter.value = BoardFilter() }
}
