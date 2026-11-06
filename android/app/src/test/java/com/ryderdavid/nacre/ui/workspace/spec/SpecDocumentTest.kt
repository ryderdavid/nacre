package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.ChangeArtifact
import com.ryderdavid.nacre.data.model.ChangeSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SpecDocumentTest {
    private val agentChat = """
        # Spec Delta

        ## Purpose

        Chats survive drops.

        ## ADDED Requirements

        ### Requirement: A dropped stream resumes from the last event
        When the app reconnects, it SHALL ask nacred for the events after the last one it shows,
        and **SHALL NOT** show an event twice.

        #### Scenario: Airplane mode mid-reply
        - **WHEN** the phone loses its connection for 30 seconds
          while Claude replies
        - **THEN** the reply continues where it stopped
        - **AND** nothing is doubled

        ## MODIFIED Requirements

        ### Requirement: Chats are long-lived and resumable
        A chat SHALL survive the app closing and a half-written message.

        ```
        ### Requirement: not a heading inside code
        ```

        ## REMOVED Requirements

        ### Requirement: Reconnect restarts the reply
        **Reason**: Replaced by resuming.
        **Migration**: None.

        ## RENAMED Requirements

        - FROM: `### Requirement: Chat tab`
        - TO: `### Requirement: Chat destination`
    """.trimIndent()

    @Test
    fun `deltas carry Added, Modified, Removed and Renamed`() {
        val d = SpecDeltas.parse("agent-chat", agentChat)
        assertEquals("Spec Delta", d.title)
        assertEquals(
            listOf(
                DeltaKind.ADDED to "A dropped stream resumes from the last event",
                DeltaKind.MODIFIED to "Chats are long-lived and resumable",
                DeltaKind.REMOVED to "Reconnect restarts the reply",
                DeltaKind.RENAMED to "Chat destination",
            ),
            d.requirements.map { it.kind to it.name },
        )
        assertEquals(listOf("Added", "Modified", "Removed", "Renamed"), d.requirements.map { it.kind!!.label })
        assertEquals("Chat tab", d.requirements[3].from)
        assertTrue(d.preface.startsWith("## Purpose"))
        assertTrue("Chats survive drops." in d.preface)
    }

    @Test
    fun `scenarios split into steps with their keywords and continuation lines`() {
        val r = SpecDeltas.parse("agent-chat", agentChat).requirements[0]
        assertTrue(r.body.startsWith("When the app reconnects"))
        val s = r.scenarios.single()
        assertEquals("Airplane mode mid-reply", s.name)
        assertEquals(
            listOf(
                ScenarioStep("When", "the phone loses its connection for 30 seconds while Claude replies"),
                ScenarioStep("Then", "the reply continues where it stopped"),
                ScenarioStep("And", "nothing is doubled"),
            ),
            s.steps,
        )
    }

    @Test
    fun `a heading inside a code fence stays in the requirement's text`() {
        val modified = SpecDeltas.parse("agent-chat", agentChat).requirements[1]
        assertTrue("### Requirement: not a heading inside code" in modified.body)
    }

    @Test
    fun `a removed requirement keeps its reason`() {
        val removed = SpecDeltas.parse("agent-chat", agentChat).requirements[2]
        assertTrue(removed.body.contains("**Reason**: Replaced by resuming."))
    }

    @Test
    fun `a plain spec has requirements without delta labels`() {
        val d = SpecDeltas.parse("x", "# X\n\n### Requirement: Plain\nIt SHALL work.\n")
        assertNull(d.requirements.single().kind)
    }

    @Test
    fun `inline markdown becomes styled plain text`() {
        val s = StyledText.parse("It **SHALL NOT** use `op`, see [the docs](https://x) and *this*.")
        assertEquals("It SHALL NOT use op, see the docs and this.", s.text)
        assertEquals(
            listOf(InlineStyle.BOLD to "SHALL NOT", InlineStyle.CODE to "op", InlineStyle.LINK to "the docs", InlineStyle.ITALIC to "this"),
            s.spans.map { it.style to s.text.substring(it.start, it.end) },
        )
        assertEquals("2 * 3 = 6", StyledText.parse("2 * 3 = 6").text)
    }

    @Test
    fun `markdown blocks for proposal, design and tasks`() {
        val blocks = MarkdownBlocks.parse("# Tasks\n\n## 1. Bridge\n\n- [ ] 1.1 Mint a token\n- [x] 1.2 Mark routes\n  with more text\n\nA paragraph\nover two lines.\n\n1. first\n\n---\n\n| a | b |\n|---|---|\n| 1 | 2 |")
        assertEquals(
            listOf(BlockType.TITLE, BlockType.HEADING, BlockType.ITEM, BlockType.ITEM, BlockType.PARAGRAPH, BlockType.ITEM, BlockType.RULE, BlockType.CODE),
            blocks.map { it.type },
        )
        assertEquals(listOf("☐", "☑"), blocks.slice(2..3).map { it.marker })
        assertEquals("1.2 Mark routes with more text", blocks[3].content.text)
        assertEquals("A paragraph over two lines.", blocks[4].content.text)
        assertEquals("1.", blocks[5].marker)
        assertEquals("| a | b |\n| 1 | 2 |", blocks[7].content.text)
    }

    private fun specDoc() = ReaderDocument.build(
        DocKind.SPECS, "offline-chat-resume",
        listOf(
            ChangeArtifact("openspec/changes/offline-chat-resume/specs/spec-viewer/spec.md", "## ADDED Requirements\n\n### Requirement: Reader\nIt SHALL read.\n"),
            ChangeArtifact("openspec/changes/offline-chat-resume/proposal.md", "# Proposal"),
            ChangeArtifact("openspec/changes/offline-chat-resume/specs/agent-chat/spec.md", agentChat),
        ),
    )

    @Test
    fun `spec changes read capability by capability with labelled requirements`() {
        val doc = specDoc()
        assertEquals("Spec changes", doc.title)
        assertEquals("openspec/changes/offline-chat-resume/specs", doc.path)
        val caps = doc.blocks.filter { it.type == BlockType.CAPABILITY }.map { it.content.text }
        assertEquals(listOf("agent-chat", "spec-viewer"), caps)
        val reqs = doc.blocks.filter { it.type == BlockType.REQUIREMENT }
        assertEquals(listOf(DeltaKind.ADDED, DeltaKind.MODIFIED, DeltaKind.REMOVED, DeltaKind.RENAMED, DeltaKind.ADDED), reqs.map { it.delta })
        val when_ = doc.blocks.first { it.type == BlockType.STEP }
        assertEquals("When the phone loses its connection for 30 seconds while Claude replies", when_.content.text)
        assertEquals(InlineSpan(0, 4, InlineStyle.BOLD), when_.content.spans.first())
        assertEquals("A dropped stream resumes from the last event", when_.requirement)
    }

    @Test
    fun `outline, next and previous change, and anchors`() {
        val doc = specDoc()
        val reqIndexes = doc.blocks.indices.filter { doc.blocks[it].type == BlockType.REQUIREMENT }
        assertEquals(reqIndexes, doc.stops)
        assertEquals(reqIndexes[0], doc.next(0))
        assertEquals(reqIndexes[1], doc.next(reqIndexes[0]))
        assertEquals(reqIndexes[0], doc.previous(reqIndexes[1]))
        assertNull(doc.previous(reqIndexes[0]))
        assertTrue(doc.outline.any { it.label == "Reconnect restarts the reply" && it.delta == DeltaKind.REMOVED })
        assertEquals(doc.blocks.indexOfFirst { it.type == BlockType.CAPABILITY && it.content.text == "spec-viewer" }, doc.indexOf("spec-viewer"))
        assertEquals(reqIndexes[1], doc.indexOf("chats are long-lived and resumable"))
    }

    @Test
    fun `find matches case-insensitively in reading order`() {
        val doc = specDoc()
        val m = doc.find("shall")
        assertTrue(m.size >= 4)
        assertEquals(m.sortedWith(compareBy({ it.block }, { it.start })), m)
        m.forEach { assertEquals("shall", doc.blocks[it.block].content.text.substring(it.start, it.end).lowercase()) }
        assertTrue(doc.find("  ").isEmpty())
    }

    @Test
    fun `asking about a selection in a requirement attaches that requirement`() {
        val doc = specDoc()
        val body = doc.blocks.indexOfFirst { it.requirement == "A dropped stream resumes from the last event" && it.type == BlockType.PARAGRAPH }
        val ref = doc.reference(body, " SHALL NOT show an event twice ")
        assertEquals("requirement", ref.kind)
        assertEquals("A dropped stream resumes from the last event", ref.label)
        assertEquals("openspec/changes/offline-chat-resume/specs/agent-chat/spec.md#A dropped stream resumes from the last event", ref.id)
        assertEquals("SHALL NOT show an event twice", ref.text)
    }

    @Test
    fun `asking outside a requirement attaches the selection or the document`() {
        val doc = ReaderDocument.build(DocKind.PROPOSAL, "offline-chat-resume", listOf(ChangeArtifact("openspec/changes/offline-chat-resume/proposal.md", "# Proposal\n\nWhy it matters.")))
        val sel = doc.reference(1, "it matters")
        assertEquals("selection", sel.kind)
        assertEquals("offline-chat-resume · Proposal", sel.label)
        assertEquals("it matters", sel.text)
        val whole = doc.reference(0, "")
        assertEquals("document", whole.kind)
        assertEquals("openspec/changes/offline-chat-resume/proposal.md", whole.id)
        // Elsewhere, Next and Previous go section by section.
        assertTrue(doc.stops.isEmpty())
    }

    @Test
    fun `a document the change doesn't have is empty`() {
        assertTrue(ReaderDocument.build(DocKind.DESIGN, "x", listOf(ChangeArtifact("openspec/changes/x/proposal.md", "# P"))).isEmpty)
    }

    @Test
    fun `search finds requirement changes by name`() {
        val changes = listOf(ChangeSummary(id = "c1", changeName = "offline-chat-resume", phase = "awaiting_approval", seedIds = listOf("NAC-1")))
        val deltas = mapOf("c1" to listOf(SpecDeltas.parse("agent-chat", agentChat)))
        val model = SpecList.build(changes, query = "resumes", deltas = deltas)
        assertEquals(listOf(RequirementHit("c1", "offline-chat-resume", "agent-chat", "A dropped stream resumes from the last event", DeltaKind.ADDED)), model.requirements)
        assertEquals(listOf("Chat destination"), SpecList.build(changes, query = "chat tab", deltas = deltas).requirements.map { it.requirement })
        assertTrue(SpecList.build(changes, deltas = deltas).requirements.isEmpty())
    }
}
