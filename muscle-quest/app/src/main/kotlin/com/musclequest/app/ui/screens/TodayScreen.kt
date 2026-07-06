package com.musclequest.app.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.Insight
import com.musclequest.app.domain.Phase
import com.musclequest.app.ui.FuelUiState
import com.musclequest.app.ui.TaskUi
import com.musclequest.app.ui.TodayUiState
import com.musclequest.app.ui.components.MacroBar
import com.musclequest.app.ui.components.ProgressRing
import com.musclequest.app.ui.components.categoryColor
import com.musclequest.app.ui.components.toneColor
import com.musclequest.app.ui.theme.Ember
import com.musclequest.app.ui.theme.MQ
import java.time.format.DateTimeFormatter
import java.util.Locale

private val dateFmt = DateTimeFormatter.ofPattern("EEEE, MMM d", Locale.US)

@Composable
fun TodayScreen(
    state: TodayUiState,
    insights: List<Insight>,
    fuel: FuelUiState,
    onToggle: (TaskUi) -> Unit,
    onOpenCycle: () -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "hero") { HeroCard(state) }

        state.status?.let { status ->
            item(key = "phase") { PhaseBanner(status, onOpenCycle) }
        }

        if (insights.isNotEmpty()) {
            item(key = "coach") { CoachRow(insights) }
        }

        item(key = "fuel") { FuelSnapshot(fuel) }

        item(key = "checklist_header") { ChecklistHeader(state) }

        items(state.tasks, key = { it.task.id }) { taskUi ->
            TaskCard(taskUi, onToggle)
        }

        item(key = "bottom_spacer") { Spacer(Modifier.height(88.dp)) }
    }
}

// ---------------------------------------------------------------- 1. Hero --

@Composable
private fun HeroCard(state: TodayUiState) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.primaryContainer,
        ),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                // Layered wash of the primary over the container for a
                // gradient feel without leaving the theme palette.
                .background(
                    Brush.horizontalGradient(
                        listOf(
                            Color.Transparent,
                            MaterialTheme.colorScheme.primary.copy(alpha = 0.12f),
                        ),
                    ),
                )
                .padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(
                    state.date.format(dateFmt).uppercase(Locale.US),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.8f),
                )
                Spacer(Modifier.height(4.dp))
                Text(
                    "Lv ${state.level.level} · ${state.level.rank}",
                    style = MaterialTheme.typography.headlineMedium,
                    color = MaterialTheme.colorScheme.onPrimaryContainer,
                )
                Spacer(Modifier.height(6.dp))
                Text(
                    "${state.level.xpIntoLevel} / ${state.level.xpForNextLevel} XP to Lv ${state.level.level + 1}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.8f),
                )
            }
            Spacer(Modifier.width(12.dp))
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                ProgressRing(
                    progress = state.level.progress,
                    size = 84.dp,
                    stroke = 9.dp,
                    color = MaterialTheme.colorScheme.primary,
                    track = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.12f),
                ) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text(
                            "${state.streak}🔥",
                            style = MaterialTheme.typography.titleLarge,
                            color = MaterialTheme.colorScheme.onPrimaryContainer,
                        )
                        Text(
                            "streak",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.7f),
                        )
                    }
                }
                Spacer(Modifier.height(8.dp))
                Surface(
                    shape = CircleShape,
                    color = MaterialTheme.colorScheme.primary,
                    contentColor = MaterialTheme.colorScheme.onPrimary,
                ) {
                    Text(
                        "+${state.xpEarnedToday} XP today",
                        style = MaterialTheme.typography.labelMedium,
                        modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp),
                    )
                }
            }
        }
    }
}

// -------------------------------------------------------- 2. Phase banner --

@Composable
private fun phaseColor(phase: Phase): Color = when (phase) {
    Phase.ON_CYCLE -> MaterialTheme.colorScheme.primary
    Phase.PCT -> Ember
    Phase.RECOVERY -> MQ.recovery
    Phase.MAINTENANCE -> MQ.hydration
}

