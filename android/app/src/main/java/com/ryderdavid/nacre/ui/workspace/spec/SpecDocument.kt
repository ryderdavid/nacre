package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.ChangeArtifact
import com.ryderdavid.nacre.ui.workspace.ChatReference

// ---------- requirement deltas ----------

/** How a change alters a requirement; shown as a text label, never by color alone. */
enum class DeltaKind(val label: String) {
    ADDED("Added"),
    MODIFIED("Modified"),
    REMOVED("Removed"),
    RENAMED("Renamed"),
}

data class ScenarioStep(
    /** When, Then, And, Given or But; empty for a line without one. */
    val keyword: String,
    val text: String,
)

data class Scenario(val name: String, val steps: List<ScenarioStep>)

data class RequirementDelta(
    /** Null in a spec that isn't a delta (no ADDED/MODIFIED/REMOVED/RENAMED heading above it). */
    val kind: DeltaKind?,
    val name: String,
    /** The requirement's text (markdown) before its scenarios. */
    val body: String,
    val scenarios: List<Scenario>,
    /** The old name of a renamed requirement. */
    val from: String = "",
)

/** One capability's spec delta (`specs/<capability>/spec.md` in a change). */
data class SpecDelta(
    val capability: String,
    val title: String,
    /** Markdown outside the requirements, e.g. "## Purpose" and its text. */
    val preface: String,
    val requirements: List<RequirementDelta>,
)

/** Parses OpenSpec spec deltas: `## ADDED Requirements`, `### Requirement: …`, `#### Scenario: …`. */
object SpecDeltas {
    private val DELTA_HEADING = Regex("""^##\s+(ADDED|MODIFIED|REMOVED|RENAMED)\s+Requirements?\s*$""", RegexOption.IGNORE_CASE)
    private val REQUIREMENT = Regex("""^###\s+Requirement:\s*(.+?)\s*$""", RegexOption.IGNORE_CASE)
    private val SCENARIO = Regex("""^####\s+Scenario:\s*(.+?)\s*$""", RegexOption.IGNORE_CASE)
    private val STEP = Regex("""^\s*[-*]\s*\*\*(WHEN|THEN|AND|GIVEN|BUT)\*\*:?\s*(.*)$""", RegexOption.IGNORE_CASE)
    private val RENAME = Regex("""^\s*[-*]\s*(FROM|TO):\s*`?\s*(?:###\s*)?(?:Requirement:\s*)?(.+?)\s*`?\s*$""", RegexOption.IGNORE_CASE)
    private val FENCE = Regex("""^\s*(```|~~~)""")

