package com.ryderdavid.nacre.data.chat

import java.io.IOException

/** The connection dropped and couldn't be brought back in time. */
internal class TurnLostException(cause: Throwable) : IOException("connection lost", cause)

/** How long to keep trying to reattach before telling the user, counted while the phone is awake. */
internal const val RECONNECT_BUDGET_MS = 3 * 60_000L

/**
 * A pause that ran this much longer than asked was spent asleep or frozen:
 * the phone wasn't trying, so the time doesn't count against the budget.
 */
internal const val SLEPT_MS = 20_000L

/** The most one reattach attempt counts for: longer, and the phone was asleep during it. */
internal const val ATTEMPT_MAX_MS = 60_000L

/** 1 s, 2 s, 4 s, 8 s, then every 10 s. */
internal fun reconnectDelay(attempt: Int): Long = minOf(10_000L, 1_000L shl (attempt - 1).coerceIn(0, 4))

/**
 * HTTP answers that mean "not right now" rather than "no": a proxy or a
 * restarting computer in between. Worth reattaching after, like a drop.
 */
internal val TRANSIENT_HTTP = setOf(408, 425, 429, 502, 503, 504)
