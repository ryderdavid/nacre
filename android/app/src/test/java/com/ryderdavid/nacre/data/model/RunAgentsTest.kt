package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** The implement screen's agent switch: what each agent offers and how flipping it resets. */
class RunAgentsTest {
    @Test fun claudeIsFirstAndTheDefault() {
        assertEquals(listOf("Claude", "Codex"), RUN_AGENTS.map { it.label })
        assertEquals(CLAUDE_AGENT, runAgent(null))
        assertEquals(CODEX_AGENT, runAgent(AGENT_CODEX))
    }

    @Test fun eachAgentListsItsFourFamiliesAtTheirLatestVersion() {
        assertEquals(listOf("Fable 5.1", "Opus 5.5", "Sonnet 5.5", "Haiku 5.5"), CLAUDE_AGENT.models.map { it.label })
        assertEquals(listOf("claude-fable-5-1", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"), CLAUDE_AGENT.models.map { it.id })
        assertEquals(listOf("GPT-6 Astra", "GPT-6.1 Sol", "GPT-6.1 Terra", "GPT-6 Luna"), CODEX_AGENT.models.map { it.label })
        assertEquals(listOf("gpt-6-astra", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-luna"), CODEX_AGENT.models.map { it.id })
    }

    @Test fun effortLevelsAreTheAgentsOwn() {
        CLAUDE_AGENT.models.forEach { assertEquals(listOf("low", "medium", "high", "xhigh", "max"), it.efforts) }
        assertEquals(listOf("low", "medium", "high", "xhigh", "max", "ultra"), CODEX_AGENT.model("gpt-6.1-sol").efforts)
        assertEquals(listOf("low", "medium", "high", "xhigh", "max"), CODEX_AGENT.model("gpt-6-luna").efforts)
        (CLAUDE_AGENT.models + CODEX_AGENT.models).forEach { m -> m.efforts.forEach { assertTrue(it in EFFORT_LABELS, it) } }
    }

    @Test fun defaults() {
        assertEquals(AgentChoice(CLAUDE_AGENT, "claude-opus-5-5", "medium"), AgentChoice.defaults(CLAUDE_AGENT))
        assertEquals(AgentChoice(CODEX_AGENT, "gpt-6.1-sol", "high"), AgentChoice.defaults(CODEX_AGENT))
    }

    @Test fun flippingTheSwitchForgetsEarlierChoices() {
        val picked = AgentChoice.defaults(CLAUDE_AGENT).withModel("claude-sonnet-5-5").withEffort("high")
        assertEquals(AgentChoice(CLAUDE_AGENT, "claude-sonnet-5-5", "high"), picked)
        // The sheet flips by starting over from the agent's defaults: nothing of `picked` carries.
        assertEquals(AgentChoice(CODEX_AGENT, "gpt-6.1-sol", "high"), AgentChoice.defaults(CODEX_AGENT))
        assertEquals(AgentChoice(CLAUDE_AGENT, "claude-opus-5-5", "medium"), AgentChoice.defaults(CLAUDE_AGENT))
    }

    @Test fun theOtherAgentsModelsAndEffortsCannotBePicked() {
        val codex = AgentChoice.defaults(CODEX_AGENT)
        assertEquals("gpt-6.1-sol", codex.withModel("claude-sonnet-5-5").model)
        assertEquals("medium", AgentChoice.defaults(CLAUDE_AGENT).withEffort("ultra").effort)
        // Ultra drops back to the default on a model that doesn't take it.
        assertEquals("high", codex.withEffort("ultra").withModel("gpt-6-luna").effort)
        assertEquals("max", codex.withEffort("max").withModel("gpt-6-luna").effort)
    }

    @Test fun theRequestCarriesTheSwitchedAgentAndItsOptions() {
        val json = BridgeClient.json
        assertEquals(
            """{"mode":"auto","model":"claude-opus-5-5","effort":"medium","worktree":true}""",
            json.encodeToString(RunRequest.serializer(), AgentChoice.defaults(CLAUDE_AGENT).request("auto", null, true)),
        )
        val codex = AgentChoice.defaults(CODEX_AGENT).request("plan", "be brief", false)
        assertEquals(AGENT_CODEX, codex.agent)
        assertEquals("gpt-6.1-sol", codex.model)
        assertEquals("high", codex.effort)
        assertNull(AgentChoice.defaults(CLAUDE_AGENT).request("auto", null, false).agent)
    }
}
