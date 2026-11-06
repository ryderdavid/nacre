package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.model.Decision

/** Posts and clears "needs you" notifications for owner decisions. */
interface DecisionNotices {
    fun postDecision(d: Decision)
    fun clearDecision(d: Decision)
}

/**
 * Announces owner decisions as a chat permission request is announced: one notification per decision
 * that newly waits, posted only while the app is in the background, and cleared once the decision is
 * answered, here or elsewhere. Every change of every decision passes through [seen], whichever way it
 * arrived (`/v1/events`, a chat's log, or the answer Ryder just sent from this device).
 *
 * A decision is announced at most once. One already seen answered, deferred or otherwise settled is
 * never announced again, so a stale "pending" arriving after Ryder's own answer stays quiet. [announced]
 * is shared with the background check, so neither announces what the other already has.
 */
class DecisionAlerts(
    private val notices: DecisionNotices,
    private val inBackground: () -> Boolean,
    private val announced: AnnouncedDecisions = AnnouncedDecisions(),
) {
    private val posted = mutableSetOf<String>()
    private val settled = mutableSetOf<String>()

    @Synchronized
    fun seen(d: Decision) {
        val key = d.key
        if (!d.pending) {
            settled += key
            if (key in posted || announced.contains(key)) notices.clearDecision(d)
            return
        }
        if (key in settled || key in posted || announced.contains(key) || !inBackground()) return
        posted += key
        announced.add(key)
        notices.postDecision(d)
    }

    companion object {
        /** The notification's title and text: what is asked, then where (project · computer · revision) or its first consequence. */
        fun text(d: Decision): Pair<String, String> {
            val title = d.title.ifBlank { "A decision needs you" }
            val text = d.subtitle.ifBlank { d.consequences.firstOrNull().orEmpty() }.ifBlank { d.repoName }
            return title to text
        }
    }
}
