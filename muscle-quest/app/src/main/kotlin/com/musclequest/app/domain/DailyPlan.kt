package com.musclequest.app.domain

import java.time.DayOfWeek
import java.time.LocalDate

enum class TaskCategory { DOSE, TRAINING, NUTRITION, HYDRATION, RECOVERY }

/**
 * One item on the daily checklist. [id] is stable across days so completions
 * can be joined against the plan for any date.
 */
data class PlannedTask(
    val id: String,
    val time: String,
    val title: String,
    val detail: String,
    val xp: Int,
    val category: TaskCategory,
)

/**
 * Generates the day's checklist from the cycle phase and day of week.
 *
 * Weekday rhythm is anchored on the 5:30 AM workout: AM dose on waking at
 * 4:30 (empty stomach, >= 60 min before the first calories), train fasted-ish,
 * shake after, meals through early afternoon, then a food gap so the 4:30 PM
 * dose lands on an empty stomach, dinner 45 min later, lights out at 9.
 * Weekends shift everything two hours later and swap the workout for recovery.
 */
object DailyPlanner {

    const val PERFECT_DAY_BONUS = 25
    const val PERFECT_DAY_TASK_ID = "bonus_perfect_day"
    const val PR_TASK_PREFIX = "pr_bonus:"
    const val PR_XP = 30
    const val FUEL_PROTEIN_TASK_ID = "fuel_protein"
    const val FUEL_PROTEIN_XP = 20

    fun tasksFor(date: LocalDate, status: CycleStatus): List<PlannedTask> {
        val weekend = date.dayOfWeek == DayOfWeek.SATURDAY || date.dayOfWeek == DayOfWeek.SUNDAY
        val tasks = mutableListOf<PlannedTask>()

        // --- Morning ---
        when (status.phase) {
            Phase.ON_CYCLE -> tasks += PlannedTask(
                id = "am_dose",
                time = if (weekend) "6:30 AM" else "4:30 AM",
                title = "AM andro dose",
                detail = "1 tablet of each stack product on a fully empty stomach. " +
                    "No food, shakes, or caloric drinks for 30–60 min.",
                xp = 15,
                category = TaskCategory.DOSE,
            )
            Phase.PCT -> tasks += PlannedTask(
                id = "pct_dose",
                time = if (weekend) "9:15 AM" else "7:15 AM",
                title = "Alpha-AF — 3 capsules",
                detail = "Take all 3 PCT capsules with your first meal. Every day for 30 days.",
                xp = 15,
                category = TaskCategory.DOSE,
            )
            else -> Unit
        }

        tasks += PlannedTask(
            id = "wake_water",
            time = if (weekend) "6:30 AM" else "4:30 AM",
            title = "Hydrate on waking",
            detail = "16–20 oz water with a pinch of salt (sodium, potassium, magnesium matter on cycle).",
            xp = 5,
            category = TaskCategory.HYDRATION,
        )

        // --- Training ---
        if (!weekend) {
            tasks += PlannedTask(
                id = "workout",
                time = "5:30 AM",
                title = WorkoutPlan.forDay(date.dayOfWeek)?.title ?: "Training",
                detail = "Log your sets in the Train tab, then check this off. ~60 minutes.",
                xp = 50,
                category = TaskCategory.TRAINING,
            )
            tasks += PlannedTask(
                id = "pwo_shake",
                time = "6:40 AM",
                title = "Post-workout shake",
                detail = "1–2 scoops whey + 5 g creatine in water. Creatine every day, trained or not.",
                xp = 10,
                category = TaskCategory.NUTRITION,
            )
        } else {
            tasks += PlannedTask(
                id = "recovery_walk",
                time = "9:00 AM",
                title = "Active recovery",
                detail = "20–40 min easy walk. Rest days grow muscle — move a little, lift nothing.",
                xp = 20,
                category = TaskCategory.RECOVERY,
            )
            tasks += PlannedTask(
                id = "creatine_rest",
                time = "10:00 AM",
                title = "Whey + creatine",
                detail = "1 scoop whey + 5 g creatine. Rest days count — keep saturation up.",
                xp = 10,
                category = TaskCategory.NUTRITION,
            )
        }

        // --- Meals ---
        tasks += PlannedTask(
            id = "meal_1",
            time = if (weekend) "9:15 AM" else "7:15 AM",
            title = "Meal 1 — rice + eggs & chicken",
            detail = "2 cups cooked rice, 3 whole eggs or 6 oz chicken. First calories of the day.",
            xp = 10,
            category = TaskCategory.NUTRITION,
        )
        tasks += PlannedTask(
            id = "meal_2",
            time = if (weekend) "12:00 PM" else "10:30 AM",
            title = "Meal 2 — chicken + rice",
            detail = "6–8 oz chicken breast, 1.5–2 cups cooked rice.",
            xp = 10,
            category = TaskCategory.NUTRITION,
        )
        tasks += PlannedTask(
            id = "meal_3",
            time = if (weekend) "2:30 PM" else "1:00 PM",
            title = "Meal 3 — steak or chicken + rice",
            detail = "6–8 oz steak or chicken, 2 cups cooked rice. Last food before the PM dose window.",
            xp = 10,
            category = TaskCategory.NUTRITION,
        )

        // --- Evening ---
        if (status.phase == Phase.ON_CYCLE) {
            tasks += PlannedTask(
                id = "pm_dose",
                time = if (weekend) "6:30 PM" else "4:30 PM",
                title = "PM andro dose",
                detail = "1 tablet of each product, 12 h after the AM dose. Empty stomach: " +
                    "≥2 h after meal 3, wait 30+ min before dinner.",
                xp = 15,
                category = TaskCategory.DOSE,
            )
        }
        tasks += PlannedTask(
            id = "meal_4",
            time = if (weekend) "7:15 PM" else "5:15 PM",
            title = "Dinner — steak + rice + veg",
            detail = "8 oz steak, 2 cups cooked rice, green vegetables. Biggest plate of the day.",
            xp = 10,
            category = TaskCategory.NUTRITION,
        )
        tasks += PlannedTask(
            id = "meal_5",
            time = "8:00 PM",
            title = "Evening shake",
            detail = "1 scoop whey (or casein) — protein before the overnight fast.",
            xp = 5,
            category = TaskCategory.NUTRITION,
        )
        tasks += PlannedTask(
            id = "water_goal",
            time = "All day",
            title = "1 gallon of water",
            detail = "Push fluids hard on cycle — aim for a gallon spread across the day.",
            xp = 10,
            category = TaskCategory.HYDRATION,
        )
        tasks += PlannedTask(
            id = "sleep",
            time = if (weekend) "10:00 PM" else "9:00 PM",
            title = "Lights out",
            detail = "7.5 h minimum. Growth happens in deep sleep — protect it like a PR.",
            xp = 15,
            category = TaskCategory.RECOVERY,
        )

        return tasks
    }
}
