package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.NewEffortPrefill

/**
 * Whether New effort is offered for a project: it has OpenSpec ([openspec]) and its computer, reporting
 * [health], drafts New effort chats and raises intake from a Draft card.
 */
fun offersNewEffort(openspec: Boolean, health: Health?): Boolean = openspec && draftsIntake(health)

/** The board a New effort chat drafts for, and what New effort was opened with (found work, a follow-on). */
data class NewEffortOrigin(val repoKey: String, val prefill: NewEffortPrefill? = null)

/** The composer text a prefilled New effort chat opens with; null when it wasn't prefilled. */
fun composerPrefill(prefill: NewEffortPrefill?): String? {
    if (prefill == null) return null
    val title = prefill.title.trim()
    val body = prefill.description.trim()
    val text = when {
        body.isEmpty() -> title
        title.isEmpty() || body.startsWith(title) -> body
        else -> "$title\n\n$body"
    }
    return text.ifBlank { null }
}
