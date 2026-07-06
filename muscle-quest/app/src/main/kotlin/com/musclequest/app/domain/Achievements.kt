package com.musclequest.app.domain

data class Achievement(
    val id: String,
    val title: String,
    val description: String,
    val emoji: String,
    val xpReward: Int,
)

/** Snapshot of lifetime stats used to evaluate achievement conditions. */
data class StatsSnapshot(
    val workoutsCompleted: Int,
    /** Workouts whose completion was checked off before 6:00 AM local time. */
    val earlyWorkouts: Int,
    val currentStreak: Int,
    val bothDoseDays: Int,
    val prCount: Int,
    val totalVolumeLbs: Long,
    val waterGoalDays: Int,
    val level: Int,
    val phase: Phase,
    val cycleCompleted: Boolean,
    val pctCompleted: Boolean,
    /** Days the protein target was hit (fuel_protein bonus banked). */
    val proteinTargetDays: Int = 0,
    /** Distinct days with at least one food entry logged. */
    val foodLoggedDays: Int = 0,
)

object Achievements {

    val ALL = listOf(
        Achievement("first_blood", "First Blood", "Complete your first workout", "🩸", 25),
        Achievement("early_bird", "Early Bird", "Complete 5 pre-6 AM workouts", "🌅", 50),
        Achievement("week_warrior", "Week Warrior", "Hit a 7-day perfect streak", "🔥", 75),
        Achievement("iron_month", "Iron Month", "Hit a 30-day perfect streak", "🗓️", 200),
        Achievement("dose_discipline", "Clockwork", "Log both doses on 14 different days", "⏰", 75),
        Achievement("pr_machine", "PR Machine", "Set 10 personal records", "📈", 100),
        Achievement("ton_club", "Ton Club", "Lift 100,000 lb of total volume", "🏗️", 150),
        Achievement("hydro_homie", "Hydro Homie", "Hit the water goal 10 times", "💧", 50),
        Achievement("level_10", "Double Digits", "Reach level 10", "🔟", 100),
        Achievement("cycle_complete", "Full Send", "Complete the active cycle", "🏁", 250),
        Achievement("pct_complete", "Landed the Plane", "Finish all 30 days of PCT", "🛬", 300),
        Achievement("protein_pro", "Protein Pro", "Hit your protein target 7 days", "🍗", 75),
        Achievement("fuel_accountant", "Fuel Accountant", "Log food on 14 different days", "📒", 50),
    )

    fun byId(id: String): Achievement? = ALL.firstOrNull { it.id == id }

    /** Returns achievements whose conditions [stats] now satisfies. */
    fun earned(stats: StatsSnapshot): List<Achievement> = ALL.filter { a ->
        when (a.id) {
            "first_blood" -> stats.workoutsCompleted >= 1
            "early_bird" -> stats.earlyWorkouts >= 5
            "week_warrior" -> stats.currentStreak >= 7
            "iron_month" -> stats.currentStreak >= 30
            "dose_discipline" -> stats.bothDoseDays >= 14
            "pr_machine" -> stats.prCount >= 10
            "ton_club" -> stats.totalVolumeLbs >= 100_000
            "hydro_homie" -> stats.waterGoalDays >= 10
            "level_10" -> stats.level >= 10
            "cycle_complete" -> stats.cycleCompleted
            "pct_complete" -> stats.pctCompleted
            "protein_pro" -> stats.proteinTargetDays >= 7
            "fuel_accountant" -> stats.foodLoggedDays >= 14
            else -> false
        }
    }
}