    fun parse(capability: String, markdown: String): SpecDelta {
        var title = ""
        val preface = mutableListOf<String>()
        val requirements = mutableListOf<RequirementDelta>()
        var kind: DeltaKind? = null
        var inDeltaSection = false
        var renameFrom = ""

        var reqName: String? = null
        var reqKind: DeltaKind? = null
        val reqBody = mutableListOf<String>()
        val scenarios = mutableListOf<Scenario>()
        var scenarioName: String? = null
        val steps = mutableListOf<ScenarioStep>()
        var fenced = false

        fun flushScenario() {
            scenarioName?.let { scenarios += Scenario(it, steps.toList()) }
            scenarioName = null
            steps.clear()
        }

        fun flushRequirement() {
            flushScenario()
            reqName?.let { requirements += RequirementDelta(reqKind, it, reqBody.joinToString("\n").trim(), scenarios.toList()) }
            reqName = null
            reqBody.clear()
            scenarios.clear()
        }

        for (raw in markdown.lines()) {
            val line = raw.trimEnd()
            if (FENCE.containsMatchIn(line)) fenced = !fenced
            if (!fenced && !FENCE.containsMatchIn(line)) {
                if (line.startsWith("# ") && title.isEmpty() && reqName == null) { title = line.removePrefix("# ").trim(); continue }
                DELTA_HEADING.matchEntire(line)?.let { m ->
                    flushRequirement()
                    kind = DeltaKind.valueOf(m.groupValues[1].uppercase())
                    inDeltaSection = true
                    continue
                }
                if (line.startsWith("## ")) {
                    flushRequirement()
                    kind = null
                    inDeltaSection = false
                    preface += line
                    continue
                }
                REQUIREMENT.matchEntire(line)?.let { m ->
                    flushRequirement()
                    reqName = m.groupValues[1]
                    reqKind = kind
                    continue
                }
                if (kind == DeltaKind.RENAMED && reqName == null) {
                    RENAME.matchEntire(line)?.let { m ->
                        if (m.groupValues[1].equals("FROM", true)) renameFrom = m.groupValues[2]
                        else {
                            requirements += RequirementDelta(DeltaKind.RENAMED, m.groupValues[2], "", emptyList(), from = renameFrom)
                            renameFrom = ""
                        }
                        continue
                    }
                }
                if (reqName != null) {
                    SCENARIO.matchEntire(line)?.let { m ->
                        flushScenario()
                        scenarioName = m.groupValues[1]
                        continue
                    }
                    if (line.startsWith("### ")) {
                        // Some other heading ends the requirement.
                        flushRequirement()
                        if (!inDeltaSection) preface += line
                        continue
                    }
                }
            }
            when {
                scenarioName != null -> {
                    if (line.isBlank()) continue
                    val step = STEP.matchEntire(line)
                    when {
                        step != null -> steps += ScenarioStep(step.groupValues[1].lowercase().replaceFirstChar { it.uppercase() }, step.groupValues[2].trim())
                        steps.isNotEmpty() && raw.startsWith(" ") -> steps[steps.lastIndex] = steps.last().let { it.copy(text = (it.text + " " + line.trim()).trim()) }
                        else -> steps += ScenarioStep("", line.trim().removePrefix("- ").removePrefix("* "))
                    }
                }
                reqName != null -> reqBody += raw
                !inDeltaSection -> preface += raw
            }
        }
        flushRequirement()
        return SpecDelta(capability, title, preface.joinToString("\n").trim(), requirements)
    }
}

// ---------- inline text ----------

enum class InlineStyle { BOLD, ITALIC, CODE, LINK }

data class InlineSpan(val start: Int, val end: Int, val style: InlineStyle)

/** Plain text (what is shown, selected and searched) with its inline styles. */
data class StyledText(val text: String, val spans: List<InlineSpan> = emptyList()) {
    companion object {
        /** Strips inline markdown (`**bold**`, `*italic*`, `` `code` ``, `[link](url)`) into styled plain text. */
        fun parse(md: String): StyledText {
            val out = StringBuilder()
            val spans = mutableListOf<InlineSpan>()
            var i = 0
            fun wrap(inner: String, style: InlineStyle, recurse: Boolean) {
                val start = out.length
                if (recurse) {
                    val nested = parse(inner)
                    out.append(nested.text)
                    nested.spans.forEach { spans += InlineSpan(it.start + start, it.end + start, it.style) }
                } else out.append(inner)
                if (out.length > start) spans += InlineSpan(start, out.length, style)
            }
            while (i < md.length) {
                val ch = md[i]
                when {
                    ch == '\\' && i + 1 < md.length && md[i + 1] in "\\`*_[]()#" -> { out.append(md[i + 1]); i += 2 }
                    ch == '`' -> {
                        val close = md.indexOf('`', i + 1)
                        if (close > i) { wrap(md.substring(i + 1, close), InlineStyle.CODE, false); i = close + 1 } else { out.append(ch); i++ }
                    }
                    md.startsWith("**", i) -> {
                        val close = md.indexOf("**", i + 2)
                        if (close > i + 2) { wrap(md.substring(i + 2, close), InlineStyle.BOLD, true); i = close + 2 } else { out.append("**"); i += 2 }
                    }
                    ch == '*' && i + 1 < md.length && !md[i + 1].isWhitespace() -> {
                        val close = md.indexOf('*', i + 1)
                        if (close > i + 1 && !md[close - 1].isWhitespace()) { wrap(md.substring(i + 1, close), InlineStyle.ITALIC, true); i = close + 1 } else { out.append(ch); i++ }
                    }
                    ch == '[' -> {
                        val close = md.indexOf(']', i + 1)
                        val paren = if (close > 0 && close + 1 < md.length && md[close + 1] == '(') md.indexOf(')', close + 2) else -1
                        if (close > i && paren > close) { wrap(md.substring(i + 1, close), InlineStyle.LINK, true); i = paren + 1 } else { out.append(ch); i++ }
                    }
                    else -> { out.append(ch); i++ }
                }
            }
            return StyledText(out.toString(), spans)
        }
    }
}

