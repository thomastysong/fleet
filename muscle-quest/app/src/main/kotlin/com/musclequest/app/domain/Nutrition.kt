package com.musclequest.app.domain

import kotlin.math.roundToInt

/**
 * Training-day multipliers for total daily energy expenditure. Values are the
 * standard Harris/Mifflin activity factors; MODERATE fits 5 lifting sessions
 * a week with an otherwise seated job.
 */
enum class ActivityLevel(val label: String, val factor: Double) {
    SEDENTARY("Desk job, little else", 1.35),
    MODERATE("Lifting 4–6×/wk", 1.55),
    HIGH("Lifting + active job", 1.725),
}

data class BodyProfile(
    val ageYears: Int,
    val heightInches: Int,
    val weightLbs: Double,
    val isMale: Boolean,
    val activity: ActivityLevel,
    /** Daily surplus over TDEE. Lean-bulk range is roughly +250–500. */
    val surplusKcal: Int,
)

data class MacroTargets(
    val kcal: Int,
    val proteinG: Int,
    val carbsG: Int,
    val fatG: Int,
    val waterOz: Int,
    val bmrKcal: Int,
    val tdeeKcal: Int,
)

/**
 * Evidence-based targets, recomputed from the live profile so they track the
 * user's actual body weight as it changes:
 *
 * - BMR via Mifflin-St Jeor (the equation with the best validation in
 *   healthy adults): 10·kg + 6.25·cm − 5·age (+5 male / −161 female).
 * - TDEE = BMR × activity factor; bulk intake = TDEE + surplus.
 * - Protein 1.2 g/lb — top of the 1.6–2.2 g/kg meta-analytic range, right
 *   for a hormonally-assisted surplus where protein synthesis is elevated.
 * - Fat 0.35 g/lb (~25% of kcal) to support hormone production; the
 *   remaining calories go to carbs to fuel training.
 * - Water ~0.75 oz/lb, floored at a gallon while on cycle.
 */
object NutritionEngine {

    fun targets(p: BodyProfile): MacroTargets {
        val kg = p.weightLbs * 0.45359237
        val cm = p.heightInches * 2.54
        val bmr = 10.0 * kg + 6.25 * cm - 5.0 * p.ageYears + if (p.isMale) 5.0 else -161.0
        val tdee = bmr * p.activity.factor
        val kcal = tdee + p.surplusKcal
        val proteinG = (p.weightLbs * 1.2).roundToInt()
        val fatG = (p.weightLbs * 0.35).roundToInt()
        val carbsG = (((kcal - proteinG * 4 - fatG * 9) / 4).roundToInt()).coerceAtLeast(0)
        return MacroTargets(
            kcal = kcal.roundToInt(),
            proteinG = proteinG,
            carbsG = carbsG,
            fatG = fatG,
            waterOz = (p.weightLbs * 0.75).roundToInt().coerceAtLeast(128),
            bmrKcal = bmr.roundToInt(),
            tdeeKcal = tdee.roundToInt(),
        )
    }

    /**
     * Lean-bulk weekly gain band: 0.25–0.5% of body weight per week. Faster
     * than this and the extra is mostly fat; slower wastes the cycle.
     */
    fun weeklyGainBandLbs(weightLbs: Double): ClosedFloatingPointRange<Double> =
        (weightLbs * 0.0025)..(weightLbs * 0.005)

    /** Epley one-rep-max estimate; only meaningful for reps in ~1–12. */
    fun estimate1Rm(weightLbs: Double, reps: Int): Double? =
        if (reps < 1 || weightLbs <= 0) null
        else if (reps == 1) weightLbs
        else weightLbs * (1 + reps / 30.0)
}

/** One-tap foods matching the rice/chicken/steak/whey plan, real macros. */
data class FoodPreset(
    val name: String,
    val emoji: String,
    val kcal: Int,
    val proteinG: Int,
    val carbsG: Int,
    val fatG: Int,
)

object FoodPresets {
    val ALL = listOf(
        FoodPreset("Meal 1 — 2c rice + 3 eggs", "🍳", 625, 27, 91, 16),
        FoodPreset("Meal 2 — 6oz chicken + rice", "🍗", 640, 60, 79, 7),
        FoodPreset("Meal 3 — 6oz steak + 2c rice", "🥩", 755, 52, 90, 19),
        FoodPreset("Dinner — 8oz steak + rice + veg", "🍽️", 920, 66, 95, 25),
        FoodPreset("Whey shake (1 scoop)", "🥤", 120, 24, 3, 2),
        FoodPreset("Whey + creatine PWO", "💪", 240, 48, 6, 3),
        FoodPreset("1 cup cooked rice", "🍚", 205, 4, 45, 1),
        FoodPreset("6 oz chicken breast", "🐔", 280, 53, 0, 6),
        FoodPreset("8 oz sirloin steak", "🥩", 460, 58, 0, 24),
        FoodPreset("3 whole eggs", "🥚", 215, 19, 1, 15),
    )
}
