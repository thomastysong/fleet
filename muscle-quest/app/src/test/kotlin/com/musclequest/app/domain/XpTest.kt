package com.musclequest.app.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class XpTest {

    @Test
    fun `zero xp is level one rookie`() {
        val info = Xp.levelInfo(0)
        assertEquals(1, info.level)
        assertEquals("Rookie", info.rank)
    }

    @Test
    fun `levels are monotonic in xp`() {
        var last = 1
        for (xp in 0L..20_000L step 250L) {
            val level = Xp.levelInfo(xp).level
            assertTrue(level >= last)
            last = level
        }
    }

    @Test
    fun `level boundary is exact`() {
        val boundary = Xp.xpToReachLevel(5)
        assertEquals(4, Xp.levelInfo(boundary - 1).level)
        assertEquals(5, Xp.levelInfo(boundary).level)
    }

    @Test
    fun `streak bonus caps at 20`() {
        assertEquals(0, Xp.streakBonus(0))
        assertEquals(10, Xp.streakBonus(5))
        assertEquals(20, Xp.streakBonus(10))
        assertEquals(20, Xp.streakBonus(100))
    }

    @Test
    fun `ranks resolve for any level`() {
        assertEquals("Rookie", Xp.rankForLevel(1))
        assertEquals("Gainz Goblin", Xp.rankForLevel(11))
        assertEquals("Living Legend", Xp.rankForLevel(99))
    }
}
