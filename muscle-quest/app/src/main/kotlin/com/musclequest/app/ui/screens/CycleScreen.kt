package com.musclequest.app.ui.screens

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.Settings
import com.musclequest.app.domain.CycleEngine
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.Phase
import com.musclequest.app.ui.theme.Ember
import com.musclequest.app.ui.theme.MQ
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

private val dayFmt = DateTimeFormatter.ofPattern("MMM d", Locale.US)
private val fullFmt = DateTimeFormatter.ofPattern("MMM d, yyyy", Locale.US)

private enum class NodeState { DONE, CURRENT, FUTURE }

private fun nodeState(
    nodePhase: Phase,
    endExclusive: LocalDate?,
    today: LocalDate,
    status: CycleStatus,
): NodeState = when {
    status.daysUntilStart == 0 && status.phase == nodePhase -> NodeState.CURRENT
    endExclusive != null && !today.isBefore(endExclusive) -> NodeState.DONE
    else -> NodeState.FUTURE
}

@Composable
fun CycleScreen(settings: Settings?, status: CycleStatus?) {
    if (settings == null || status == null) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
        return
    }

    val start = settings.cycleStart
    val onEnd = start.plusDays(settings.activeWeeks * 7L)
    val pctEnd = onEnd.plusDays(CycleEngine.PCT_DAYS.toLong())
    val recoveryEnd = pctEnd.plusDays(CycleEngine.RECOVERY_DAYS.toLong())
    val postLabsDate = pctEnd.plusDays(14)
    val today = LocalDate.now()

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Column {
                Text("Cycle Map", style = MaterialTheme.typography.headlineMedium)
                Spacer(Modifier.height(4.dp))
                Text(
                    "One full run: active stack → PCT → recovery. No overlap, no shortcuts.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        item {
            Column {
                CheckpointRow(
                    label = "🩸 Baseline labs",
                    dateLabel = "before ${start.format(dayFmt)}",
                    done = !today.isBefore(start),
                    isFirst = true,
                )
                TimelineNodeRow(
                    title = "Active cycle",
                    color = MaterialTheme.colorScheme.primary,
                    dateLabel = "${start.format(dayFmt)} → ${onEnd.minusDays(1).format(fullFmt)}",
                    durationLabel = "${settings.activeWeeks} weeks",
                    state = nodeState(Phase.ON_CYCLE, onEnd, today, status),
                    countdownDays = status.daysUntilStart,
                    currentStatus = status,
                )
                TimelineNodeRow(
                    title = "PCT — Alpha-AF",
                    color = Ember,
                    dateLabel = "${onEnd.format(dayFmt)} → ${pctEnd.minusDays(1).format(fullFmt)}",
                    durationLabel = "${CycleEngine.PCT_DAYS} days",
                    state = nodeState(Phase.PCT, pctEnd, today, status),
                    currentStatus = status,
                )
                TimelineNodeRow(
                    title = "Full recovery",
                    color = MQ.recovery,
                    dateLabel = "${pctEnd.format(dayFmt)} → ${recoveryEnd.minusDays(1).format(fullFmt)}",
                    durationLabel = "8 weeks",
                    state = nodeState(Phase.RECOVERY, recoveryEnd, today, status),
                    currentStatus = status,
                )
                CheckpointRow(
                    label = "🩸 Post-cycle labs",
                    dateLabel = "~${postLabsDate.format(fullFmt)}",
                    done = today.isAfter(postLabsDate),
                )
                TimelineNodeRow(
                    title = "Next cycle eligible",
                    color = MQ.gold,
                    dateLabel = recoveryEnd.format(fullFmt),
                    durationLabel = "Only with baseline blood work",
                    state = nodeState(Phase.MAINTENANCE, null, today, status),
                    currentStatus = status,
                    isLast = true,
                )
            }
        }
        item { RulesCard() }
        item { DisclaimerCard() }
        item { Spacer(Modifier.height(88.dp)) }
    }
}

@Composable
private fun TimelineNodeRow(
    title: String,
    color: Color,
    dateLabel: String,
    durationLabel: String,
    state: NodeState,
    currentStatus: CycleStatus,
    isFirst: Boolean = false,
    isLast: Boolean = false,
    countdownDays: Int = 0,
) {
    val lineColor = MaterialTheme.colorScheme.outlineVariant
    Row(
        Modifier
            .fillMaxWidth()
            .height(IntrinsicSize.Min),
    ) {
        Column(
            Modifier
                .width(36.dp)
                .fillMaxHeight(),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                Modifier
                    .height(20.dp)
                    .width(2.dp)
                    .background(if (isFirst) Color.Transparent else lineColor),
            )
            PhaseDot(color, state)
            if (!isLast) {
                Box(
                    Modifier
                        .weight(1f)
                        .width(2.dp)
                        .background(lineColor),
                )
            } else {
                Spacer(Modifier.weight(1f))
            }
        }
        Spacer(Modifier.width(10.dp))
        Box(
            Modifier
                .weight(1f)
                .padding(bottom = if (isLast) 0.dp else 12.dp),
        ) {
            PhaseNodeCard(title, color, dateLabel, durationLabel, state, currentStatus, countdownDays)
        }
    }
}

@Composable
private fun PhaseDot(color: Color, state: NodeState) {
    when (state) {
        NodeState.CURRENT -> Box(
            Modifier
                .size(20.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = 0.30f)),
            contentAlignment = Alignment.Center,
        ) {
            Box(
                Modifier
                    .size(11.dp)
                    .clip(CircleShape)
                    .background(color),
            )
        }
        NodeState.DONE -> Box(
            Modifier
                .size(14.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = 0.55f)),
        )
        NodeState.FUTURE -> Box(
            Modifier
                .size(14.dp)
                .border(2.dp, color.copy(alpha = 0.65f), CircleShape),
        )
    }
}

