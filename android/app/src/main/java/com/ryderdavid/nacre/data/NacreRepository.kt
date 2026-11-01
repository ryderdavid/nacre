package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.MoveAction
import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.bridge.RoundMessage
import com.ryderdavid.nacre.data.bridge.StreamMessage
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.AttentionOption
import com.ryderdavid.nacre.data.model.ChangeEvent
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.FoundRequest
import com.ryderdavid.nacre.data.model.FoundResult
import com.ryderdavid.nacre.data.model.IntakeDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.DaemonUpdate
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.StartRound
import com.ryderdavid.nacre.data.model.UpdateIssue
import com.ryderdavid.nacre.data.settings.Host
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import java.util.concurrent.ConcurrentHashMap

data class Loadable<T>(val data: T? = null, val loading: Boolean = false, val error: String? = null)

/**
 * A board's beads, plus its specification changes: null when the computer
 * doesn't report them (older nacred), which hides spec chips and the Proposals lane.
 */
data class BoardSnapshot(val repo: Repo, val index: BoardIndex, val changes: ChangesList? = null) {
    val changesById get() = changes?.changes?.associateBy { it.id }.orEmpty()
}

sealed interface Connection {
    data object Unconfigured : Connection
    data object Connecting : Connection
    data class Online(val health: Health) : Connection
    data class Offline(val reason: String) : Connection
}

/** A paired computer and how reaching it is going. */
data class HostState(val host: Host, val connection: Connection) {
    val online get() = connection is Connection.Online
}

/**
 * One repository as the user sees it: the same git remote checked out on several
 * computers is one group. The board shows [home]'s beads; runs may go to any member.
 */
data class RepoGroup(val id: String, val home: Repo, val members: List<Repo>) {
    val isShared get() = members.size > 1
}

/** A user-visible notice raised by the data layer (shown as a snackbar). */
data class Notice(val text: String, val undo: (suspend () -> Unit)? = null)

/**
 * Single source of truth for everything the app shows from the paired computers.
 * Repos are addressed by key ("<host>~<repo id>", see [Keys]) because ids are only
 * unique per computer.
 */
