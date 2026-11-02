package com.ryderdavid.nacre.ui.repos

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.claude.BeadChatItem
import com.ryderdavid.nacre.data.claude.ChatMode
import com.ryderdavid.nacre.data.model.AttentionList
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class RoundQuestionsTest {
    private val json = """{"items":[{"id":"x","kind":"round_question","repo":"r1","bead_id":"N-1","round_id":"R9",
        "title":"2 questions before work starts on N-1","why":"w",
        "options":[{"type":"action","label":"Accept all 2 recommendations","method":"POST","path":"/v1/rounds/R9/answers/recommended","recommended":true}],
        "questions":[{"id":"q1","text":"Which moment?","reason":"Covers the visible wait.",
          "options":[{"type":"action","label":"Drafting","method":"POST","path":"/v1/rounds/R9/answers","body":{"question_id":"q1","answer":"Drafting"},"recommended":true},
                     {"type":"action","label":"Gap","method":"POST","path":"/v1/rounds/R9/answers","body":{"question_id":"q1","answer":"Gap"}}]}]}]}"""

    @Test fun decodesQuestionsWithTheirRecommendation() {
        val item = BridgeClient.json.decodeFromString<AttentionList>(json).items.single()
        val q = item.questions.single()
        assertTrue(item.options.single().recommended)
        assertTrue(q.options[0].recommended && !q.options[1].recommended)
        assertEquals("Covers the visible wait.", q.reason)
    }

    @Test fun chatIsToldTheQuestionAndRecommendation() {
        val q = BridgeClient.json.decodeFromString<AttentionList>(json).items.single().questions.single()
        val about = questionContext(q)
        assertTrue("Which moment?" in about && "Drafting (recommended)" in about && "Covers the visible wait." in about)
        val mine = BeadChatItem.Mine("Why not the gap?", ChatMode.ASK, about)
        assertTrue(mine.prompt.startsWith(about) && mine.prompt.endsWith("Why not the gap?"))
        assertEquals("plain", BeadChatItem.Mine("plain", ChatMode.ASK).prompt)
    }

    @Test fun ownAnswerPostsLikeTheListedAnswers() {
        val o = ownAnswer("R9", "q1", "Both, but the gap first")
        assertEquals("/v1/rounds/R9/answers", o.path)
        assertEquals("q1", o.body!!["question_id"]!!.jsonPrimitive.content)
        assertEquals("Both, but the gap first", o.body!!["answer"]!!.jsonPrimitive.content)
    }
}
