package com.musclequest.app.data

import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/** One checked-off task (or awarded bonus) on a given day. */
@Entity(
    tableName = "completions",
    indices = [Index(value = ["epochDay", "taskId"], unique = true)],
)
data class Completion(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val epochDay: Long,
    val taskId: String,
    val xp: Int,
    val completedAtMillis: Long,
)

/** One logged working set. */
@Entity(tableName = "workout_sets", indices = [Index("epochDay"), Index("exercise")])
data class WorkoutSet(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val epochDay: Long,
    val exercise: String,
    val weightLbs: Double,
    val reps: Int,
    val isPr: Boolean,
)

/** Morning body-weight entry, one per day. */
@Entity(tableName = "weight_entries")
data class WeightEntry(
    @PrimaryKey val epochDay: Long,
    val weightLbs: Double,
)

@Entity(tableName = "unlocked_achievements")
data class UnlockedAchievement(
    @PrimaryKey val achievementId: String,
    val unlockedAtMillis: Long,
)

/** One logged food/meal with its macros. */
@Entity(tableName = "food_entries", indices = [Index("epochDay")])
data class FoodEntry(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val epochDay: Long,
    val name: String,
    val kcal: Int,
    val proteinG: Int,
    val carbsG: Int,
    val fatG: Int,
    val loggedAtMillis: Long,
)
