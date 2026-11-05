package com.ryderdavid.nacre.ui.bead

import kotlinx.coroutines.flow.combine
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Keys
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.UpdateIssue
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** One computer a bead can be sent to; [health] is what it last reported, null while unreachable. */
data class RunTarget(val repo: Repo, val online: Boolean, val health: Health? = null) {
    /** Why this computer can't run the bead with Codex, or null when it can. */
    val codexBlocker: String? get() = when {
        !online -> "${repo.hostName.ifBlank { "This computer" }} isn't reachable right now"
        else -> health?.codexBlocker
    }
}

class BeadDetailViewModel(
    private val repo: NacreRepository,
    private val settings: SettingsStore,
    val repoKey: String,
    val beadId: String,
) : ViewModel() {
    val index: StateFlow<BoardIndex?> = repo.board(repoKey).map { it.data?.index }
        .stateIn(viewModelScope, SharingStarted.Eagerly, repo.board(repoKey).value.data?.index)

    val issue: StateFlow<Issue?> = index.map { it?.byId?.get(beadId) }
        .stateIn(viewModelScope, SharingStarted.Eagerly, index.value?.byId?.get(beadId))

    /** Runs of this bead on any computer that has this repository. */
    val runs: StateFlow<List<Run>> = combine(repo.runs, repo.groups) { list, _ ->
        val keys = repo.siblingKeys(repoKey)
        list.filter { it.repoKey in keys && it.issueId == beadId }
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    val rounds: StateFlow<List<Round>> = combine(repo.rounds, repo.groups) { list, _ ->
        val keys = repo.siblingKeys(repoKey)
        list.filter { it.repoKey in keys }
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    /** Where this bead can run: this repository on each computer, with that computer's status. */
    val runTargets: StateFlow<List<RunTarget>> = combine(repo.groups, repo.hosts) { _, hosts ->
        val members = repo.groupOf(repoKey)?.members ?: listOfNotNull(repo.repo(repoKey))
        members.map { m ->
            val c = hosts.firstOrNull { it.host.id == m.hostId }?.connection
            RunTarget(m, c is Connection.Online, (c as? Connection.Online)?.health)
        }
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    /** The computer this repository last ran beads on (default: the board's own). */
    suspend fun preferredRunKey(): String {
        val g = repo.groupOf(repoKey) ?: return repoKey
        val host = settings.current().runHosts[g.id]
        return g.members.firstOrNull { it.hostId == host }?.key ?: repoKey
    }

    val prefs = settings.settings

    val comments = MutableStateFlow<List<Comment>>(emptyList())
    val busy = MutableStateFlow(false)

    init {
        load()
    }

    fun load() = viewModelScope.launch {
        runCatching { repo.detail(repoKey, beadId) }
            .onSuccess { comments.value = it.comments }
            .onFailure { if (issue.value == null) repo.notify("Couldn't load $beadId: ${it.friendly()}") }
    }

    private fun act(label: String, block: suspend () -> Unit) = viewModelScope.launch {
        busy.value = true
        try {
            block()
        } catch (e: Exception) {
            repo.notify("$label failed: ${e.friendly()}")
        } finally {
            busy.value = false
        }
    }

    fun save(update: UpdateIssue) = act("Saving") { repo.update(repoKey, beadId, update) }

    fun setStatus(status: String) = act("Updating status") {
        val cur = issue.value ?: return@act
        when {
            status == "closed" -> repo.close(repoKey, beadId, null)
            cur.isClosed -> {
                repo.reopen(repoKey, beadId)
                if (status != "open") repo.update(repoKey, beadId, UpdateIssue(status = status))
            }
            else -> repo.update(repoKey, beadId, UpdateIssue(status = status))
        }
    }

    fun close(reason: String?) = act("Closing") { repo.close(repoKey, beadId, reason?.ifBlank { null }) }

    /** Signs off a bead left for Ryder's check, or sends it back with what isn't right. */
    fun signOff(approve: Boolean, note: String) = act(if (approve) "Signing off" else "Sending back") {
        repo.signOff(repoKey, beadId, approve, note.trim())
    }

    fun toggleNeedsYou() = act("Updating") {
        val cur = issue.value ?: return@act
        val labels = if ("human" in cur.labels) cur.labels - "human" else cur.labels + "human"
        repo.update(repoKey, beadId, UpdateIssue(labels = labels))
    }

    fun addComment(text: String) = act("Commenting") { comments.value = repo.comment(repoKey, beadId, text) }

    fun addBlocker(id: String) = act("Adding dependency") { repo.addDep(repoKey, beadId, id) }
    fun removeBlocker(id: String) = act("Removing dependency") { repo.removeDep(repoKey, beadId, id) }

    /** A one-bead round: its own worktree, every gate and the build, commit, land, push, close. */
    fun startWork(targetKey: String, onStarted: (Round) -> Unit) = act("Starting the work") {
        val round = repo.startRound(targetKey, listOf(beadId), landWhenReady = true)
        repo.groupOf(targetKey)?.let { settings.setRunHost(it.id, Keys.host(targetKey)) }
        onStarted(round)
    }

    /** Works this epic's open beads in one round on its branch, landing when they pass. */
    fun workEpic(onStarted: (Round) -> Unit) = act("Starting the work") {
        onStarted(repo.startRound(repoKey, emptyList(), epic = beadId, landWhenReady = true, finish = true))
    }

    /** Merges this finished epic into the main branch; [onDone] gets the round, or null when it simply closed. */
    fun finishEpic(onDone: (Round?) -> Unit) = act("Merging the epic") {
        val result = repo.finishEpic(repoKey, beadId)
        load()
        onDone(result.round)
    }

    fun startRun(targetKey: String, req: RunRequest, onStarted: (Run) -> Unit) = act("Starting run") {
        val run = repo.startRun(targetKey, beadId, req)
        repo.groupOf(targetKey)?.let { settings.setRunHost(it.id, Keys.host(targetKey)) }
        onStarted(run)
    }
}
