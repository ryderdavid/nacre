package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.friendly
import kotlinx.coroutines.CancellationException

/**
 * Dismiss and Resume on a failed draft placeholder. Both go to the computer and then the board is
 * refreshed from what it reports; nothing is changed locally first, so a failure leaves the error
 * placeholder exactly where it was and [notify] shows why (a snackbar on the board).
 *
 * - [dismiss] calls `POST …/changes/{id}/draft/dismiss` (idempotent, so retrying is safe).
 * - [resume] sends the change's existing `resume` action through the placeholder's seed. A refusal
 *   shows nacred's message and the placeholder stays failed.
 */
class DraftActions(
    private val dismiss: suspend (changeId: String) -> Unit,
    private val resume: suspend (beadId: String) -> Unit,
    private val notify: (String) -> Unit,
) {
    /** True when nacred recorded the dismissal. */
    suspend fun dismiss(item: LaneItem.DraftPlaceholder): Boolean {
        if (!item.dismissable) return false
        return attempt("Couldn't dismiss the failed draft") { dismiss(item.changeId) }
    }

    /** True when nacred accepted the resume. */
    suspend fun resume(item: LaneItem.DraftPlaceholder): Boolean {
        if (!item.resumable || item.resumeBead.isBlank()) return false
        return attempt("Couldn't resume drafting") { resume(item.resumeBead) }
    }

    private suspend fun attempt(what: String, call: suspend () -> Unit): Boolean = try {
        call()
        true
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        notify(e.friendly().takeIf { it.isNotBlank() }?.let { "$what: $it" } ?: "$what.")
        false
    }
}
