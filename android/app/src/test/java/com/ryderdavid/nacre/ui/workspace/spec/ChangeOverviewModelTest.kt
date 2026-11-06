package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_RUNNING
import com.ryderdavid.nacre.data.model.ChangeArtifact
import com.ryderdavid.nacre.data.model.ChangeEvidence
import com.ryderdavid.nacre.data.model.ChangeFinding
import com.ryderdavid.nacre.data.model.ChangeHistory
import com.ryderdavid.nacre.data.model.ChangeSection
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ROUND_IMPLEMENTING
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundBead
import com.ryderdavid.nacre.data.model.SectionSummary
import com.ryderdavid.nacre.data.model.SpecChange
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ChangeOverviewModelTest {
    private val sections = listOf(
        ChangeSection("s1", "Event ids on every chat event", "- [ ] 1.1 a\n- [x] 1.2 b\n- [ ] 1.3 c", "NAC-axe.1"),
        ChangeSection("s2", "Resume from the last event", "- [ ] 2.1 a\n- [ ] 2.2 b\n- [ ] 2.3 c", "NAC-axe.2"),
        ChangeSection("s3", "Drafts survive a reconnect", "- [ ] 3.1 a\n- [ ] 3.2 b", "NAC-axe.4"),
        ChangeSection("s4", "Manual device check", "- [ ] 4.1 a", "NAC-axe.5"),
    )

    private fun change(phase: String = "ready", revision: String = "rev4abcdef01", approved: String = revision, findings: List<ChangeFinding> = emptyList(), history: List<ChangeHistory> = emptyList(), evidence: List<ChangeEvidence> = emptyList()) =
        SpecChange(
            id = "chg-1", seedId = "NAC-axe", epicId = "NAC-axe", changeName = "offline-chat-resume", phase = phase,
            revision = revision, approvedRevision = approved, summary = "Keep agentic chats alive through brief drops",
            artifacts = listOf(
                ChangeArtifact("openspec/changes/offline-chat-resume/tasks.md", ""),
                ChangeArtifact("openspec/changes/offline-chat-resume/proposal.md", ""),
                ChangeArtifact("openspec/changes/offline-chat-resume/specs/agent-chat/spec.md", ""),
                ChangeArtifact("openspec/changes/offline-chat-resume/specs/chat-presentation/spec.md", ""),
            ),
            findings = findings, sections = sections, history = history, evidence = evidence,
        )

    private fun summary(vararg evidence: String) = ChangeSummary(
        id = "chg-1", changeName = "offline-chat-resume", phase = "ready", revision = "rev4abcdef01", approvedRevision = "rev4abcdef01",
        seedIds = listOf("NAC-axe"), sections = sections.mapIndexed { i, s -> SectionSummary(s.id, s.title, s.beadId, evidence[i]) },
    )

    private val stoppedRound = Round(
        id = "rdto3k", state = ROUND_IMPLEMENTING, createdAt = "2026-11-05T10:00:00Z",
        beads = listOf(RoundBead("NAC-axe.2", state = BEAD_PARKED, reason = "Gates failed: unit tests", runId = "run-9")),
    )

    @Test
    fun `the approved plan sits beside live progress from beads`() {
        val o = ChangeOverviewModel.build(change(), summary("landed", "in_round", "landed", "none"), listOf(stoppedRound))
        assertTrue(o.approved)
        assertEquals("Plan, as approved", o.planTitle)
        assertEquals(listOf(SectionState.LANDED, SectionState.FAILED, SectionState.LANDED, SectionState.WAITING), o.plan.map { it.state })
        assertEquals(SectionProgress(landed = 2, verified = 0, working = 0, failed = 1, total = 4), o.progress)
        assertEquals(listOf(3, 3, 2, 1), o.plan.map { it.tasks })
        assertEquals("3 tasks · NAC-axe.1", o.plan[0].subtitle)
    }

    @Test
    fun `a failed section shows Failed and its evidence opens that round`() {
        val o = ChangeOverviewModel.build(change(), summary("landed", "in_round", "landed", "none"), listOf(stoppedRound))
        val failed = o.plan[1]
        assertEquals(SectionState.FAILED, failed.state)
        assertEquals(EvidenceLink(EvidenceKind.ROUND, "rdto3k"), failed.link)
        assertEquals("Gates failed: unit tests", failed.note)
        assertTrue(EvidenceLink(EvidenceKind.ROUND, "rdto3k") in o.evidence)
    }

    @Test
    fun `a landed round no longer makes its parked bead fail`() {
        val landed = stoppedRound.copy(state = ROUND_LANDED)
        val o = ChangeOverviewModel.build(change(), summary("landed", "none", "landed", "none"), listOf(landed))
        assertEquals(SectionState.WAITING, o.plan[1].state)
        // Its round is still evidence for the section.
        assertEquals(EvidenceLink(EvidenceKind.ROUND, "rdto3k"), o.plan[1].link)
    }

    @Test
    fun `a running bead is in a round, and a verified one with a human check needs Ryder`() {
        val running = Round(id = "r2", state = ROUND_IMPLEMENTING, beads = listOf(RoundBead("NAC-axe.5", state = BEAD_RUNNING)))
        val evidence = listOf(ChangeEvidence("s3", "NAC-axe.4", runId = "run-3", roundId = "r1", verified = true, humanCheck = true))
        val o = ChangeOverviewModel.build(change(evidence = evidence), summary("landed", "none", "verified", "none"), listOf(running))
        assertEquals(SectionState.CHECK, o.plan[2].state)
        assertEquals(EvidenceLink(EvidenceKind.ROUND, "r1"), o.plan[2].link)
        assertEquals(SectionState.WORKING, o.plan[3].state)
    }

    @Test
    fun `before approval the plan has no progress and the change waits for Ryder`() {
        val o = ChangeOverviewModel.build(change(phase = "awaiting_approval", approved = ""), null)
        assertFalse(o.approved)
        assertEquals("Plan, as proposed", o.planTitle)
        assertTrue(o.plan.all { it.state == null })
        assertNull(o.progress)
        assertTrue(o.waiting)
        assertEquals(SpecGroup.WAITING, o.group)
        assertEquals("Revision rev4abc, not approved yet", o.revisionText)
    }

    @Test
    fun `a stale change says why`() {
        val o = ChangeOverviewModel.build(change(phase = "stale", revision = "rev5", approved = "rev4").copy(staleReason = "agent-chat changed on main."), null)
        assertEquals("agent-chat changed on main. A new review is needed before its sections can run.", o.stale)
        assertTrue(o.waiting)
        assertEquals("Revision rev5 · approved rev4 before it changed", o.revisionText)
    }

    @Test
    fun `the review verdict and its findings`() {
        val findings = listOf(
            ChangeFinding("f1", "medium", "Name the retry limit", disposition = "resolved"),
            ChangeFinding("f2", "low", "Typo in design", disposition = "accepted"),
        )
        val history = listOf(ChangeHistory("t1", "review", "round 1"), ChangeHistory("t2", "review", "round 2"))
        val passed = ChangeOverviewModel.build(change(findings = findings, history = history), summary("landed", "landed", "landed", "none"))
        assertEquals("Passed", passed.review.verdict)
        assertEquals(ReviewTone.PASSED, passed.review.tone)
        assertEquals("2 notes, after 2 rounds", passed.review.detail)
        assertEquals("3 of 4 sections landed · review passed, 2 notes, after 2 rounds", passed.summaryLine)

        val blocked = ChangeOverviewModel.review(change(phase = "needs_attention", findings = listOf(ChangeFinding("f3", "blocker", "Data loss on resume", disposition = "unresolved", needsOwner = true))))
        assertEquals("Changes requested", blocked.verdict)
        assertEquals(1, blocked.blocking)
        assertEquals("Blocker", blocked.findings.single().severity)
        assertEquals("Open", blocked.findings.single().disposition)
        assertTrue(blocked.findings.single().needsOwner)

        assertEquals("Review under way", ChangeOverviewModel.review(change(phase = "reviewing")).verdict)
        assertEquals("Not reviewed yet", ChangeOverviewModel.review(change(phase = "drafting")).verdict)
    }

    @Test
    fun `documents follow the selector's order whatever order the artifacts come in`() {
        val o = ChangeOverviewModel.build(change(), summary("none", "none", "none", "none"))
        assertEquals(listOf(DocKind.PROPOSAL, DocKind.TASKS, DocKind.SPECS), o.documents)
    }

    @Test
    fun `evidence lists rounds, runs and beads once each`() {
        val evidence = listOf(
            ChangeEvidence("s1", "NAC-axe.1", runId = "run-1", roundId = "rdto3k", landed = true, verified = true),
            ChangeEvidence("s3", "NAC-axe.4", runId = "run-3", roundId = "rdto3k", landed = true, verified = true),
        )
        val o = ChangeOverviewModel.build(change(evidence = evidence), summary("landed", "in_round", "landed", "none"), listOf(stoppedRound))
        assertEquals(
            listOf(
                EvidenceLink(EvidenceKind.ROUND, "rdto3k"), EvidenceLink(EvidenceKind.RUN, "run-1"), EvidenceLink(EvidenceKind.RUN, "run-3"),
                EvidenceLink(EvidenceKind.BEAD, "NAC-axe.1"), EvidenceLink(EvidenceKind.BEAD, "NAC-axe.2"), EvidenceLink(EvidenceKind.BEAD, "NAC-axe.4"), EvidenceLink(EvidenceKind.BEAD, "NAC-axe.5"),
            ),
            o.evidence,
        )
    }

    @Test
    fun `without the board's summary the record's own evidence decides`() {
        val evidence = listOf(ChangeEvidence("s1", "NAC-axe.1", runId = "run-1", roundId = "r1", roundState = "landed", landed = true, verified = true))
        val o = ChangeOverviewModel.build(change(evidence = evidence), null)
        assertEquals(SectionState.LANDED, o.plan[0].state)
        assertEquals(SectionState.WAITING, o.plan[1].state)
    }

    @Test
    fun `the headline is the summary's first sentence without markdown`() {
        val summary = "Let users attach **images** to `bead` chats. More detail follows.\n\n- **Storage:** on the phone"
        kotlin.test.assertEquals("Let users attach images to bead chats.", headlineOf(summary))
        kotlin.test.assertTrue(headlineOf("x".repeat(400)).length <= 110)
    }

    // NAC-pjq.8: a stale change whose round landed shows its sections landed, as the round says, not Waiting.
    @Test
    fun `sections a landed round merged count as landed while the change is stale`() {
        val note = "Review and approve the current revision before its implementation can be verified. Round rdtn: landed."
        val evidence = sections.map { s -> ChangeEvidence(s.id, s.beadId, roundId = "rdtn", roundState = "landed", merged = s.id != "s4", note = note) }
        val stale = change(phase = "stale", evidence = evidence)
        val o = ChangeOverviewModel.build(stale, summary("none", "none", "none", "none").copy(phase = "stale"))
        assertEquals(listOf(SectionState.LANDED, SectionState.LANDED, SectionState.LANDED, SectionState.WAITING), o.plan.map { it.state })
        assertEquals(3, o.progress!!.landed)
        assertTrue(o.summaryLine, o.summaryLine.startsWith("3 of 4 sections landed"))
    }

    @Test
    fun `an older nacred's landed round still counts from the round list`() {
        val landed = Round(
            id = "rdtn", state = ROUND_LANDED, createdAt = "2026-11-03T07:09:27Z", landedCommit = "e5ebe5a",
            beads = sections.map { RoundBead(it.beadId, specChangeId = "chg-1", specRevision = "rev4abcdef01", state = BEAD_INTEGRATED, mergeCommit = "m" + it.id) },
        )
        val o = ChangeOverviewModel.build(change(phase = "stale"), summary("none", "none", "none", "none").copy(phase = "stale"), listOf(landed))
        assertTrue(o.plan.all { it.state == SectionState.LANDED })
        assertEquals("4 of 4 sections landed", o.progress!!.headline)
        // A round for an older revision of the change doesn't count.
        val older = landed.copy(beads = landed.beads.map { it.copy(specRevision = "rev3") })
        val o2 = ChangeOverviewModel.build(change(phase = "stale"), summary("none", "none", "none", "none").copy(phase = "stale"), listOf(older))
        assertTrue(o2.plan.none { it.state == SectionState.LANDED })
    }

    // NAC-pjq.3: an untracked change's plan and progress come from its tasks.
    @Test
    fun `an untracked change shows progress from its tasks`() {
        val tasks = listOf(ChangeSection("tasks-1", "1. First", "- [x] 1.1 a"), ChangeSection("tasks-2", "2. Device checks", "- [ ] 2.1 b"))
        val c = SpecChange(id = "chat-first-harness", seedId = "", changeName = "chat-first-harness", phase = "ready", revision = "abc", sections = tasks, untracked = true)
        val s = ChangeSummary(
            id = "chat-first-harness", changeName = "chat-first-harness", phase = "ready", untracked = true, tasksDone = 1, tasksTotal = 2, hasReview = true,
            sections = listOf(SectionSummary("tasks-1", "1. First", evidence = "landed"), SectionSummary("tasks-2", "2. Device checks", evidence = "none")),
        )
        val o = ChangeOverviewModel.build(c, s)
        assertFalse(o.approved)
        assertEquals("Plan, from tasks.md", o.planTitle)
        assertEquals("Progress from tasks.md", o.progressLabel)
        assertEquals(listOf(SectionState.DONE, SectionState.WAITING), o.plan.map { it.state })
        assertEquals("1 of 2 sections done", o.progress!!.headline)
        assertEquals(SpecGroup.IN_PROGRESS, o.group)
        assertEquals("Not tracked by Nacre · 1 of 2 tasks done", o.status)
        assertEquals("Not reviewed by Nacre", o.review.verdict)
        assertEquals(ReviewTone.NONE, o.review.tone)
    }
}
