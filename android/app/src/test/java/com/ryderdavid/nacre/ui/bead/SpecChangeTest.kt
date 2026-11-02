package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.ChangeEvidence
import com.ryderdavid.nacre.data.model.ChangeSection
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.data.model.StartRound
import com.ryderdavid.nacre.ui.rounds.roundCandidates
import com.ryderdavid.nacre.ui.rounds.specificationCandidates
import com.ryderdavid.nacre.ui.rounds.specificationUnavailable
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SpecChangeTest {
    private val approved = SpecChange(
        id = "flow1", seedId = "NAC-1", epicId = "NAC-E", phase = "ready", revision = "exact-content", approvedRevision = "exact-content",
        sections = listOf(ChangeSection("stable-A", "Interface", beadId = "NAC-1.2"), ChangeSection("stable-B", "Verification", beadId = "NAC-1.1")),
    )

    @Test fun bridgePayloadPreservesDecisionHistoryAndNullableGoCollections() {
        val raw = """{"available":true,"change":{"id":"flow1","seed_id":"NAC-1","epic_id":"NAC-1","phase":"awaiting_approval","revision":"digest-v2","route":"amend","references":["openspec/changes/flow/proposal.md"],"artifacts":[{"path":"proposal.md","content":"# Intent\nPreserve drafts"}],"sections":[{"id":"opaque-1","title":"1. Interaction","bead_id":"NAC-1.1","depends_on":null}],"findings":[{"id":"R1","severity":"blocking","summary":"Recovery missing","detail":"Survive closing","disposition":"resolved"}],"history":[{"at":"2026-10-28T05:00:00Z","kind":"review","text":"Independent review accepted"}],"retired_sections":null,"evidence":null,"future_field":true}}"""
        val change = BridgeClient.json.decodeFromString<ChangeEnvelope>(raw).change!!
        assertEquals("digest-v2", change.revision)
        assertEquals("resolved", change.findings.single().disposition)
        assertEquals("Independent review accepted", change.history.single().text)
        assertEquals(emptyList(), change.sections.single().dependsOn)
        assertEquals(emptyList(), change.evidence)
        assertEquals("# Intent\nPreserve drafts", change.artifacts.single().content)
        assertEquals("approve", changeNextAction(change)?.action)
        assertTrue(change.implementationIds.isEmpty())
    }

    @Test fun staleAndPartialApprovalsCannotImplement() {
        assertEquals(listOf("NAC-1.2", "NAC-1.1"), approved.implementationIds)
        val stale = approved.copy(approvedRevision = "old-digest")
        assertFalse(stale.hasCurrentApproval)
        assertEquals("refresh", changeNextAction(stale)?.action)
        assertTrue(approved.copy(phase = "stale").implementationIds.isEmpty())
        assertTrue(approved.copy(sections = approved.sections + ChangeSection("unmapped", "Pending")).implementationIds.isEmpty())
        assertTrue(approved.copy(sections = approved.sections.map { it.copy(beadId = "same") }).implementationIds.isEmpty())
    }

    @Test fun closedCheckboxesAndUnmatchedEvidenceDoNotVerifySections() {
        val evidence = listOf(
            ChangeEvidence("stable-A", "NAC-1.2", verified = true), // no actual run
            ChangeEvidence("stable-B", "wrong-bead", runId = "run0", verified = true),
            ChangeEvidence("retired-section", "NAC-1.1", runId = "run1", verified = true),
        )
        assertEquals(0, approved.copy(evidence = evidence).verifiedSections)
        val valid = approved.copy(evidence = approved.sections.map { ChangeEvidence(it.id, it.beadId, runId = "run-${it.id}", roundId = "round1", verified = true, buildVerified = true, humanCheck = true) })
        assertEquals(2, valid.verifiedSections)
        assertEquals("View progress", changeNextAction(valid)?.label)
        assertEquals("Working", changeStatusTitle(valid))
        assertEquals(0, valid.copy(phase = "stale").verifiedSections)
        assertEquals("revise", changeNextAction(valid.copy(phase = "stale"))?.action)
    }

    @Test fun passingIndividualRunsDoNotEstablishAFinalVerifiedBuild() {
        val runsOnly = approved.copy(evidence = approved.sections.map { ChangeEvidence(it.id, it.beadId, runId = "run-${it.id}", roundId = "round1", verified = true) })
        assertEquals(2, runsOnly.verifiedSections)
        assertEquals(0, runsOnly.buildVerifiedSections)
        assertFalse(runsOnly.implementationReadyForReview)
        assertEquals("Working", changeStatusTitle(runsOnly))
        assertEquals("review", changeNextAction(runsOnly)?.action)
    }

    @Test fun activeRoundAndInterruptedPromotionKeepTheExistingLoopMoving() {
        val active = approved.copy(evidence = listOf(ChangeEvidence("stable-A", "NAC-1.2", roundId = "round-existing", roundState = "needs_decision")))
        assertEquals("round-existing", active.activeRoundId)
        assertEquals("View progress", changeNextAction(active)?.label)
        assertEquals("review", changeNextAction(active)?.action)
        assertEquals("review", changeNextAction(active, "Keep this for later")?.action)
        assertEquals("revise", changeNextAction(approved, "Change the interaction")?.action)
        val stalePaused = active.copy(phase = "stale", evidence = active.evidence.map { it.copy(roundState = "paused") })
        assertEquals("round-existing", stalePaused.activeRoundId)
        assertEquals("review", changeNextAction(stalePaused, "Revise after this round")?.action)
        assertEquals(0, stalePaused.verifiedSections)
        assertEquals("", approved.copy(evidence = active.evidence.map { it.copy(roundState = "cancelled") }).activeRoundId)
        assertEquals("resume", changeNextAction(approved.copy(phase = "interrupted", error = "A child was created but the decision comment needs retrying."))?.action)
    }

    @Test fun approvedRoundContainsOnlyCurrentSectionMappingsInSpecOrder() {
        val board = BoardIndex(listOf(
            Issue("unrelated", "Other work"),
            Issue("NAC-1.1", "Verification", labels = listOf("spec-section:stable-B")),
            Issue("NAC-1.2", "Interface", labels = listOf("specflow:flow1")),
        ))
        assertEquals(listOf("NAC-1.2", "NAC-1.1"), specificationCandidates(board, approved).map { it.id })
        assertEquals(listOf("unrelated"), roundCandidates(board).map { it.id })
        assertNull(specificationUnavailable(board, approved))
        assertTrue(specificationUnavailable(BoardIndex(board.issues.dropLast(1)), approved)!!.contains("aren't loaded"))
    }

    @Test fun aLaterRoundOmitsClosedSectionsWithoutTreatingClosureAsVerification() {
        val partial = BoardIndex(listOf(Issue("NAC-1.2", "Landed section", status = "closed"), Issue("NAC-1.1", "Remaining section")))
        assertEquals(listOf("NAC-1.1"), specificationCandidates(partial, approved).map { it.id })
        assertNull(specificationUnavailable(partial, approved))
        val closed = BoardIndex(partial.issues.map { it.copy(status = "closed") })
        assertTrue(specificationCandidates(closed, approved).isEmpty())
        assertTrue(specificationUnavailable(closed, approved)!!.contains("reopen"))
        assertFalse(approved.implementationReadyForReview)
    }

    @Test fun approvalAndExecutionCarryExactRevisionAndStartWork() {
        val action = BridgeClient.json.encodeToString(ChangeAction("approve", "digest-reviewed", "keep navigation", run = true))
        assertTrue(action.contains("\"revision\":\"digest-reviewed\""))
        assertTrue(action.contains("\"run\":true"))
        assertEquals("Approve and run", changeNextAction(approved.copy(phase = "awaiting_approval"), "some feedback")?.label)
        val round = BridgeClient.json.encodeToString(StartRound(beads = approved.implementationIds, budgetUsd = 5.0, specChangeId = approved.id, specRevision = approved.revision))
        assertTrue(round.contains("\"spec_change_id\":\"flow1\""))
        assertTrue(round.contains("\"spec_revision\":\"exact-content\""))
        assertTrue(round.contains("\"budget_usd\":5.0"))
        assertEquals("approve", changeNextAction(approved.copy(phase = "awaiting_approval"))?.action)
        assertEquals("run", changeNextAction(approved)?.action)
        assertNull(changeNextAction(approved.copy(phase = "future_phase")))
        assertNull(changeNextAction(approved.copy(phase = "covered")))
        assertNull(changeNextAction(approved.copy(phase = "reviewing")))
    }

    @Test fun adoptableChangesDecodeAndAdoptedFinishedWorkOffersNoImplementation() {
        val raw = """{"available":true,"change":null,"adoptable":[{"change_name":"other-work","tasks_done":0,"tasks_total":3},{"change_name":"retain-drafts","why":"Drafts vanish","tasks_done":3,"tasks_total":4,"has_review":true,"suggested":true},{"change_name":"half-done","suggested":true,"problem":"proposal is missing tasks.md"}]}"""
        val envelope = BridgeClient.json.decodeFromString<ChangeEnvelope>(raw)
        assertEquals(3, envelope.adoptable.size)
        assertEquals("retain-drafts", envelope.suggestedAdoption?.changeName)
        assertTrue(envelope.suggestedAdoption!!.hasReview)

        val request = BridgeClient.json.encodeToString(ChangeAction.serializer(), ChangeAction("adopt", changeName = "retain-drafts", recordedReview = true))
        assertTrue("\"change_name\":\"retain-drafts\"" in request && "\"recorded_review\":true" in request, request)

        val landed = approved.copy(route = "adopted", evidence = approved.sections.map { ChangeEvidence(it.id, it.beadId, landed = true, verified = true, buildVerified = true) })
        assertTrue(landed.allLanded)
        assertEquals("epic", changeNextAction(landed)?.action)
        assertEquals("Adopted as written", changeRouteLabel(landed.route))
        val partly = landed.copy(evidence = landed.evidence.take(1))
        assertFalse(partly.allLanded)
        assertEquals("run", changeNextAction(partly)?.action)
    }

    @Test fun workUntilDoneAndEpicFinishingTravelOverTheBridge() {
        val body = BridgeClient.json.encodeToString(StartRound.serializer(), StartRound(beads = listOf("NAC-1"), landWhenReady = true))
        assertTrue("\"land_when_ready\":true" in body, body)
        val epicRound = BridgeClient.json.encodeToString(StartRound.serializer(), StartRound(epic = "NAC-E", landWhenReady = true, finish = true))
        assertTrue("\"epic\":\"NAC-E\"" in epicRound, epicRound)
        assertTrue("\"finish\":true" in epicRound, epicRound)
        assertFalse("\"goal\"" in epicRound, epicRound)
        val finish = BridgeClient.json.decodeFromString<com.ryderdavid.nacre.data.model.EpicFinish>("""{"round":{"id":"r1","state":"integrating","target":"main","finish_epic":"NAC-E","beads":[]}}""")
        assertEquals("NAC-E", finish.round?.finishEpic)
        assertFalse(finish.closed)
        assertTrue(BridgeClient.json.decodeFromString<com.ryderdavid.nacre.data.model.EpicFinish>("""{"closed":true}""").closed)
        assertEquals("epic/nac-mp6", com.ryderdavid.nacre.data.model.epicBranch("NAC-mp6"))
        assertEquals("Finishing NAC-E: merging epic/nac-e into main", com.ryderdavid.nacre.ui.rounds.beadSummary(finish.round!!))
    }

    @Test fun anAdoptedChangeStoppedByItsReviewCanBeReviewedAgainAfterHandFixes() {
        val stopped = approved.copy(phase = "needs_attention", approvedRevision = "", adoption = com.ryderdavid.nacre.data.model.ChangeAdoption())
        assertEquals("resume", changeNextAction(stopped)?.action)
        assertEquals("revise", changeNextAction(stopped, feedback = "Add the failure path")?.action)
        assertEquals("revise", changeNextAction(stopped.copy(adoption = null))?.action)
        assertEquals("resume", changeNextAction(stopped.copy(phase = "stale"))?.action)
    }
    @Test fun phasesUseTheSingleWorkflowVocabulary() {
        listOf("drafting", "reviewing", "promoting").forEach { assertEquals("Planning", changePhaseLabel(it)) }
        assertEquals("Ready for your approval", changePhaseLabel("awaiting_approval"))
        listOf("needs_attention", "stale", "interrupted").forEach { assertEquals("Needs attention", changePhaseLabel(it)) }
        assertEquals("Ready", changeStatusTitle(approved))
        assertEquals("Working", changeStatusTitle(approved.copy(evidence = listOf(ChangeEvidence("stable-A", "NAC-1.2", roundId = "r", roundState = "implementing")))))
        assertEquals("Done", changeStatusTitle(approved.copy(phase = "archived")))
        assertNull(changeNextAction(approved.copy(phase = "awaiting_approval", revision = "")))
        assertNull(changeNextAction(approved.copy(phase = "archived")))
    }

    @Test fun epicActionsPreferAnActiveRoundThenRunOrFinish() {
        assertEquals("Running", epicPrimaryAction(2, 3, true))
        assertEquals("Running", epicPrimaryAction(0, 3, true))
        assertEquals("Run epic", epicPrimaryAction(2, 3, false))
        assertEquals("Finish epic", epicPrimaryAction(0, 3, false))
        assertNull(epicPrimaryAction(0, 0, false))
    }

    @Test fun machineLabelsAreHiddenAndClosingIsOnlyInOverflow() {
        listOf("specflow:flow1", "spec-section:A", "round:r", "nacre-worker").forEach { assertTrue(isMachineLabel(it)) }
        listOf("human", "ui", "found:flow1").forEach { assertFalse(isMachineLabel(it)) }
        assertFalse("closed" in STATUSES)
    }

}
