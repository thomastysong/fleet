package com.musclequest.app.domain

import java.time.LocalDate
import java.time.temporal.ChronoUnit

/** The four phases of a complete andro cycle, in order. */
enum class Phase(val title: String, val tagline: String) {
    ON_CYCLE("ON CYCLE", "Active stack — dose twice daily, eat big, train hard"),
    PCT("POST-CYCLE (PCT)", "Alpha-AF 3 caps with first meal — cement your gains"),
    RECOVERY("RECOVERY", "All andro products off — keep training and eating"),
    MAINTENANCE("OFF CYCLE", "Base building — train, eat, sleep, repeat"),
}

data class CycleStatus(
    val phase: Phase,
    /** 1-based day within the current phase. */
    val dayInPhase: Int,
    /** Total days in the current phase, or -1 if open-ended. */
    val phaseLengthDays: Int,
    /** 1-based week within the current phase. */
    val weekInPhase: Int,
    /** Days until the cycle starts, when today is before the start date (0 otherwise). */
    val daysUntilStart: Int = 0,
) {
    val progress: Float
        get() = if (phaseLengthDays <= 0) 0f else (dayInPhase.toFloat() / phaseLengthDays).coerceIn(0f, 1f)
}

/**
 * Pure calendar math for the cycle: given a start date and an active length in
 * weeks, maps any date onto ON_CYCLE -> PCT (30 days) -> RECOVERY (56 days) ->
 * MAINTENANCE.
 */
object CycleEngine {
    const val PCT_DAYS = 30
    const val RECOVERY_DAYS = 56
    const val DEFAULT_ACTIVE_WEEKS = 4
    const val MAX_ACTIVE_WEEKS = 8

    fun status(today: LocalDate, cycleStart: LocalDate, activeWeeks: Int): CycleStatus {
        val onDays = activeWeeks * 7
        val day = ChronoUnit.DAYS.between(cycleStart, today).toInt()
        return when {
            day < 0 -> CycleStatus(Phase.MAINTENANCE, 1, -1, 1, daysUntilStart = -day)
            day < onDays -> CycleStatus(Phase.ON_CYCLE, day + 1, onDays, day / 7 + 1)
            day < onDays + PCT_DAYS -> {
                val d = day - onDays
                CycleStatus(Phase.PCT, d + 1, PCT_DAYS, d / 7 + 1)
            }
            day < onDays + PCT_DAYS + RECOVERY_DAYS -> {
                val d = day - onDays - PCT_DAYS
                CycleStatus(Phase.RECOVERY, d + 1, RECOVERY_DAYS, d / 7 + 1)
            }
            else -> {
                val d = day - onDays - PCT_DAYS - RECOVERY_DAYS
                CycleStatus(Phase.MAINTENANCE, d + 1, -1, d / 7 + 1)
            }
        }
    }
}
