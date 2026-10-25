package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.claude.DraftTurn

/** Everything typed or drafted into the new-bead form, so a dismissed form can come back as it was. */
data class NewBeadForm(
    val title: String = "",
    val description: String = "",
    val acceptance: String = "",
    val design: String = "",
    val type: String = "task",
    val priority: Int = 2,
    val labels: List<String> = emptyList(),
    val parent: String? = null,
    val questions: List<String> = emptyList(),
    val chat: List<DraftTurn> = emptyList(),
    val message: String = "",
    val drafted: Boolean = false,
) {
    /** True when there is something worth keeping: text typed in, or a draft from Claude. */
    val hasContent: Boolean
        get() = listOf(title, description, acceptance, design, message).any { it.isNotBlank() } || drafted || chat.isNotEmpty()
}

/**
 * The new-bead form left behind on each board when it was dismissed (NAC-cjm). Tapping outside,
 * the back gesture and dragging the handle down all close the form and keep what was in it; it
 * comes back on the next open until the bead is created or the draft is discarded. Held in memory
 * only, so it lasts for the process and no longer.
 */
class NewBeadDrafts {
    private val drafts = mutableMapOf<String, NewBeadForm>()

    fun get(repoKey: String): NewBeadForm? = synchronized(drafts) { drafts[repoKey] }

    /** Keep [form] for [repoKey]; an empty form clears it instead. */
    fun put(repoKey: String, form: NewBeadForm) = synchronized(drafts) {
        if (form.hasContent) drafts[repoKey] = form else drafts.remove(repoKey)
    }

    fun clear(repoKey: String) = synchronized(drafts) { drafts.remove(repoKey) }
}
