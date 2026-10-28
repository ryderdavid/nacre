package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.DaemonUpdate
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class UpdateComputer(
    val id: String,
    val reportedId: String,
    val online: Boolean,
    val status: suspend () -> DaemonUpdate,
    val start: suspend () -> DaemonUpdate,
)

/** Jobs belong to the repository, so leaving Settings does not drop an update.
 * Server status survives app/process restarts; refresh reattaches without POST. */
class DaemonUpdates(
    private val scope: CoroutineScope,
    private val pollMillis: Long = 2_000,
    private val attempts: Int = 800,
) {
    private val mutable = MutableStateFlow<Map<String, DaemonUpdate>>(emptyMap())
    val states = mutable.asStateFlow()
    private val jobs = mutableMapOf<String, Job>()

    @Synchronized fun remove(id: String) { jobs.remove(id)?.cancel(); mutable.update { it - id } }
    fun refresh(computers: List<UpdateComputer>) = computers.forEach { begin(it, false) }
    fun startAll(computers: List<UpdateComputer>) = computers.forEach { begin(it, true) }

    private fun put(id: String, state: DaemonUpdate) { mutable.update { it + (id to state) } }
    @Synchronized private fun begin(c: UpdateComputer, start: Boolean) {
        if (jobs[c.id]?.isActive == true && mutable.value[c.id]?.active == true) return
        val previous = mutable.value[c.id]
        if (!c.online) {
            put(c.id, DaemonUpdate(phase = "offline", message = "Offline — reconnect this computer, then try again."))
            return
        }
        put(c.id, DaemonUpdate(phase = if (start) "checking" else "loading", message = if (start) "Requesting update…" else "Checking update status…", active = true))
        val job = scope.launch(start = CoroutineStart.LAZY) {
            var operation = ""
            var awaiting = start
            try {
                // Establish which operation existed before this request. A lost
                // POST must not be "confirmed" by yesterday's successful update.
                val before = c.status()
                if (before.hostId.isNotEmpty() && c.reportedId.isNotEmpty() && before.hostId != c.reportedId)
                    throw BridgeException("A different computer answered. Reconnect the correct computer before updating.")
                val requested = start && !before.active
                awaiting = requested
                var state = try {
                    if (requested) c.start() else before
                } catch (e: Exception) {
                    if (e is CancellationException) throw e
                    if (e is BridgeException && e.code != 0) throw e
                    // POST may have reached the daemon. Reconcile with GET only.
                    if (!requested) throw e
                    null
                }
                repeat(attempts) {
                    if (state != null) {
                        val received = state
                        if (received.hostId.isNotEmpty() && c.reportedId.isNotEmpty() && received.hostId != c.reportedId)
                            throw BridgeException("A different computer answered. Reconnect the correct computer before updating.")
                        if (operation.isNotEmpty() && received.id != operation)
                            throw BridgeException("The update operation changed. Refresh Settings to inspect its current result.")
                        if (received.id.isNotEmpty()) operation = received.id
                        if (received.phase == "updated" && !received.verifiedUpdate)
                            throw BridgeException("The running daemon does not match the update result. Check this computer.")
                        if (received.phase == "interrupted" && received.active)
                            throw BridgeException(received.message)
                        if (awaiting && (received.phase == "idle" || received.id.isEmpty() || received.id == before.id))
                            throw BridgeException("The update request was not confirmed. Try again.")
                        awaiting = false
                        if (!start && received.phase == "idle" && previous?.phase == "failed") {
                            put(c.id, previous)
                            return@launch
                        }
                        put(c.id, received)
                        if (!received.active) return@launch
                    } else {
                        put(c.id, (mutable.value[c.id] ?: DaemonUpdate()).copy(phase = "reconnecting", message = "Waiting for this computer to reconnect…", active = true))
                    }
                    delay(pollMillis)
                    state = try { c.status() } catch (e: Exception) {
                        if (e is CancellationException) throw e
                        if (e is BridgeException && e.code in listOf(401, 403, 404, 405, 501)) throw e
                        null
                    }
                }
                put(c.id, DaemonUpdate(phase = "unknown", message = "Update result is not confirmed. Reconnect or reopen Settings to check this computer."))
            } catch (e: Exception) {
                if (e is CancellationException) throw e
                val old = e is BridgeException && e.code in listOf(404, 405, 501)
                put(c.id, DaemonUpdate(phase = if (old) "unavailable" else "failed", message = if (old)
                    "Update nacred manually on this computer once to enable updates from Settings."
                else e.message ?: "Could not check this computer's update."))
            }
        }
        jobs[c.id] = job
        job.start()
    }
}
