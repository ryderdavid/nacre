package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.ChangeDraft
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.CurrentSpec
import com.ryderdavid.nacre.data.model.SectionSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SpecListModelTest {
    private fun change(
        id: String, phase: String, updated: String = "2026-11-0${id.length % 9 + 1}T00:00:00Z",
        sections: List<SectionSummary> = emptyList(), draft: ChangeDraft? = null, revision: String = "",
    ) = ChangeSummary(id = id, changeName = id, phase = phase, revision = revision, seedIds = listOf("NAC-$id"), sections = sections, updatedAt = updated, draft = draft)

    private fun sections(vararg evidence: String) = evidence.mapIndexed { i, e -> SectionSummary("s$i", "Section $i", "NAC-b.$i", e) }

    @Test
    fun `changes are grouped by phase in lifecycle order`() {
        val model = SpecList.build(
            listOf(
                change("done-one", "ready", sections = sections("landed", "landed")),
                change("set-up-gates", "drafting"),
                change("chat-first", "awaiting_approval"),
                change("offline-resume", "ready", sections = sections("landed", "in_round", "none")),
                change("being-checked", "reviewing"),
                change("old", "archived"),
            ),
        )
        assertEquals(listOf(SpecGroup.DRAFTING, SpecGroup.WAITING, SpecGroup.IN_PROGRESS, SpecGroup.READY_TO_ARCHIVE), model.groups.map { it.first })
        assertEquals(setOf("set-up-gates", "being-checked"), model.groups[0].second.map { it.name }.toSet())
        assertEquals(listOf("chat-first"), model.groups[1].second.map { it.name })
        assertEquals(listOf("offline-resume"), model.groups[2].second.map { it.name })
        assertEquals(listOf("done-one"), model.groups[3].second.map { it.name })
        // Archived changes have become specs and aren't listed.
        assertEquals(5, model.changeCount)
    }

    @Test
    fun `everything that waits on Ryder is under Waiting for you`() {
        for (phase in listOf("awaiting_approval", "needs_attention", "stale", "interrupted", "covered")) {
            assertEquals(phase, SpecGroup.WAITING, SpecList.group(change("c", phase)))
        }
        assertEquals(SpecGroup.WAITING, SpecList.group(change("c", "drafting", draft = ChangeDraft(ChangeDraft.FAILED, "out of budget"))))
        assertEquals(SpecGroup.DRAFTING, SpecList.group(change("c", "drafting", draft = ChangeDraft(ChangeDraft.PENDING))))
        assertEquals(SpecGroup.IN_PROGRESS, SpecList.group(change("c", "promoting")))
        assertNull(SpecList.group(change("c", "archived")))
    }

    @Test
    fun `an approved change with nothing landed yet is in progress, not ready to archive`() {
        assertEquals(SpecGroup.IN_PROGRESS, SpecList.group(change("c", "ready")))
        assertEquals(SpecGroup.IN_PROGRESS, SpecList.group(change("c", "ready", sections = sections("landed", "verified"))))
        assertEquals(SpecGroup.READY_TO_ARCHIVE, SpecList.group(change("c", "ready", sections = sections("landed"))))
    }

    @Test
    fun `section progress comes from beads and a parked bead shows failed`() {
        val c = change("offline-resume", "ready", sections = sections("landed", "in_round", "landed", "landed", "none"))
        val p = SpecList.progress(c, failedBeads = setOf("NAC-b.1"))!!
        assertEquals(SectionProgress(landed = 3, verified = 0, working = 0, failed = 1, total = 5), p)
        assertEquals(1, p.waiting)
        assertEquals("3 of 5 sections landed · 1 failed", p.text)
        val row = SpecList.row(c, setOf("NAC-b.1"))!!
        assertEquals("3 of 5 sections landed · 1 failed", row.status)
    }

    @Test
    fun `no progress before approval`() {
        assertNull(SpecList.progress(change("c", "awaiting_approval", sections = sections("none", "none")), emptySet()))
    }

    @Test
    fun `waiting rows say what they wait on in words`() {
        assertEquals("Reviewed, waiting for your approval", SpecList.row(change("c", "awaiting_approval"), emptySet())!!.status)
        assertEquals("Drafting failed: out of budget", SpecList.row(change("c", "drafting", draft = ChangeDraft(ChangeDraft.FAILED, "out of budget")), emptySet())!!.status)
    }

    @Test
    fun `groups list the most recently updated first`() {
        val model = SpecList.build(
            listOf(
                change("older", "drafting", updated = "2026-11-01T00:00:00Z"),
                change("newer", "drafting", updated = "2026-11-05T00:00:00Z"),
            ),
        )
        assertEquals(listOf("newer", "older"), model.groups.single().second.map { it.name })
    }

    @Test
    fun `specs come from the changes' spec deltas, most recent change first`() {
        val changes = listOf(
            change("chat-first", "awaiting_approval", updated = "2026-11-06T00:00:00Z"),
            change("offline-resume", "ready", updated = "2026-11-01T00:00:00Z"),
        )
        val paths = mapOf(
            "chat-first" to listOf("openspec/changes/chat-first/proposal.md", "openspec/changes/chat-first/specs/agent-chat/spec.md", "openspec/changes/chat-first/specs/spec-viewer/spec.md"),
            "offline-resume" to listOf("openspec/changes/offline-resume/specs/agent-chat/spec.md"),
        )
        val model = SpecList.build(changes, paths, acceptedSpecs = 18)
        assertEquals(listOf("agent-chat", "spec-viewer"), model.specs.map { it.capability })
        assertEquals(listOf("chat-first", "offline-resume"), model.specs[0].changes)
        assertEquals("chat-first", model.specs[0].changeId)
        assertEquals("2 changes · 18 specs", model.countsText)
    }

    @Test
    fun `every current spec is listed, those an open change touches marked with it`() {
        val changes = listOf(change("chat-first", "awaiting_approval"))
        val paths = mapOf(
            "chat-first" to listOf(
                "openspec/changes/chat-first/specs/agent-chat/spec.md",
                "openspec/changes/chat-first/specs/owner-decisions/spec.md",
            ),
        )
        val current = listOf(
            CurrentSpec("agent-chat", listOf("Chat runs the full agent harness on the host"), "openspec/specs/agent-chat/spec.md"),
            CurrentSpec("beads-board", listOf("Lanes by status", "Drag to move"), "openspec/specs/beads-board/spec.md"),
        )
        val model = SpecList.build(changes, paths, acceptedSpecs = 18, current = current)
        assertTrue(model.allSpecs)
        assertEquals(listOf("agent-chat", "beads-board", "owner-decisions"), model.specs.map { it.capability })
        val (chat, board, decisions) = model.specs
        assertEquals("chat-first", chat.changeId)
        assertEquals(listOf("Chat runs the full agent harness on the host"), chat.requirements)
        // A spec no open change touches has no change to open; it shows its requirements.
        assertNull(board.changeId)
        assertEquals(2, board.requirements.size)
        // A capability the change adds isn't in openspec/specs yet.
        assertEquals(false, decisions.current)
        assertEquals(listOf("chat-first"), decisions.changes)
        // The inventory is the project's count.
        assertEquals("1 change · 2 specs", model.countsText)
    }

    @Test
    fun `search finds a current spec by one of its requirements`() {
        val current = listOf(CurrentSpec("beads-board", listOf("Lanes by status", "Drag to move")), CurrentSpec("agent-chat", listOf("Chat runs on the host")))
        assertEquals(listOf("beads-board"), SpecList.build(emptyList(), query = "drag", current = current).specs.map { it.capability })
    }

    @Test
    fun `an older computer without the inventory lists only the specs open changes touch`() {
        val changes = listOf(change("chat-first", "awaiting_approval"))
        val paths = mapOf("chat-first" to listOf("openspec/changes/chat-first/specs/agent-chat/spec.md"))
        val model = SpecList.build(changes, paths, acceptedSpecs = 18, current = null)
        assertEquals(false, model.allSpecs)
        assertEquals(listOf("agent-chat"), model.specs.map { it.capability })
        assertEquals("1 change · 18 specs", model.countsText)
    }

    @Test
    fun `capability comes only from a spec delta path`() {
        assertEquals("agent-chat", SpecList.capabilityOf("openspec/changes/x/specs/agent-chat/spec.md"))
        assertNull(SpecList.capabilityOf("openspec/changes/x/proposal.md"))
        assertNull(SpecList.capabilityOf("openspec/changes/x/specs/agent-chat/notes.md"))
    }

    @Test
    fun `search narrows changes and specs and keeps the counts`() {
        val changes = listOf(change("chat-first", "awaiting_approval"), change("set-up-gates", "drafting"))
        val paths = mapOf("chat-first" to listOf("openspec/changes/chat-first/specs/spec-viewer/spec.md"))
        val byName = SpecList.build(changes, paths, query = "GATES")
        assertEquals(listOf("set-up-gates"), byName.rows.map { it.name })
        assertTrue(byName.specs.isEmpty())
        assertEquals(2, byName.changeCount)

        val bySpec = SpecList.build(changes, paths, query = "viewer")
        assertTrue(bySpec.rows.isEmpty())
        assertEquals(listOf("spec-viewer"), bySpec.specs.map { it.capability })

        val byGroup = SpecList.build(changes, paths, query = "waiting")
        assertEquals(listOf("chat-first"), byGroup.rows.map { it.name })

        assertTrue(SpecList.build(changes, paths, query = "nothing like it").isEmpty)
    }

    @Test
    fun `an Open change target resolves by id, name or bead`() {
        val changes = listOf(change("chg-1", "ready").copy(changeName = "offline-resume", epicId = "NAC-epic"))
        assertEquals("chg-1", SpecList.resolveTarget("chg-1", changes))
        assertEquals("chg-1", SpecList.resolveTarget("offline-resume", changes))
        assertEquals("chg-1", SpecList.resolveTarget("NAC-chg-1", changes))
        assertEquals("chg-1", SpecList.resolveTarget("NAC-epic", changes))
        assertEquals("unknown", SpecList.resolveTarget("unknown", changes))
    }

    @Test
    fun `revisions are shortened and the change opens through its seed`() {
        val c = change("c", "ready", revision = "0123456789abcdef")
        assertEquals("0123456", SpecList.row(c, emptySet())!!.revision)
        assertEquals("NAC-c", SpecList.openId(c))
        assertEquals("NAC-epic", SpecList.openId(c.copy(seedIds = emptyList(), epicId = "NAC-epic")))
    }

    // NAC-pjq.3: the Spec panel lists the checkout's changes nacred has no record for, by phase.
    @Test
    fun `the checkout's untracked changes are listed by phase`() {
        val list = com.ryderdavid.nacre.data.model.ChangesList(
            changes = listOf(change("chat-image-attachments", "stale", revision = "r1").copy(missing = true)),
            untracked = listOf(
                ChangeSummary(id = "chat-first-harness", changeName = "chat-first-harness", phase = "ready", untracked = true, tasksDone = 38, tasksTotal = 40,
                    sections = sections("landed", "none").map { it.copy(beadId = "") }),
                ChangeSummary(id = "set-up-gates", changeName = "set-up-gates", phase = "ready", untracked = true, tasksDone = 4, tasksTotal = 4,
                    sections = sections("landed").map { it.copy(beadId = "") }),
                ChangeSummary(id = "drafting-settles-its-findings", changeName = "drafting-settles-its-findings", phase = "awaiting_approval", untracked = true, tasksTotal = 5, hasReview = true),
                ChangeSummary(id = "pending-proposal-placeholders", changeName = "pending-proposal-placeholders", phase = "awaiting_approval", untracked = true, tasksTotal = 13,
                    archivedAs = "2026-11-02-pending-proposal-placeholders"),
            ),
        )
        val model = SpecList.build(list.forSpec)
        assertEquals("5 changes", model.countsText)
        val byName = model.rows.associateBy { it.name }
        assertEquals(SpecGroup.IN_PROGRESS, byName.getValue("chat-first-harness").group)
        assertEquals("1 of 2 sections done", byName.getValue("chat-first-harness").progress!!.headline)
        assertEquals(SpecGroup.READY_TO_ARCHIVE, byName.getValue("set-up-gates").group)
        assertEquals("Every task checked · not tracked by Nacre", byName.getValue("set-up-gates").status)
        assertEquals(SpecGroup.WAITING, byName.getValue("drafting-settles-its-findings").group)
        assertEquals("Written outside Nacre, nothing started · has its own review", byName.getValue("drafting-settles-its-findings").status)
        assertTrue(byName.getValue("pending-proposal-placeholders").status.startsWith("A copy of archived 2026-11-02-pending-proposal-placeholders"))
        assertEquals("Its folder is gone from the checkout", byName.getValue("chat-image-attachments").status)
        // The board's own list stays the recorded changes.
        assertEquals(listOf("chat-image-attachments"), list.changes.map { it.id })
    }
}