class NacreRepository(
    private val settings: SettingsStore,
    private val http: OkHttpClient,
    private val scope: CoroutineScope,
) {
    private class Paired(val host: Host, val client: BridgeClient)

    private val paired: StateFlow<Map<String, Paired>> = settings.settings
        .map { s -> s.hosts }
        .distinctUntilChanged()
        .map { hosts -> hosts.associate { h -> h.id to Paired(h, BridgeClient(h.url, h.token, http)) } }
        .stateIn(scope, SharingStarted.Eagerly, emptyMap())

    private val connections = MutableStateFlow<Map<String, Connection>>(emptyMap())
    private val daemonUpdates = DaemonUpdates(scope)
    val daemonUpdateStates = daemonUpdates.states

    private fun updateComputers() = paired.value.values.map { p ->
        UpdateComputer(p.host.id, p.host.hostId, connections.value[p.host.id] is Connection.Online,
            status = { p.client.daemonUpdate().also { refreshUpdatedComputer(p, it) } },
            start = { p.client.startDaemonUpdate().also { refreshUpdatedComputer(p, it) } })
    }

    private suspend fun refreshUpdatedComputer(p: Paired, update: DaemonUpdate) {
        if (!update.verifiedUpdate) return
        try {
            val health = p.client.health()
            if (paired.value[p.host.id] === p && health.hostId == p.host.hostId) {
                connections.update { it + (p.host.id to Connection.Online(health)) }
            }
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            // A transient refresh failure must not replace the durable update result.
        }
    }
    fun refreshDaemonUpdates() = daemonUpdates.refresh(updateComputers())
    fun updateConnectedComputers() = daemonUpdates.startAll(updateComputers())

    val hosts: StateFlow<List<HostState>> = combine(paired, connections) { p, c ->
        p.values.map { HostState(it.host, c[it.host.id] ?: Connection.Connecting) }
    }.stateIn(scope, SharingStarted.Eagerly, emptyList())

    /** One summary for places that show a single status line. */
    val connection: StateFlow<Connection> = hosts.map { hs ->
        when {
            hs.isEmpty() -> Connection.Unconfigured
            hs.any { it.online } -> hs.first { it.online }.connection
            hs.any { it.connection == Connection.Connecting } -> Connection.Connecting
            else -> hs.first().connection
        }
    }.stateIn(scope, SharingStarted.Eagerly, Connection.Unconfigured)

    private val repoLists = MutableStateFlow<Map<String, List<Repo>>>(emptyMap())
    private val repoErrors = MutableStateFlow<Map<String, String>>(emptyMap())
    private val reposLoading = MutableStateFlow(false)

    /** Every repo on every computer, each tagged with its computer. */
    val repos: StateFlow<Loadable<List<Repo>>> = combine(repoLists, repoErrors, reposLoading, paired) { lists, errs, loading, p -> reposState(lists, errs, loading, p) }
        .stateIn(scope, SharingStarted.Eagerly, Loadable(loading = true))

    private fun reposState(lists: Map<String, List<Repo>>, errs: Map<String, String>, loading: Boolean, p: Map<String, Paired>): Loadable<List<Repo>> {
        val all = p.keys.flatMap { lists[it].orEmpty() }
        val settled = p.keys.all { it in lists || it in errs }
        return when {
            p.isEmpty() -> Loadable(emptyList())
            all.isEmpty() && !settled -> Loadable(null, loading = true)
            all.isEmpty() && errs.isNotEmpty() -> Loadable(null, loading, errs.values.first())
            else -> Loadable(all, loading)
        }
    }

    /** Repos grouped by git remote; computer order (pairing order) breaks ties for the home. */
    val groups: StateFlow<List<RepoGroup>> = combine(repos, settings.settings) { r, s ->
        groupRepos(r.data.orEmpty(), s.hosts.map { it.id }, s.homeHosts)
    }.stateIn(scope, SharingStarted.Eagerly, emptyList())

    private val boards = ConcurrentHashMap<String, MutableStateFlow<Loadable<BoardSnapshot>>>()

    private val runLists = MutableStateFlow<Map<String, List<Run>>>(emptyMap())
    val runs: StateFlow<List<Run>> = runLists.map { m -> m.values.flatten().sortedByDescending { it.createdAt } }
        .stateIn(scope, SharingStarted.Eagerly, emptyList())

    private val roundLists = MutableStateFlow<Map<String, List<Round>>>(emptyMap())

    /** Every round on every computer, newest first. */
    val rounds: StateFlow<List<Round>> = roundLists.map { m -> m.values.flatten().sortedByDescending { it.createdAt } }
        .stateIn(scope, SharingStarted.Eagerly, emptyList())

    /** Each round change as (before, after), for notifications. */
    private val _roundChanges = MutableSharedFlow<Pair<Round?, Round>>(extraBufferCapacity = 16)
    val roundChanges: SharedFlow<Pair<Round?, Round>> = _roundChanges

    private val _notices = MutableSharedFlow<Notice>(extraBufferCapacity = 8)
    val notices: SharedFlow<Notice> = _notices

    private val attentionLists = MutableStateFlow<Map<String, List<AttentionItem>>>(emptyMap())

    /**
     * Needs you across computers, deduped by item id, questions and decisions
     * first, then approvals, then landing and archive, newest first within each.
     */
    val attention: StateFlow<List<AttentionItem>> = combine(attentionLists, paired) { m, p ->
        p.keys.flatMap { m[it].orEmpty() }.distinctBy { it.id }
            .sortedWith(compareBy<AttentionItem> { it.group }.thenByDescending { it.createdAt })
    }.stateIn(scope, SharingStarted.Eagerly, emptyList())

    /** Whether any paired computer reports Needs you at all. */
    private val attentionHosts = MutableStateFlow<Set<String>>(emptySet())
    val attentionSupported: StateFlow<Boolean> = attentionHosts.map { it.isNotEmpty() }.stateIn(scope, SharingStarted.Eagerly, false)

    private val working = MutableStateFlow<Set<String>>(emptySet())

    /**
     * Changes being drafted, reviewed or promoted, as "<host>~<change id>": while
     * any are, the app keeps watching for their events as it does for runs.
     */
    val workingChanges: StateFlow<Set<String>> = working

    /** Each change phase transition, with its computer, for notifications. */
    private val _changeEvents = MutableSharedFlow<Pair<String, ChangeEvent>>(extraBufferCapacity = 16)
    val changeEvents: SharedFlow<Pair<String, ChangeEvent>> = _changeEvents

    /** Fires the run whenever a run reaches a terminal state (for notifications). */
    private val _finishedRuns = MutableSharedFlow<Run>(extraBufferCapacity = 8)
    val finishedRuns: SharedFlow<Run> = _finishedRuns

    private val eventJobs = ConcurrentHashMap<String, Job>()
    @Volatile private var eventsWanted = false

    init {
        scope.launch {
            var previous = emptySet<String>()
            paired.collect { p ->
                (previous - p.keys).forEach(::forget)
                // Clients are rebuilt whenever the list of computers changes; restart their streams.
                eventJobs.values.forEach { it.cancel() }
                eventJobs.clear()
                previous = p.keys
                refreshRepos()
                refreshRuns()
                refreshRounds()
                refreshAttention()
                if (eventsWanted) startEvents()
            }
        }
    }

    private fun forget(hostId: String) {
        daemonUpdates.remove(hostId)
        repoLists.update { it - hostId }
        repoErrors.update { it - hostId }
        runLists.update { it - hostId }
        roundLists.update { it - hostId }
        attentionLists.update { it - hostId }
        attentionHosts.update { it - hostId }
        connections.update { it - hostId }
        boards.keys.filter { Keys.host(it) == hostId }.forEach { boards.remove(it) }
    }

    private fun pairedFor(key: String): Paired =
        paired.value[Keys.host(key)] ?: throw BridgeException(if (paired.value.isEmpty()) "Pair with a computer in Settings first" else "That computer is no longer paired")

    private fun clientFor(key: String) = pairedFor(key).client

    fun hostName(hostId: String) = paired.value[hostId]?.host?.name.orEmpty()

    fun notify(text: String, undo: (suspend () -> Unit)? = null) {
        _notices.tryEmit(Notice(text, undo))
    }

    private fun Repo.tag(h: Host) = copy(hostId = h.id, hostName = h.name)
    private fun Run.tag(h: Host) = copy(hostId = h.id, hostName = h.name)
    private fun Round.tag(h: Host) = copy(hostId = h.id, hostName = h.name)

    // ---------- connection + live events ----------

    /** Start the event streams (one per computer) while the app is in the foreground. */
    fun startEvents() {
        eventsWanted = true
        paired.value.values.forEach { p ->
            if (eventJobs[p.host.id]?.isActive == true) return@forEach
            eventJobs[p.host.id] = scope.launch { followHost(p) }
        }
    }

    fun stopEvents() {
        eventsWanted = false
        eventJobs.values.forEach { it.cancel() }
        eventJobs.clear()
    }

    private suspend fun followHost(p: Paired) {
        val id = p.host.id
        var backoff = 1_000L
        while (scope.isActive && paired.value[id] === p) {
            try {
                connections.update { it + (id to Connection.Connecting) }
                val health = p.client.health()
                connections.update { it + (id to Connection.Online(health)) }
                if (health.hostId.isNotEmpty() && health.hostId != p.host.hostId) settings.setReportedHostId(id, health.hostId)
                backoff = 1_000L
                p.client.events().collect { msg ->
                    when (msg) {
                        is StreamMessage.RunChanged -> upsertRun(msg.run.tag(p.host))
                        is StreamMessage.RoundChanged -> { upsertRound(msg.round.tag(p.host)); attentionSoon(p) }
                        is StreamMessage.RepoChanged -> {
                            Keys.of(id, msg.repoId).let { key -> if (boards.containsKey(key)) refreshBoard(key, quiet = true) }
                            attentionSoon(p)
                        }
                        is StreamMessage.ChangeChanged -> {
                            _changeEvents.tryEmit(id to msg.event)
                            val k = Keys.of(id, msg.event.changeId)
                            working.update { if (msg.event.phase in WORKING_PHASES) it + k else it - k }
                            Keys.of(id, msg.event.repoId).let { key -> if (boards.containsKey(key)) refreshBoard(key, quiet = true) }
                            attentionSoon(p)
                        }
                        else -> Unit
                    }
                }
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                connections.update { it + (id to Connection.Offline(e.friendly())) }
            }
            delay(backoff)
            backoff = (backoff * 2).coerceAtMost(30_000L)
        }
    }

    /** Checks a computer that is not paired yet (or re-checks one that is). */
    suspend fun probe(url: String, token: String): Result<Health> = runCatching { BridgeClient(url, token, http).health() }

    // ---------- repos ----------

    fun refreshRepos(rescan: Boolean = false) = scope.launch {
        val p = paired.value
        if (p.isEmpty()) return@launch
        reposLoading.value = true
        p.values.map { h ->
            async {
                try {
                    val list = h.client.repos(rescan).map { it.tag(h.host) }
                    repoLists.update { it + (h.host.id to list) }
                    repoErrors.update { it - h.host.id }
                    if (connections.value[h.host.id] !is Connection.Online) {
                        connections.update { it + (h.host.id to Connection.Online(Health(name = "nacred", hostName = h.host.name))) }
                    }
                } catch (e: Exception) {
                    repoErrors.update { it + (h.host.id to "${h.host.name}: ${e.friendly()}") }
                    connections.update { it + (h.host.id to Connection.Offline(e.friendly())) }
                }
            }
        }.awaitAll()
        reposLoading.value = false
    }

    fun repo(key: String): Repo? = repos.value.data?.firstOrNull { it.key == key } ?: boards[key]?.value?.data?.repo

    fun groupOf(key: String): RepoGroup? = groups.value.firstOrNull { g -> g.members.any { it.key == key } }

    /** Keys of the same repository on every computer (just [key] when it is on one). */
    fun siblingKeys(key: String): Set<String> = groupOf(key)?.members?.map { it.key }?.toSet() ?: setOf(key)

    suspend fun setHome(group: RepoGroup, hostId: String) = settings.setHomeHost(group.id, hostId)

    // ---------- boards ----------

    fun board(key: String): StateFlow<Loadable<BoardSnapshot>> =
        boards.getOrPut(key) { MutableStateFlow(Loadable()) }.also { if (it.value.data == null && !it.value.loading) refreshBoard(key) }

    /** The board's beads, fetching them first when this board hasn't been looked at yet. */
    suspend fun awaitBoard(key: String): BoardSnapshot {
        val flow = boards.getOrPut(key) { MutableStateFlow(Loadable()) }
        flow.value.data?.let { return it }
        refreshBoard(key).join()
        return flow.value.data ?: throw BridgeException(flow.value.error ?: "Couldn't load that board")
    }

    fun refreshBoard(key: String, quiet: Boolean = false) = scope.launch {
        val flow = boards.getOrPut(key) { MutableStateFlow(Loadable()) }
        if (!quiet) flow.update { it.copy(loading = true, error = null) }
        try {
            val p = pairedFor(key)
            val (b, changes) = coroutineScope {
                val changes = async { runCatching { p.client.changes(Keys.id(key)) }.getOrElse { flow.value.data?.changes } }
                p.client.board(Keys.id(key)) to changes.await()
            }
            val repo = b.repo.tag(p.host)
            flow.value = Loadable(BoardSnapshot(repo, BoardIndex(b.issues), changes))
            repoLists.update { m -> m + (p.host.id to m[p.host.id].orEmpty().map { if (it.id == repo.id) repo else it }) }
        } catch (e: Exception) {
            flow.update { it.copy(loading = false, error = e.friendly()) }
        }
    }

    private fun patchLocal(key: String, issue: Issue) {
        boards[key]?.update { l ->
            val snap = l.data ?: return@update l
            val list = snap.index.issues.let { all -> if (all.any { it.id == issue.id }) all.map { if (it.id == issue.id) issue else it } else all + issue }
            l.copy(data = snap.copy(index = BoardIndex(list)))
        }
    }

    fun issue(key: String, id: String): Issue? = boards[key]?.value?.data?.index?.byId?.get(id)

    suspend fun change(key: String, id: String): ChangeEnvelope = clientFor(key).change(Keys.id(key), id)

    suspend fun changeAction(key: String, id: String, action: ChangeAction): ChangeEnvelope =
        clientFor(key).changeAction(Keys.id(key), id, action).also { env ->
            env.change?.takeIf { it.isWorking }?.let { c -> working.update { it + Keys.of(Keys.host(key), c.id) } }
            refreshBoard(key, quiet = true); refreshAttention(Keys.host(key))
        }

    /** The board's changes as last loaded; null when its computer doesn't report them. */
    fun changes(key: String): ChangesList? = boards[key]?.value?.data?.changes

    /** Intake's verdict on a New effort draft (one bounded call on [key]'s computer). */
    suspend fun intake(key: String, draft: IntakeDraft): IntakeVerdict = clientFor(key).intake(Keys.id(key), draft)

    /** Answers found work on a promoted change. */
    suspend fun found(key: String, changeId: String, req: FoundRequest): FoundResult =
        clientFor(key).found(Keys.id(key), changeId, req).also { refreshBoard(key, quiet = true); refreshAttention(Keys.host(key)) }

    // ---------- Needs you ----------

    private val attentionJobs = ConcurrentHashMap<String, Job>()

    /** Reloads Needs you from every computer, or just [hostId]. */
    fun refreshAttention(hostId: String? = null) = scope.launch {
        paired.value.values.filter { hostId == null || it.host.id == hostId }.map { p -> async { loadAttention(p) } }.awaitAll()
    }

    private suspend fun loadAttention(p: Paired) {
        try {
            val items = p.client.attention()
            if (items == null) {
                attentionHosts.update { it - p.host.id }
                attentionLists.update { it - p.host.id }
            } else {
                attentionHosts.update { it + p.host.id }
                attentionLists.update { it + (p.host.id to items.map { i -> i.copy(hostId = p.host.id, hostName = p.host.name) }) }
            }
        } catch (e: Exception) {
            if (e is kotlinx.coroutines.CancellationException) throw e
            // Keep the last list: an unreachable computer's items are still waiting.
        }
    }

    /** Coalesces a burst of events into one reload, a moment later. */
    private fun attentionSoon(p: Paired) {
        if (attentionJobs[p.host.id]?.isActive == true) return
        attentionJobs[p.host.id] = scope.launch { delay(1_500); loadAttention(p) }
    }

    /** Sends a Needs you action exactly as its computer described it, then reloads. */
    suspend fun perform(item: AttentionItem, option: AttentionOption) {
        val p = paired.value[item.hostId] ?: throw BridgeException("That computer is no longer paired")
        p.client.perform(option.method, option.path, option.body)
        refreshAttention(item.hostId).join()
        if (boards.containsKey(item.repoKey)) refreshBoard(item.repoKey, quiet = true)
        refreshRounds()
    }

    /** Optimistic drag-and-drop move with undo. */
    fun move(key: String, issue: Issue, action: MoveAction) = scope.launch {
        val before = issue
        val optimistic = when (action) {
            is MoveAction.SetStatus -> issue.copy(status = action.status)
            MoveAction.Close -> issue.copy(status = "closed", closedAt = java.time.Instant.now().toString())
            is MoveAction.SetPriority -> issue.copy(priority = action.priority)
            is MoveAction.SetType -> issue.copy(type = action.type)
        }
        patchLocal(key, optimistic)
        try {
            val c = clientFor(key)
            val repoId = Keys.id(key)
            val saved = when (action) {
                is MoveAction.SetStatus -> c.update(repoId, issue.id, UpdateIssue(status = action.status))
                MoveAction.Close -> c.close(repoId, issue.id)
                is MoveAction.SetPriority -> c.update(repoId, issue.id, UpdateIssue(priority = action.priority))
                is MoveAction.SetType -> c.update(repoId, issue.id, UpdateIssue(type = action.type))
            }
            patchLocal(key, saved)
            val what = when (action) {
                is MoveAction.SetStatus -> "Moved ${issue.id} to ${action.status.replace('_', ' ')}"
                MoveAction.Close -> "Closed ${issue.id}"
                is MoveAction.SetPriority -> "${issue.id} is now P${action.priority}"
                is MoveAction.SetType -> "${issue.id} is now a ${action.type}"
            }
            notify(what) { restore(key, before) }
        } catch (e: Exception) {
            patchLocal(key, before)
            notify("Couldn't move ${issue.id}: ${e.friendly()}")
        }
    }

    private suspend fun restore(key: String, before: Issue) {
        val c = clientFor(key)
        val repoId = Keys.id(key)
        val now = issue(key, before.id)
        val saved = if (before.isClosed) c.close(repoId, before.id, before.closeReason.ifBlank { null })
        else {
            if (now?.isClosed == true) c.reopen(repoId, before.id)
            c.update(repoId, before.id, UpdateIssue(status = before.status, priority = before.priority, type = before.type))
        }
        patchLocal(key, saved)
    }

    suspend fun update(key: String, id: String, body: UpdateIssue): Issue =
        clientFor(key).update(Keys.id(key), id, body).also { patchLocal(key, it) }

    suspend fun create(key: String, body: CreateIssue): Issue =
        clientFor(key).create(Keys.id(key), body).also { patchLocal(key, it) }

    suspend fun close(key: String, id: String, reason: String?): Issue = clientFor(key).close(Keys.id(key), id, reason).also { patchLocal(key, it) }
    suspend fun reopen(key: String, id: String): Issue = clientFor(key).reopen(Keys.id(key), id).also { patchLocal(key, it) }
    suspend fun detail(key: String, id: String) = clientFor(key).detail(Keys.id(key), id).also { patchLocal(key, it.issue) }
    suspend fun comment(key: String, id: String, text: String) = clientFor(key).comment(Keys.id(key), id, text)
    suspend fun addDep(key: String, issue: String, dependsOn: String, type: String = "blocks") {
        clientFor(key).addDep(Keys.id(key), issue, dependsOn, type); refreshBoard(key, quiet = true)
    }
    suspend fun removeDep(key: String, issue: String, dependsOn: String) {
        clientFor(key).removeDep(Keys.id(key), issue, dependsOn); refreshBoard(key, quiet = true)
    }

    // ---------- runs ----------

    fun refreshRuns() = scope.launch {
        paired.value.values.map { p ->
            async { runCatching { p.client.runs() }.onSuccess { list -> runLists.update { it + (p.host.id to list.map { r -> r.tag(p.host) }) } } }
        }.awaitAll()
    }

    private fun upsertRun(run: Run) {
        val prev = runLists.value[run.hostId]?.firstOrNull { it.id == run.id }
        runLists.update { m -> m + (run.hostId to (listOf(run) + m[run.hostId].orEmpty().filterNot { it.id == run.id })) }
        if (prev?.isActive == true && !run.isActive) _finishedRuns.tryEmit(run)
    }

    /**
     * Starts a run of [issueId] in the repo [repoKey] (which may be another computer's
     * copy of the board's repository). A 404 there means that copy lacks the bead.
     */
    // ---------- Claude on a paired computer ----------

    /** Runs one Refine or Draft call on [key]'s computer, on its Claude subscription. */
    suspend fun claudeJson(key: String, req: AssistRequest): AssistResult = clientFor(key).claudeJson(Keys.id(key), req)

    /** Runs one board-assistant turn on [key]'s computer, streamed as it goes. */
    fun claudeStream(key: String, req: AssistRequest): Flow<AssistMessage> = clientFor(key).claudeStream(Keys.id(key), req)

    /** Reattaches to a turn on [key]'s computer after the connection dropped. */
    fun claudeResume(key: String, turn: String, after: Long): Flow<AssistMessage> = clientFor(key).claudeResume(turn, after)

    /** Stops a turn on [key]'s computer. */
    suspend fun claudeCancel(key: String, turn: String) = clientFor(key).claudeCancel(turn)

    suspend fun startRun(repoKey: String, issueId: String, req: RunRequest): Run {
        val p = pairedFor(repoKey)
        val run = try {
            p.client.startRun(Keys.id(repoKey), issueId, req)
        } catch (e: BridgeException) {
            if (e.code == 404) throw BridgeException("$issueId isn't in ${p.host.name}'s copy of these beads. Sync the beads to it first, or run it on another computer.", e.detail, 404)
            throw e
        }
        return run.tag(p.host).also(::upsertRun)
    }

    suspend fun cancelRun(key: String): Run { val p = pairedFor(key); return p.client.cancel(Keys.id(key)).tag(p.host).also(::upsertRun) }
    suspend fun followUp(key: String, message: String, mode: String?): Run {
        val p = pairedFor(key); return p.client.followUp(Keys.id(key), message, mode).tag(p.host).also(::upsertRun)
    }

    suspend fun runDetail(key: String): BridgeClient.RunDetail {
        val p = pairedFor(key)
        val d = p.client.run(Keys.id(key))
        return d.copy(run = d.run.tag(p.host))
    }

    fun runStream(key: String, after: Long): Flow<StreamMessage> {
        val p = pairedFor(key)
        return p.client.runStream(Keys.id(key), after).map { if (it is StreamMessage.RunChanged) StreamMessage.RunChanged(it.run.tag(p.host)) else it }
    }

    fun noteRun(run: Run) = upsertRun(run)

    // ---------- rounds ----------

    /** Loads every computer's rounds; an older nacred without rounds simply has none. */
    fun refreshRounds() = scope.launch {
        paired.value.values.map { p ->
            async { runCatching { p.client.rounds() }.onSuccess { list -> roundLists.update { it + (p.host.id to list.map { r -> r.tag(p.host) }) } } }
        }.awaitAll()
    }

    private fun upsertRound(round: Round) {
        val prev = roundLists.value[round.hostId]?.firstOrNull { it.id == round.id }
        roundLists.update { m -> m + (round.hostId to (listOf(round) + m[round.hostId].orEmpty().filterNot { it.id == round.id })) }
        _roundChanges.tryEmit(prev to round)
    }

    fun round(key: String): Round? = rounds.value.firstOrNull { it.key == key }

    /** Starts a round of [beads] in [repoKey]'s repository on that computer. */
    suspend fun startRound(repoKey: String, beads: List<String>, budgetUsd: Double? = null, specChangeId: String? = null, specRevision: String? = null): Round {
        val p = pairedFor(repoKey)
        return p.client.startRound(Keys.id(repoKey), StartRound(beads = beads, budgetUsd = budgetUsd, specChangeId = specChangeId, specRevision = specRevision)).tag(p.host).also(::upsertRound)
    }

    suspend fun roundDetail(key: String): BridgeClient.RoundDetail {
        val p = pairedFor(key)
        val d = p.client.round(Keys.id(key))
        return d.copy(round = d.round.tag(p.host)).also { upsertRound(it.round) }
    }

    fun roundStream(key: String, after: Long): Flow<RoundMessage> {
        val p = pairedFor(key)
        return p.client.roundStream(Keys.id(key), after).map {
            if (it is RoundMessage.Changed) RoundMessage.Changed(it.round.tag(p.host)).also { c -> upsertRound(c.round) } else it
        }
    }

    private suspend fun roundAction(key: String, f: suspend BridgeClient.(String) -> Round): Round {
        val p = pairedFor(key)
        return p.client.f(Keys.id(key)).tag(p.host).also(::upsertRound)
    }

    suspend fun answer(key: String, questionId: String, answer: String) = roundAction(key) { answer(it, questionId, answer) }
    suspend fun unlink(key: String, bead: String, from: String) = roundAction(key) { editPlan(it, unlink = listOf(bead to from)) }
    suspend fun acceptDependency(key: String, bead: String, on: String) = roundAction(key) { editPlan(it, accept = listOf(bead to on)) }
    suspend fun setRoundLimits(key: String, budgetUsd: Double, minutes: Int = 0) = roundAction(key) { setLimits(it, budgetUsd, minutes) }
    suspend fun land(key: String) = roundAction(key) { land(it) }
    suspend fun cancelRound(key: String) = roundAction(key) { cancelRound(it) }
    suspend fun resumeRound(key: String) = roundAction(key) { resumeRound(it) }
    suspend fun retryBead(key: String, bead: String) = roundAction(key) { retryBead(it, bead) }
    suspend fun chooseRound(key: String, choice: String, budgetUsd: Double = 0.0, minutes: Int = 0) =
        roundAction(key) { chooseRound(it, choice, budgetUsd, minutes) }
}

