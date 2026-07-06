package com.musclequest.app.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.musclequest.app.domain.ActivityLevel
import com.musclequest.app.domain.BodyProfile
import com.musclequest.app.domain.CycleEngine
import java.time.LocalDate
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "settings")

data class Settings(
    val cycleStartEpochDay: Long,
    val activeWeeks: Int,
    val remindersEnabled: Boolean,
    val ageYears: Int,
    val heightInches: Int,
    val isMale: Boolean,
    val activity: ActivityLevel,
    val surplusKcal: Int,
    /** Fallback until the first weigh-in is logged. */
    val startWeightLbs: Double,
) {
    val cycleStart: LocalDate get() = LocalDate.ofEpochDay(cycleStartEpochDay)

    fun profile(currentWeightLbs: Double?): BodyProfile = BodyProfile(
        ageYears = ageYears,
        heightInches = heightInches,
        weightLbs = currentWeightLbs ?: startWeightLbs,
        isMale = isMale,
        activity = activity,
        surplusKcal = surplusKcal,
    )
}

class SettingsStore(private val context: Context) {

    private object Keys {
        val CYCLE_START = longPreferencesKey("cycle_start_epoch_day")
        val ACTIVE_WEEKS = intPreferencesKey("active_weeks")
        val REMINDERS = booleanPreferencesKey("reminders_enabled")
        val AGE = intPreferencesKey("age_years")
        val HEIGHT_IN = intPreferencesKey("height_inches")
        val IS_MALE = booleanPreferencesKey("is_male")
        val ACTIVITY = stringPreferencesKey("activity_level")
        val SURPLUS = intPreferencesKey("surplus_kcal")
        val START_WEIGHT = intPreferencesKey("start_weight_lbs")
    }

    val settings: Flow<Settings> = context.dataStore.data.map { prefs ->
        Settings(
            cycleStartEpochDay = prefs[Keys.CYCLE_START] ?: LocalDate.now().toEpochDay(),
            activeWeeks = prefs[Keys.ACTIVE_WEEKS] ?: CycleEngine.DEFAULT_ACTIVE_WEEKS,
            remindersEnabled = prefs[Keys.REMINDERS] ?: true,
            ageYears = prefs[Keys.AGE] ?: 31,
            heightInches = prefs[Keys.HEIGHT_IN] ?: 72,
            isMale = prefs[Keys.IS_MALE] ?: true,
            activity = prefs[Keys.ACTIVITY]?.let { name ->
                ActivityLevel.entries.firstOrNull { it.name == name }
            } ?: ActivityLevel.MODERATE,
            surplusKcal = prefs[Keys.SURPLUS] ?: 400,
            startWeightLbs = (prefs[Keys.START_WEIGHT] ?: 170).toDouble(),
        )
    }

    /**
     * The cycle-start default must be made durable on first launch: a value
     * synthesized at read time re-anchors to "today" on every process start,
     * so the cycle would sit on Day 1 forever and never reach PCT.
     */
    suspend fun ensureInitialized() {
        context.dataStore.edit { prefs ->
            if (prefs[Keys.CYCLE_START] == null) {
                prefs[Keys.CYCLE_START] = LocalDate.now().toEpochDay()
            }
        }
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

    suspend fun setAge(years: Int) {
        context.dataStore.edit { it[Keys.AGE] = years.coerceIn(16, 90) }
    }

    suspend fun setHeightInches(inches: Int) {
        context.dataStore.edit { it[Keys.HEIGHT_IN] = inches.coerceIn(48, 90) }
    }

    suspend fun setIsMale(male: Boolean) {
        context.dataStore.edit { it[Keys.IS_MALE] = male }
    }

    suspend fun setActivity(level: ActivityLevel) {
        context.dataStore.edit { it[Keys.ACTIVITY] = level.name }
    }

    suspend fun setSurplus(kcal: Int) {
        context.dataStore.edit { it[Keys.SURPLUS] = kcal.coerceIn(0, 1000) }
    }

    suspend fun setStartWeight(lbs: Int) {
        context.dataStore.edit { it[Keys.START_WEIGHT] = lbs.coerceIn(80, 400) }
    }
}
