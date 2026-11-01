package com.ryderdavid.nacre.ui.bead

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.Loadable
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Survives closing the dialog. Work itself belongs to nacred, never to this screen. */
class ChangeViewModel(
    private val repository: NacreRepository,
    private val settings: SettingsStore,
    val repoKey: String,
    private val beadId: String,
) : ViewModel() {
    val state = MutableStateFlow(Loadable<ChangeEnvelope>())
    val sending = MutableStateFlow(false)
    val feedback = MutableStateFlow("")
    val draftReady = MutableStateFlow(false)
    private val requests = Mutex()
    private val draftWrites = Mutex()
    private var draftKey = "$repoKey:bead:$beadId"
    private var actionFailure: String? = null

    suspend fun refresh(clearActionError: Boolean = false) = requests.withLock {
        if (clearActionError) actionFailure = null
        state.value = state.value.copy(loading = true)
        try {
            val envelope = repository.change(repoKey, beadId)
            loadDraft(envelope)
            // A background poll must not make a rejected action disappear before it is read.
            state.value = Loadable(envelope, error = actionFailure)
        } catch (e: CancellationException) {
            state.value = state.value.copy(loading = false)
            throw e
        } catch (e: Exception) {
            // Keep the last artifact snapshot readable during a connection interruption.
            state.value = state.value.copy(loading = false, error = e.friendly())
        }
    }

    private suspend fun loadDraft(envelope: ChangeEnvelope) = draftWrites.withLock {
        val key = envelope.change?.let { "$repoKey:change:${it.id}" } ?: draftKey
        if (!draftReady.value || key != draftKey) {
            val saved = settings.changeFeedback(key)
            if (feedback.value.isNotEmpty() && key != draftKey) {
                settings.setChangeFeedback(key, feedback.value)
                settings.setChangeFeedback(draftKey, "")
            } else feedback.value = saved
            draftKey = key
            draftReady.value = true
        }
    }

    fun setFeedback(value: String) {
        feedback.value = value
        viewModelScope.launch { draftWrites.withLock { settings.setChangeFeedback(draftKey, value) } }
    }

    /** Takes over a change written outside Nacre, reviewed as it stands. */
    fun adopt(changeName: String, recordedReview: Boolean) =
        send("adopt", ChangeAction("adopt", changeName = changeName, recordedReview = recordedReview))

    fun act(action: String) = send(action, null)

    private fun send(action: String, request: ChangeAction?) {
        if (sending.value) return
        sending.value = true
        val revision = state.value.data?.change?.revision.orEmpty()
        val submitted = feedback.value
        viewModelScope.launch {
            requests.withLock {
                try {
                    val envelope = repository.changeAction(repoKey, beadId, request ?: ChangeAction(action, revision, submitted.trim()))
                    loadDraft(envelope)
                    actionFailure = null
                    state.value = Loadable(envelope)
                    // Typing while the request travels must not erase a newer draft.
                    if (envelope.available && envelope.change != null && envelope.change.error.isBlank() && action in setOf("develop", "revise") && feedback.value == submitted) setFeedback("")
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    actionFailure = e.friendly()
                    state.value = state.value.copy(error = actionFailure)
                } finally {
                    sending.value = false
                }
            }
        }
    }
}