@Composable
private fun PhaseBanner(status: CycleStatus, onOpenCycle: () -> Unit) {
    val accent = if (status.daysUntilStart > 0) Ember else phaseColor(status.phase)
    Card(
        onClick = onOpenCycle,
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(
            modifier = Modifier.padding(start = 16.dp, end = 8.dp, top = 14.dp, bottom = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                if (status.daysUntilStart > 0) {
                    Text(
                        "Starts in ${status.daysUntilStart} day" +
                            if (status.daysUntilStart == 1) "" else "s",
                        style = MaterialTheme.typography.titleMedium,
                        color = accent,
                    )
                    Text(
                        "${status.phase.title} until then — ${status.phase.tagline}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Text(
                        status.phase.title,
                        style = MaterialTheme.typography.titleMedium,
                        color = accent,
                    )
                    Text(
                        status.phase.tagline,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (status.phaseLengthDays > 0) {
                        Spacer(Modifier.height(10.dp))
                        val animated by animateFloatAsState(
                            targetValue = status.progress,
                            animationSpec = tween(700),
                            label = "phase",
                        )
                        LinearProgressIndicator(
                            progress = { animated },
                            color = accent,
                            trackColor = MaterialTheme.colorScheme.surfaceVariant,
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(6.dp)
                                .clip(CircleShape),
                        )
                        Spacer(Modifier.height(4.dp))
                        Row {
                            Text(
                                "Day ${status.dayInPhase} of ${status.phaseLengthDays}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            Spacer(Modifier.weight(1f))
                            Text(
                                "Week ${status.weekInPhase}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    } else {
                        Spacer(Modifier.height(4.dp))
                        Text(
                            "Week ${status.weekInPhase} · Day ${status.dayInPhase}",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            Icon(
                Icons.Filled.ChevronRight,
                contentDescription = "Open cycle map",
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
    }
}

// --------------------------------------------------------- 3. Coach cards --

@Composable
private fun CoachRow(insights: List<Insight>) {
    Column {
        Text("Coach", style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        LazyRow(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            items(insights, key = { it.id }) { insight ->
                InsightCard(insight)
            }
        }
    }
}

@Composable
private fun InsightCard(insight: Insight) {
    val accent = toneColor(insight.tone)
    Card(
        modifier = Modifier
            .width(300.dp)
            .height(160.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(Modifier.fillMaxSize()) {
            Box(
                Modifier
                    .fillMaxHeight()
                    .width(4.dp)
                    .background(accent),
            )
            Column(Modifier.padding(12.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(insight.emoji, style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.width(8.dp))
                    Text(
                        insight.title,
                        style = MaterialTheme.typography.titleSmall,
                        color = accent,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Spacer(Modifier.height(6.dp))
                Text(
                    insight.body,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}

// ------------------------------------------------------- 4. Fuel snapshot --

@Composable
private fun FuelSnapshot(fuel: FuelUiState) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Fuel",
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                if (fuel.proteinHit) {
                    Surface(
                        shape = CircleShape,
                        color = MaterialTheme.colorScheme.primary.copy(alpha = 0.15f),
                    ) {
                        Text(
                            "🎯 protein banked",
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp),
                        )
                    }
                }
            }
            Spacer(Modifier.height(12.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    ProgressRing(
                        progress = if (fuel.targets.kcal > 0) {
                            fuel.totals.kcal / fuel.targets.kcal.toFloat()
                        } else {
                            0f
                        },
                        size = 56.dp,
                        stroke = 6.dp,
                        color = Ember,
                    ) {
                        Text(
                            "${fuel.totals.kcal}",
                            style = MaterialTheme.typography.labelMedium,
                            fontWeight = FontWeight.Bold,
                        )
                    }
                    Spacer(Modifier.height(2.dp))
                    Text(
                        "of ${fuel.targets.kcal} kcal",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.width(16.dp))
                Column(
                    Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    MacroBar(
                        label = "Protein",
                        value = fuel.totals.proteinG,
                        target = fuel.targets.proteinG,
                        unit = "g",
                        color = MaterialTheme.colorScheme.primary,
                    )
                    MacroBar(
                        label = "Carbs",
                        value = fuel.totals.carbsG,
                        target = fuel.targets.carbsG,
                        unit = "g",
                        color = MQ.nutrition,
                    )
                    MacroBar(
                        label = "Fat",
                        value = fuel.totals.fatG,
                        target = fuel.targets.fatG,
                        unit = "g",
                        color = MQ.recovery,
                    )
                }
            }
        }
    }
}

// ----------------------------------------------------------- 5. Checklist --

@Composable
private fun ChecklistHeader(state: TodayUiState) {
    val done = state.tasks.count { it.done }
    val total = state.tasks.size
    Column {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                "The checklist",
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
            )
            Text(
                "$done / $total done",
                style = MaterialTheme.typography.labelMedium,
                color = if (total > 0 && done == total) {
                    MaterialTheme.colorScheme.primary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
        }
        AnimatedVisibility(
            visible = state.perfectDay,
            enter = fadeIn() + expandVertically(),
            exit = fadeOut() + shrinkVertically(),
        ) {
            PerfectDayBanner(Modifier.padding(top = 12.dp))
        }
    }
}

@Composable
private fun PerfectDayBanner(modifier: Modifier = Modifier) {
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(
                    Brush.horizontalGradient(
                        listOf(Ember.copy(alpha = 0.22f), MQ.gold.copy(alpha = 0.25f)),
                    ),
                )
                .padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("🏆", style = MaterialTheme.typography.headlineMedium)
            Spacer(Modifier.width(12.dp))
            Column {
                Text(
                    "PERFECT DAY",
                    style = MaterialTheme.typography.headlineSmall,
                    color = Ember,
                )
                Text(
                    "Every box checked — bonus XP banked",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun TaskCard(taskUi: TaskUi, onToggle: (TaskUi) -> Unit) {
    val task = taskUi.task
    val accent = categoryColor(task.category)
    val container by animateColorAsState(
        targetValue = if (taskUi.done) {
            MaterialTheme.colorScheme.surfaceVariant
        } else {
            MaterialTheme.colorScheme.surface
        },
        animationSpec = tween(300),
        label = "taskContainer",
    )
    val barColor by animateColorAsState(
        targetValue = if (taskUi.done) accent.copy(alpha = 0.35f) else accent,
        animationSpec = tween(300),
        label = "taskBar",
    )
    val contentAlpha by animateFloatAsState(
        targetValue = if (taskUi.done) 0.6f else 1f,
        animationSpec = tween(300),
        label = "taskAlpha",
    )
    val checkScale by animateFloatAsState(
        targetValue = if (taskUi.done) 1.15f else 1f,
        animationSpec = spring(dampingRatio = Spring.DampingRatioMediumBouncy),
        label = "taskCheck",
    )
    Card(
        onClick = { onToggle(taskUi) },
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = container),
    ) {
        Row(
            modifier = Modifier.height(IntrinsicSize.Min),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier
                    .fillMaxHeight()
                    .width(5.dp)
                    .background(barColor),
            )
            Column(
                Modifier
                    .weight(1f)
                    .padding(start = 12.dp, top = 10.dp, bottom = 10.dp)
                    .alpha(contentAlpha),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        task.time,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        "+${task.xp} XP",
                        style = MaterialTheme.typography.labelSmall,
                        color = accent,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier
                            .background(accent.copy(alpha = 0.14f), CircleShape)
                            .padding(horizontal = 8.dp, vertical = 1.dp),
                    )
                }
                Spacer(Modifier.height(2.dp))
                Text(
                    task.title,
                    style = MaterialTheme.typography.titleMedium,
                    textDecoration = if (taskUi.done) TextDecoration.LineThrough else TextDecoration.None,
                )
                Text(
                    task.detail,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Checkbox(
                checked = taskUi.done,
                onCheckedChange = { onToggle(taskUi) },
                colors = CheckboxDefaults.colors(
                    checkedColor = accent,
                    uncheckedColor = accent.copy(alpha = 0.6f),
                    checkmarkColor = MaterialTheme.colorScheme.surface,
                ),
                modifier = Modifier
                    .padding(horizontal = 4.dp)
                    .scale(checkScale),
            )
        }
    }
}
