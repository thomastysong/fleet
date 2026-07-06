package com.musclequest.app.ui.screens

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.WorkoutSet
import com.musclequest.app.domain.ExercisePlan
import com.musclequest.app.ui.ExerciseStats
import com.musclequest.app.ui.WorkoutUiState
import com.musclequest.app.ui.components.ProgressRing
import com.musclequest.app.ui.theme.MQ
import kotlin.math.roundToInt
import kotlinx.coroutines.delay

@Composable
fun WorkoutScreen(
    state: WorkoutUiState,
    onLogSet: (String, Double, Int) -> Unit,
    onDeleteSet: (WorkoutSet) -> Unit,
    onFinishWorkout: () -> Unit,
) {
    val workout = state.workout
    if (workout == null) {
        RestDay()
        return
    }

    // One shared rest timer for the whole screen: exercise name -> end timestamp.
    // Wall-clock based (System.currentTimeMillis) so it survives recomposition.
    var restTimer by remember { mutableStateOf<Pair<String, Long>?>(null) }
    var nowMillis by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(restTimer) {
        val timer = restTimer ?: return@LaunchedEffect
        while (true) {
            val now = System.currentTimeMillis()
            nowMillis = now
            if (now >= timer.second) {
                restTimer = null
                break
            }
            delay(1000L)
        }
    }

    Box(Modifier.fillMaxSize()) {
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                HeaderCard(
                    title = workout.title,
                    focus = workout.focus,
                    todayVolume = state.todayVolume,
                    lastWeekVolume = state.lastWeekVolume,
                )
            }
            items(workout.exercises, key = { it.name }) { exercise ->
                ExerciseCard(
                    exercise = exercise,
                    sets = state.sets.filter { it.exercise == exercise.name },
                    stats = state.stats[exercise.name],
                    onLogSet = onLogSet,
                    onDeleteSet = onDeleteSet,
                    onRestStart = {
                        val start = System.currentTimeMillis()
                        nowMillis = start
                        restTimer = exercise.name to (start + exercise.restSec * 1000L)
                    },
                )
            }
            item {
                Button(
                    onClick = onFinishWorkout,
                    enabled = !state.workoutDone,
                    shape = MaterialTheme.shapes.medium,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(54.dp),
                ) {
                    AnimatedContent(targetState = state.workoutDone, label = "finishLabel") { done ->
                        Text(
                            if (done) "Workout complete ✔ (+50 XP banked)" else "Finish workout — claim 50 XP",
                            style = MaterialTheme.typography.titleMedium,
                        )
                    }
                }
            }
            item { Spacer(Modifier.height(88.dp)) }
        }

        // Rest countdown pill, pinned near the top; tap to cancel.
        AnimatedVisibility(
            visible = restTimer != null,
            enter = fadeIn() + slideInVertically { -it },
            exit = fadeOut() + slideOutVertically { -it },
            modifier = Modifier
                .align(Alignment.TopCenter)
                .padding(top = 10.dp),
        ) {
            val timer = restTimer ?: return@AnimatedVisibility
            RestTimerPill(
                exerciseName = timer.first,
                endAtMillis = timer.second,
                nowMillis = nowMillis,
                totalSec = workout.exercises.firstOrNull { it.name == timer.first }?.restSec ?: 60,
                onCancel = { restTimer = null },
            )
        }
    }
}

