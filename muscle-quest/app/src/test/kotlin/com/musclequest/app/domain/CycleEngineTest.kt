package com.musclequest.app.domain

import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Test

class CycleEngineTest {

    private val start: LocalDate = LocalDate.of(2026, 7, 6) // a Monday

    @Test
    fun `day one is on-cycle week one`() {
        val s = CycleEngine.status(start, start, 4)
        assertEquals(Phase.ON_CYCLE, s.phase)
        assertEquals(1, s.dayInPhase)
        assertEquals(1, s.weekInPhase)
        assertEquals(28, s.phaseLengthDays)
    }

    @Test
    fun `last active day of a 4-week cycle`() {
        val s = CycleEngine.status(start.plusDays(27), start, 4)
        assertEquals(Phase.ON_CYCLE, s.phase)
        assertEquals(28, s.dayInPhase)
        assertEquals(4, s.weekInPhase)
    }

    @Test
    fun `pct begins the day after the active cycle ends`() {
        val s = CycleEngine.status(start.plusDays(28), start, 4)
        assertEquals(Phase.PCT, s.phase)
        assertEquals(1, s.dayInPhase)
        assertEquals(30, s.phaseLengthDays)
    }

    @Test
    fun `recovery begins after 30 days of pct`() {
        val s = CycleEngine.status(start.plusDays(28 + 30), start, 4)
        assertEquals(Phase.RECOVERY, s.phase)
        assertEquals(1, s.dayInPhase)
        assertEquals(56, s.phaseLengthDays)
    }

    @Test
    fun `maintenance after full recovery`() {
        val s = CycleEngine.status(start.plusDays(28 + 30 + 56), start, 4)
        assertEquals(Phase.MAINTENANCE, s.phase)
        assertEquals(1, s.dayInPhase)
    }

    @Test
    fun `eight week cycle keeps pct aligned`() {
        val s = CycleEngine.status(start.plusDays(56), start, 8)
        assertEquals(Phase.PCT, s.phase)
        assertEquals(1, s.dayInPhase)
    }

    @Test
    fun `before the start date reports days until start`() {
        val s = CycleEngine.status(start.minusDays(3), start, 4)
        assertEquals(Phase.MAINTENANCE, s.phase)
        assertEquals(3, s.daysUntilStart)
    }
}
