package com.ryderdavid.nacre.data.claude

/**
 * The prompts behind Refine, Draft and the board assistant, kept in one place
 * so a bead refined on the Mac reads the same as one refined through the
 * Claude API. The schemas live next door in [ClaudeService]; both backends
 * send those too, which is what keeps the two answers parseable by the same
 * types and reviewable on the same screens.
 */
object Prompts {
    const val EDITOR =
        "You are an expert engineering lead who writes beads (issues) for the bd issue tracker. Beads are read by coding agents and by the owner, so they must be specific, self-contained and testable. You never pad or invent facts."

    private const val REFINE_DEFAULT =
        "Make this bead clear and ready for an agent to pick up: a crisp title, a description that says why and what, and testable acceptance criteria."

    const val ADVISOR =
        "You are an expert engineering lead answering questions about one bead (issue) in the bd issue tracker. You explain, assess and advise; you never change the bead. You are direct and specific, and you say so when the bead, its comments or the code don't settle a question."

    fun refine(ctx: BeadContext, instruction: String, history: List<DraftTurn> = emptyList()): String = buildString {
        appendLine("Improve this bead from the \"${ctx.repoName}\" beads (bd) issue tracker according to the request below.")
        appendLine()
        conversation(history, "Earlier in this conversation about the bead (questions and answers may bear on the request):")
        appendLine("<request>${instruction.ifBlank { REFINE_DEFAULT }}</request>")
        appendLine()
        appendLine(ctx.render())
        appendLine()
        appendLine("Return only fields you are changing; use null for everything you leave as is. Keep the author's facts, ids, paths and decisions; do not invent requirements. Write in plain, direct markdown. Propose children only when the request asks to split or break down the work. In `summary`, say in one or two sentences what you changed and why.")
    }

    /**
     * A question about a bead, answered in prose. [readsRepo] when the answer may
     * come from reading the repository too, as it does on the Mac.
     */
    fun ask(ctx: BeadContext, history: List<DraftTurn>, question: String, readsRepo: Boolean): String = buildString {
        appendLine("Ryder is asking about this bead from the \"${ctx.repoName}\" beads (bd) issue tracker.")
        appendLine()
        appendLine(ctx.render())
        appendLine()
        conversation(history, "Earlier in this conversation:")
        appendLine("<question>$question</question>")
        appendLine()
        if (readsRepo) appendLine("You are in the repository the bead belongs to and may read its code, git history and other beads (read-only bd commands) when the question needs them. Look before you answer a question about the code; don't guess.")
        appendLine("Answer the question in `answer`: short, scannable markdown for a phone screen, with ids, paths and names as they appear. Don't propose edits to the bead unless asked what you would change; Ryder switches to Refine mode to make changes.")
    }

    private fun StringBuilder.conversation(history: List<DraftTurn>, intro: String) {
        if (history.isEmpty()) return
        appendLine(intro)
        appendLine("<conversation>")
        history.takeLast(24).forEach { appendLine(if (it.fromUser) "<ryder>${it.text}</ryder>" else "<you>${it.text}</you>") }
        appendLine("</conversation>")
        appendLine()
    }

    fun draft(repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): String = buildString {
        appendLine("Turn this quick note into a well-formed bead for the \"$repoName\" beads (bd) issue tracker.")
        appendLine()
        appendLine("<note>$rough</note>")
        parentTitle?.let { appendLine("It will be a child of: $it") }
        if (existingTitles.isNotEmpty()) {
            appendLine("Existing open beads (avoid duplicating them):")
            existingTitles.take(60).forEach { appendLine("- $it") }
        }
        appendLine()
        appendLine("Choose issue_type from bug, feature, task, chore, epic, decision, spike. Priority is 0 (critical) to 4 (backlog); default 2. Keep the title under 80 characters and imperative. Only add acceptance criteria and design notes you can justify from the note; leave them empty otherwise. Labels: at most 3, lowercase, only if clearly useful.")
        appendLine("In open_questions, list up to 4 short questions whose answers the note leaves open and that would change the bead (scope, priority, approach); leave it empty when nothing is unclear. Leave reply empty.")
    }

