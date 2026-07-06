package com.musclequest.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.EmojiEvents
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.WorkoutSet
import com.musclequest.app.domain.ExercisePlan
import com.musclequest.app.ui.WorkoutUiState

@Composable
fun WorkoutScreen(
    state: WorkoutUiState,
    onLogSet: (exercise: String, weightLbs: Double, reps: Int) -> Unit,
    onDeleteSet: (WorkoutSet) -> Unit,
    onFinishWorkout: () -> Unit,
) {
    val workout = state.workout
    if (workout == null) {
        RestDay()
        return
    }
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Column {
                Text(workout.title, style = MaterialTheme.typography.headlineMedium)
                Text(
                    workout.focus,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(4.dp))
                Text(
                    "Today's volume: ${"%,d".format(state.todayVolume.toLong())} lb",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.secondary,
                )
            }
        }
        items(workout.exercises, key = { it.name }) { exercise ->
            ExerciseCard(
                exercise = exercise,
                sets = state.sets.filter { it.exercise == exercise.name },
                onLogSet = onLogSet,
                onDeleteSet = onDeleteSet,
            )
        }
        item {
            Button(
                onClick = onFinishWorkout,
                enabled = !state.workoutDone,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(if (state.workoutDone) "Workout complete ✔ (+50 XP banked)" else "Finish workout — claim 50 XP")
            }
        }
        item { Spacer(Modifier.height(64.dp)) }
    }
}

@Composable
private fun RestDay() {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("😴", style = MaterialTheme.typography.headlineMedium)
        Text("Rest day", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(8.dp))
        Text(
            "No lifting today. Muscle is built between workouts — eat your meals, " +
                "take your walk, hit your doses, and sleep like it's your job.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun ExerciseCard(
    exercise: ExercisePlan,
    sets: List<WorkoutSet>,
    onLogSet: (String, Double, Int) -> Unit,
    onDeleteSet: (WorkoutSet) -> Unit,
) {
    var weightText by rememberSaveable(exercise.name) { mutableStateOf("") }
    var repsText by rememberSaveable(exercise.name) { mutableStateOf("") }

    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(12.dp)) {
            Text(exercise.name, style = MaterialTheme.typography.titleMedium)
            Text(
                "${exercise.sets} sets × ${exercise.reps} · rest ${exercise.restSec}s" +
                    if (exercise.note.isNotEmpty()) " · ${exercise.note}" else "",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            sets.forEach { set ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (set.isPr) {
                        Icon(
                            Icons.Filled.EmojiEvents,
                            contentDescription = "PR",
                            tint = MaterialTheme.colorScheme.secondary,
                        )
                        Spacer(Modifier.width(4.dp))
                    }
                    Text(
                        "${if (set.weightLbs % 1.0 == 0.0) set.weightLbs.toInt() else set.weightLbs} lb × ${set.reps}",
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.weight(1f),
                    )
                    IconButton(onClick = { onDeleteSet(set) }) {
                        Icon(
                            Icons.Filled.Delete,
                            contentDescription = "Delete set",
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            Spacer(Modifier.height(4.dp))
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