// ---------- blocks ----------

enum class BlockType { TITLE, HEADING, PARAGRAPH, ITEM, CODE, QUOTE, RULE, CAPABILITY, REQUIREMENT, SCENARIO, STEP }

/** One block of a rendered document; [content] is its selectable, searchable text. */
data class ReaderBlock(
    val type: BlockType,
    val content: StyledText,
    /** Heading level, or a list item's depth. */
    val level: Int = 0,
    /** A list item's marker: "•", "1.", "☐" or "☑". */
    val marker: String = "",
    /** On a requirement: how the change alters it. */
    val delta: DeltaKind? = null,
    /** On a renamed requirement: its old name. */
    val from: String = "",
    /** In spec changes: the capability the block belongs to. */
    val capability: String = "",
    /** In spec changes: the requirement the block belongs to. */
    val requirement: String = "",
)

/** Splits markdown into blocks: headings, paragraphs, list items, code, quotes and rules. */
object MarkdownBlocks {
    private val HEADING = Regex("""^(#{1,6})\s+(.*?)\s*#*\s*$""")
    private val ITEM = Regex("""^(\s*)([-*+]|\d+[.)])\s+(?:\[([ xX])]\s+)?(.*)$""")
    private val RULE = Regex("""^\s*([-*_])(\s*\1){2,}\s*$""")
    private val FENCE = Regex("""^\s*(```|~~~)""")

    fun parse(md: String): List<ReaderBlock> {
        val blocks = mutableListOf<ReaderBlock>()
        val para = mutableListOf<String>()
        val code = mutableListOf<String>()
        var fenced = false

        fun flushPara() {
            if (para.isNotEmpty()) blocks += ReaderBlock(BlockType.PARAGRAPH, StyledText.parse(para.joinToString(" ") { it.trim() }))
            para.clear()
        }

        for (raw in md.lines()) {
            val line = raw.trimEnd()
            if (FENCE.containsMatchIn(line)) {
                if (fenced) { blocks += ReaderBlock(BlockType.CODE, StyledText(code.joinToString("\n"))); code.clear() } else flushPara()
                fenced = !fenced
                continue
            }
            if (fenced) { code += raw; continue }
            val heading = HEADING.matchEntire(line)
            val item = ITEM.matchEntire(line)
            when {
                line.isBlank() -> flushPara()
                heading != null -> {
                    flushPara()
                    val level = heading.groupValues[1].length
                    blocks += ReaderBlock(if (level == 1) BlockType.TITLE else BlockType.HEADING, StyledText.parse(heading.groupValues[2]), level = level)
                }
                RULE.matches(line) -> { flushPara(); blocks += ReaderBlock(BlockType.RULE, StyledText("")) }
                item != null -> {
                    flushPara()
                    val bullet = item.groupValues[2]
                    val marker = when (item.groupValues[3]) {
                        " " -> "☐"
                        "x", "X" -> "☑"
                        else -> if (bullet[0].isDigit()) bullet.replace(')', '.') else "•"
                    }
                    blocks += ReaderBlock(BlockType.ITEM, StyledText.parse(item.groupValues[4]), level = item.groupValues[1].length / 2, marker = marker)
                }
                line.trimStart().startsWith(">") -> {
                    flushPara()
                    val text = line.trimStart().removePrefix(">").trim()
                    val last = blocks.lastOrNull()
                    if (last?.type == BlockType.QUOTE) blocks[blocks.lastIndex] = ReaderBlock(BlockType.QUOTE, StyledText.parse((last.content.text + " " + text).trim()))
                    else blocks += ReaderBlock(BlockType.QUOTE, StyledText.parse(text))
                }
                line.trimStart().startsWith("|") -> {
                    flushPara()
                    // Tables read as monospace rows; separator rows are dropped.
                    if (!line.replace("|", "").replace("-", "").replace(":", "").isBlank()) {
                        val last = blocks.lastOrNull()
                        if (last?.type == BlockType.CODE && last.marker == "table") blocks[blocks.lastIndex] = last.copy(content = StyledText(last.content.text + "\n" + line.trim()))
                        else blocks += ReaderBlock(BlockType.CODE, StyledText(line.trim()), marker = "table")
                    }
                }
                para.isEmpty() && raw.startsWith("  ") && blocks.lastOrNull()?.type == BlockType.ITEM -> {
                    // A continuation line of the item above.
                    val last = blocks.last()
                    blocks[blocks.lastIndex] = last.copy(content = StyledText.parse(itemSource(last) + " " + line.trim()))
                }
                else -> para += line
            }
        }
        if (fenced && code.isNotEmpty()) blocks += ReaderBlock(BlockType.CODE, StyledText(code.joinToString("\n")))
        flushPara()
        return blocks
    }

