package com.ryderdavid.nacre.ui.rounds

import com.ryderdavid.nacre.data.model.Repo

/** True when [repo] can't take a round only because it has no nacre.yaml yet. */
fun needsGateSetup(repo: Repo?): Boolean = repo != null && repo.gates.isEmpty() && repo.gatesError.isBlank()
