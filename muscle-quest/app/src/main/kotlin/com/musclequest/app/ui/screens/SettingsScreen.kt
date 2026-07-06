package com.musclequest.app.ui.screens

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateIntAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.Remove
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.Settings
import com.musclequest.app.domain.ActivityLevel
import com.musclequest.app.domain.MacroTargets
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs

private val fmt = DateTimeFormatter.ofPattern("EEEE, MMM d, yyyy", Locale.US)

private val SURPLUS_OPTIONS = listOf(
    250 to "Lean",
    400 to "Standard",
    500 to "Aggressive",
)

private fun kcalFmt(value: Int): String = "%,d".format(Locale.US, value)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    settings: Settings?,
    targets: MacroTargets,
    onSetCycleStart: (LocalDate) -> Unit,
    onSetActiveWeeks: (Int) -> Unit,
    onSetReminders: (Boolean) -> Unit,
    onSetAge: (Int) -> Unit,
    onSetHeight: (Int) -> Unit,
    onSetIsMale: (Boolean) -> Unit,
    onSetActivity: (ActivityLevel) -> Unit,
    onSetSurplus: (Int) -> Unit,
) {
    if (settings == null) return
    var showDatePicker by remember { mutableStateOf(false) }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Column {
                Text("Settings", style = MaterialTheme.typography.headlineMedium)
                Text(
                    "Dial in your cycle, body, and fuel math.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        item { CycleStartCard(settings) { showDatePicker = true } }
        item { CycleLengthCard(settings.activeWeeks, onSetActiveWeeks) }
        item {
            BodyProfileCard(
                settings = settings,
                onSetAge = onSetAge,
                onSetHeight = onSetHeight,
                onSetIsMale = onSetIsMale,
                onSetActivity = onSetActivity,
                onSetSurplus = onSetSurplus,
            )
        }
        item { TargetsCard(targets) }
        item { RemindersCard(settings.remindersEnabled, onSetReminders) }
        item { Spacer(Modifier.height(88.dp)) }
    }

    if (showDatePicker) {
        val state = rememberDatePickerState(
            initialSelectedDateMillis = settings.cycleStart.atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli(),
        )
        DatePickerDialog(
            onDismissRequest = { showDatePicker = false },
            confirmButton = {
                TextButton(onClick = {
                    state.selectedDateMillis?.let { millis ->
                        onSetCycleStart(Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate())
                    }
                    showDatePicker = false
                }) { Text("Set") }
            },
            dismissButton = {
                TextButton(onClick = { showDatePicker = false }) { Text("Cancel") }
            },
        ) {
            DatePicker(state = state)
        }
    }
}

/* ---------------------------------------------------------------- cards */

@Composable
private fun CycleStartCard(settings: Settings, onChangeDate: () -> Unit) {
    SettingsCard {
        Eyebrow("📅 CYCLE START")
        Spacer(Modifier.height(2.dp))
        Text("Cycle start date", style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(4.dp))
        Text(
            settings.cycleStart.format(fmt),
            style = MaterialTheme.typography.titleLarge,
            color = MaterialTheme.colorScheme.primary,
        )
        Spacer(Modifier.height(10.dp))
        OutlinedButton(onClick = onChangeDate) {
            Icon(Icons.Filled.DateRange, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text("Change start date")
        }
        Spacer(Modifier.height(6.dp))
        Text(
            "Tip: start on a Monday so week boundaries line up with your training split.",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun CycleLengthCard(activeWeeks: Int, onSetActiveWeeks: (Int) -> Unit) {
    SettingsCard {
        Eyebrow("⏳ CYCLE LENGTH")
        Spacer(Modifier.height(2.dp))
        Text("Active cycle length", style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(4.dp))
        Text(
            "4 weeks is the conservative first run, 6 a middle path once " +
                "you know your tolerance, 8 the maximum for experienced " +
                "users monitoring tolerance only.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(10.dp))
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            listOf(4, 6, 8).forEachIndexed { index, weeks ->
                SegmentedButton(
                    selected = activeWeeks == weeks,
                    onClick = { onSetActiveWeeks(weeks) },
                    shape = SegmentedButtonDefaults.itemShape(index = index, count = 3),
                ) {
                    Text("$weeks wk")
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BodyProfileCard(
    settings: Settings,
    onSetAge: (Int) -> Unit,
    onSetHeight: (Int) -> Unit,
    onSetIsMale: (Boolean) -> Unit,
    onSetActivity: (ActivityLevel) -> Unit,
    onSetSurplus: (Int) -> Unit,
) {
    SettingsCard {
        Eyebrow("💪 ABOUT YOU")
        Spacer(Modifier.height(2.dp))
        Text("Body profile", style = MaterialTheme.typography.titleMedium)
        Text(
            "Feeds the BMR and TDEE math below.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(10.dp))

        StepperRow(
            label = "Age",
            value = "${settings.ageYears} yr",
            onDecrement = { onSetAge(settings.ageYears - 1) },
            onIncrement = { onSetAge(settings.ageYears + 1) },
        )
        Spacer(Modifier.height(6.dp))
        StepperRow(
            label = "Height",
            value = "${settings.heightInches / 12}'${settings.heightInches % 12}\"",
            onDecrement = { onSetHeight(settings.heightInches - 1) },
            onIncrement = { onSetHeight(settings.heightInches + 1) },
        )

        Spacer(Modifier.height(12.dp))
        GroupLabel("Sex")
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            SegmentedButton(
                selected = settings.isMale,
                onClick = { onSetIsMale(true) },
                shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
            ) { Text("Male") }
            SegmentedButton(
                selected = !settings.isMale,
                onClick = { onSetIsMale(false) },
                shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
            ) { Text("Female") }
        }

        Spacer(Modifier.height(12.dp))
        GroupLabel("Activity level")
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            ActivityLevel.entries.forEachIndexed { index, level ->
                SegmentedButton(
                    selected = settings.activity == level,
                    onClick = { onSetActivity(level) },
                    shape = SegmentedButtonDefaults.itemShape(
                        index = index,
                        count = ActivityLevel.entries.size,
                    ),
                    icon = {},
                ) {
                    Text(
                        level.label,
                        style = MaterialTheme.typography.labelSmall,
                        textAlign = TextAlign.Center,
                        maxLines = 2,
                    )
                }
            }
        }

        Spacer(Modifier.height(12.dp))
        GroupLabel("Daily surplus")
        val nearestSurplus = SURPLUS_OPTIONS.minBy { abs(it.first - settings.surplusKcal) }.first
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            SURPLUS_OPTIONS.forEachIndexed { index, (kcal, name) ->
                SegmentedButton(
                    selected = nearestSurplus == kcal,
                    onClick = { onSetSurplus(kcal) },
                    shape = SegmentedButtonDefaults.itemShape(
                        index = index,
                        count = SURPLUS_OPTIONS.size,
                    ),
                    icon = {},
                ) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text(name, style = MaterialTheme.typography.labelMedium)
                        Text(
                            "+$kcal",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun TargetsCard(targets: MacroTargets) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .animateContentSize(),
        shape = MaterialTheme.shapes.large,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.primaryContainer),
        colors = CardDefaults.cardColors(),
    ) {
        Column(Modifier.fillMaxWidth().padding(16.dp)) {
            Eyebrow("🎯 THE SCIENCE")
            Spacer(Modifier.height(2.dp))
            Text("Your targets", style = MaterialTheme.typography.titleMedium)
            Text(
                "Live from your profile and latest weigh-in.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(10.dp))

            TargetRow("🧬", "BMR (Mifflin-St Jeor)", "${kcalFmt(targets.bmrKcal)} kcal")
            TargetRow("⚡", "TDEE", "${kcalFmt(targets.tdeeKcal)} kcal")

            HorizontalDivider(
                Modifier.padding(vertical = 6.dp),
                color = MaterialTheme.colorScheme.outlineVariant,
            )

            val animatedKcal by animateIntAsState(
                targetValue = targets.kcal,
                animationSpec = tween(600),
                label = "dailyKcal",
            )
            Row(
                Modifier.fillMaxWidth().padding(vertical = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    "🔥  Daily target",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.weight(1f),
                )
                Row {
                    Text(
                        kcalFmt(animatedKcal),
                        style = MaterialTheme.typography.headlineMedium,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.alignByBaseline(),
                    )
                    Text(
                        " kcal",
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.alignByBaseline(),
                    )
                }
            }

            HorizontalDivider(
                Modifier.padding(vertical = 6.dp),
                color = MaterialTheme.colorScheme.outlineVariant,
            )

            TargetRow("🥩", "Protein", "${targets.proteinG} g")
            TargetRow("🍚", "Carbs", "${targets.carbsG} g")
            TargetRow("🥑", "Fat", "${targets.fatG} g")
            TargetRow("💧", "Water", "${targets.waterOz} oz")

            Spacer(Modifier.height(8.dp))
            Text(
                "Recalculates automatically as your logged weight changes.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun TargetRow(emoji: String, label: String, value: String) {
    Row(
        Modifier.fillMaxWidth().padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            "$emoji  $label",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(value, style = MaterialTheme.typography.titleSmall)
    }
}

@Composable
private fun RemindersCard(enabled: Boolean, onSetReminders: (Boolean) -> Unit) {
    val eyebrowColor by animateColorAsState(
        targetValue = if (enabled) {
            MaterialTheme.colorScheme.primary
        } else {
            MaterialTheme.colorScheme.onSurfaceVariant
        },
        label = "reminderTint",
    )
    SettingsCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(
                    if (enabled) "🔔 REMINDERS" else "🔕 REMINDERS",
                    style = MaterialTheme.typography.labelMedium,
                    color = eyebrowColor,
                )
                Spacer(Modifier.height(2.dp))
                Text("Daily reminders", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(2.dp))
                Text(
                    "Dose + wake-up, PM dose window, evening wind-down. " +
                        "Phase-aware; weekend times shift later.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.width(12.dp))
            Switch(checked = enabled, onCheckedChange = onSetReminders)
        }
    }
}

/* -------------------------------------------------------------- helpers */

@Composable
private fun SettingsCard(content: @Composable ColumnScope.() -> Unit) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.fillMaxWidth().padding(16.dp), content = content)
    }
}

@Composable
private fun Eyebrow(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.primary,
    )
}

@Composable
private fun GroupLabel(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Spacer(Modifier.height(4.dp))
}

/** "- value +" row with tonal icon buttons and an animated value swap. */
@Composable
private fun StepperRow(
    label: String,
    value: String,
    onDecrement: () -> Unit,
    onIncrement: () -> Unit,
) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        FilledTonalIconButton(onClick = onDecrement, modifier = Modifier.size(36.dp)) {
            Icon(
                Icons.Filled.Remove,
                contentDescription = "Decrease $label",
                modifier = Modifier.size(18.dp),
            )
        }
        AnimatedContent(targetState = value, label = "stepper-$label") { v ->
            Text(
                v,
                style = MaterialTheme.typography.titleLarge,
                textAlign = TextAlign.Center,
                modifier = Modifier.widthIn(min = 76.dp),
            )
        }
        FilledTonalIconButton(onClick = onIncrement, modifier = Modifier.size(36.dp)) {
            Icon(
                Icons.Filled.Add,
                contentDescription = "Increase $label",
                modifier = Modifier.size(18.dp),
            )
        }
    }
}
