package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.claude.DraftTurn
import com.ryderdavid.nacre.data.model.NewEffortPrefill

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

    /**
     * Keep [form] under [key]. A form with nothing typed in, or one still exactly as it was
     * opened ([start], such as an untouched prefill), clears it instead.
     */
    fun put(key: String, form: NewBeadForm, start: NewBeadForm = NewBeadForm()) = synchronized(drafts) {
        if (form.hasContent && form != start) drafts[key] = form else drafts.remove(key)
    }

    fun clear(repoKey: String) = synchronized(drafts) { drafts.remove(repoKey) }
}

/**
 * Where a new-bead form's draft is kept: a plain new bead and a new effort each have one per
 * board, and a prefilled form (found work, a follow-on change) has one per thing it came from,
 * so edits to one prefill never turn up in another.
 */
fun newBeadDraftKey(repoKey: String, effort: Boolean, prefill: NewEffortPrefill?): String = when {
    prefill == null -> if (effort) "$repoKey#effort" else repoKey
    prefill.followOnOf.isNotBlank() -> "$repoKey#follow-on#${prefill.followOnOf}"
    prefill.foundComment != null -> "$repoKey#found#${prefill.foundComment.bead}#${prefill.foundComment.commentId}"
    else -> "$repoKey#prefill#${prefill.title}"
}
