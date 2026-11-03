package com.ryderdavid.nacre.ui.bead

/** Internal workflow labels are retained when editing, but never shown as user labels. */
fun isMachineLabel(label: String): Boolean =
    label == com.ryderdavid.nacre.data.model.SIGNOFF || listOf("specflow:", "round:", "spec-section:", "nacre-").any(label::startsWith)
