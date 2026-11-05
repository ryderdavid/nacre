package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.board.NewBeadForm
import com.ryderdavid.nacre.data.model.ChatDraft
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.IntakeDraft
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.NewEffortPrefill

/** What the board's create action opens. */
enum class NewEffortStart {
    /** OpenSpec repository on a computer with Chat: a New effort chat drafts the effort. */
    Chat,
    /** OpenSpec repository on an older nacred without `agent_chat`: the New effort sheet, no draft step. */
    Sheet,
    /** Repository without OpenSpec: the New bead form, with no AI draft step and no intake. */
    Form,
}

/** Where New effort starts for a repository with ([openspec]) or without OpenSpec on a computer reporting [health]. */
fun newEffortStart(openspec: Boolean, health: Health?): NewEffortStart = when {
    !openspec -> NewEffortStart.Form
    servesChat(health) -> NewEffortStart.Chat
    else -> NewEffortStart.Sheet
}

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

/**
 * Continue on a Draft card: the New effort sheet for [repoKey], opened at its intake step with
 * [draft], carrying any relation [prefill] had (related to an epic, a follow-on, the found comment).
 */
data class DraftContinuation(val repoKey: String, val draft: ChatDraft, val prefill: NewEffortPrefill?)

/**
 * Where Continue on [chat]'s [draft] leads: the board the New effort chat was started for, with
 * its prefill, or the chat's own repository. Null for an all-boards chat, which has no board.
 */
fun draftContinuation(chat: ChatRecord?, chatKey: String, draft: ChatDraft, origin: NewEffortOrigin?): DraftContinuation? {
    if (origin != null) return DraftContinuation(origin.repoKey, draft, origin.prefill)
    val repo = chat?.repo?.ifBlank { null } ?: return null
    return DraftContinuation(Keys.of(Keys.host(chatKey), repo), draft, null)
}

/** The sheet's form as a draft from Chat fills it. */
fun draftForm(draft: ChatDraft, parent: String? = null, types: List<String>): NewBeadForm = NewBeadForm(
    title = draft.title.trim(),
    description = draft.description.trim(),
    acceptance = draft.acceptance.trim(),
    design = draft.design.trim(),
    type = draft.type.lowercase().takeIf { it in types } ?: "task",
    parent = parent,
)

/** What intake reads from the sheet's form. */
fun intakeDraftOf(form: NewBeadForm) =
    IntakeDraft(form.title.trim(), form.description, form.design, form.acceptance, form.type, form.parent.orEmpty())

/** Runs intake on [form] for [repoKey]; the verdict, or the failure the sheet shows. */
suspend fun runIntake(repoKey: String, form: NewBeadForm, intake: suspend (String, IntakeDraft) -> IntakeVerdict): Result<IntakeVerdict> =
    try {
        Result.success(intake(repoKey, intakeDraftOf(form)))
    } catch (e: kotlinx.coroutines.CancellationException) {
        throw e
    } catch (e: Exception) {
        Result.failure(e)
    }

/** The bead the sheet creates: the form, with the relations the prefill carried (an out-of-scope bead is related to its epic, not a child). */
fun createIssueOf(form: NewBeadForm, prefill: NewEffortPrefill?) = CreateIssue(
    title = form.title.trim(), description = form.description.ifBlank { null }, type = form.type, priority = form.priority,
    parent = form.parent, labels = form.labels.ifEmpty { null }, design = form.design.ifBlank { null }, acceptance = form.acceptance.ifBlank { null },
    related = prefill?.related?.ifEmpty { null }, followOnOf = prefill?.followOnOf?.ifBlank { null }, foundComment = prefill?.foundComment,
)
