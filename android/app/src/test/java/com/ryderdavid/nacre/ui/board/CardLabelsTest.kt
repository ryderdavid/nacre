package com.ryderdavid.nacre.ui.board

import kotlin.test.*

class CardLabelsTest {
    @Test fun internalLabelsAreHiddenButUserLabelsRemain() {
        listOf("specflow:approved", "round:123", "spec-section:2", "nacre-internal").forEach { assertTrue(isInternalCardLabel(it)) }
        listOf("bug", "release", "roundtable", "nacre", "specflow").forEach { assertFalse(isInternalCardLabel(it)) }
    }
}