    /** An item's text as markdown again, close enough to re-parse with a continuation appended. */
    private fun itemSource(b: ReaderBlock): String {
        var s = b.content.text
        b.content.spans.sortedByDescending { it.start }.forEach { span ->
            val mark = when (span.style) { InlineStyle.BOLD -> "**"; InlineStyle.ITALIC -> "*"; InlineStyle.CODE -> "`"; InlineStyle.LINK -> "" }
            s = s.substring(0, span.start) + mark + s.substring(span.start, span.end) + mark + s.substring(span.end)
        }
        return s
    }
}

// ---------- the reader's document ----------

data class OutlineEntry(val block: Int, val label: String, val depth: Int, val delta: DeltaKind? = null)

data class FindMatch(val block: Int, val start: Int, val end: Int)

/** A document as the reader shows it: blocks, plus what the outline, find and next/previous change use. */
data class ReaderDocument(
    val kind: DocKind,
    val changeName: String,
    /** The artifact path, or the change's specs folder for spec changes. */
    val path: String,
    val blocks: List<ReaderBlock>,
    /** For spec changes: each capability's spec.md path. */
    val specPaths: Map<String, String> = emptyMap(),
) {
    val title get() = if (kind == DocKind.SPECS) "Spec changes" else kind.label
    val label get() = "$changeName · $title"
    val isEmpty get() = blocks.isEmpty()

    val outline: List<OutlineEntry>
        get() = blocks.mapIndexedNotNull { i, b ->
            when (b.type) {
                BlockType.TITLE -> OutlineEntry(i, b.content.text, 0)
                BlockType.HEADING -> if (b.level <= 3) OutlineEntry(i, b.content.text, b.level - 1) else null
                BlockType.CAPABILITY -> OutlineEntry(i, b.content.text, 0)
                BlockType.REQUIREMENT -> OutlineEntry(i, b.content.text, 1, b.delta)
                else -> null
            }
        }

    /** Where Next and Previous go: each requirement change in spec changes, each section elsewhere. */
    val stops: List<Int>
        get() = if (kind == DocKind.SPECS) blocks.indices.filter { blocks[it].type == BlockType.REQUIREMENT }
        else blocks.indices.filter { blocks[it].type == BlockType.HEADING && blocks[it].level <= 3 }

    fun next(from: Int): Int? = stops.firstOrNull { it > from }
    fun previous(from: Int): Int? = stops.lastOrNull { it < from }

    /** Case-insensitive matches of [query], in reading order. */
    fun find(query: String): List<FindMatch> {
        val q = query.trim()
        if (q.isEmpty()) return emptyList()
        return blocks.flatMapIndexed { i, b ->
            val text = b.content.text
            generateSequence(text.indexOf(q, ignoreCase = true).takeIf { it >= 0 }) { prev -> text.indexOf(q, prev + q.length, ignoreCase = true).takeIf { it >= 0 } }
                .map { FindMatch(i, it, it + q.length) }.toList()
        }
    }

    /** The block an anchor names: a capability, a requirement or a heading. */
    fun indexOf(anchor: String): Int? {
        if (anchor.isBlank()) return null
        return blocks.indexOfFirst { it.type == BlockType.CAPABILITY && it.content.text == anchor }.takeIf { it >= 0 }
            ?: blocks.indexOfFirst { it.type == BlockType.REQUIREMENT && it.content.text.equals(anchor, true) }.takeIf { it >= 0 }
            ?: blocks.indexOfFirst { (it.type == BlockType.HEADING || it.type == BlockType.TITLE) && it.content.text.equals(anchor, true) }.takeIf { it >= 0 }
    }

    /**
     * What Ask attaches to the composer. Inside a requirement it is that requirement (with the selected
     * sentence, if any); elsewhere the selection; with nothing selected, the document.
     */
    fun reference(block: Int?, selected: String): ChatReference {
        val b = block?.let { blocks.getOrNull(it) }
        val quote = selected.trim()
        return when {
            b != null && b.requirement.isNotBlank() -> ChatReference(
                kind = "requirement",
                id = (specPaths[b.capability] ?: path) + "#" + b.requirement,
                label = b.requirement,
                text = quote,
            )
            quote.isNotEmpty() -> ChatReference("selection", path, label, quote)
            else -> ChatReference("document", path, label)
        }
    }

    companion object {
        /** Builds [kind] from a change's artifacts; empty when the change has no such document. */
        fun build(kind: DocKind, changeName: String, artifacts: List<ChangeArtifact>): ReaderDocument {
            if (kind != DocKind.SPECS) {
                val a = artifacts.firstOrNull { DocKind.of(it.path) == kind }
                return ReaderDocument(kind, changeName, a?.path ?: "openspec/changes/$changeName/${kind.name.lowercase()}.md", a?.let { MarkdownBlocks.parse(it.content) }.orEmpty())
            }
            val specs = artifacts.mapNotNull { a -> SpecList.capabilityOf(a.path)?.let { it to a } }.sortedBy { it.first }
            val blocks = mutableListOf<ReaderBlock>()
            specs.forEach { (cap, a) ->
                val delta = SpecDeltas.parse(cap, a.content)
                blocks += ReaderBlock(BlockType.CAPABILITY, StyledText(cap), capability = cap)
                MarkdownBlocks.parse(delta.preface).forEach { blocks += it.copy(capability = cap) }
                delta.requirements.forEach { r ->
                    blocks += ReaderBlock(BlockType.REQUIREMENT, StyledText(r.name), delta = r.kind, from = r.from, capability = cap, requirement = r.name)
                    MarkdownBlocks.parse(r.body).forEach { blocks += it.copy(capability = cap, requirement = r.name) }
                    r.scenarios.forEach { s ->
                        blocks += ReaderBlock(BlockType.SCENARIO, StyledText("Scenario: ${s.name}"), capability = cap, requirement = r.name)
                        s.steps.forEach { step ->
                            val styled = StyledText.parse(step.text)
                            blocks += ReaderBlock(
                                BlockType.STEP,
                                if (step.keyword.isEmpty()) styled
                                else StyledText(step.keyword + " " + styled.text, listOf(InlineSpan(0, step.keyword.length, InlineStyle.BOLD)) + styled.spans.map { it.copy(start = it.start + step.keyword.length + 1, end = it.end + step.keyword.length + 1) }),
                                capability = cap, requirement = r.name,
                            )
                        }
                    }
                }
            }
            val folder = specs.firstOrNull()?.second?.path?.substringBeforeLast("/specs/")?.plus("/specs") ?: "openspec/changes/$changeName/specs"
            return ReaderDocument(DocKind.SPECS, changeName, folder, blocks, specs.associate { (cap, a) -> cap to a.path })
        }
    }
}
