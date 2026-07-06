package com.musclequest.app.ui.screens

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.WeightEntry
import com.musclequest.app.domain.Achievement
import com.musclequest.app.domain.Achievements
import com.musclequest.app.domain.NutritionEngine
import com.musclequest.app.ui.ProgressUiState
import com.musclequest.app.ui.TodayUiState
import com.musclequest.app.ui.components.ProgressRing
import com.musclequest.app.ui.theme.Ember

@Composable
fun ProgressScreen(
    progress: ProgressUiState,
    today: TodayUiState,
    onLogWeight: (Double) -> Unit,
) {
    val unlockedCount = Achievements.ALL.count { it.id in progress.unlockedIds }
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Text("Progress", style = MaterialTheme.typography.headlineMedium)
        }
        item { LevelCard(today) }
        item { StatsGrid(progress, today) }
        item { WeightCard(progress.weights, onLogWeight) }
        item {
            Row(
                Modifier.fillMaxWidth().padding(top = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    "Achievements",
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    "$unlockedCount / ${Achievements.ALL.size}",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
        items(Achievements.ALL.chunked(2), key = { it.first().id }) { pair ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .height(IntrinsicSize.Max),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                pair.forEach { achievement ->
                    AchievementCard(
                        achievement = achievement,
                        unlocked = achievement.id in progress.unlockedIds,
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxHeight(),
                    )
                }
                if (pair.size == 1) Spacer(Modifier.weight(1f))
            }
        }
        item { Spacer(Modifier.height(88.dp)) }
    }
}

// ---------------------------------------------------------------- level card

