package com.ryderdavid.nacre.data.claude

/**
 * One tool the board assistant can use, described without reference to where
 * the model runs. The Claude API backend turns these into tool definitions;
 * the Mac backend renders them into a catalogue Claude Code reads, because
 * there the model proposes calls instead of making them.
 */
data class ToolSpec(
    val name: String,
    val description: String,
    /**
     * JSON Schema for the arguments. Every property is required unless named in [optional];
     * strict tools cap a request at 24 optional and 16 union-typed parameters, so optional
     * values are left out rather than made nullable, and most "no value" cases are an empty string.
     */
    val props: Map<String, Any>,
    /** Writes change beads, so they always wait for the user's approval in the app. */
    val write: Boolean,
    /** Arguments that must be present and non-blank for the call to be worth running. */
    val required: List<String> = emptyList(),
    /** Properties the model may leave out; leaving one out means "no value" or "unchanged". */
    val optional: Set<String> = emptySet(),
    /** Spends Ryder's subscription on paid calls: always asks, even in auto-apply mode. */
    val spends: Boolean = false,
)

/** Whether this call spends: a spending tool, or a create that also develops a proposal. */
fun spends(name: String, input: kotlinx.serialization.json.JsonObject?): Boolean =
    BOARD_TOOLS.firstOrNull { it.name == name }?.spends == true ||
        (name == "create_bead" && (input?.get("then_develop") as? kotlinx.serialization.json.JsonPrimitive)?.content == "true")

private fun str(desc: String) = mapOf("type" to "string", "description" to desc)
private fun typed(type: String, desc: String) = mapOf("type" to type, "description" to desc)
private fun strings(desc: String) = mapOf("type" to "array", "items" to mapOf("type" to "string"), "description" to desc)

/** Everything the assistant can do on a board, in the order it usually needs them. */
val BOARD_TOOLS: List<ToolSpec> = listOf(
    ToolSpec(
        "list_beads",
        "List beads on this board as compact lines (id, lane, priority, type, title, blockers). Use lane to narrow down and query for text search.",
        mapOf(
            "lane" to str("ready | doing | blocked | later | done | open (everything not done); leave out for all"),
            "query" to str("Text to match in id, title, labels and description; leave out for no filter"),
            "limit" to typed("integer", "Maximum rows; 50 suits most questions"),
        ),
        write = false, optional = setOf("lane", "query"),
    ),
    ToolSpec(
        "get_bead", "Get one bead in full: fields, dependencies, children and comments.",
        mapOf("id" to str("Bead id")), write = false, required = listOf("id"),
    ),
    ToolSpec(
        "update_bead",
        "Change fields of a bead. Leave out the fields to keep as they are. labels replaces the whole set. Requires the user's approval.",
        mapOf(
            "id" to str("Bead id"),
            "title" to typed("string", "New title"),
            "description" to typed("string", "New description (markdown)"),
            "design" to typed("string", "New design notes"),
            "acceptance_criteria" to typed("string", "New acceptance criteria"),
            "notes" to typed("string", "New notes"),
            "status" to typed("string", "open | in_progress | blocked | deferred"),
            "priority" to typed("integer", "0-4"),
            "issue_type" to typed("string", "bug | feature | task | chore | epic | decision | spike"),
            "assignee" to typed("string", "Assignee name"),
            "labels" to strings("The complete new label set"),
        ),
        write = true, required = listOf("id"),
        optional = setOf("title", "description", "design", "acceptance_criteria", "notes", "status", "priority", "issue_type", "assignee", "labels"),
    ),
    ToolSpec(
        "create_bead", "Create a new bead. Requires the user's approval.",
        mapOf(
            "title" to str("Imperative title under 80 characters"),
            "description" to str("Why and what (markdown)"),
            "issue_type" to str("bug | feature | task | chore | epic | decision | spike"),
            "priority" to mapOf("type" to "integer", "description" to "0-4"),
            "parent" to str("Parent bead id, e.g. an epic, or empty"),
            "acceptance_criteria" to str("Testable acceptance criteria, or empty"),
            "labels" to mapOf("type" to "array", "items" to mapOf("type" to "string")),
            "then_develop" to typed("boolean", "Also develop a reviewed OpenSpec proposal for it (runs Claude on the computer; always asks Ryder). Only for features or epics that change behavior a spec or active change covers"),
        ),
        write = true, required = listOf("title"), optional = setOf("then_develop"),
    ),
    ToolSpec(
        "develop_proposal",
        "Develop a reviewed OpenSpec proposal for an existing bead: the computer compares it with the accepted specs and every active change, writes a new change or an amendment, and has it independently reviewed. It runs Claude on Ryder's computer, so it always asks him first. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "feedback" to str("What the proposal should take into account, or empty")),
        write = true, required = listOf("id"), optional = setOf("feedback"), spends = true,
    ),
    ToolSpec(
        "found_something",
        "Report work found while implementing a specification change, on its epic, a section bead or found bead (found_in). answer: how_order (only how or in what order: a bead under the change's epic), amend (it changes what done means: filed for Ryder to revise the change or make it a follow-on) or out_of_scope (a new bead related to the epic). Requires the user's approval.",
        mapOf(
            "found_in" to str("The bead the work was found in: the change's epic, a section bead or a found bead"),
            "note" to str("One or two sentences: what was found"),
            "answer" to str("how_order | amend | out_of_scope"),
        ),
        write = true, required = listOf("found_in", "note", "answer"),
    ),
    ToolSpec(
        "close_bead", "Close a bead as done. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "reason" to str("One-line close reason")),
        write = true, required = listOf("id"),
    ),
    ToolSpec(
        "add_comment", "Add a comment to a bead. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "text" to str("Comment text (markdown)")),
        write = true, required = listOf("id", "text"),
    ),
    ToolSpec(
        "add_dependency", "Record that one bead waits on (is blocked by) another. Both beads must be on the same board. Requires the user's approval.",
        mapOf("issue_id" to str("The bead that waits"), "depends_on_id" to str("The bead it waits on")),
        write = true, required = listOf("issue_id", "depends_on_id"),
    ),
    ToolSpec(
        "run_in_claude_code",
        "Dispatch a bead to Claude Code on Ryder's Mac Studio to do the work. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "instructions" to str("Extra instructions for the coding agent, or empty")),
        write = true, required = listOf("id"),
    ),
    ToolSpec(
        "start_round",
        "Hand several beads on this board to its computer as one round: it reviews them together first, asks Ryder only where they disagree, works each on its own branch, checks each against the repository's gates, and merges them into one commit that passes the gates and builds. Use it when Ryder wants several beads worked until they're done. Requires the user's approval.",
        mapOf(
            "ids" to strings("The bead ids to finish together"),
            "budget_usd" to typed("number", "Spending limit for the round in USD, or 0 for the computer's default"),
        ),
        write = true, required = listOf("ids"),
    ),
)

