package com.musclequest.app.data

import androidx.room.withTransaction
import com.musclequest.app.domain.Achievement
import com.musclequest.app.domain.DailyPlanner
import com.musclequest.app.domain.PlannedTask
import com.musclequest.app.domain.Xp
import java.time.LocalDate

enum class ToggleResult { CHECKED, CHECKED_PERFECT, UNCHECKED }

/**
 * Single write-side entry point. All XP is stored as rows in `completions`
 * (task check-offs, perfect-day bonuses, PR bonuses, achievement rewards) so
 * total XP is always SUM(xp) — no separate ledger to drift out of sync.
 */
class Repository(private val db: AppDatabase) {

    val completionDao get() = db.completionDao()
    val workoutSetDao get() = db.workoutSetDao()
    val weightDao get() = db.weightDao()
    val achievementDao get() = db.achievementDao()

    /**
     * Toggles a checklist task. Completion state is re-read from the database
     * inside a transaction (never trusted from the UI snapshot) so rapid taps
     * can't award the perfect-day bonus on an incomplete day or skip it on a
     * complete one. Checking the final open task awards the bonus
     * (base + streak bonus); unchecking any task revokes it.
     */
    suspend fun toggleTask(
        date: LocalDate,
        task: PlannedTask,
        plannedTasks: List<PlannedTask>,
        streakBeforeToday: Int,
        nowMillis: Long,
    ): ToggleResult = db.withTransaction {
        val epochDay = date.toEpochDay()
        val existing = completionDao.forDayOnce(epochDay).map { it.taskId }.toSet()
        if (task.id in existing) {
            completionDao.delete(epochDay, task.id)
            completionDao.delete(epochDay, DailyPlanner.PERFECT_DAY_TASK_ID)
            ToggleResult.UNCHECKED
        } else {
            completionDao.insert(
                Completion(epochDay = epochDay, taskId = task.id, xp = task.xp, completedAtMillis = nowMillis),
            )
            val doneNow = existing + task.id
            if (plannedTasks.all { it.id in doneNow }) {
                val bonus = DailyPlanner.PERFECT_DAY_BONUS + Xp.streakBonus(streakBeforeToday + 1)
                completionDao.insert(
                    Completion(
                        epochDay = epochDay,
                        taskId = DailyPlanner.PERFECT_DAY_TASK_ID,
                        xp = bonus,
                        completedAtMillis = nowMillis,
                    ),
                )
                ToggleResult.CHECKED_PERFECT
            } else {
                ToggleResult.CHECKED
            }
        }
    }

    /**
     * Logs a set; returns true when it is a new all-time weight PR for the
     * exercise (which also banks a PR bonus completion).
     */
    suspend fun logSet(date: LocalDate, exercise: String, weightLbs: Double, reps: Int, nowMillis: Long): Boolean =
        db.withTransaction {
            val epochDay = date.toEpochDay()
            val previousBest = workoutSetDao.maxWeightFor(exercise) ?: 0.0
            val isPr = weightLbs > previousBest && reps > 0
            workoutSetDao.insert(
                WorkoutSet(epochDay = epochDay, exercise = exercise, weightLbs = weightLbs, reps = reps, isPr = isPr),
            )
            if (isPr) {
                completionDao.insert(
                    Completion(
                        epochDay = epochDay,
                        taskId = "${DailyPlanner.PR_TASK_PREFIX}$exercise:$nowMillis",
                        xp = DailyPlanner.PR_XP,
                        completedAtMillis = nowMillis,
                    ),
                )
            }
            isPr
        }

    suspend fun logWeight(date: LocalDate, weightLbs: Double) {
        weightDao.upsert(WeightEntry(epochDay = date.toEpochDay(), weightLbs = weightLbs))
    }

    suspend fun unlock(achievement: Achievement, date: LocalDate, nowMillis: Long) {
        achievementDao.insert(UnlockedAchievement(achievement.id, nowMillis))
        completionDao.insert(
            Completion(
                epochDay = date.toEpochDay(),
                taskId = "achievement:${achievement.id}",
                xp = achievement.xpReward,
                completedAtMillis = nowMillis,
            ),
        )
    }
}
