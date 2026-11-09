package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.WorkSummary
import java.time.Instant

/** One row of a conversation: a message or step, a decision card, or a work card. */
sealed interface TimelineRow {
    val key: String

    data class Item(val item: ChatItem) : TimelineRow {
        override val key get() = item.key
    }

    data class Card(val decision: Decision) : TimelineRow {
        override val key get() = "d:" + decision.id
    }

    data class Work(val work: WorkSummary) : TimelineRow {
        override val key get() = "w:" + work.kind + work.id
    }
}

/**
 * Places a chat's decision and work cards where they happened among its messages, by time: a card goes
 * after the last message logged at or before it was raised. Cards used to follow every message, so a
 * chat's old cards stayed at the bottom and new messages landed above them, out of sight.
 */
object ChatTimeline {
    fun merge(items: List<ChatItem>, times: Map<String, String>, decisions: List<Decision>, work: List<WorkSummary>): List<TimelineRow> {
        val dated = mutableListOf<Pair<Instant, TimelineRow>>()
        val undated = mutableListOf<TimelineRow>()
        for (d in decisions) instant(d.createdAt)?.let { dated += it to TimelineRow.Card(d) } ?: undated.add(TimelineRow.Card(d))
        for (w in work) instant(w.startedAt)?.let { dated += it to TimelineRow.Work(w) } ?: undated.add(TimelineRow.Work(w))
        val cards = dated.sortedBy { it.first }
        val rows = ArrayList<TimelineRow>(items.size + cards.size)
        var next = 0
        for (item in items) {
            val at = times[item.key]?.let(::instant)
            if (at != null) while (next < cards.size && cards[next].first.isBefore(at)) rows += cards[next++].second
            rows += TimelineRow.Item(item)
        }
        while (next < cards.size) rows += cards[next++].second
        // A card with no time can't be placed; it goes last, as every card did before.
        rows += undated
        return rows
    }

    /**
     * The conversation's items as receipts go: one Ask row per decision (the first time it was asked), none
     * for a decision the chat raised ([raised] ids), whose own receipt sits where it was raised.
     */
    fun receipts(items: List<ChatItem>, raised: Set<String>): List<ChatItem> {
        val seen = HashSet<String>()
        return items.filter { it !is ChatItem.Ask || (it.decisionId !in raised && seen.add(it.decisionId)) }
    }

    private fun instant(s: String): Instant? = if (s.isBlank()) null else runCatching { Instant.parse(s) }.getOrNull()
}
