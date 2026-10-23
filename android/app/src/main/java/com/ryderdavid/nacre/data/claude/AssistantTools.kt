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
    /** JSON Schema for the arguments. Strict tools require every property, so optional ones are nullable. */
    val props: Map<String, Any>,
    /** Writes change beads, so they always wait for the user's approval in the app. */
    val write: Boolean,
    /** Arguments that must be present and non-blank for the call to be worth running. */
    val required: List<String> = emptyList(),
)

private fun str(desc: String) = mapOf("type" to "string", "description" to desc)
private fun nullable(type: String, desc: String) =
    mapOf("anyOf" to listOf(mapOf("type" to type), mapOf("type" to "null")), "description" to desc)

private val stringsOrNull = mapOf(
    "anyOf" to listOf(mapOf("type" to "array", "items" to mapOf("type" to "string")), mapOf("type" to "null")),
    "description" to "The complete new label set, or null to leave the labels alone",
)

/** Everything the assistant can do on a board, in the order it usually needs them. */
val BOARD_TOOLS: List<ToolSpec> = listOf(
    ToolSpec(
        "list_beads",
        "List beads on this board as compact lines (id, lane, priority, type, title, blockers). Use lane to narrow down and query for text search.",
        mapOf(
            "lane" to nullable("string", "ready | doing | blocked | later | done | open (everything not done), or null for all"),
            "query" to nullable("string", "Text to match in id, title, labels and description, or null"),
            "limit" to nullable("integer", "Maximum rows, default 50"),
        ),
        write = false,
    ),
    ToolSpec(
        "get_bead", "Get one bead in full: fields, dependencies, children and comments.",
        mapOf("id" to str("Bead id")), write = false, required = listOf("id"),
    ),
    ToolSpec(
        "update_bead",
        "Change fields of a bead. Pass null for fields to leave unchanged. labels replaces the whole set. Requires the user's approval.",
        mapOf(
            "id" to str("Bead id"),
            "title" to nullable("string", "New title"),
            "description" to nullable("string", "New description (markdown)"),
            "design" to nullable("string", "New design notes"),
            "acceptance_criteria" to nullable("string", "New acceptance criteria"),
            "notes" to nullable("string", "New notes"),
            "status" to nullable("string", "open | in_progress | blocked | deferred"),
            "priority" to nullable("integer", "0-4"),
            "issue_type" to nullable("string", "bug | feature | task | chore | epic | decision | spike"),
            "assignee" to nullable("string", "Assignee name"),
            "labels" to stringsOrNull,
        ),
        write = true, required = listOf("id"),
    ),
    ToolSpec(
        "create_bead", "Create a new bead. Requires the user's approval.",
        mapOf(
            "title" to str("Imperative title under 80 characters"),
            "description" to str("Why and what (markdown)"),
            "issue_type" to str("bug | feature | task | chore | epic | decision | spike"),
            "priority" to mapOf("type" to "integer", "description" to "0-4"),
            "parent" to nullable("string", "Parent bead id, e.g. an epic"),
            "acceptance_criteria" to str("Testable acceptance criteria, or empty"),
            "labels" to mapOf("type" to "array", "items" to mapOf("type" to "string")),
        ),
        write = true, required = listOf("title"),
    ),
    ToolSpec(
        "close_bead", "Close a bead as done. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "reason" to nullable("string", "One-line close reason")),
        write = true, required = listOf("id"),
    ),
    ToolSpec(
        "add_comment", "Add a comment to a bead. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "text" to str("Comment text (markdown)")),
        write = true, required = listOf("id", "text"),
    ),
    ToolSpec(
        "add_dependency", "Record that one bead waits on (is blocked by) another. Requires the user's approval.",
        mapOf("issue_id" to str("The bead that waits"), "depends_on_id" to str("The bead it waits on")),
        write = true, required = listOf("issue_id", "depends_on_id"),
    ),
    ToolSpec(
        "run_in_claude_code",
        "Dispatch a bead to Claude Code on Ryder's Mac Studio to do the work. Requires the user's approval.",
        mapOf("id" to str("Bead id"), "instructions" to nullable("string", "Extra instructions for the coding agent")),
        write = true, required = listOf("id"),
    ),
)

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
            val type = meta?.get("type") as? String ?: "string or null"
            val desc = meta?.get("description") as? String ?: ""
            val need = if (field in spec.required) " (required)" else ""
            append("- `").append(field).append("`: ").append(type).append(need)
            if (desc.isNotBlank()) append(" — ").append(desc)
            append('\n')
        }
    }.trimEnd()
}
