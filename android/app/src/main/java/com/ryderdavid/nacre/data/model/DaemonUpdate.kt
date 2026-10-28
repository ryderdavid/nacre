package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class DaemonUpdate(
    val available: Boolean = false,
    val id: String = "",
    @SerialName("host_id") val hostId: String = "",
    val phase: String = "idle",
    val message: String = "",
    @SerialName("current_version") val currentVersion: String = "",
    @SerialName("current_commit") val currentCommit: String = "",
    @SerialName("target_version") val targetVersion: String = "",
    @SerialName("target_commit") val targetCommit: String = "",
    val active: Boolean = false,
) {
    val verifiedUpdate: Boolean get() = phase == "updated" && !active &&
        targetVersion.isNotBlank() && targetCommit.isNotBlank() &&
        currentVersion == targetVersion && currentCommit == targetCommit

    val problem: Boolean get() = phase in setOf("failed", "offline", "unavailable", "interrupted", "recovery_failed", "rolled_back", "unknown")
}
