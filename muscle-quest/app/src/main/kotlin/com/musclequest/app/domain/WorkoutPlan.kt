package com.musclequest.app.domain

import java.time.DayOfWeek

data class ExercisePlan(
    val name: String,
    val sets: Int,
    val reps: String,
    val restSec: Int,
    val note: String = "",
)

data class DayWorkout(
    val title: String,
    val focus: String,
    val exercises: List<ExercisePlan>,
)

/**
 * Monday-to-Friday 5:30 AM split. Saturday and Sunday are rest days and
 * return null. Progression rule: when the top of the rep range is hit on
 * every set, add 5 lb (upper) or 10 lb (lower) next session.
 */
object WorkoutPlan {

    fun forDay(day: DayOfWeek): DayWorkout? = when (day) {
        DayOfWeek.MONDAY -> DayWorkout(
            title = "Push — Chest & Triceps",
            focus = "Heavy pressing to open the week",
            exercises = listOf(
                ExercisePlan("Barbell Bench Press", 4, "6–8", 180, "Top set hard, leave 1 rep in the tank"),
                ExercisePlan("Incline Dumbbell Press", 3, "8–10", 120),
                ExercisePlan("Weighted Dip", 3, "8–12", 120, "Bodyweight if needed"),
                ExercisePlan("Cable Fly", 3, "12–15", 90, "Squeeze, slow negative"),
                ExercisePlan("Overhead Triceps Extension", 3, "10–12", 90),
                ExercisePlan("Triceps Pushdown", 3, "12–15", 60),
            ),
        )
        DayOfWeek.TUESDAY -> DayWorkout(
            title = "Pull — Back & Biceps",
            focus = "Deadlift day — brace hard",
            exercises = listOf(
                ExercisePlan("Deadlift", 3, "5", 240, "Heavy but crisp — no grinding reps on cycle"),
                ExercisePlan("Pull-Up", 4, "6–10", 150, "Add weight when you clear 10"),
                ExercisePlan("Barbell Row", 3, "8–10", 120),
                ExercisePlan("Face Pull", 3, "15", 60, "Rear delts & rotator health"),
                ExercisePlan("Barbell Curl", 3, "8–12", 90),
                ExercisePlan("Hammer Curl", 3, "10–12", 60),
            ),
        )
        DayOfWeek.WEDNESDAY -> DayWorkout(
            title = "Legs — Quads, Hams & Calves",
            focus = "The growth day — don’t skip it",
            exercises = listOf(
                ExercisePlan("Back Squat", 4, "6–8", 210, "Depth first, load second"),
                ExercisePlan("Romanian Deadlift", 3, "8–10", 150),
                ExercisePlan("Leg Press", 3, "10–12", 120),
                ExercisePlan("Walking Lunge", 3, "12 / leg", 90),
                ExercisePlan("Standing Calf Raise", 4, "12–15", 60, "Pause at the stretch"),
                ExercisePlan("Hanging Leg Raise", 3, "12–15", 60),
            ),
        )
        DayOfWeek.THURSDAY -> DayWorkout(
            title = "Shoulders & Arms",
            focus = "Delts wide, arms full",
            exercises = listOf(
                ExercisePlan("Overhead Press", 4, "6–8", 180),
                ExercisePlan("Dumbbell Lateral Raise", 4, "12–15", 60, "Light and strict beats heavy and sloppy"),
                ExercisePlan("Rear Delt Fly", 3, "15", 60),
                ExercisePlan("EZ-Bar Curl", 3, "8–12", 90),
                ExercisePlan("Skullcrusher", 3, "8–12", 90),
                ExercisePlan("Barbell Shrug", 3, "12", 90),
            ),
        )
        DayOfWeek.FRIDAY -> DayWorkout(
            title = "Full Body — Power & Pump",
            focus = "Hit everything once more, then earn the weekend",
            exercises = listOf(
                ExercisePlan("Front Squat", 3, "6", 180),
                ExercisePlan("Incline Barbell Press", 3, "6–8", 150),
                ExercisePlan("Weighted Chin-Up", 3, "6–8", 150),
                ExercisePlan("Seated Cable Row", 3, "10–12", 90),
                ExercisePlan("Dumbbell Curl + Pushdown superset", 3, "12 + 12", 75),
                ExercisePlan("Farmer’s Carry", 3, "40 yd", 120, "Heavy — grip, traps, core"),
            ),
        )
        else -> null
    }
}
