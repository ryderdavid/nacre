package com.ryderdavid.nacre.ui.runs

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.bridge.StreamMessage
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.RunEvent
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** A transcript entry: most events stand alone; a tool call carries its result. */
sealed interface Entry {
    val key: String

    data class Single(val event: RunEvent) : Entry {
        override val key get() = "e${event.seq}"
    }

    data class Tool(val use: RunEvent, val result: RunEvent?) : Entry {
        override val key get() = "t${use.seq}"
    }
}

fun buildTranscript(events: List<RunEvent>): List<Entry> {
    val results = events.filter { it.kind == "tool_result" }.associateBy { it.toolId }
    return events.mapNotNull { e ->
        when (e.kind) {
            "tool_use" -> Entry.Tool(e, results[e.toolId])
            "tool_result" -> if (events.any { it.kind == "tool_use" && it.toolId == e.toolId }) null else Entry.Single(e)
            "status" -> null
            else -> Entry.Single(e)
        }
    }
}

class RunViewModel(private val repo: NacreRepository, val runKey: String) : ViewModel() {
    val run = MutableStateFlow<Run?>(repo.runs.value.firstOrNull { it.key == runKey })
    private val events = MutableStateFlow<List<RunEvent>>(emptyList())
    val transcript: StateFlow<List<Entry>> = events.map(::buildTranscript).stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())
    val error = MutableStateFlow<String?>(null)
    val live = MutableStateFlow(false)
    val sending = MutableStateFlow(false)

    init {
        viewModelScope.launch { follow() }
    }

    private suspend fun follow() {
        try {
            val d = repo.runDetail(runKey)
            run.value = d.run
            events.value = d.events
            repo.noteRun(d.run)
        } catch (e: Exception) {
            error.value = e.friendly()
        }
        var backoff = 1_000L
        while (viewModelScope.isActive) {
            try {
                val after = events.value.lastOrNull()?.seq ?: 0
                repo.runStream(runKey, after).collect { msg ->
                    when (msg) {
                        StreamMessage.Open -> { live.value = true; error.value = null; backoff = 1_000L }
                        is StreamMessage.Event -> if (events.value.none { it.seq == msg.event.seq }) events.value = events.value + msg.event
                        is StreamMessage.RunChanged -> { run.value = msg.run; repo.noteRun(msg.run) }
                        is StreamMessage.RepoChanged, is StreamMessage.RoundChanged, is StreamMessage.ChangeChanged -> Unit
                    }
                }
            } catch (e: Exception) {
                if (run.value?.isActive == true) error.value = e.friendly()
            }
            live.value = false
            delay(backoff)
            backoff = (backoff * 2).coerceAtMost(15_000L)
        }
    }

    fun cancel() = viewModelScope.launch {
        runCatching { repo.cancelRun(runKey) }.onSuccess { run.value = it }.onFailure { repo.notify("Couldn't cancel: ${it.friendly()}") }
    }

    fun followUp(message: String, mode: String?) = viewModelScope.launch {
        sending.value = true
        runCatching { repo.followUp(runKey, message, mode) }
            .onSuccess { run.value = it }
            .onFailure { repo.notify("Couldn't send: ${it.friendly()}") }
        sending.value = false
    }

    fun sendLive(message: String) = viewModelScope.launch {
        sending.value = true
        runCatching { repo.sendLive(runKey, message) }
            .onSuccess { run.value = it }
            .onFailure { repo.notify("Couldn't send to the live session: ${it.friendly()}") }
        sending.value = false
    }
}

class RunsViewModel(private val repo: NacreRepository) : ViewModel() {
    val runs: StateFlow<List<Run>> = repo.runs.map { list -> list.filter { it.inRunsList } }
        .stateIn(viewModelScope, SharingStarted.Eagerly, repo.runs.value.filter { it.inRunsList })
    val rounds = repo.rounds
    val hosts = repo.hosts
    val refreshing = MutableStateFlow(false)

    fun refresh() = viewModelScope.launch {
        refreshing.value = true
        repo.refreshRounds()
        repo.refreshRuns().join()
        refreshing.value = false
    }
}
