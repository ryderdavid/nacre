package com.ryderdavid.nacre.data.claude

import com.anthropic.core.JsonValue
import com.anthropic.models.messages.ContentBlockParam
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.ToolUseBlockParam
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ToolHistoryTest {
    private fun toolUse(id: String) = ContentBlockParam.ofToolUse(
        ToolUseBlockParam.builder().id(id).name("run_in_claude_code").input(JsonValue.from(mapOf("id" to "NAC-1"))).build()
    )

    private val user = MessageParam.builder().role(MessageParam.Role.USER).content("start everything").build()

    @Test fun unansweredParallelCallsGetNotRunResults() {
        val assistant = MessageParam.builder().role(MessageParam.Role.ASSISTANT).contentOfBlockParams(listOf(toolUse("t1"), toolUse("t2"))).build()
        val fix = answerDanglingToolUse(listOf(user, assistant))!!
        assertEquals(MessageParam.Role.USER, fix.role())
        val results = fix.content().blockParams().get().map { it.toolResult().get() }
        assertEquals(listOf("t1", "t2"), results.map { it.toolUseId() }, "every call answered, in order")
        assertTrue(results.all { it.isError().get() })
    }

    @Test fun answeredHistoryNeedsNothing() {
        val assistant = MessageParam.builder().role(MessageParam.Role.ASSISTANT).contentOfBlockParams(listOf(toolUse("t1"))).build()
        val answered = toolResults(listOf("t1"), mapOf("t1" to ("Started" to false)))
        assertNull(answerDanglingToolUse(listOf(user, assistant, answered)))
        assertNull(answerDanglingToolUse(listOf(user)))
        val ok = answered.content().blockParams().get().single().toolResult().get()
        assertEquals(false, ok.isError().get())
    }
}