@Composable
private fun PhaseNodeCard(
    title: String,
    color: Color,
    dateLabel: String,
    durationLabel: String,
    state: NodeState,
    currentStatus: CycleStatus,
    countdownDays: Int,
) {
    val containerColor = when (state) {
        NodeState.CURRENT -> MaterialTheme.colorScheme.surfaceVariant
        NodeState.DONE -> MaterialTheme.colorScheme.surface
        NodeState.FUTURE -> Color.Transparent
    }
    val border = when (state) {
        NodeState.CURRENT -> BorderStroke(1.dp, color.copy(alpha = 0.45f))
        NodeState.FUTURE -> BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
        NodeState.DONE -> null
    }
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .alpha(if (state == NodeState.DONE) 0.65f else 1f),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = border,
    ) {
        Column(
            Modifier
                .padding(16.dp)
                .animateContentSize(),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    title,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                when {
                    state == NodeState.CURRENT -> StatusChip("NOW", color)
                    state == NodeState.DONE -> Text(
                        "DONE ✓",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    countdownDays > 0 -> StatusChip("IN ${countdownDays}D", color)
                }
            }
            Spacer(Modifier.height(4.dp))
            Text(
                dateLabel,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(2.dp))
            Text(
                durationLabel,
                style = MaterialTheme.typography.labelMedium,
                color = color,
            )
            if (state == NodeState.CURRENT && currentStatus.phaseLengthDays > 0) {
                Spacer(Modifier.height(10.dp))
                val progress by animateFloatAsState(
                    targetValue = currentStatus.progress,
                    animationSpec = tween(700),
                    label = "phaseProgress",
                )
                LinearProgressIndicator(
                    progress = { progress },
                    modifier = Modifier.fillMaxWidth(),
                    color = color,
                    trackColor = MaterialTheme.colorScheme.surface,
                )
                Spacer(Modifier.height(6.dp))
                Text(
                    "Day ${currentStatus.dayInPhase} of ${currentStatus.phaseLengthDays} · week ${currentStatus.weekInPhase}",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun StatusChip(text: String, color: Color) {
    Box(
        Modifier
            .clip(CircleShape)
            .background(color.copy(alpha = 0.18f))
            .padding(horizontal = 10.dp, vertical = 4.dp),
    ) {
        Text(
            text,
            style = MaterialTheme.typography.labelSmall,
            color = color,
            fontWeight = FontWeight.Bold,
        )
    }
}

@Composable
private fun CheckpointRow(
    label: String,
    dateLabel: String,
    done: Boolean,
    isFirst: Boolean = false,
) {
    val lineColor = MaterialTheme.colorScheme.outlineVariant
    Row(
        Modifier
            .fillMaxWidth()
            .height(IntrinsicSize.Min),
    ) {
        Column(
            Modifier
                .width(36.dp)
                .fillMaxHeight(),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                Modifier
                    .height(12.dp)
                    .width(2.dp)
                    .background(if (isFirst) Color.Transparent else lineColor),
            )
            Box(
                Modifier
                    .size(8.dp)
                    .clip(CircleShape)
                    .background(MaterialTheme.colorScheme.error),
            )
            Box(
                Modifier
                    .weight(1f)
                    .width(2.dp)
                    .background(lineColor),
            )
        }
        Spacer(Modifier.width(10.dp))
        Surface(
            shape = MaterialTheme.shapes.small,
            color = MaterialTheme.colorScheme.error.copy(alpha = 0.10f),
            modifier = Modifier
                .padding(bottom = 12.dp)
                .alpha(if (done) 0.6f else 1f),
        ) {
            Text(
                "$label · $dateLabel",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            )
        }
    }
}

@Composable
private fun RulesCard() {
    val rules = listOf(
        "Doses 10–12 h apart, every day — rest days included.",
        "Every dose on an empty stomach; wait 30–60 min before eating.",
        "PCT starts the day after the last active dose: Alpha-AF, 3 caps with the first meal, 30 days straight.",
        "After PCT: 8 full weeks with zero andro products — next cycle " +
            "only after blood work is back to baseline.",
        "Calorie surplus every day — never cut on cycle.",
        "Zero alcohol during the cycle and PCT.",
        "1 gallon of water daily; keep sodium, potassium, magnesium up.",
        "7–8 h of sleep. Bed by 9 PM on training nights.",
    )
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Text("Non-negotiables", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            rules.forEachIndexed { index, rule ->
                Row(verticalAlignment = Alignment.Top) {
                    Box(
                        Modifier
                            .padding(top = 7.dp)
                            .size(6.dp)
                            .clip(CircleShape)
                            .background(MaterialTheme.colorScheme.primary),
                    )
                    Spacer(Modifier.width(10.dp))
                    Text(rule, style = MaterialTheme.typography.bodyMedium)
                }
                if (index != rules.lastIndex) Spacer(Modifier.height(6.dp))
            }
        }
    }
}

@Composable
private fun DisclaimerCard() {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = Ember.copy(alpha = 0.10f)),
        border = BorderStroke(1.dp, Ember.copy(alpha = 0.35f)),
    ) {
        Column(Modifier.padding(16.dp)) {
            Text("⚕️ Health notice", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(6.dp))
            Text(
                "Andro compounds alter your hormonal environment. Get baseline blood work " +
                    "(lipids, liver enzymes, total/free testosterone) before and after the cycle, " +
                    "and talk to a physician first. This app tracks your plan — it is not medical advice.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
