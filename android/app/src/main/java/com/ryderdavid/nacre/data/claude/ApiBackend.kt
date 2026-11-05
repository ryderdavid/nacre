package com.ryderdavid.nacre.data.claude

import com.anthropic.helpers.MessageAccumulator
import com.anthropic.models.messages.Base64ImageSource
import com.anthropic.models.messages.ContentBlockParam
import com.anthropic.models.messages.ImageBlockParam
import com.anthropic.models.messages.MessageCreateParams
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.StopReason
import com.anthropic.models.messages.TextBlockParam
import com.anthropic.models.messages.ThinkingConfigAdaptive
import com.anthropic.models.messages.Tool
import com.anthropic.models.messages.ToolResultBlockParam
import com.anthropic.models.messages.ToolUnion
import com.ryderdavid.nacre.data.claude.ClaudeService.Companion.toJsonString
import com.ryderdavid.nacre.data.settings.AppSettings
import com.anthropic.core.JsonValue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.Base64

/**
 * Runs Refine, Ask and Draft against the Claude API with the key on this phone.
 * Chat replaced the screens that called it; it is kept with the API-key
 * setting for now (agent-chat-tab leaves both in place).
 */
class ApiBackend(
    private val service: ClaudeService,
    private val settings: suspend () -> AppSettings,
) : ClaudeBackend {
    override val runner = ClaudeRunner.API

    override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>): RefineProposal =
        service.refine(settings(), ctx, instruction, history)

    override suspend fun ask(repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String): String =
        service.ask(settings(), ctx, history, question)

    override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft =
        service.draft(settings(), repoName, rough, parentTitle, existingTitles)

    override suspend fun reviseDraft(
        repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): BeadDraft = service.reviseDraft(settings(), repoName, current, history, message, parentTitle, existingTitles)
}

internal fun textBlock(text: String): ContentBlockParam = ContentBlockParam.ofText(TextBlockParam.builder().text(text).build())
