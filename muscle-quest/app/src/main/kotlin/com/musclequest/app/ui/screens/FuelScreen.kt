package com.musclequest.app.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.musclequest.app.data.FoodEntry
import com.musclequest.app.domain.FoodPreset
import com.musclequest.app.ui.FuelUiState
import com.musclequest.app.ui.components.MacroBar
import com.musclequest.app.ui.components.ProgressRing
import com.musclequest.app.ui.theme.Ember
import com.musclequest.app.ui.theme.MQ
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

private val timeFmt = DateTimeFormatter.ofPattern("h:mm a", Locale.US)

@Composable
fun FuelScreen(
    state: FuelUiState,
    onLogPreset: (FoodPreset) -> Unit,
    onLogCustom: (String, Int, Int, Int, Int) -> Unit, // name, kcal, proteinG, carbsG, fatG
    onDelete: (FoodEntry) -> Unit,
) {
    var showCustomDialog by remember { mutableStateOf(false) }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { FuelHeader() }
        item(key = "dashboard") { DashboardCard(state) }

        item(key = "quick_add_header") { SectionHeader("⚡ Quick add") }
        items(state.presets.chunked(2)) { pair -> PresetRow(pair, onLogPreset) }
        item(key = "custom_button") {
            OutlinedButton(
                onClick = { showCustomDialog = true },
                modifier = Modifier.fillMaxWidth(),
                shape = MaterialTheme.shapes.medium,
            ) {
                Icon(Icons.Filled.Add, contentDescription = null)
                Spacer(Modifier.width(8.dp))
                Text("Custom entry…")
            }
        }

        item(key = "log_header") { SectionHeader("📋 Today's log") }
        if (state.entries.isEmpty()) {
            item(key = "empty_log") { EmptyLogCard() }
        } else {
            items(state.entries.reversed(), key = { it.id }) { entry ->
                LogRow(entry, onDelete, Modifier.animateItem())
            }
        }

        item(key = "footer") {
            Text(
                "Pace your protein: ~0.4 g/kg per meal across 4–5 feedings beats one giant dinner.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 4.dp),
            )
        }
        item(key = "bottom_spacer") { Spacer(Modifier.height(88.dp)) }
    }

    if (showCustomDialog) {
        CustomEntryDialog(
            onConfirm = { name, kcal, protein, carbs, fat ->
                onLogCustom(name, kcal, protein, carbs, fat)
                showCustomDialog = false
            },
            onDismiss = { showCustomDialog = false },
        )
    }
}

