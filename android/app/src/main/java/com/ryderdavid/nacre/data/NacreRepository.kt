package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.board.changeEventNeedsRefresh
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.MoveAction
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.bridge.HostEvent
import com.ryderdavid.nacre.data.bridge.RoundMessage
import com.ryderdavid.nacre.data.bridge.StreamMessage
import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.EpicFinish
import com.ryderdavid.nacre.data.model.AttentionOption
import com.ryderdavid.nacre.data.model.ChangeEvent
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.FoundRequest
import com.ryderdavid.nacre.data.model.FoundResult
import com.ryderdavid.nacre.data.model.IntakeDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.RepoConfigDraft
import com.ryderdavid.nacre.data.model.SavedRepoConfig
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
import kotlinx.coroutines.flow.onEach
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

/** How often a running card check is asked how it's going. */
private const val CARD_CHECK_POLL_MS = 3_000L

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
) : com.ryderdavid.nacre.data.chat.ChatLink {
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
    /** Paired computers whose nacred predates Needs you, so its newer features stay empty there. */
    private val outdatedHosts = MutableStateFlow<Map<String, String>>(emptyMap())
    val outdatedComputers: StateFlow<List<String>> = outdatedHosts.map { it.values.sorted() }.stateIn(scope, SharingStarted.Eagerly, emptyList())
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
        outdatedHosts.update { it - hostId }
        connections.update { it - hostId }
        boards.keys.filter { Keys.host(it) == hostId }.forEach { boards.remove(it) }
    }

    private fun pairedFor(key: String): Paired =
        paired.value[Keys.host(key)] ?: throw BridgeException(if (paired.value.isEmpty()) "Pair with a computer in Settings first" else "That computer is no longer paired")

    private fun clientFor(key: String) = pairedFor(key).client

    fun hostName(hostId: String) = paired.value[hostId]?.host?.name.orEmpty()

    suspend fun claudeSignIn(hostId: String) = pairedFor(hostId).client.claudeSignIn()
    suspend fun startClaudeSignIn(hostId: String) = pairedFor(hostId).client.startClaudeSignIn().url
    suspend fun finishClaudeSignIn(hostId: String, code: String) = pairedFor(hostId).client.finishClaudeSignIn(code)

    /** The paired computer an error about [hostName] means: the only one, or the one by that name. */
    fun hostCalled(hostName: String): String? = paired.value.values.let { all ->
        all.singleOrNull()?.host?.id
            ?: all.firstOrNull { p -> p.host.name.equals(hostName, true) || (connections.value[p.host.id] as? Connection.Online)?.health?.hostName.equals(hostName, true) }?.host?.id
    }

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
                seen.update { it + (id to System.currentTimeMillis()) }
                if (com.ryderdavid.nacre.data.chat.servesChat(health)) scope.launch {
                    runCatching { p.client.chats() }.onSuccess { list -> chatLists.update { it + (id to list) } }
                }
                if (health.hostId.isNotEmpty() && health.hostId != p.host.hostId) settings.setReportedHostId(id, health.hostId)
                backoff = 1_000L
                p.client.events().collect { ev ->
                    val msg = when (ev) {
                        is HostEvent.DecisionChanged -> { learnDecision(ev.decision.copy(hostId = id)); return@collect }
                        is HostEvent.Stream -> ev.message
                    }
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
                            Keys.of(id, msg.event.repoId).let { key ->
                                // A change the summary doesn't list yet is fetched, never slotted in by guess.
                                val changes = boards[key]?.value?.data?.changes
                                if (boards.containsKey(key) && changeEventNeedsRefresh(changes, msg.event.changeId, msg.event.phase)) refreshBoard(key, quiet = true)
                            }
                            attentionSoon(p)
                        }
                        else -> Unit
                    }
                }
            } catch (e: Exception) {
                if (e is kotlinx.coroutines.CancellationException) throw e
                if (connections.value[id] is Connection.Online) seen.update { it + (id to System.currentTimeMillis()) }
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

    /** An untracked change of the checkout, by its folder name. */
    suspend fun untrackedChange(key: String, name: String): ChangeEnvelope = clientFor(key).untrackedChange(Keys.id(key), name)

    suspend fun changeAction(key: String, id: String, action: ChangeAction): ChangeEnvelope =
        clientFor(key).changeAction(Keys.id(key), id, action).also { env ->
            env.change?.takeIf { it.isWorking }?.let { c -> working.update { it + Keys.of(Keys.host(key), c.id) } }
            refreshBoard(key, quiet = true); refreshAttention(Keys.host(key))
        }

    /** Dismisses a change's failed draft, then refreshes the board. A failure throws and changes nothing locally. */
    suspend fun dismissDraft(key: String, changeId: String): ChangeSummary =
        clientFor(key).dismissDraft(Keys.id(key), changeId).also { refreshBoard(key, quiet = true); refreshAttention(Keys.host(key)) }

    /** The board's changes as last loaded; null when its computer doesn't report them. */
    fun changes(key: String): ChangesList? = boards[key]?.value?.data?.changes

    /** Intake's verdict on a New effort draft (one bounded call on [key]'s computer). */
    suspend fun intake(key: String, draft: IntakeDraft): IntakeVerdict = clientFor(key).intake(Keys.id(key), draft)

    /** Has [key]'s computer draft a nacre.yaml for it (one bounded Claude call). */
    suspend fun draftRepoConfig(key: String): RepoConfigDraft = clientFor(key).draftRepoConfig(Keys.id(key))

    /** Saves and commits an approved nacre.yaml, then reloads the repository so its gates show. */
    suspend fun saveRepoConfig(key: String, yaml: String): SavedRepoConfig =
        clientFor(key).saveRepoConfig(Keys.id(key), yaml).also { refreshRepos(); refreshBoard(key, quiet = true) }

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
                outdatedHosts.update { it + (p.host.id to p.host.name) }
            } else {
                attentionHosts.update { it + p.host.id }
                outdatedHosts.update { it - p.host.id }
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
    // ---- Chat first: projects, work and decisions ----

    private val seen = MutableStateFlow<Map<String, Long>>(emptyMap())

    /** When each computer was last heard from (epoch millis), for "Last checked 12 min ago". */
    val lastSeen: StateFlow<Map<String, Long>> = seen

    private val projectLists = MutableStateFlow<Map<String, List<com.ryderdavid.nacre.data.model.ProjectSummary>?>>(emptyMap())

    /** Each computer's projects by host id; null for a computer whose nacred predates `/projects`. */
    val projectsByHost: StateFlow<Map<String, List<com.ryderdavid.nacre.data.model.ProjectSummary>?>> = projectLists

    fun refreshProjects() = scope.launch {
        paired.value.values.forEach { h ->
            launch {
                val list = runCatching { h.client.projects() }.getOrElse { e -> if (e is kotlinx.coroutines.CancellationException) throw e; return@launch }
                projectLists.update { it + (h.host.id to list) }
                // The computer answers but its projects were never read (the first read came while nacred
                // was starting, or was lost): read them again rather than show "0 projects" for good.
                if (repoLists.value[h.host.id] == null) refreshRepos()
            }
        }
    }

    private val workLists = MutableStateFlow<Map<String, List<com.ryderdavid.nacre.data.model.WorkSummary>>>(emptyMap())

    /** Open work by repository key, as nacred reports it, or from its rounds on an older nacred. */
    val work: StateFlow<Map<String, List<com.ryderdavid.nacre.data.model.WorkSummary>>> = combine(workLists, rounds) { reported, rs ->
        val fromRounds = rs.groupBy { Keys.of(it.hostId, it.repoId) }.mapValues { (_, list) -> list.map(ChatFirstData::work) }
        fromRounds + reported
    }.stateIn(scope, SharingStarted.Eagerly, emptyMap())

    fun refreshWork(key: String) = scope.launch {
        val list = runCatching { clientFor(key).work(Keys.id(key)) }.getOrElse { e -> if (e is kotlinx.coroutines.CancellationException) throw e; return@launch } ?: return@launch
        workLists.update { it + (key to list.map { w -> w }) }
    }

    private val decisionLists = MutableStateFlow<Map<String, List<com.ryderdavid.nacre.data.model.Decision>?>>(emptyMap())

    /**
     * Open decisions by repository key. A computer without decisions shows its attention items as
     * decisions instead, answered by performing the item's own action.
     */
    val decisions: StateFlow<Map<String, List<com.ryderdavid.nacre.data.model.Decision>>> = combine(decisionLists, attention) { reported, items ->
        val legacy = items.groupBy { it.repoKey }
        val keys = reported.keys + legacy.keys
        keys.associateWith { k -> reported[k] ?: legacy[k].orEmpty().map(ChatFirstData::decision) }
    }.stateIn(scope, SharingStarted.Eagerly, emptyMap())

    fun refreshDecisions(key: String) = scope.launch {
        val host = Keys.host(key)
        val list = runCatching { clientFor(key).decisions(Keys.id(key)) }.getOrElse { e -> if (e is kotlinx.coroutines.CancellationException) throw e; return@launch }
        decisionLists.update { it + (key to list?.map { d -> d.copy(hostId = host) }) }
    }

    private val decisionReads = ConcurrentHashMap<String, Job>()
    private val _decisionChanges = MutableSharedFlow<com.ryderdavid.nacre.data.model.Decision>(extraBufferCapacity = 32)

    /** Every decision as it is created or changes, from any source, tagged with its computer (for notifications). */
    val decisionChanges: SharedFlow<com.ryderdavid.nacre.data.model.Decision> = _decisionChanges

    /**
     * Records a decision that changed (from `/v1/events`, a chat's log or an answer) without waiting for a
     * refresh, keyed by its computer and repository. A project whose decisions were never read is read
     * whole instead, so its queue never shows just the one that happened to change.
     */
    override fun learnDecision(d: com.ryderdavid.nacre.data.model.Decision) {
        if (d.hostId.isBlank() || d.repo.isBlank()) return
        val key = Keys.of(d.hostId, d.repo)
        var known = true
        decisionLists.update { m ->
            val list = m[key]
            if (list == null) { known = false; return@update m }
            m + (key to upsertDecision(list, d))
        }
        if (!known && decisionReads[key]?.isActive != true) decisionReads[key] = refreshDecisions(key)
        _decisionChanges.tryEmit(d)
    }

    /** Where the queue should open: at a decision a notification or the Spec panel pointed at. */
    data class QueueFocus(val repoKey: String, val decisionId: String?, val open: Boolean, val at: Long = System.currentTimeMillis())

    private val queueFocusState = MutableStateFlow<QueueFocus?>(null)
    val queueFocus: StateFlow<QueueFocus?> = queueFocusState

    /**
     * Points [repoKey]'s queue at [decisionId] (shown first). With [open], the workspace's strip opens the
     * queue when it next shows that project, as a notification asks.
     */
    fun focusQueue(repoKey: String, decisionId: String?, open: Boolean = false) {
        queueFocusState.value = QueueFocus(repoKey, decisionId, open)
    }

    /** The queue was opened as asked; it keeps its focus until it closes. */
    fun queueOpened() = queueFocusState.update { it?.copy(open = false) }

    /** The queue for [repoKey] closed: its focus is spent. */
    fun clearQueueFocus(repoKey: String) = queueFocusState.update { if (it?.repoKey == repoKey) null else it }

    private val specLists = MutableStateFlow<Map<String, List<com.ryderdavid.nacre.data.model.CurrentSpec>?>>(emptyMap())

    /**
     * Each project's current specs (openspec/specs) by repository key: absent until read, null from a
     * nacred without the inventory, where the Spec panel lists only the specs open changes touch.
     */
    val currentSpecs: StateFlow<Map<String, List<com.ryderdavid.nacre.data.model.CurrentSpec>?>> = specLists

    fun refreshSpecs(key: String) = scope.launch {
        val list = runCatching { clientFor(key).specs(Keys.id(key)) }.getOrElse { e -> if (e is kotlinx.coroutines.CancellationException) throw e; return@launch }
        specLists.update { it + (key to list) }
    }

    /**
     * Answers a decision. An attention-derived decision from an older nacred performs the item's own
     * action; otherwise nacred revalidates and executes, and the decision's new state is returned.
     */
    suspend fun answerDecision(d: com.ryderdavid.nacre.data.model.Decision, answer: com.ryderdavid.nacre.data.model.DecisionAnswer): com.ryderdavid.nacre.data.model.Decision {
        if (d.id.startsWith("att_")) {
            val item = attention.value.firstOrNull { "att_" + it.id == d.id && it.hostId == d.hostId } ?: throw BridgeException("That item is no longer waiting")
            val actions = item.options.filter { it.type == "action" && it.disabled.isBlank() }
            val option = answer.option?.toIntOrNull()?.let { actions.getOrNull(it) } ?: throw BridgeException("Pick an answer")
            perform(item, option)
            return d.copy(state = "done")
        }
        val sent = clientFor(Keys.of(d.hostId, d.repo)).answerDecision(d.id, answer).copy(hostId = d.hostId)
        // Got it leaves the queue even from a nacred that doesn't echo the flag.
        val out = if (answer.acknowledge == true) sent.copy(acknowledged = true) else sent
        learnDecision(out)
        return out
    }

    /** [list] with [d] in place of the decision with its id, or added. */
    private fun upsertDecision(list: List<com.ryderdavid.nacre.data.model.Decision>, d: com.ryderdavid.nacre.data.model.Decision) =
        if (list.any { it.id == d.id }) list.map { if (it.id == d.id) d else it } else list + d

    /** Cancels a round from a work card, reporting the outcome as a notice. */
    fun cancelRoundQuietly(roundKey: String) = scope.launch {
        runCatching { cancelRound(roundKey) }.onSuccess { notify("Round cancelled; its branches are kept") }.onFailure { notify("Couldn't cancel the round: ${it.message}") }
    }

    suspend fun renewDecision(d: com.ryderdavid.nacre.data.model.Decision): com.ryderdavid.nacre.data.model.Decision {
        val out = clientFor(Keys.of(d.hostId, d.repo)).renewDecision(d.id).copy(hostId = d.hostId)
        learnDecision(out)
        return out
    }

    /** Puts back a card the card check closed; it waits on Ryder again and checks leave it alone. */
    suspend fun reopenDecision(d: com.ryderdavid.nacre.data.model.Decision): com.ryderdavid.nacre.data.model.Decision {
        val out = clientFor(Keys.of(d.hostId, d.repo)).reopenDecision(d.id).copy(hostId = d.hostId)
        learnDecision(out)
        return out
    }

    private val _cardChecks = MutableStateFlow<Map<String, com.ryderdavid.nacre.data.model.CardCheck>>(emptyMap())

    /** Each project's running or last card check, by project key, as the queue and Projects show it. */
    val cardChecks: StateFlow<Map<String, com.ryderdavid.nacre.data.model.CardCheck>> = _cardChecks
    private val cardCheckJobs = ConcurrentHashMap<String, Job>()

    /**
     * Starts the project's card check (nacred looks at its open cards for work already handled
     * elsewhere) and follows it to the end, then re-reads the project's decisions and the projects'
     * counts. A check this app is already following isn't started twice; a nacred without card checks
     * leaves nothing behind.
     */
    fun checkCards(key: String) {
        if (cardCheckJobs[key]?.isActive == true) return
        cardCheckJobs[key] = scope.launch {
            val client = runCatching { clientFor(key) }.getOrNull() ?: return@launch
            // Shown as checking at once: nacred lists the queue before it answers, which takes a few seconds.
            val before = _cardChecks.value[key]
            _cardChecks.update { it + (key to com.ryderdavid.nacre.data.model.CardCheck(repo = Keys.id(key), running = true)) }
            var c = runCatching { client.startCardCheck(Keys.id(key)) }.getOrNull() ?: run {
                _cardChecks.update { if (before == null) it - key else it + (key to before) }
                return@launch
            }
            _cardChecks.update { it + (key to c) }
            val ran = c.running
            while (c.running) {
                delay(CARD_CHECK_POLL_MS)
                c = runCatching { client.cardCheck(Keys.id(key)) }.getOrNull() ?: break
                _cardChecks.update { it + (key to c) }
            }
            if (ran) { refreshDecisions(key); refreshProjects() }
        }
    }

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
    suspend fun signOff(key: String, id: String, approve: Boolean, note: String): Issue =
        clientFor(key).signOff(Keys.id(key), id, approve, note).also { patchLocal(key, it) }
    suspend fun reopen(key: String, id: String): Issue = clientFor(key).reopen(Keys.id(key), id).also { patchLocal(key, it) }
    suspend fun detail(key: String, id: String) = clientFor(key).detail(Keys.id(key), id).also { patchLocal(key, it.issue) }
    suspend fun comment(key: String, id: String, text: String, fromClaude: Boolean = false) = clientFor(key).comment(Keys.id(key), id, text, fromClaude)
    suspend fun addDep(key: String, issue: String, dependsOn: String, type: String = "blocks") {
        clientFor(key).addDep(Keys.id(key), issue, dependsOn, type); refreshBoard(key, quiet = true)
    }
    suspend fun removeDep(key: String, issue: String, dependsOn: String) {
        clientFor(key).removeDep(Keys.id(key), issue, dependsOn); refreshBoard(key, quiet = true)
    }

    // ---------- chats ----------

    private val chatLists = MutableStateFlow<Map<String, List<com.ryderdavid.nacre.data.model.ChatRecord>>>(emptyMap())

    /** Every chat on every paired computer that serves Chat, newest first, with notices for those that don't. */
    val chats: StateFlow<com.ryderdavid.nacre.data.chat.ChatListing> = combine(hosts, chatLists) { h, l ->
        com.ryderdavid.nacre.data.chat.mergeChatLists(h, l)
    }.stateIn(scope, SharingStarted.Eagerly, com.ryderdavid.nacre.data.chat.ChatListing())

    /** The health computer [hostId] reported; null while it isn't connected. */
    fun health(hostId: String): Health? = (connections.value[hostId] as? Connection.Online)?.health

    override suspend fun refreshChats(): com.ryderdavid.nacre.data.chat.ChatListing {
        coroutineScope {
            paired.value.values.filter { com.ryderdavid.nacre.data.chat.servesChat(health(it.host.id)) }.map { p ->
                async {
                    runCatching { p.client.chats() }.onSuccess { list -> chatLists.update { it + (p.host.id to list) } }
                }
            }.awaitAll()
        }
        return com.ryderdavid.nacre.data.chat.mergeChatLists(hosts.value, chatLists.value)
    }

    /** Puts a chat record nacred just answered with into the list. */
    fun noteChat(hostId: String, chat: com.ryderdavid.nacre.data.model.ChatRecord) {
        chatLists.update { m ->
            // A single chat's record carries no work links: those come only with the list, so keep the listed ones.
            val old = m[hostId].orEmpty().firstOrNull { it.id == chat.id }
            val merged = if (chat.work.isEmpty() && old != null) chat.copy(work = old.work) else chat
            m + (hostId to (listOf(merged) + m[hostId].orEmpty().filterNot { it.id == chat.id }))
        }
    }

    private fun forgetChat(key: String) {
        chatLists.update { m -> m + (Keys.host(key) to m[Keys.host(key)].orEmpty().filterNot { it.id == Keys.id(key) }) }
    }

    override suspend fun createChat(hostId: String, body: com.ryderdavid.nacre.data.model.CreateChat): com.ryderdavid.nacre.data.model.ChatRecord =
        pairedFor(hostId).client.createChat(body).also { noteChat(hostId, it) }.copy(hostId = hostId, hostName = hostName(hostId))

    override suspend fun chatDetail(key: String): BridgeClient.ChatDetail =
        clientFor(key).chat(Keys.id(key)).also { noteChat(Keys.host(key), it.chat) }

    override fun chatStream(key: String, after: Long): Flow<com.ryderdavid.nacre.data.bridge.ChatStreamMessage> =
        clientFor(key).chatStream(Keys.id(key), after).onEach { msg ->
            // Keeps the list, and the Chat tab's badge, in step with what the stream says.
            when (msg) {
                is com.ryderdavid.nacre.data.bridge.ChatStreamMessage.State -> noteChat(Keys.host(key), msg.chat)
                is com.ryderdavid.nacre.data.bridge.ChatStreamMessage.Event -> when (msg.event.kind) {
                    com.ryderdavid.nacre.data.model.ChatWire.PERMISSION -> patchChat(key) { it.copy(state = com.ryderdavid.nacre.data.model.ChatWire.WAITING_PERMISSION) }
                    com.ryderdavid.nacre.data.model.ChatWire.PERMISSION_ANSWER ->
                        patchChat(key) { if (it.waiting) it.copy(state = com.ryderdavid.nacre.data.model.ChatWire.RUNNING) else it }
                    else -> Unit
                }
                else -> Unit
            }
        }

    private fun patchChat(key: String, change: (com.ryderdavid.nacre.data.model.ChatRecord) -> com.ryderdavid.nacre.data.model.ChatRecord) {
        chatLists.update { m ->
            val list = m[Keys.host(key)] ?: return@update m
            m + (Keys.host(key) to list.map { if (it.id == Keys.id(key)) change(it) else it })
        }
    }

    override suspend fun sendChat(key: String, body: com.ryderdavid.nacre.data.model.SendChatMessage): com.ryderdavid.nacre.data.model.ChatSendResult =
        clientFor(key).sendChat(Keys.id(key), body).also { r -> r.chat?.let { noteChat(Keys.host(key), it) } }

    override suspend fun cancelChat(key: String) = clientFor(key).cancelChat(Keys.id(key))

    override suspend fun chatImage(key: String, image: String): ByteArray = clientFor(key).chatImage(Keys.id(key), image)
    override suspend fun chatMockup(key: String, name: String): ByteArray = clientFor(key).chatMockup(Keys.id(key), name)

    override suspend fun raiseDraftIntake(key: String, seq: Long): com.ryderdavid.nacre.data.chat.DraftIntakeResult = try {
        val d = clientFor(key).raiseDraftIntake(Keys.id(key), seq).copy(hostId = Keys.host(key))
        learnDecision(d)
        com.ryderdavid.nacre.data.chat.DraftIntakeResult.Raised(d)
    } catch (e: kotlinx.coroutines.CancellationException) {
        throw e
    } catch (e: com.ryderdavid.nacre.data.bridge.BridgeException) {
        if (e.code == 410) com.ryderdavid.nacre.data.chat.DraftIntakeResult.Gone
        else com.ryderdavid.nacre.data.chat.DraftIntakeResult.Failed(e.friendly())
    } catch (e: Exception) {
        com.ryderdavid.nacre.data.chat.DraftIntakeResult.Failed(e.friendly())
    }

    override suspend fun deleteChat(key: String) {
        clientFor(key).deleteChat(Keys.id(key))
        forgetChat(key)
    }

    override suspend fun answerPermission(key: String, request: String, allow: Boolean, note: String?): com.ryderdavid.nacre.data.model.PermissionOutcome =
        clientFor(key).answerPermission(Keys.id(key), request, com.ryderdavid.nacre.data.model.PermissionAnswer(allow, note?.takeIf { it.isNotBlank() }))
            .also { o -> o.chat?.let { noteChat(Keys.host(key), it) } }

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
    suspend fun startRun(repoKey: String, issueId: String, req: RunRequest): Run {
        val p = pairedFor(repoKey)
        val run = try {
            p.client.startRun(Keys.id(repoKey), issueId, req.copy(goal = null))
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
    suspend fun startRound(repoKey: String, beads: List<String>, budgetUsd: Double? = null, specChangeId: String? = null, specRevision: String? = null, landWhenReady: Boolean? = null, epic: String? = null, finish: Boolean = false): Round {
        val p = pairedFor(repoKey)
        val round = p.client.startRound(Keys.id(repoKey), StartRound(beads = beads, epic = epic, budgetUsd = budgetUsd, specChangeId = specChangeId, specRevision = specRevision, landWhenReady = landWhenReady, finish = finish)).tag(p.host)
        return round.also(::upsertRound)
    }

    /** Merges a finished epic's branch into the main branch, or closes it when there's nothing to merge. */
    suspend fun finishEpic(repoKey: String, epic: String): EpicFinish {
        val p = pairedFor(repoKey)
        val result = p.client.finishEpic(Keys.id(repoKey), epic)
        refreshBoard(repoKey, quiet = true); refreshAttention(Keys.host(repoKey))
        return result.copy(round = result.round?.tag(p.host)?.also(::upsertRound))
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
    suspend fun roundChanges(key: String) = pairedFor(key).client.roundChanges(Keys.id(key))
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
