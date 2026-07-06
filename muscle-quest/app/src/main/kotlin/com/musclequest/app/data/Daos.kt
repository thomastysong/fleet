package com.musclequest.app.data

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import kotlinx.coroutines.flow.Flow

@Dao
interface CompletionDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(completion: Completion)

    @Query("DELETE FROM completions WHERE epochDay = :epochDay AND taskId = :taskId")
    suspend fun delete(epochDay: Long, taskId: String)

    @Query(
        "DELETE FROM completions WHERE id = (SELECT id FROM completions " +
            "WHERE epochDay = :epochDay AND taskId LIKE :taskIdPattern " +
            "ORDER BY completedAtMillis DESC LIMIT 1)",
    )
    suspend fun deleteLatestMatching(epochDay: Long, taskIdPattern: String)

    @Query("SELECT * FROM completions WHERE epochDay = :epochDay")
    fun forDay(epochDay: Long): Flow<List<Completion>>

    @Query("SELECT * FROM completions WHERE epochDay = :epochDay")
    suspend fun forDayOnce(epochDay: Long): List<Completion>

    @Query("SELECT completedAtMillis FROM completions WHERE taskId = 'workout'")
    suspend fun workoutCompletionTimes(): List<Long>

    @Query("SELECT * FROM completions WHERE epochDay >= :sinceEpochDay ORDER BY epochDay DESC")
    fun since(sinceEpochDay: Long): Flow<List<Completion>>

    @Query("SELECT COALESCE(SUM(xp), 0) FROM completions")
    fun totalXp(): Flow<Long>

    @Query("SELECT COUNT(*) FROM completions WHERE taskId = 'workout'")
    fun workoutCount(): Flow<Int>

    @Query(
        "SELECT COUNT(*) FROM (SELECT epochDay FROM completions WHERE taskId IN ('am_dose','pm_dose') " +
            "GROUP BY epochDay HAVING COUNT(DISTINCT taskId) = 2)",
    )
    fun bothDoseDays(): Flow<Int>

    @Query("SELECT COUNT(*) FROM completions WHERE taskId = 'water_goal'")
    fun waterGoalDays(): Flow<Int>
}

@Dao
interface WorkoutSetDao {
    @Insert
    suspend fun insert(set: WorkoutSet)

    @Query("DELETE FROM workout_sets WHERE id = :id")
    suspend fun delete(id: Long)

    @Query("SELECT * FROM workout_sets WHERE epochDay = :epochDay ORDER BY id")
    fun forDay(epochDay: Long): Flow<List<WorkoutSet>>

    @Query("SELECT MAX(weightLbs) FROM workout_sets WHERE exercise = :exercise")
    suspend fun maxWeightFor(exercise: String): Double?

    @Query("SELECT COALESCE(SUM(weightLbs * reps), 0) FROM workout_sets")
    fun totalVolume(): Flow<Double>

    @Query("SELECT COUNT(*) FROM workout_sets WHERE isPr = 1")
    fun prCount(): Flow<Int>

    @Query(
        "SELECT epochDay, COALESCE(SUM(weightLbs * reps), 0) AS volume FROM workout_sets " +
            "WHERE epochDay >= :sinceEpochDay GROUP BY epochDay ORDER BY epochDay",
    )
    fun dailyVolumeSince(sinceEpochDay: Long): Flow<List<DailyVolume>>
}

data class DailyVolume(val epochDay: Long, val volume: Double)

@Dao
interface WeightDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entry: WeightEntry)

    @Query("SELECT * FROM weight_entries ORDER BY epochDay")
    fun all(): Flow<List<WeightEntry>>
}

@Dao
interface AchievementDao {
    /** Returns -1 when the achievement was already unlocked (conflict ignored). */
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(unlocked: UnlockedAchievement): Long

    @Query("SELECT * FROM unlocked_achievements")
    fun all(): Flow<List<UnlockedAchievement>>
}