/**
 * Groups repos that share a git remote. Members are listed in pairing order of their
 * computers; the home is the member on the computer chosen in [homeHosts], else the first.
 */
fun groupRepos(repos: List<Repo>, hostOrder: List<String>, homeHosts: Map<String, String>): List<RepoGroup> {
    val order = hostOrder.withIndex().associate { (i, id) -> id to i }
    return repos.groupBy { it.groupId }.map { (id, all) ->
        // One checkout per computer: extra clones or worktrees on the same machine
        // would read as separate computers, so keep the shortest path (the main checkout).
        val members = all.groupBy { it.hostId }.values.map { same -> same.minBy { it.path.length } }
        val sorted = members.sortedBy { order[it.hostId] ?: Int.MAX_VALUE }
        val home = sorted.firstOrNull { it.hostId == homeHosts[id] } ?: sorted.first()
        RepoGroup(id, home, sorted)
    }
}

/** Phases in which a change's work runs on the computer. */
val WORKING_PHASES = setOf("drafting", "reviewing", "promoting")

fun Throwable.friendly(): String = when (this) {
    is BridgeException -> message.orEmpty()
    is java.net.ConnectException, is java.net.SocketTimeoutException, is java.net.UnknownHostException, is java.net.NoRouteToHostException ->
        "Can't reach that computer. Check Tailscale and that nacred is running."
    // "Software caused connection abort", "Connection reset" and friends: the link dropped mid-call.
    is java.net.SocketException, is java.io.EOFException, is okhttp3.internal.http2.StreamResetException ->
        "Lost the connection to that computer."
    else -> message ?: javaClass.simpleName
}