@Composable
private fun LevelCard(today: TodayUiState) {
    val level = today.level
    Card(Modifier.fillMaxWidth(), shape = MaterialTheme.shapes.large) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            ProgressRing(
                progress = level.progress,
                size = 96.dp,
                stroke = 10.dp,
            ) {
                Text(
                    "Lv ${level.level}",
                    style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            Spacer(Modifier.width(16.dp))
            Column(Modifier.weight(1f)) {
                Text(level.rank, style = MaterialTheme.typography.headlineSmall)
                Spacer(Modifier.height(4.dp))
                Text(
                    "%,d XP total".format(today.totalXp),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    "%,d XP to next level".format(
                        (level.xpForNextLevel - level.xpIntoLevel).coerceAtLeast(0),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

// ---------------------------------------------------------------- stats grid

@Composable
private fun StatsGrid(progress: ProgressUiState, today: TodayUiState) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            StatCard("🏋️", "Workouts", "${progress.workoutsCompleted}", Modifier.weight(1f))
            StatCard("🏅", "PRs", "${progress.prCount}", Modifier.weight(1f))
        }
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            StatCard(
                "🏗️", "Volume", formatVolume(progress.totalVolume), Modifier.weight(1f),
                valueStyle = MaterialTheme.typography.headlineSmall,
            )
            StatCard("🔥", "Day streak", "${today.streak}", Modifier.weight(1f))
        }
    }
}

/** Exact pounds while the number is small; rounded thousands once it isn't. */
private fun formatVolume(lbs: Long): String =
    if (lbs < 10_000) "%,d lb".format(lbs) else "%,dk lb".format((lbs + 500) / 1000)

@Composable
private fun StatCard(
    emoji: String,
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    valueStyle: TextStyle = MaterialTheme.typography.headlineMedium,
) {
    Card(modifier, shape = MaterialTheme.shapes.medium) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 14.dp, horizontal = 8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(emoji, style = MaterialTheme.typography.titleLarge)
            Spacer(Modifier.height(4.dp))
            Text(
                value,
                style = valueStyle,
                color = MaterialTheme.colorScheme.primary,
                maxLines = 1,
            )
            Text(
                label,
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

// --------------------------------------------------------------- body weight

@Composable
private fun WeightCard(weights: List<WeightEntry>, onLogWeight: (Double) -> Unit) {
    var weightText by rememberSaveable { mutableStateOf("") }
    val sorted = remember(weights) { weights.sortedBy { it.epochDay } }
    Card(
        Modifier
            .fillMaxWidth()
            .animateContentSize(),
        shape = MaterialTheme.shapes.large,
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Body weight",
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                sorted.lastOrNull()?.let {
                    Text(
                        "${fmtLb(it.weightLbs)} lb",
                        style = MaterialTheme.typography.headlineMedium,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            if (sorted.size < 2) {
                Spacer(Modifier.height(8.dp))
                Text(
                    "No trend yet — log at least two weigh-ins. Weigh 2×/week under " +
                        "identical conditions (same scale, first thing in the morning, " +
                        "before food) and your chart appears here.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                val latest = sorted.last().weightLbs
                val (slopePerDay, intercept) = remember(sorted) { leastSquares(sorted) }
                val weeklyTrend = slopePerDay * 7
                val band = NutritionEngine.weeklyGainBandLbs(latest)
                Spacer(Modifier.height(12.dp))
                WeightChart(sorted, slopePerDay, intercept)
                Spacer(Modifier.height(8.dp))
                val verdictColor = when {
                    weeklyTrend > band.endInclusive -> Ember
                    weeklyTrend < band.start -> MaterialTheme.colorScheme.onSurfaceVariant
                    else -> MaterialTheme.colorScheme.primary
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        "%+.1f lb/wk".format(weeklyTrend),
                        style = MaterialTheme.typography.titleMedium,
                        color = verdictColor,
                        modifier = Modifier.weight(1f),
                    )
                    Text(
                        "lean-bulk band: +%.1f–%.1f lb/wk".format(band.start, band.endInclusive),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Spacer(Modifier.height(12.dp))
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

/** Least-squares fit of weight over epochDay: returns (slope lb/day, intercept). */
private fun leastSquares(weights: List<WeightEntry>): Pair<Double, Double> {
    val xMean = weights.map { it.epochDay.toDouble() }.average()
    val yMean = weights.map { it.weightLbs }.average()
    var num = 0.0
    var den = 0.0
    weights.forEach {
        val dx = it.epochDay - xMean
        num += dx * (it.weightLbs - yMean)
        den += dx * dx
    }
    val slope = if (den == 0.0) 0.0 else num / den
    return slope to (yMean - slope * xMean)
}

private fun fmtLb(v: Double): String =
    if (v == v.toLong().toDouble()) v.toLong().toString() else "%.1f".format(v)

@Composable
private fun WeightChart(
    weights: List<WeightEntry>,
    slopePerDay: Double,
    intercept: Double,
) {
    val minW = weights.minOf { it.weightLbs }
    val maxW = weights.maxOf { it.weightLbs }
    val range = (maxW - minW).coerceAtLeast(1.0)
    val minDay = weights.first().epochDay
    val maxDay = weights.last().epochDay
    val daySpan = (maxDay - minDay).coerceAtLeast(1L)

    val lineColor = MaterialTheme.colorScheme.primary
    val gridColor = MaterialTheme.colorScheme.outlineVariant
    val labelStyle = MaterialTheme.typography.labelSmall.copy(
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    val textMeasurer = rememberTextMeasurer()

    Canvas(
        Modifier
            .fillMaxWidth()
            .height(150.dp),
    ) {
        val maxLabel = textMeasurer.measure(fmtLb(maxW), labelStyle)
        val minLabel = textMeasurer.measure(fmtLb(minW), labelStyle)
        val padLeft = maxOf(maxLabel.size.width, minLabel.size.width) + 8.dp.toPx()
        val padTop = 6.dp.toPx()
        val padBottom = 6.dp.toPx()
        val padRight = 6.dp.toPx()
        val w = size.width - padLeft - padRight
        val h = size.height - padTop - padBottom

        fun xFor(day: Long): Float = padLeft + w * (day - minDay) / daySpan.toFloat()
        fun yFor(v: Double): Float = padTop + h * (1f - ((v - minW) / range).toFloat())

        // Axis min/max labels + dashed guide lines.
        drawText(
            maxLabel,
            topLeft = Offset(0f, (yFor(maxW) - maxLabel.size.height / 2f).coerceAtLeast(0f)),
        )
        drawText(
            minLabel,
            topLeft = Offset(
                0f,
                (yFor(minW) - minLabel.size.height / 2f)
                    .coerceAtMost(size.height - minLabel.size.height),
            ),
        )
        val guide = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 6.dp.toPx()))
        drawLine(
            color = gridColor,
            start = Offset(padLeft, yFor(maxW)),
            end = Offset(padLeft + w, yFor(maxW)),
            strokeWidth = 1.dp.toPx(),
            pathEffect = guide,
        )
        drawLine(
            color = gridColor,
            start = Offset(padLeft, yFor(minW)),
            end = Offset(padLeft + w, yFor(minW)),
            strokeWidth = 1.dp.toPx(),
            pathEffect = guide,
        )

        // Least-squares trend, dashed Ember, clamped to the plot area.
        drawLine(
            color = Ember,
            start = Offset(
                xFor(minDay),
                yFor(intercept + slopePerDay * minDay).coerceIn(padTop, padTop + h),
            ),
            end = Offset(
                xFor(maxDay),
                yFor(intercept + slopePerDay * maxDay).coerceIn(padTop, padTop + h),
            ),
            strokeWidth = 2.dp.toPx(),
            cap = StrokeCap.Round,
            pathEffect = PathEffect.dashPathEffect(floatArrayOf(10.dp.toPx(), 8.dp.toPx())),
        )

        // Weight polyline over the trend, dots on every entry.
        val points = weights.map { Offset(xFor(it.epochDay), yFor(it.weightLbs)) }
        for (i in 0 until points.size - 1) {
            drawLine(
                color = lineColor,
                start = points[i],
                end = points[i + 1],
                strokeWidth = 3.dp.toPx(),
                cap = StrokeCap.Round,
            )
        }
        points.forEach { drawCircle(color = lineColor, radius = 3.dp.toPx(), center = it) }
        drawCircle(color = Ember, radius = 4.dp.toPx(), center = points.last())
    }
}

// -------------------------------------------------------------- achievements

@Composable
private fun AchievementCard(
    achievement: Achievement,
    unlocked: Boolean,
    modifier: Modifier = Modifier,
) {
    val emojiScale by animateFloatAsState(
        targetValue = if (unlocked) 1f else 0.9f,
        animationSpec = tween(300),
        label = "achievementScale",
    )
    Card(
        modifier = modifier,
        shape = MaterialTheme.shapes.medium,
        colors = CardDefaults.cardColors(
            containerColor = if (unlocked) {
                MaterialTheme.colorScheme.primaryContainer
            } else {
                MaterialTheme.colorScheme.surface
            },
        ),
    ) {
        Column(
            Modifier
                .fillMaxSize()
                .alpha(if (unlocked) 1f else 0.55f)
                .padding(14.dp),
        ) {
            Text(
                if (unlocked) achievement.emoji else "🔒",
                style = MaterialTheme.typography.displaySmall,
                modifier = Modifier.scale(emojiScale),
            )
            Spacer(Modifier.height(8.dp))
            Text(
                achievement.title,
                style = MaterialTheme.typography.titleSmall,
                color = if (unlocked) {
                    MaterialTheme.colorScheme.onPrimaryContainer
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
            )
            Spacer(Modifier.height(2.dp))
            Text(
                achievement.description,
                style = MaterialTheme.typography.bodySmall,
                color = if (unlocked) {
                    MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.8f)
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
            Spacer(Modifier.weight(1f))
            Spacer(Modifier.height(8.dp))
            Box(
                Modifier.background(
                    color = if (unlocked) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.surfaceVariant
                    },
                    shape = MaterialTheme.shapes.small,
                ),
            ) {
                Text(
                    "+${achievement.xpReward} XP",
                    style = MaterialTheme.typography.labelSmall,
                    color = if (unlocked) {
                        MaterialTheme.colorScheme.onPrimary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                )
            }
        }
    }
}
