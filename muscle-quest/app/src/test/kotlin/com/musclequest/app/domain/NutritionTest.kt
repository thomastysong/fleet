package com.musclequest.app.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NutritionTest {

    private val user = BodyProfile(
        ageYears = 31, heightInches = 72, weightLbs = 170.0,
        isMale = true, activity = ActivityLevel.MODERATE, surplusKcal = 400,
    )

    @Test
    fun `mifflin st jeor bmr for the reference user`() {
        // 10*77.11 + 6.25*182.88 - 5*31 + 5 = 1764
        assertEquals(1764, NutritionEngine.targets(user).bmrKcal)
    }

    @Test
    fun `tdee and bulk kcal land in the documented range`() {
        val t = NutritionEngine.targets(user)
        assertEquals(2734, t.tdeeKcal)
        assertTrue("kcal ${t.kcal}", t.kcal in 3100..3160)
    }

    @Test
    fun `protein is 1_2 g per lb and fat 0_35`() {
        val t = NutritionEngine.targets(user)
        assertEquals(204, t.proteinG)
        // 170 * 0.35 = 59.4999… in binary floating point
        assertTrue("fat ${t.fatG}", t.fatG in 59..60)
        assertTrue("carbs ${t.carbsG}", t.carbsG in 435..455)
    }

    @Test
    fun `water floors at a gallon`() {
        assertEquals(128, NutritionEngine.targets(user).waterOz)
        val heavy = user.copy(weightLbs = 220.0)
        assertEquals(165, NutritionEngine.targets(heavy).waterOz)
    }

    @Test
    fun `targets scale with body weight`() {
        val heavier = NutritionEngine.targets(user.copy(weightLbs = 180.0))
        val base = NutritionEngine.targets(user)
        assertTrue(heavier.kcal > base.kcal)
        assertEquals(216, heavier.proteinG)
    }

    @Test
    fun `lean bulk band is a quarter to half percent weekly`() {
        val band = NutritionEngine.weeklyGainBandLbs(170.0)
        assertEquals(0.425, band.start, 1e-9)
        assertEquals(0.85, band.endInclusive, 1e-9)
    }

    @Test
    fun `epley one rep max`() {
        assertEquals(225.0, NutritionEngine.estimate1Rm(225.0, 1)!!, 1e-9)
        assertEquals(200.0 * (1 + 5 / 30.0), NutritionEngine.estimate1Rm(200.0, 5)!!, 1e-9)
        assertNull(NutritionEngine.estimate1Rm(200.0, 0))
    }
}
