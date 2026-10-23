package com.ryderdavid.nacre.ui.bead

import kotlinx.coroutines.flow.combine
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Keys
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.claude.BeadContext
import com.ryderdavid.nacre.data.claude.ChildDraft
import com.ryderdavid.nacre.data.claude.ClaudeBackends
import com.ryderdavid.nacre.data.claude.ClaudeUnavailableException
import com.ryderdavid.nacre.data.claude.RefineProposal
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.UpdateIssue
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

sealed interface RefineState {
    data object Idle : RefineState
    data object Thinking : RefineState
    data class Ready(val proposal: RefineProposal) : RefineState
    /** [offerApiKey] when the user could get further by switching to the Claude API. */
    data class Failed(val message: String, val offerApiKey: Boolean = false) : RefineState
}

/** One computer a bead can be sent to. */
data class RunTarget(val repo: Repo, val online: Boolean)

/** Which parts of a refine proposal the user kept. */
data class RefineSelection(val fields: Set<String>, val children: Set<Int>)

class BeadDetailViewModel(
    private val repo: NacreRepository,
    private val backends: ClaudeBackends,
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

    /** Where this bead can run: this repository on each computer, with that computer's status. */
    val runTargets: StateFlow<List<RunTarget>> = combine(repo.groups, repo.hosts) { _, hosts ->
        val members = repo.groupOf(repoKey)?.members ?: listOfNotNull(repo.repo(repoKey))
        members.map { m -> RunTarget(m, hosts.firstOrNull { it.host.id == m.hostId }?.online == true) }
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
    val refine = MutableStateFlow<RefineState>(RefineState.Idle)

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

    fun toggleNeedsYou() = act("Updating") {
        val cur = issue.value ?: return@act
        val labels = if ("human" in cur.labels) cur.labels - "human" else cur.labels + "human"
        repo.update(repoKey, beadId, UpdateIssue(labels = labels))
    }

    fun addComment(text: String) = act("Commenting") { comments.value = repo.comment(repoKey, beadId, text) }
    fun addBlocker(id: String) = act("Adding dependency") { repo.addDep(repoKey, beadId, id) }
    fun removeBlocker(id: String) = act("Removing dependency") { repo.removeDep(repoKey, beadId, id) }

    fun startRun(targetKey: String, req: RunRequest, onStarted: (Run) -> Unit) = act("Starting run") {
        val run = repo.startRun(targetKey, beadId, req)
        repo.groupOf(targetKey)?.let { settings.setRunHost(it.id, Keys.host(targetKey)) }
        onStarted(run)
    }

    // ---- Claude: refine ----

    fun requestRefine(instruction: String) = viewModelScope.launch {
        val cur = issue.value ?: return@launch
        val ix = index.value ?: return@launch
        refine.value = RefineState.Thinking
        val s = settings.current()
        refine.value = try {
            val parent = ix.byId[cur.parent]
            val ctx = BeadContext(
                repoName = repo.repo(repoKey)?.name ?: repoKey,
                id = cur.id, title = cur.title, type = cur.type, priority = cur.priority, status = cur.status,
                labels = cur.labels, description = cur.description, design = cur.design,
                acceptance = cur.acceptance, notes = cur.notes,
                parentTitle = parent?.let { "${it.id} ${it.title}" },
                siblingTitles = parent?.let { p -> ix.childrenOf(p.id).filter { it.id != cur.id }.map { "${it.id} ${it.title}" } }.orEmpty(),
                childTitles = ix.childrenOf(cur.id).map { "${it.id} ${it.title}" },
            )
            RefineState.Ready(backends.of(s).refine(repoKey, ctx, instruction))
        } catch (e: ClaudeUnavailableException) {
            RefineState.Failed(e.message ?: "Claude isn't available right now", e.offerApiKey && s.apiKeyConfigured)
        } catch (e: Exception) {
            RefineState.Failed(e.message ?: "Claude couldn't refine this bead")
        }
    }

    /** Switches the Claude features to this phone's API key, after the Mac let us down. */
    fun useApiKey() = viewModelScope.launch {
        settings.update { it.copy(claudeRunner = AppSettings.RUNNER_API) }
    }

    fun dismissRefine() {
        refine.value = RefineState.Idle
    }

    fun applyRefine(p: RefineProposal, sel: RefineSelection, onDone: () -> Unit) = act("Applying changes") {
        fun <T> pick(key: String, v: T?) = if (key in sel.fields) v else null
        val update = UpdateIssue(
            title = pick("title", p.title), description = pick("description", p.description),
            design = pick("design", p.design), acceptance = pick("acceptance_criteria", p.acceptance_criteria),
            notes = pick("notes", p.notes), priority = pick("priority", p.priority),
            type = pick("issue_type", p.issue_type), labels = pick("labels", p.labels),
        )
        if (update != UpdateIssue()) repo.update(repoKey, beadId, update)
        p.children.filterIndexed { i, _ -> i in sel.children }.forEach { c: ChildDraft ->
            repo.create(
                repoKey,
                CreateIssue(
                    title = c.title, description = c.description.ifBlank { null }, type = c.issue_type,
                    priority = c.priority.coerceIn(0, 4), parent = beadId, acceptance = c.acceptance_criteria.ifBlank { null },
                ),
            )
        }
        val n = sel.fields.size + sel.children.size
        repo.notify("Applied $n change${if (n == 1) "" else "s"} from Claude")
        refine.value = RefineState.Idle
        onDone()
    }
}
