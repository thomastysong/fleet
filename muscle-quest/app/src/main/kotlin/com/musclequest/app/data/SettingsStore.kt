package com.musclequest.app.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.musclequest.app.domain.CycleEngine
import java.time.LocalDate
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "settings")

data class Settings(
    val cycleStartEpochDay: Long,
    val activeWeeks: Int,
    val remindersEnabled: Boolean,
) {
    val cycleStart: LocalDate get() = LocalDate.ofEpochDay(cycleStartEpochDay)
}

class SettingsStore(private val context: Context) {

    private object Keys {
        val CYCLE_START = longPreferencesKey("cycle_start_epoch_day")
        val ACTIVE_WEEKS = intPreferencesKey("active_weeks")
        val REMINDERS = booleanPreferencesKey("reminders_enabled")
    }

    val settings: Flow<Settings> = context.dataStore.data.map { prefs ->
        Settings(
            cycleStartEpochDay = prefs[Keys.CYCLE_START] ?: LocalDate.now().toEpochDay(),
            activeWeeks = prefs[Keys.ACTIVE_WEEKS] ?: CycleEngine.DEFAULT_ACTIVE_WEEKS,
            remindersEnabled = prefs[Keys.REMINDERS] ?: true,
        )
    }

    suspend fun setCycleStart(date: LocalDate) {
        context.dataStore.edit { it[Keys.CYCLE_START] = date.toEpochDay() }
    }

    suspend fun setActiveWeeks(weeks: Int) {
        context.dataStore.edit {
            it[Keys.ACTIVE_WEEKS] = weeks.coerceIn(CycleEngine.DEFAULT_ACTIVE_WEEKS, CycleEngine.MAX_ACTIVE_WEEKS)
        }
    }

    suspend fun setRemindersEnabled(enabled: Boolean) {
        context.dataStore.edit { it[Keys.REMINDERS] = enabled }
    }
}