@Composable
private fun HeaderCard(
    title: String,
    focus: String,
    todayVolume: Double,
    lastWeekVolume: Double?,
) {
    val animatedVolume by animateFloatAsState(
        targetValue = todayVolume.toFloat(),
        animationSpec = tween(600),
        label = "volume",
    )
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(18.dp)) {
            Text(title, style = MaterialTheme.typography.headlineMedium)
            Text(
                focus,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(12.dp))
            Text(
                "TODAY'S VOLUME",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(verticalAlignment = Alignment.Bottom) {
                Text(
                    "%,d".format(animatedVolume.toLong()),
                    style = MaterialTheme.typography.displaySmall,
                    color = MaterialTheme.colorScheme.primary,
                )
                Spacer(Modifier.width(6.dp))
                Text(
                    "lb",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 7.dp),
                )
            }
            if (lastWeekVolume != null) {
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Pill(
                        text = "Last week: ${"%,d".format(lastWeekVolume.toLong())} lb",
                        container = MaterialTheme.colorScheme.surfaceVariant,
                        content = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (todayVolume > lastWeekVolume) {
                        Pill(
                            text = "▲ ahead",
                            container = MaterialTheme.colorScheme.primaryContainer,
                            content = MaterialTheme.colorScheme.onPrimaryContainer,
                        )
                    } else {
                        Pill(
                            text = "target to beat",
                            container = MaterialTheme.colorScheme.surfaceVariant,
                            content = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ExerciseCard(
    exercise: ExercisePlan,
    sets: List<WorkoutSet>,
    stats: ExerciseStats?,
    onLogSet: (String, Double, Int) -> Unit,
    onDeleteSet: (WorkoutSet) -> Unit,
    onRestStart: () -> Unit,
) {
    var weightText by rememberSaveable(exercise.name) { mutableStateOf("") }
    var repsText by rememberSaveable(exercise.name) { mutableStateOf("") }

    Card(
        shape = MaterialTheme.shapes.medium,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .animateContentSize()
                .padding(14.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    exercise.name,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                val complete = sets.size >= exercise.sets
                Pill(
                    text = "${sets.size}/${exercise.sets}",
                    container = if (complete) {
                        MaterialTheme.colorScheme.primaryContainer
                    } else {
                        MaterialTheme.colorScheme.surfaceVariant
                    },
                    content = if (complete) {
                        MaterialTheme.colorScheme.onPrimaryContainer
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
            }
            Text(
                "${exercise.sets} sets × ${exercise.reps} · rest ${exercise.restSec}s",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (exercise.note.isNotEmpty()) {
                Text(
                    exercise.note,
                    style = MaterialTheme.typography.bodySmall,
                    fontStyle = FontStyle.Italic,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // Progressive-overload cues: what you have to beat today.
            val hasStats = stats != null &&
                (stats.allTimeBestLbs != null || stats.lastTopSet != null || stats.est1RmLbs != null)
            if (hasStats && stats != null) {
                Spacer(Modifier.height(8.dp))
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    stats.allTimeBestLbs?.let {
                        Pill(
                            text = "🏆 Best ${it.roundToInt()} lb",
                            container = MQ.gold.copy(alpha = 0.16f),
                            content = MaterialTheme.colorScheme.onSurface,
                        )
                    }
                    stats.lastTopSet?.let {
                        Pill(
                            text = "Last: ${it.weightLbs.trimLb()}×${it.reps}",
                            container = MaterialTheme.colorScheme.surfaceVariant,
                            content = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    stats.est1RmLbs?.let {
                        Pill(
                            text = "e1RM ≈ ${it.roundToInt()} lb",
                            container = MaterialTheme.colorScheme.primaryContainer,
                            content = MaterialTheme.colorScheme.onPrimaryContainer,
                        )
                    }
                }
            }

            if (sets.isNotEmpty()) Spacer(Modifier.height(6.dp))
            sets.forEach { set ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        "${set.weightLbs.trimLb()} lb × ${set.reps}",
                        style = MaterialTheme.typography.bodyMedium,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier.weight(1f),
                    )
                    if (set.isPr) {
                        Pill(
                            text = "🎉 PR",
                            container = MaterialTheme.colorScheme.secondaryContainer,
                            content = MaterialTheme.colorScheme.onSecondaryContainer,
                        )
                        Spacer(Modifier.width(4.dp))
                    }
                    IconButton(onClick = { onDeleteSet(set) }) {
                        Icon(
                            Icons.Filled.Delete,
                            contentDescription = "Delete set",
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            Spacer(Modifier.height(6.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = weightText,
                    onValueChange = { weightText = it },
                    label = { Text("lb") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                OutlinedTextField(
                    value = repsText,
                    onValueChange = { repsText = it },
                    label = { Text("reps") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                Button(
                    onClick = {
                        val w = weightText.toDoubleOrNull()
                        val r = repsText.toIntOrNull()
                        if (w != null && w > 0 && r != null && r > 0) {
                            onLogSet(exercise.name, w, r)
                            onRestStart()
                        }
                    },
                    enabled = weightText.toDoubleOrNull()?.let { it > 0 } == true &&
                        repsText.toIntOrNull()?.let { it > 0 } == true,
                ) {
                    Text("Log")
                }
            }
        }
    }
}

@Composable
private fun RestTimerPill(
    exerciseName: String,
    endAtMillis: Long,
    nowMillis: Long,
    totalSec: Int,
    onCancel: () -> Unit,
) {
    val remainingMs = (endAtMillis - nowMillis).coerceAtLeast(0L)
    val remainingSec = ((remainingMs + 999) / 1000).toInt()
    val fraction = if (totalSec > 0) remainingMs.toFloat() / (totalSec * 1000f) else 0f
    Surface(
        shape = CircleShape,
        color = MaterialTheme.colorScheme.secondaryContainer,
        shadowElevation = 8.dp,
        modifier = Modifier
            .clip(CircleShape)
            .clickable(onClick = onCancel),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
        ) {
            ProgressRing(
                progress = fraction,
                size = 30.dp,
                stroke = 4.dp,
                color = MaterialTheme.colorScheme.secondary,
                track = MaterialTheme.colorScheme.onSecondaryContainer.copy(alpha = 0.2f),
            )
            Spacer(Modifier.width(10.dp))
            Column {
                Text(
                    "Rest: %d:%02d — %s".format(remainingSec / 60, remainingSec % 60, exerciseName),
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSecondaryContainer,
                )
                Text(
                    "tap to skip",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSecondaryContainer.copy(alpha = 0.7f),
                )
            }
        }
    }
}

@Composable
private fun RestDay() {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(24.dp),
        contentAlignment = Alignment.Center,
    ) {
        Card(
            shape = MaterialTheme.shapes.large,
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(28.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text("😴", style = MaterialTheme.typography.displaySmall)
                Spacer(Modifier.height(8.dp))
                Text("Rest day", style = MaterialTheme.typography.headlineMedium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "No lifting today. Muscle is built between workouts — eat your meals, " +
                        "take your walk, hit your doses, and sleep like it's your job.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
            }
        }
    }
}

/** Small rounded status chip used for stats, deltas, PR badges and set counts. */
@Composable
private fun Pill(text: String, container: Color, content: Color) {
    Surface(shape = CircleShape, color = container) {
        Text(
            text,
            style = MaterialTheme.typography.labelMedium,
            fontWeight = FontWeight.Bold,
            color = content,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
        )
    }
}

private fun Double.trimLb(): String =
    if (this % 1.0 == 0.0) this.toInt().toString() else this.toString()
