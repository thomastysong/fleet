package com.musclequest.app.domain

data class LevelInfo(
    val level: Int,
    val rank: String,
    val xpIntoLevel: Long,
    val xpForNextLevel: Long,
) {
    val progress: Float
        get() = if (xpForNextLevel <= 0) 1f else (xpIntoLevel.toFloat() / xpForNextLevel).coerceIn(0f, 1f)
}

/**
 * XP -> level curve and rank titles. Cumulative XP required to *reach* level
 * n is 60 * (n-1)^2, so early levels come fast (level 2 at 60 XP — day one)
 * and the grind steepens from there.
 */
object Xp {

    private val RANKS = listOf(
        1 to "Rookie",
        2 to "Novice Lifter",
        3 to "Iron Initiate",
        5 to "Gym Regular",
        7 to "Muscle Apprentice",
        10 to "Gainz Goblin",
        13 to "Protein Powered",
        16 to "Beast Mode",
        20 to "Mass Monster",
        25 to "Steel Titan",
        30 to "Living Legend",
    )

    fun xpToReachLevel(level: Int): Long {
        val n = (level - 1).coerceAtLeast(0).toLong()
        return 60L * n * n
    }

    fun levelInfo(totalXp: Long): LevelInfo {
        var level = 1
        while (xpToReachLevel(level + 1) <= totalXp) level++
        val floor = xpToReachLevel(level)
        val ceil = xpToReachLevel(level + 1)
        return LevelInfo(
            level = level,
            rank = rankForLevel(level),
            xpIntoLevel = totalXp - floor,
            xpForNextLevel = ceil - floor,
        )
    }

    fun rankForLevel(level: Int): String =
        RANKS.last { level >= it.first }.second

    /** Extra XP layered onto the perfect-day bonus: +2 per streak day, capped at +20. */
    fun streakBonus(streakDays: Int): Int = (streakDays * 2).coerceAtMost(20)
}