    /** Revises a draft from a chat message that answers an open question or asks to change a field. */
    fun reviseDraft(
        repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): String = buildString {
        appendLine("You drafted the bead below for the \"$repoName\" beads (bd) issue tracker; the owner may have edited it since. Revise it according to their latest message.")
        appendLine()
        appendLine(current.render())
        parentTitle?.let { appendLine("It will be a child of: $it") }
        if (existingTitles.isNotEmpty()) {
            appendLine("Existing open beads (avoid duplicating them):")
            existingTitles.take(60).forEach { appendLine("- $it") }
        }
        if (history.isNotEmpty()) {
            appendLine("<conversation>")
            history.forEach { appendLine(if (it.fromUser) "<owner>${it.text}</owner>" else "<you>${it.text}</you>") }
            appendLine("</conversation>")
        }
        appendLine()
        appendLine("<message>$message</message>")
        appendLine()
        appendLine("If the message answers one of the open questions, work the answer into the fields it affects and drop that question. If it asks to change a named field (title, description, design, acceptance criteria, type, priority or labels), change that field as asked. Keep everything else as it is unless the message implies it should change, and keep the owner's own edits. Return the whole revised draft. Keep open_questions to those still unanswered, plus any new one the message raises. In reply, tell the owner in one sentence what you changed.")
    }

    /**
     * What the board assistant's own system prompt gains when it runs as
     * `claude -p` on the Mac: that it reads with bd itself, and that it cannot
     * change anything — it proposes, Ryder approves on the phone, and the app
     * applies the change through nacred's REST API.
     */
    fun macAssistant(base: String, tools: List<ToolSpec> = BOARD_TOOLS): String = buildString {
        appendLine(base)
        appendLine()
        appendLine("## How you work here")
        appendLine()
        appendLine("You are running as Claude Code on Ryder's Mac, in the repository this board belongs to.")
        appendLine()
        appendLine("To look things up, run read-only bd commands with Bash: `bd list`, `bd show <id>`, `bd comments <id>`, `bd ready`, `bd blocked`, `bd stats`, `bd search <text>`, `bd dep tree <id>`. Add `--json` when you want structured output. Nothing else is available to you — no files, no web, no other commands — and anything that would change a bead is refused.")
        appendLine()
        appendLine("You cannot change beads yourself, and you must not try. Instead put every change you want in `proposals`, each one naming a tool and its arguments from the catalogue below. Ryder sees each proposal in the app, approves or declines it, and the app applies the approved ones. So propose concrete edits rather than asking whether to make them, and say in `reply` what you are proposing and why. If a following turn tells you what was applied or declined, carry on from there and never re-propose something that was declined.")
        appendLine()
        appendLine("`reply` is what Ryder reads: short, scannable markdown for a phone screen, beads by id and title. Do not put JSON in it, and do not repeat the proposals field by field — he can see them.")
        appendLine()
        appendLine("## Proposal catalogue")
        appendLine()
        append(writeToolCatalogue(tools))
    }

    /**
     * The shape a Mac-hosted turn answers in: prose for Ryder, plus the writes
     * it would like to make.
     */
    fun proposalsSchema(tools: List<ToolSpec> = BOARD_TOOLS): Map<String, Any> = mapOf(
        "type" to "object",
        "additionalProperties" to false,
        "required" to listOf("reply", "proposals"),
        "properties" to mapOf(
            "reply" to mapOf("type" to "string", "description" to "Your answer to Ryder, as short markdown"),
            "proposals" to mapOf(
                "type" to "array",
                "description" to "Changes for Ryder to approve, in the order they should be applied; empty when you are only answering",
                "items" to mapOf(
                    "type" to "object",
                    "additionalProperties" to false,
                    "required" to listOf("tool", "input"),
                    "properties" to mapOf(
                        "tool" to mapOf("type" to "string", "enum" to tools.filter { it.write }.map { it.name }, "description" to "A tool from the proposal catalogue"),
                        "input" to mapOf("type" to "object", "description" to "That tool's arguments, exactly as its catalogue entry describes"),
                    ),
                ),
            ),
        ),
    )

    /** What the next turn is told about the proposals the user just ruled on. */
    fun outcomes(answers: List<ToolAnswer>): String = buildString {
        appendLine("Ryder reviewed your proposals in the app. Here is what happened, in order:")
        appendLine()
        answers.forEach { a ->
            append("- ").append(a.name).append(": ")
            appendLine(if (a.isError) "NOT APPLIED — ${a.content.trim()}" else "applied — ${a.content.trim()}")
        }
        appendLine()
        appendLine("Take that as done. Do not repeat anything that was declined. If this finishes the request, reply with a one-line confirmation and no proposals; otherwise continue with the next step.")
    }
}
