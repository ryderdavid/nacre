package com.ryderdavid.nacre.ui.rounds

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.bridge.RoundMessage
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundEvent
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** Follows one round live and carries out what Ryder decides on it. */
class RoundViewModel(private val repo: NacreRepository, val roundKey: String) : ViewModel() {
    val round = MutableStateFlow(repo.round(roundKey))
    val events = MutableStateFlow<List<RoundEvent>>(emptyList())
    val error = MutableStateFlow<String?>(null)
    val busy = MutableStateFlow(false)

    init {
        viewModelScope.launch { follow() }
    }

    private suspend fun follow() {
        try {
            val d = repo.roundDetail(roundKey)
            round.value = d.round
            events.value = d.events
        } catch (e: Exception) {
            error.value = e.friendly()
        }
        var backoff = 1_000L
        while (viewModelScope.isActive) {
            try {
                val after = events.value.lastOrNull()?.seq ?: 0
                repo.roundStream(roundKey, after).collect { msg ->
                    backoff = 1_000L
                    error.value = null
                    when (msg) {
                        is RoundMessage.Event -> if (events.value.none { it.seq == msg.event.seq }) events.value = events.value + msg.event
                        is RoundMessage.Changed -> round.value = msg.round
                    }
                }
            } catch (e: Exception) {
                if (round.value?.isActive == true) error.value = e.friendly()
            }
            if (round.value?.isActive == false) return
            delay(backoff)
            backoff = (backoff * 2).coerceAtMost(30_000L)
        }
    }

    private fun act(done: String? = null, block: suspend () -> Round) = viewModelScope.launch {
        busy.value = true
        try {
            round.value = block()
            done?.let(repo::notify)
        } catch (e: Exception) {
            repo.notify(e.friendly())
        } finally {
            busy.value = false
        }
    }

    fun answer(questionId: String, answer: String) = act { repo.answer(roundKey, questionId, answer) }
    fun unlink(bead: String, from: String) = act("$bead no longer waits for $from") { repo.unlink(roundKey, bead, from) }
    fun acceptDependency(bead: String, on: String) = act("$bead now depends on $on in bd") { repo.acceptDependency(roundKey, bead, on) }
    fun land() = act("Landing…") { repo.land(roundKey) }
    fun cancel() = act("Round cancelled; its branches are kept") { repo.cancelRound(roundKey) }
    fun resume() = act { repo.resumeRound(roundKey) }
    fun retry(bead: String) = act("$bead is back in the round") { repo.retryBead(roundKey, bead) }
    fun setBudget(usd: Double) = act { repo.setRoundLimits(roundKey, usd) }

    /** Takes one of a stopped round's choices; [usd] is the budget a choice asks for. */
    fun choose(choice: String, usd: Double = 0.0) = act { repo.chooseRound(roundKey, choice, usd) }
}