/** Needs you, across computers; offered on the API backend (the Mac backend gets it in the prompt). */
val NEEDS_YOU = ToolSpec(
    "needs_you",
    "List everything waiting on Ryder across his computers: decisions, proposals ready for approval, blocked or stale reviews, round questions, parked beads, rounds ready to merge, changes ready to archive and found work, each with why. Call it first when Ryder asks what to work on next or what needs him.",
    emptyMap(), write = false,
)

/** Lists the boards; only offered when one conversation spans every board. */
val LIST_BOARDS = ToolSpec(
    "list_boards",
    "List every board (project) you can reach, with its bead counts, computers and path. It reads no board, so it is cheap: call it before anything else, then use the counts to choose which boards to read.",
    mapOf("query" to str("Text to match in the board name, path or git remote; leave out for all boards")),
    write = false, optional = setOf("query"),
)

/**
 * The tools for one conversation. Across every board the assistant can also list the
 * boards, and every board-scoped tool takes the board it acts on.
 */
fun assistantTools(everyBoard: Boolean): List<ToolSpec> {
    if (!everyBoard) return listOf(NEEDS_YOU) + BOARD_TOOLS
    return listOf(LIST_BOARDS, NEEDS_YOU) + BOARD_TOOLS.map { spec ->
        if (spec.name == "list_beads") spec.copy(
            description = spec.description.replace("on this board ", ""),
            props = mapOf("board" to str("Board name from list_boards. Leaving it out sweeps the ${AllBoardsRouter.MAX_BOARDS_SWEPT} most recently updated boards (a few rows from each, hidden boards skipped); prefer naming the boards whose counts matter")) + spec.props,
            optional = spec.optional + "board",
        ) else spec.copy(
            props = mapOf("board" to str("Board name, exactly as list_boards reports it")) + spec.props,
            required = listOf("board") + spec.required,
        )
    }
}

/** Tools that change beads, and so can only run once the user approves them. */
internal val WRITE_TOOLS: Set<String> = BOARD_TOOLS.filter { it.write }.map { it.name }.toSet()

internal val REQUIRED: Map<String, List<String>> = BOARD_TOOLS.associate { it.name to it.required }

/**
 * The write tools written out for a model that cannot call them — it proposes
 * them instead and the app applies the ones the user approves.
 */
internal fun writeToolCatalogue(tools: List<ToolSpec> = BOARD_TOOLS): String = tools.filter { it.write }.joinToString("\n\n") { spec ->
    buildString {
        append("### ").append(spec.name).append('\n')
        append(spec.description.removeSuffix(" Requires the user's approval.")).append('\n')
        spec.props.forEach { (field, schema) ->
            val meta = schema as? Map<*, *>
            val type = meta?.get("type") as? String ?: "string"
            val desc = meta?.get("description") as? String ?: ""
            val need = when (field) {
                in spec.required -> " (required)"
                in spec.optional -> " (optional)"
                else -> ""
            }
            append("- `").append(field).append("`: ").append(type).append(need)
            if (desc.isNotBlank()) append(" — ").append(desc)
            append('\n')
        }
    }.trimEnd()
}