@Composable
private fun FuelHeader() {
    Column {
        Text("Fuel", style = MaterialTheme.typography.headlineMedium)
        Text(
            "Targets from YOUR body — Mifflin-St Jeor, live-updated with your weight.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun SectionHeader(title: String) {
    Text(
        title,
        style = MaterialTheme.typography.titleMedium,
        modifier = Modifier.padding(top = 4.dp),
    )
}

@Composable
private fun DashboardCard(state: FuelUiState) {
    val totals = state.totals
    val targets = state.targets
    val overTarget = targets.kcal > 0 && totals.kcal > targets.kcal * 1.1
    val ringColor by animateColorAsState(
        targetValue = if (overTarget) Ember else MaterialTheme.colorScheme.primary,
        label = "kcalRingColor",
    )
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            ProgressRing(
                progress = if (targets.kcal > 0) totals.kcal.toFloat() / targets.kcal else 0f,
                size = 130.dp,
                stroke = 12.dp,
                color = ringColor,
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(
                        "${totals.kcal}",
                        style = MaterialTheme.typography.headlineMedium,
                        color = ringColor,
                    )
                    Text(
                        "of ${targets.kcal} kcal",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Spacer(Modifier.height(16.dp))
            MacroBar(
                label = "Protein",
                value = totals.proteinG,
                target = targets.proteinG,
                unit = "g",
                color = MaterialTheme.colorScheme.primary,
            )
            Spacer(Modifier.height(10.dp))
            MacroBar(
                label = "Carbs",
                value = totals.carbsG,
                target = targets.carbsG,
                unit = "g",
                color = MQ.nutrition,
            )
            Spacer(Modifier.height(10.dp))
            MacroBar(
                label = "Fat",
                value = totals.fatG,
                target = targets.fatG,
                unit = "g",
                color = MQ.recovery,
            )
            Spacer(Modifier.height(12.dp))
            Text(
                "BMR ${targets.bmrKcal} · TDEE ${targets.tdeeKcal} · surplus = growth",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            AnimatedVisibility(visible = state.proteinHit) {
                Surface(
                    color = MaterialTheme.colorScheme.primaryContainer,
                    shape = MaterialTheme.shapes.small,
                    modifier = Modifier.padding(top = 10.dp),
                ) {
                    Text(
                        "🎯 Protein target hit +20 XP",
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun PresetRow(pair: List<FoodPreset>, onLogPreset: (FoodPreset) -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .height(IntrinsicSize.Min),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        pair.forEach { preset ->
            PresetCard(
                preset = preset,
                onLogPreset = onLogPreset,
                modifier = Modifier
                    .weight(1f)
                    .fillMaxHeight(),
            )
        }
        if (pair.size == 1) Spacer(Modifier.weight(1f))
    }
}

@Composable
private fun PresetCard(
    preset: FoodPreset,
    onLogPreset: (FoodPreset) -> Unit,
    modifier: Modifier = Modifier,
) {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val scale by animateFloatAsState(if (pressed) 0.94f else 1f, label = "presetScale")
    Card(
        onClick = { onLogPreset(preset) },
        modifier = modifier.graphicsLayer {
            scaleX = scale
            scaleY = scale
        },
        shape = MaterialTheme.shapes.medium,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        interactionSource = interaction,
    ) {
        Column(Modifier.padding(12.dp)) {
            Text(preset.emoji, style = MaterialTheme.typography.headlineSmall)
            Spacer(Modifier.height(4.dp))
            Text(
                preset.name,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(2.dp))
            Text(
                "${preset.kcal} kcal · ${preset.proteinG}P",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun LogRow(entry: FoodEntry, onDelete: (FoodEntry) -> Unit, modifier: Modifier = Modifier) {
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(
            Modifier.padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        entry.name,
                        style = MaterialTheme.typography.titleSmall,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        Instant.ofEpochMilli(entry.loggedAtMillis)
                            .atZone(ZoneId.systemDefault())
                            .toLocalTime()
                            .format(timeFmt),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.secondary,
                    )
                }
                Text(
                    "${entry.kcal} kcal · ${entry.proteinG}P ${entry.carbsG}C ${entry.fatG}F",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            IconButton(onClick = { onDelete(entry) }) {
                Icon(
                    Icons.Filled.Delete,
                    contentDescription = "Delete ${entry.name}",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun EmptyLogCard() {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Text(
            "Nothing logged yet — three taps on a preset and you're tracking.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(16.dp),
        )
    }
}

@Composable
private fun CustomEntryDialog(
    onConfirm: (String, Int, Int, Int, Int) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf("") }
    var kcal by remember { mutableStateOf("") }
    var protein by remember { mutableStateOf("") }
    var carbs by remember { mutableStateOf("") }
    var fat by remember { mutableStateOf("") }
    val kcalValue = kcal.toIntOrNull() ?: 0

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Custom entry", fontWeight = FontWeight.Bold) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Food name") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    MacroField("kcal", kcal, { kcal = it }, Modifier.weight(1f))
                    MacroField("Protein g", protein, { protein = it }, Modifier.weight(1f))
                }
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    MacroField("Carbs g", carbs, { carbs = it }, Modifier.weight(1f))
                    MacroField("Fat g", fat, { fat = it }, Modifier.weight(1f))
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = name.isNotBlank() && kcalValue > 0,
                onClick = {
                    onConfirm(
                        name.trim(),
                        kcalValue,
                        protein.toIntOrNull() ?: 0,
                        carbs.toIntOrNull() ?: 0,
                        fat.toIntOrNull() ?: 0,
                    )
                },
            ) { Text("Log it") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}

@Composable
private fun MacroField(
    label: String,
    value: String,
    onChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    OutlinedTextField(
        value = value,
        onValueChange = { new -> onChange(new.filter { it.isDigit() }.take(5)) },
        label = { Text(label) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = modifier,
    )
}
