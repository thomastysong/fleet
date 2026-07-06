package com.musclequest.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bedtime
import androidx.compose.material.icons.filled.DirectionsWalk
import androidx.compose.material.icons.filled.FitnessCenter
import androidx.compose.material.icons.filled.LocalFireDepartment
import androidx.compose.material.icons.filled.Medication
import androidx.compose.material.icons.filled.Restaurant
import androidx.compose.material.icons.filled.WaterDrop
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.musclequest.app.domain.Phase
import com.musclequest.app.domain.TaskCategory
import com.musclequest.app.ui.TaskUi
import com.musclequest.app.ui.TodayUiState
import java.time.format.DateTimeFormatter
import java.util.Locale

private val dateFmt = DateTimeFormatter.ofPattern("EEEE, MMM d", Locale.US)

@Composable
fun TodayScreen(state: TodayUiState, onToggle: (TaskUi) -> Unit) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item { HeaderCard(state) }
        item { PhaseBanner(state) }
        items(state.tasks, key = { it.task.id }) { taskUi ->
            TaskRow(taskUi, onToggle)
        }
        item { Spacer(Modifier.height(64.dp)) }
    }
}

@Composable
private fun HeaderCard(state: TodayUiState) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(16.dp)) {
            Text(state.date.format(dateFmt), style = MaterialTheme.typography.labelSmall)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Lv ${state.level.level} · ${state.level.rank}",
                    style = MaterialTheme.typography.headlineMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
                Spacer(Modifier.weight(1f))
                Icon(
                    Icons.Filled.LocalFireDepartment,
                    contentDescription = "Streak",
                    tint = MaterialTheme.colorScheme.secondary,
                )
                Text(
                    "${state.streak}",
                    style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.secondary,
                )
            }
            Spacer(Modifier.height(8.dp))
            LinearProgressIndicator(
                progress = { state.level.progress },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(RoundedCornerShape(4.dp)),
            )
            Spacer(Modifier.height(4.dp))
            Row {
                Text(
                    "${state.level.xpIntoLevel} / ${state.level.xpForNextLevel} XP to next level",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.weight(1f))
                Text(
                    "+${state.xpEarnedToday} XP today",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

@Composable
private fun PhaseBanner(state: TodayUiState) {
    val status = state.status ?: return
    val phaseColor = when (status.phase) {
        Phase.ON_CYCLE -> MaterialTheme.colorScheme.primary
        Phase.PCT -> MaterialTheme.colorScheme.secondary
        Phase.RECOVERY -> MaterialTheme.colorScheme.onSurfaceVariant
        Phase.MAINTENANCE -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 12.dp)) {
            val headline = if (status.daysUntilStart > 0) {
                "CYCLE STARTS IN ${status.daysUntilStart} DAY${if (status.daysUntilStart == 1) "" else "S"}"
            } else if (status.phaseLengthDays > 0) {
                "${status.phase.title} — Week ${status.weekInPhase} · Day ${status.dayInPhase} of ${status.phaseLengthDays}"
            } else {
                "${status.phase.title} — Week ${status.weekInPhase}"
            }
            Text(headline, style = MaterialTheme.typography.titleMedium, color = phaseColor)
            Text(
                status.phase.tagline,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (status.phaseLengthDays > 0) {
                Spacer(Modifier.height(8.dp))
                LinearProgressIndicator(
                    progress = { status.progress },
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(6.dp)
                        .clip(RoundedCornerShape(3.dp)),
                )
            }
            if (state.perfectDay) {
                Spacer(Modifier.height(6.dp))
                Text(
                    "🏆 PERFECT DAY — every box checked",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

private fun iconFor(category: TaskCategory): ImageVector = when (category) {
    TaskCategory.DOSE -> Icons.Filled.Medication
    TaskCategory.TRAINING -> Icons.Filled.FitnessCenter
    TaskCategory.NUTRITION -> Icons.Filled.Restaurant
    TaskCategory.HYDRATION -> Icons.Filled.WaterDrop
    TaskCategory.RECOVERY -> Icons.Filled.Bedtime
}

@Composable
private fun TaskRow(taskUi: TaskUi, onToggle: (TaskUi) -> Unit) {
    val task = taskUi.task
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (taskUi.done) {
                MaterialTheme.colorScheme.surfaceVariant
            } else {
                MaterialTheme.colorScheme.surface
            },
        ),
    ) {
        Row(
            modifier = Modifier.padding(start = 12.dp, end = 4.dp, top = 8.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                imageVector = if (task.id == "recovery_walk") Icons.Filled.DirectionsWalk else iconFor(task.category),
                contentDescription = null,
                tint = if (taskUi.done) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    MaterialTheme.colorScheme.primary
                },
            )
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        task.time,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.secondary,
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        "+${task.xp} XP",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant)
                            .padding(horizontal = 6.dp, vertical = 1.dp),
                    )
                }
                Text(
                    task.title,
                    style = MaterialTheme.typography.titleMedium,
                    textDecoration = if (taskUi.done) TextDecoration.LineThrough else TextDecoration.None,
                )
                Text(
                    task.detail,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Checkbox(checked = taskUi.done, onCheckedChange = { onToggle(taskUi) })
        }
    }
}
