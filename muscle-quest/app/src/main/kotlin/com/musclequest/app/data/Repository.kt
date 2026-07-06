package com.musclequest.app.data

import com.musclequest.app.domain.Achievement
import com.musclequest.app.domain.DailyPlanner
import com.musclequest.app.domain.PlannedTask
import com.musclequest.app.domain.Xp
import java.time.LocalDate

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
     * Toggles a checklist task. When checking the final open task of the day,
     * also awards the perfect-day bonus (base + streak bonus); unchecking any
     * task revokes it.
     */
    suspend fun toggleTask(
        date: LocalDate,
        task: PlannedTask,
        plannedTasks: List<PlannedTask>,
        completedIds: Set<String>,
        streakBeforeToday: Int,
        nowMillis: Long,
    ) {
        val epochDay = date.toEpochDay()
        if (task.id in completedIds) {
            completionDao.delete(epochDay, task.id)
            completionDao.delete(epochDay, DailyPlanner.PERFECT_DAY_TASK_ID)
        } else {
            completionDao.insert(Completion(epochDay = epochDay, taskId = task.id, xp = task.xp, completedAtMillis = nowMillis))
            val nowComplete = plannedTasks.all { it.id == task.id || it.id in completedIds }
            if (nowComplete) {
                val bonus = DailyPlanner.PERFECT_DAY_BONUS + Xp.streakBonus(streakBeforeToday + 1)
                completionDao.insert(
                    Completion(
                        epochDay = epochDay,
                        taskId = DailyPlanner.PERFECT_DAY_TASK_ID,
                        xp = bonus,
                        completedAtMillis = nowMillis,
                    ),
                )
            }
        }
    }

    /**
     * Logs a set; returns true when it is a new all-time weight PR for the
     * exercise (which also banks a PR bonus completion tied to the set id).
     */
    suspend fun logSet(date: LocalDate, exercise: String, weightLbs: Double, reps: Int, nowMillis: Long): Boolean {
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
        return isPr
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
