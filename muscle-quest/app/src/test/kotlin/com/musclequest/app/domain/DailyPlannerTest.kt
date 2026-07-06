package com.musclequest.app.domain

import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DailyPlannerTest {

    private val monday: LocalDate = LocalDate.of(2026, 7, 6)
    private val saturday: LocalDate = LocalDate.of(2026, 7, 11)

    private fun onCycle(date: LocalDate) = CycleEngine.status(date, monday, 4)

    @Test
    fun `on-cycle weekday includes both andro doses and a workout`() {
        val ids = DailyPlanner.tasksFor(monday, onCycle(monday)).map { it.id }
        assertTrue("am_dose" in ids)
        assertTrue("pm_dose" in ids)
        assertTrue("workout" in ids)
        assertFalse("pct_dose" in ids)
    }

    @Test
    fun `weekend has no workout but still doses`() {
        val ids = DailyPlanner.tasksFor(saturday, onCycle(saturday)).map { it.id }
        assertFalse("workout" in ids)
        assertTrue("recovery_walk" in ids)
        assertTrue("am_dose" in ids)
        assertTrue("pm_dose" in ids)
    }

    @Test
    fun `pct swaps andro doses for alpha-af`() {
        val pctDay = monday.plusDays(30)
        val ids = DailyPlanner.tasksFor(pctDay, onCycle(pctDay)).map { it.id }
        assertTrue("pct_dose" in ids)
        assertFalse("am_dose" in ids)
        assertFalse("pm_dose" in ids)
    }

    @Test
    fun `recovery phase has no dose tasks at all`() {
        val recoveryDay = monday.plusDays(28 + 30 + 5)
        val tasks = DailyPlanner.tasksFor(recoveryDay, onCycle(recoveryDay))
        assertTrue(tasks.none { it.category == TaskCategory.DOSE })
    }

    @Test
    fun `task ids are unique within a day`() {
        val ids = DailyPlanner.tasksFor(monday, onCycle(monday)).map { it.id }
        assertEquals(ids.size, ids.toSet().size)
    }
}
