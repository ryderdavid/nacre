package com.ryderdavid.nacre.ui

import com.ryderdavid.nacre.data.model.Keys

/** The screen Ryder was looking at, as the Chat tab talks about it. */
sealed interface ScreenOrigin {
    /** Short name for "About: <label>" and "New · <label>". */
    val label: String

    /** Boards, Inbox and Activity are about every board on the computer. */
    data class AllBoards(val screen: String) : ScreenOrigin {
        override val label get() = ALL_BOARDS
    }
    data class Board(val repoKey: String, val name: String) : ScreenOrigin {
        override val label get() = name.ifBlank { Keys.id(repoKey) }
    }
    data class Bead(val repoKey: String, val beadId: String) : ScreenOrigin {
        override val label get() = beadId
    }
    /** The change view of a bead (its seed or epic). */
    data class Change(val repoKey: String, val beadId: String) : ScreenOrigin {
        override val label get() = "change · $beadId"
    }
    data class Round(val roundKey: String) : ScreenOrigin {
        override val label get() = "round ${Keys.id(roundKey)}"
    }
    data class Run(val runKey: String) : ScreenOrigin {
        override val label get() = "run ${Keys.id(runKey)}"
    }

    companion object {
        const val ALL_BOARDS = "all boards"
    }
}
