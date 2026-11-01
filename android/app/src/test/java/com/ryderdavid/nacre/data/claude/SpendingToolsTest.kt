package com.ryderdavid.nacre.data.claude

import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class SpendingToolsTest {
    @Test fun proposalsAlwaysAsk() {
        assertTrue(spends("develop_proposal", null))
        assertTrue(spends("create_bead", buildJsonObject { put("then_develop", JsonPrimitive(true)) }))
        assertFalse(spends("create_bead", buildJsonObject { put("title", JsonPrimitive("x")) }))
        assertFalse(spends("start_round", null), "start_round keeps its current behavior")
        assertTrue("develop_proposal" in WRITE_TOOLS && "found_something" in WRITE_TOOLS)
        assertTrue(assistantTools(false).any { it.name == "needs_you" && !it.write })
        assertTrue(assistantTools(true).first { it.name == "develop_proposal" }.props.containsKey("board"))
    }
}
