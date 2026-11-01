package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GoalOptionsTest {
    @Test fun firstLaunchHasNoGoalOrGitChoices() {
        assertEquals(GoalOptions(), AppSettings().runGoal)
    }

    @Test fun pushedRequiresCommittedAndDisablingGoalKeepsChildChoices() {
        val push = GoalOptions(enabled = true).withPushed(true)
        assertTrue(push.committed)
        assertTrue(push.pushed)
        assertEquals(GoalOptions(enabled = true), push.withCommitted(false))
        assertEquals(push, push.copy(enabled = false).copy(enabled = true))
        assertFalse(push.withPushed(false).pushed)
        assertTrue(push.withPushed(false).committed)
    }

    @Test fun preferencesAndBothRequestsPreserveEveryChoice() {
        val json = BridgeClient.json
        for (enabled in listOf(false, true)) for (committed in listOf(false, true)) for (pushed in listOf(false, true)) {
            val goal = GoalOptions(enabled, committed, pushed).normalized()
            assertEquals(goal, json.decodeFromString<GoalOptions>(json.encodeToString(GoalOptions.serializer(), goal)))
            for (agent in listOf(null, AGENT_CODEX)) {
                val req = RunRequest(goal = goal, agent = agent)
                assertEquals(goal, json.decodeFromString<RunRequest>(json.encodeToString(RunRequest.serializer(), req)).goal)
            }
            val round = StartRound(beads = listOf("X-1", "X-2"), goal = goal)
            assertEquals(goal, json.decodeFromString<StartRound>(json.encodeToString(StartRound.serializer(), round)).goal)
        }
    }
}
