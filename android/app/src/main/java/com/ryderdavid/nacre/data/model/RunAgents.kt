package com.ryderdavid.nacre.data.model

/** A model the implement screen offers, with the effort levels its agent accepts for it. */
data class AgentModel(val id: String, val label: String, val efforts: List<String>)

/**
 * An agent the implement screen can send a bead to, with what it offers. [wire] is
 * [RunRequest.agent]: null for Claude Code. Each family is listed once, at its latest version.
 */
data class RunAgent(
    val wire: String?,
    val label: String,
    val models: List<AgentModel>,
    val defaultModel: String,
    val defaultEffort: String,
) {
    fun model(id: String): AgentModel = models.firstOrNull { it.id == id } ?: models.first { it.id == defaultModel }

    /** [effort] if [modelId] takes it, otherwise this agent's default. */
    fun effortFor(modelId: String, effort: String): String =
        if (effort in model(modelId).efforts) effort else defaultEffort
}

val EFFORT_LABELS = mapOf(
    "low" to "Low", "medium" to "Medium", "high" to "High",
    "xhigh" to "Extra high", "max" to "Max", "ultra" to "Ultra",
)

// `claude --effort`; every current Claude model takes all five.
private val CLAUDE_EFFORTS = listOf("low", "medium", "high", "xhigh", "max")

// Codex's supported_reasoning_levels (`codex debug models`, codex-cli 0.159.2, checked 2026-11-01): Luna stops at max.
private val CODEX_EFFORTS = listOf("low", "medium", "high", "xhigh", "max", "ultra")
private val CODEX_EFFORTS_NO_ULTRA = CODEX_EFFORTS - "ultra"

val CLAUDE_AGENT = RunAgent(
    wire = null,
    label = "Claude",
    models = listOf(
        AgentModel("claude-fable-5-1", "Fable 5.1", CLAUDE_EFFORTS),
        AgentModel("claude-opus-5-5", "Opus 5.5", CLAUDE_EFFORTS),
        AgentModel("claude-sonnet-5-5", "Sonnet 5.5", CLAUDE_EFFORTS),
        AgentModel("claude-haiku-5-5", "Haiku 5.5", CLAUDE_EFFORTS),
    ),
    defaultModel = "claude-opus-5-5",
    defaultEffort = "medium",
)

val CODEX_AGENT = RunAgent(
    wire = AGENT_CODEX,
    label = "Codex",
    models = listOf(
        AgentModel("gpt-6-astra", "GPT-6 Astra", CODEX_EFFORTS),
        AgentModel("gpt-6.1-sol", "GPT-6.1 Sol", CODEX_EFFORTS),
        AgentModel("gpt-6.1-terra", "GPT-6.1 Terra", CODEX_EFFORTS),
        AgentModel("gpt-6-luna", "GPT-6 Luna", CODEX_EFFORTS_NO_ULTRA),
    ),
    defaultModel = "gpt-6.1-sol",
    defaultEffort = "high",
)

/** In the order the implement screen's switch shows them; the first is the default. */
val RUN_AGENTS = listOf(CLAUDE_AGENT, CODEX_AGENT)

fun runAgent(wire: String?): RunAgent = RUN_AGENTS.firstOrNull { it.wire == wire } ?: CLAUDE_AGENT

/**
 * The agent-specific half of the implement screen. Built only from one [RunAgent], so a model
 * or effort of the other agent can't end up in a request.
 */
data class AgentChoice(val agent: RunAgent, val model: String, val effort: String) {
    fun withModel(id: String) = copy(model = agent.model(id).id, effort = agent.effortFor(id, effort))
    fun withEffort(level: String) = copy(effort = agent.effortFor(model, level))

    fun request(mode: String, instructions: String?, worktree: Boolean) =
        RunRequest(mode = mode, model = model, effort = effort, instructions = instructions, worktree = worktree, agent = agent.wire)

    companion object {
        /** [agent]'s defaults; flipping the switch starts over from these. */
        fun defaults(agent: RunAgent) = AgentChoice(agent, agent.defaultModel, agent.defaultEffort)
    }
}
