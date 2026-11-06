package com.ryderdavid.nacre.data

import android.content.Context
import com.ryderdavid.nacre.data.model.Decision

/**
 * The decisions already announced on this phone, kept across process restarts so the live event
 * stream ([DecisionAlerts]) and the periodic background check
 * ([com.ryderdavid.nacre.notify.DecisionCheckWorker]) never announce the same decision twice.
 * Newest last, at most [CAP] keys.
 */
class AnnouncedDecisions(private val store: Store = Memory()) {
    /** Where the keys live: in memory for tests, in shared preferences in the app. */
    interface Store {
        var keys: List<String>

        /** Whether the background check has run once; its first run only records what already waits. */
        var checked: Boolean
    }

    class Memory : Store {
        override var keys: List<String> = emptyList()
        override var checked: Boolean = false
    }

    private var cache: List<String>? = null
    private val keys: List<String> get() = cache ?: store.keys.also { cache = it }

    private fun save(next: List<String>) {
        cache = next
        store.keys = next
    }

    @Synchronized
    fun contains(key: String): Boolean = key in keys

    @Synchronized
    fun add(key: String) = save(remember(keys, listOf(key)))

    /**
     * The decisions in [open] to announce now, remembered as announced in the same step. The first
     * check ever announces nothing: what already waits was there before this check existed, and the
     * app shows it in Needs you.
     */
    @Synchronized
    fun claim(open: List<Decision>): List<Decision> {
        val fresh = newDecisions(open, keys.toSet())
        val first = !store.checked
        save(remember(keys, fresh.map { it.key }))
        store.checked = true
        return if (first) emptyList() else fresh
    }

    companion object {
        const val CAP = 400

        /** Pending decisions not yet announced, once each. Held, deferred and settled ones never are. */
        fun newDecisions(open: List<Decision>, announced: Set<String>): List<Decision> =
            open.filter { it.pending && it.key !in announced }.distinctBy { it.key }

        /** [keys] with [added] moved to the end, keeping the newest [cap]. */
        fun remember(keys: List<String>, added: List<String>, cap: Int = CAP): List<String> {
            if (added.isEmpty()) return keys
            val set = added.toSet()
            return (keys.filterNot { it in set } + added.distinct()).takeLast(cap)
        }
    }
}

/** [AnnouncedDecisions.Store] in the app's shared preferences. */
class PrefsAnnouncedStore(context: Context) : AnnouncedDecisions.Store {
    private val prefs = context.getSharedPreferences("decision_alerts", Context.MODE_PRIVATE)

    override var keys: List<String>
        get() = prefs.getString(KEYS, null)?.split('\n')?.filter { it.isNotBlank() }.orEmpty()
        set(value) { prefs.edit().putString(KEYS, value.joinToString("\n")).commit() }

    override var checked: Boolean
        get() = prefs.getBoolean(CHECKED, false)
        set(value) { if (value != checked) prefs.edit().putBoolean(CHECKED, value).commit() }

    private companion object {
        const val KEYS = "announced"
        const val CHECKED = "checked"
    }
}
