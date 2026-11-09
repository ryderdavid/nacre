package com.ryderdavid.nacre.notify

import com.ryderdavid.nacre.data.model.AttentionItem
import kotlin.test.Test
import kotlin.test.assertEquals

class ChangeNoticeTest {
    @Test fun proposalsAndArchiveReadinessAreLeftToTheNeedsYouSummary() {
        val items = listOf(
            AttentionItem("a", AttentionItem.PROPOSAL_READY, "r"),
            AttentionItem("b", AttentionItem.CHANGE_ARCHIVABLE, "r"),
            AttentionItem("c", AttentionItem.ROUND_QUESTION, "r"),
        )
        // Both are decisions (approve, finish), announced under the one summary.
        assertEquals(emptyList(), changeNotices(items, emptySet()).map { it.id })
    }
}
