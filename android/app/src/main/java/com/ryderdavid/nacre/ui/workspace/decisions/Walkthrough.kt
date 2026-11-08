package com.ryderdavid.nacre.ui.workspace.decisions

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionEvidence
import kotlinx.coroutines.delay

/**
 * One step of the Needs you walkthrough: a card's id, and why it comes where it does (blank when the
 * card check didn't rank it).
 */
data class WalkStep(val id: String, val why: String)

/** What the walkthrough's header says at step [index] (0-based) of [total]. */
fun walkHeading(index: Int, total: Int) = "Card ${index + 1} of $total"

/** What the walkthrough says when it reaches the end. */
fun walkSummary(answered: Int, skipped: Int): String = when {
    skipped == 0 -> "You answered all $answered."
    answered == 0 -> "You skipped all $skipped; they're still in the list."
    else -> "Answered $answered, skipped $skipped. The skipped ones are still in the list."
}

/**
 * The Needs you walkthrough: the cards that wait on Ryder one at a time, in the card check's order, with
 * why each comes now. Answering or deferring a card moves on by itself; Skip leaves it waiting. At the
 * end it says what was answered and offers the skipped ones again.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun Walkthrough(start: List<WalkStep>, all: List<Decision>, onEvidence: (DecisionEvidence) -> Unit, onDone: () -> Unit) {
    var steps by remember { mutableStateOf(start) }
    var index by remember { mutableIntStateOf(0) }
    val answered = remember { mutableStateListOf<String>() }
    val skipped = remember { mutableStateListOf<String>() }
    val byId = all.associateBy { it.id }
    val step = steps.getOrNull(index)
    val card = step?.let { byId[it.id] }
    val waits = card?.waitsOnYou == true

    // A card answered (or deferred, or closed elsewhere) moves on after a beat, so its new state shows.
    LaunchedEffect(step?.id, waits) {
        if (step == null || waits) return@LaunchedEffect
        if (step.id !in answered) answered += step.id
        skipped -= step.id
        delay(if (card == null) 0 else 900)
        if (steps.getOrNull(index)?.id == step.id) index++
    }

    if (step == null) {
        WalkDone(answered.size, skipped.toList(), onAgain = { ids ->
            steps = steps.filter { it.id in ids && byId[it.id]?.waitsOnYou == true }
            skipped.clear(); answered.clear(); index = 0
        }, onDone = onDone)
        return
    }
    val back = (index - 1 downTo 0).firstOrNull { byId[steps[it].id]?.waitsOnYou == true }
    val progress by animateFloatAsState((index + 1).toFloat() / steps.size, label = "walk")

    // Full height, so the sheet doesn't jump as cards of different lengths come and go.
    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                walkHeading(index, steps.size), style = MaterialTheme.typography.headlineSmallEmphasized,
                modifier = Modifier.weight(1f).semantics { heading(); liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = onDone, shapes = ButtonDefaults.shapes()) { Text("Show the list") }
        }
        LinearWavyProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth())
        if (step.why.isNotBlank()) Text(
            "Now because: ${step.why}", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary,
        )
        // The card keeps its own height; the space under a short one stays empty.
        Column(Modifier.weight(1f).verticalScroll(rememberScrollState())) {
            AnimatedContent(
                targetState = step.id, label = "card",
                transitionSpec = { (slideInHorizontally { it / 3 } + fadeIn()) togetherWith (slideOutHorizontally { -it / 3 } + fadeOut()) },
            ) { id ->
                byId[id]?.let { DecisionCard(it, onEvidence = onEvidence) }
            }
        }
        Row(Modifier.fillMaxWidth().padding(bottom = 24.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
            OutlinedButton(
                onClick = { back?.let { skipped -= steps[it].id; index = it } }, enabled = back != null,
                shapes = ButtonDefaults.shapes(), modifier = Modifier.heightIn(min = 48.dp), contentPadding = PaddingValues(start = 16.dp, end = 20.dp),
            ) {
                Icon(Icons.AutoMirrored.Rounded.ArrowBack, null, Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text("Back")
            }
            Spacer(Modifier.weight(1f))
            Button(
                onClick = { if (step.id !in answered && step.id !in skipped) skipped += step.id; index++ }, enabled = waits,
                shapes = ButtonDefaults.shapes(), modifier = Modifier.heightIn(min = 48.dp), contentPadding = PaddingValues(start = 20.dp, end = 16.dp),
            ) {
                Text(if (index == steps.lastIndex) "Skip and finish" else "Skip for now")
                Spacer(Modifier.width(8.dp))
                Icon(Icons.AutoMirrored.Rounded.ArrowForward, null, Modifier.size(18.dp))
            }
        }
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun WalkDone(answered: Int, skipped: List<String>, onAgain: (List<String>) -> Unit, onDone: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 24.dp, bottom = 32.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp), horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(Icons.Rounded.TaskAlt, null, Modifier.size(48.dp), tint = MaterialTheme.colorScheme.primary)
        Text("That's all of them", style = MaterialTheme.typography.headlineSmallEmphasized, modifier = Modifier.semantics { heading() })
        Text(walkSummary(answered, skipped.size), style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (skipped.isNotEmpty()) OutlinedButton(onClick = { onAgain(skipped) }, shapes = ButtonDefaults.shapes(), modifier = Modifier.heightIn(min = 48.dp)) {
                Text("Go through the ${skipped.size} skipped")
            }
            Button(onClick = onDone, shapes = ButtonDefaults.shapes(), modifier = Modifier.heightIn(min = 48.dp)) { Text("Back to the list") }
        }
    }
}
