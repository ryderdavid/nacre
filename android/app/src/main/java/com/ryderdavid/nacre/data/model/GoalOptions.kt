package com.ryderdavid.nacre.data.model

import kotlinx.serialization.Serializable

/** Shared by Implement and rounds; child choices survive toggling the goal off. */
@Serializable
data class GoalOptions(
    val enabled: Boolean = false,
    val committed: Boolean = false,
    val pushed: Boolean = false,
) {
    fun withCommitted(value: Boolean) = copy(committed = value, pushed = value && pushed)
    fun withPushed(value: Boolean) = copy(pushed = value, committed = value || committed)
    fun normalized() = if (pushed) copy(committed = true) else this
}
