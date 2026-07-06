package com.musclequest.app.ui.screens

import androidx.compose.foundation.Canvas
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
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
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
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.WeightEntry
import com.musclequest.app.domain.Achievements
import com.musclequest.app.ui.ProgressUiState
import com.musclequest.app.ui.TodayUiState
import com.musclequest.app.ui.theme.Volt

@Composable
fun ProgressScreen(
    progress: ProgressUiState,
    today: TodayUiState,
    onLogWeight: (Double) -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Text("Progress", style = MaterialTheme.typography.headlineMedium)
        }
        item { StatsRow(progress, today) }
        item { WeightCard(progress.weights, onLogWeight) }
        item {
            Text("Achievements", style = MaterialTheme.typography.titleLarge)
        }
        items(Achievements.ALL, key = { it.id }) { achievement ->
            val unlocked = achievement.id in progress.unlockedIds
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = if (unlocked) {
                        MaterialTheme.colorScheme.surfaceVariant
                    } else {
                        MaterialTheme.colorScheme.surface
                    },
                ),
            ) {
                Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        if (unlocked) achievement.emoji else "🔒",
                        style = MaterialTheme.typography.headlineMedium,
                    )
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) {
                        Text(achievement.title, style = MaterialTheme.typography.titleMedium)
                        Text(
                            achievement.description,
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Text(
                        "+${achievement.xpReward} XP",
                        style = MaterialTheme.typography.labelSmall,
                        color = if (unlocked) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        item { Spacer(Modifier.height(64.dp)) }
    }
}

@Composable
private fun StatsRow(progress: ProgressUiState, today: TodayUiState) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        StatCard("Workouts", "${progress.workoutsCompleted}", Modifier.weight(1f))
        StatCard("PRs", "${progress.prCount}", Modifier.weight(1f))
        StatCard("Volume", "${progress.totalVolume / 1000}k lb", Modifier.weight(1f))
        StatCard("Streak", "${today.streak}🔥", Modifier.weight(1f))
    }
}

@Composable
private fun StatCard(label: String, value: String, modifier: Modifier = Modifier) {
    Card(modifier) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 12.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(value, style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.primary)
            Text(label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun WeightCard(weights: List<WeightEntry>, onLogWeight: (Double) -> Unit) {
    var weightText by rememberSaveable { mutableStateOf("") }
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Body weight", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                weights.lastOrNull()?.let {
                    Text(
                        "${it.weightLbs} lb",
                        style = MaterialTheme.typography.titleLarge,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            if (weights.size >= 2) {
                Spacer(Modifier.height(8.dp))
                WeightChart(weights)
                val delta = weights.last().weightLbs - weights.first().weightLbs
                Text(
                    (if (delta >= 0) "+%.1f" else "%.1f").format(delta) + " lb since day one",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.secondary,
                )
            }
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = weightText,
                    onValueChange = { weightText = it },
                    label = { Text("Today's weight (lb)") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                Button(
                    onClick = {
                        weightText.toDoubleOrNull()?.let {
                            onLogWeight(it)
                            weightText = ""
                        }
                    },
                    enabled = weightText.toDoubleOrNull()?.let { it in 50.0..500.0 } == true,
                ) {
                    Text("Log")
                }
            }
        }
    }
}

@Composable
private fun WeightChart(weights: List<WeightEntry>) {
    val minW = weights.minOf { it.weightLbs }
    val maxW = weights.maxOf { it.weightLbs }
    val range = (maxW - minW).coerceAtLeast(1.0)
    val minDay = weights.first().epochDay
    val maxDay = weights.last().epochDay
    val daySpan = (maxDay - minDay).coerceAtLeast(1L)
    val lineColor = Volt
    val dotColor = Color(0xFFFF8A3D)
    Canvas(
        Modifier
            .fillMaxWidth()
            .height(120.dp),
    ) {
        val pad = 8.dp.toPx()
        val w = size.width - pad * 2
        val h = size.height - pad * 2
        val points = weights.map {
            Offset(
                x = pad + w * (it.epochDay - minDay) / daySpan.toFloat(),
                y = pad + h * (1f - ((it.weightLbs - minW) / range).toFloat()),
            )
        }
        for (i in 0 until points.size - 1) {
            drawLine(
                color = lineColor,
                start = points[i],
                end = points[i + 1],
                strokeWidth = 5f,
                cap = StrokeCap.Round,
            )
        }
        points.forEach { drawCircle(color = dotColor, radius = 6f, center = it) }
    }
}
