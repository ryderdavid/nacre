package com.ryderdavid.nacre.notify

import com.ryderdavid.nacre.data.model.AttentionItem
import kotlin.test.Test
import kotlin.test.assertEquals

class ChangeNoticeTest {
    @Test fun proposalsAndArchiveReadinessNotifyOnce() {
        val items = listOf(
            AttentionItem("a", AttentionItem.PROPOSAL_READY, "r"),
            AttentionItem("b", AttentionItem.CHANGE_ARCHIVABLE, "r"),
            AttentionItem("c", AttentionItem.ROUND_QUESTION, "r"),
        )
        assertEquals(listOf("a", "b"), changeNotices(items, emptySet()).map { it.id })
        assertEquals(listOf("b"), changeNotices(items, setOf("a")).map { it.id })
    }
}
