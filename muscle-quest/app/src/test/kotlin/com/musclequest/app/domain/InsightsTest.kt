package com.musclequest.app.domain

import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class InsightsTest {

    private val monday: LocalDate = LocalDate.of(2026, 7, 6)
    private val targets = NutritionEngine.targets(
        BodyProfile(31, 72, 170.0, true, ActivityLevel.MODERATE, 400),
    )

    private fun data(
        status: CycleStatus = CycleEngine.status(monday, monday, 4),
        weights: List<Pair<Long, Double>> = emptyList(),
        proteinToday: Int = 0,
        foodLogged: Boolean = false,
        sleepMisses: Int = 0,
        creatineDays: Int = 0,
        lastWeekVolume: Double? = null,
        streak: Int = 0,
    ) = InsightData(
        status = status,
        weights = weights,
        currentWeightLbs = 170.0,
        proteinTodayG = proteinToday,
        kcalToday = 0,
        targets = targets,
        foodLoggedToday = foodLogged,
        sleepMisses7d = sleepMisses,
        creatineDays = creatineDays,
        todayVolumeLbs = 0.0,
        lastSameWeekdayVolumeLbs = lastWeekVolume,
        streak = streak,
    )

    @Test
    fun `never more than three cards`() {
        val d = data(weights = listOf(0L to 170.0, 14L to 176.0), sleepMisses = 3, streak = 5, foodLogged = true, proteinToday = 40)
        assertTrue(InsightEngine.insightsFor(monday, d).size <= 3)
    }

    @Test
    fun `cards are sorted by priority descending`() {
        val cards = InsightEngine.insightsFor(monday, data(sleepMisses = 4))
        assertEquals(cards, cards.sortedByDescending { it.priority })
    }

    @Test
    fun `fast weight gain warns`() {
        // +6 lb in 14 days = 3 lb/week, way over the 0.85 band ceiling.
        val d = data(weights = listOf(0L to 170.0, 7L to 173.0, 14L to 176.0))
        assertTrue(InsightEngine.insightsFor(monday, d).any { it.id == "gain_fast" })
    }

    @Test
    fun `trend needs a week of data`() {
        assertNull(InsightEngine.weeklyTrendLbs(listOf(0L to 170.0, 3L to 171.0)))
        assertNull(InsightEngine.weeklyTrendLbs(listOf(0L to 170.0)))
    }

    @Test
    fun `linear gain computes exact slope`() {
        val trend = InsightEngine.weeklyTrendLbs(listOf(0L to 170.0, 7L to 171.0, 14L to 172.0))
        assertEquals(1.0, trend!!, 1e-9)
    }

    @Test
    fun `late pct pushes blood work booking`() {
        val pctDay25 = CycleEngine.status(monday.plusDays(28L + 24), monday, 4)
        assertEquals(Phase.PCT, pctDay25.phase)
        val cards = InsightEngine.insightsFor(monday.plusDays(52), data(status = pctDay25))
        assertTrue(cards.any { it.id == "bloodwork_post" })
    }

    @Test
    fun `week one on cycle sets expectations`() {
        val cards = InsightEngine.insightsFor(monday, data())
        assertTrue(cards.any { it.id == "cycle_w1" })
    }
}
