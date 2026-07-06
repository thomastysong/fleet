package com.musclequest.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.Settings
import com.musclequest.app.domain.CycleEngine
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.Phase
import com.musclequest.app.ui.theme.Ember
import com.musclequest.app.ui.theme.Volt
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

private val fmt = DateTimeFormatter.ofPattern("MMM d, yyyy", Locale.US)

private data class PhaseSpan(val phase: Phase, val name: String, val start: LocalDate, val endExclusive: LocalDate, val color: Color)

@Composable
fun CycleScreen(settings: Settings?, status: CycleStatus?) {
    if (settings == null || status == null) return
    val start = settings.cycleStart
    val onEnd = start.plusDays(settings.activeWeeks * 7L)
    val pctEnd = onEnd.plusDays(CycleEngine.PCT_DAYS.toLong())
    val recoveryEnd = pctEnd.plusDays(CycleEngine.RECOVERY_DAYS.toLong())
    val spans = listOf(
        PhaseSpan(Phase.ON_CYCLE, "Active cycle · ${settings.activeWeeks} weeks", start, onEnd, Volt),
        PhaseSpan(Phase.PCT, "PCT — Alpha-AF · 30 days", onEnd, pctEnd, Ember),
        PhaseSpan(Phase.RECOVERY, "Full recovery · 8 weeks", pctEnd, recoveryEnd, Color(0xFF6E7681)),
    )
    val today = LocalDate.now()

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Text("Cycle Map", style = MaterialTheme.typography.headlineMedium)
            Text(
                "One full run: active stack → PCT → recovery. No overlap, no shortcuts.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        item {
            Column {
                spans.forEach { span ->
                    PhaseCard(span, today)
                    Spacer(Modifier.height(8.dp))
                }
            }
        }
        item { RulesCard() }
        item { DisclaimerCard() }
        item { Spacer(Modifier.height(64.dp)) }
    }
}

@Composable
private fun PhaseCard(span: PhaseSpan, today: LocalDate) {
    val active = !today.isBefore(span.start) && today.isBefore(span.endExclusive)
    val done = !today.isBefore(span.endExclusive)
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (active) MaterialTheme.colorScheme.surfaceVariant else MaterialTheme.colorScheme.surface,
        ),
    ) {
        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(14.dp)
                    .clip(CircleShape)
                    .background(span.color),
            )
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(span.name, style = MaterialTheme.typography.titleMedium)
                Text(
                    "${span.start.format(fmt)} → ${span.endExclusive.minusDays(1).format(fmt)}",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                when {
                    done -> "DONE ✔"
                    active -> "NOW"
                    else -> "AHEAD"
                },
                style = MaterialTheme.typography.labelSmall,
                color = if (active) span.color else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .background(MaterialTheme.colorScheme.surface)
                    .padding(horizontal = 8.dp, vertical = 4.dp),
            )
        }
    }
}

@Composable
private fun RulesCard() {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Text("Non-negotiables", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(6.dp))
            listOf(
                "Doses 10–12 h apart, every day — rest days included.",
                "Every dose on an empty stomach; wait 30–60 min before eating.",
                "PCT starts the day after the last active dose: Alpha-AF, 3 caps with the first meal, 30 days straight.",
                "After PCT: 8 full weeks with zero andro products — next cycle " +
                    "only after blood work is back to baseline.",
                "Calorie surplus every day — never cut on cycle.",
                "Zero alcohol during the cycle and PCT.",
                "1 gallon of water daily; keep sodium, potassium, magnesium up.",
                "7–8 h of sleep. Bed by 9 PM on training nights.",
            ).forEach {
                Row {
                    Text("•  ", color = MaterialTheme.colorScheme.primary)
                    Text(it, style = MaterialTheme.typography.bodyMedium)
                }
                Spacer(Modifier.height(4.dp))
            }
        }
    }
}

@Composable
private fun DisclaimerCard() {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Column(Modifier.padding(14.dp)) {
            Text("⚕️ Health notice", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(4.dp))
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
