package com.musclequest.app.domain

import java.time.DayOfWeek
import java.time.LocalDate
import kotlin.math.abs

enum class InsightTone { PUSH, WARN, INFO }

data class Insight(
    val id: String,
    val emoji: String,
    val title: String,
    val body: String,
    val tone: InsightTone,
    /** Higher shows first. */
    val priority: Int,
)

/** Everything the coach looks at, pre-digested by the ViewModel. */
data class InsightData(
    val status: CycleStatus,
    /** Weight entries (epochDay, lbs) sorted ascending, last ~30 days. */
    val weights: List<Pair<Long, Double>>,
    val currentWeightLbs: Double,
    val proteinTodayG: Int,
    val kcalToday: Int,
    val targets: MacroTargets,
    val foodLoggedToday: Boolean,
    /** Missed 'sleep' check-offs in the last 7 completed days. */
    val sleepMisses7d: Int,
    /** Days creatine has been checked (any phase task counts). */
    val creatineDays: Int,
    val todayVolumeLbs: Double,
    /** Volume on the most recent same weekday with logged sets, if any. */
    val lastSameWeekdayVolumeLbs: Double?,
    val streak: Int,
)

/**
 * Deterministic, phase-aware coaching. Rules produce candidates; the top
 * three by priority are shown, with the date breaking ties so the deck
 * rotates day to day instead of pinning the same three cards forever.
 */
object InsightEngine {

    fun insightsFor(date: LocalDate, d: InsightData): List<Insight> {
        val out = mutableListOf<Insight>()
        val phase = d.status.phase
        val weekend = date.dayOfWeek == DayOfWeek.SATURDAY || date.dayOfWeek == DayOfWeek.SUNDAY

        // --- Weight trend vs lean-bulk band (needs ≥2 entries ≥7 days apart) ---
        val trend = weeklyTrendLbs(d.weights)
        val band = NutritionEngine.weeklyGainBandLbs(d.currentWeightLbs)
        if (trend != null) {
            when {
                trend > band.endInclusive * 1.3 -> out += Insight(
                    "gain_fast", "📈", "Gaining faster than lean-bulk pace",
                    "You're trending +${fmt(trend)} lb/wk; the lean band for you is " +
                        "+${fmt(band.start)}–${fmt(band.endInclusive)}. Above it, the extra is mostly fat — " +
                        "trim ~150–200 kcal (one cup of rice) and re-check in a week.",
                    InsightTone.WARN, 90,
                )
                trend < band.start * 0.5 -> out += Insight(
                    "gain_slow", "🍚", "Scale isn't moving — eat the plan",
                    "Trend is +${fmt(trend)} lb/wk against a +${fmt(band.start)}–${fmt(band.endInclusive)} target. " +
                        "A surplus you don't eat is a cycle you waste: add ~200 kcal " +
                        "(¾ cup rice or a whey shake) and hit every meal box.",
                    InsightTone.PUSH, 88,
                )
                else -> out += Insight(
                    "gain_good", "🎯", "Weight trend is dialed",
                    "+${fmt(trend)} lb/wk sits inside your lean-bulk band " +
                        "(+${fmt(band.start)}–${fmt(band.endInclusive)}). Keep intake exactly where it is.",
                    InsightTone.INFO, 40,
                )
            }
        } else if (d.weights.size < 2) {
            out += Insight(
                "weigh_in", "⚖️", "Log morning weigh-ins",
                "Two data points a week is the minimum to steer a bulk. Weigh after " +
                    "waking, after the bathroom, before food — same conditions every time.",
                InsightTone.PUSH, 55,
            )
        }

        // --- Protein / fuel ---
        if (d.foodLoggedToday && d.proteinTodayG < d.targets.proteinG) {
            val left = d.targets.proteinG - d.proteinTodayG
            if (left > 30) {
                out += Insight(
                    "protein_gap", "🍗", "$left g of protein still to bank",
                    "Muscle protein synthesis is dose-driven: ~0.4 g/kg per meal, spread " +
                        "across 4–5 feedings, beats one giant dinner. ${mealsFor(left)}",
                    InsightTone.PUSH, 80,
                )
            }
        }
        if (!d.foodLoggedToday) {
            out += Insight(
                "log_food", "📒", "Track today's fuel",
                "On ${d.targets.kcal} kcal / ${d.targets.proteinG} g protein targets, guessing " +
                    "misses by 20% either way. The presets make it three taps.",
                InsightTone.INFO, 50,
            )
        }

        // --- Phase-specific ---
        when (phase) {
            Phase.ON_CYCLE -> {
                if (d.status.weekInPhase == 1) out += Insight(
                    "cycle_w1", "🧪", "Week 1: saturation, not fireworks",
                    "Andro compounds need 1–2 weeks to build blood levels. Don't chase " +
                        "PRs yet — run your normal loads, perfect your empty-stomach dose " +
                        "timing, and let week 3 surprise you.",
                    InsightTone.INFO, 70,
                )
                if (d.status.dayInPhase >= d.status.phaseLengthDays - 6) out += Insight(
                    "pct_prep", "🛬", "PCT starts in ${d.status.phaseLengthDays - d.status.dayInPhase + 1} days",
                    "Have Alpha-AF on hand now. PCT begins the morning after your last " +
                        "andro dose — a gap between them is how gains evaporate.",
                    InsightTone.WARN, 85,
                )
                out += Insight(
                    "liver_water", "💧", "Your liver is doing overtime",
                    "Oral andros are processed hepatically. The gallon of water, the zero " +
                        "alcohol rule, and electrolytes aren't optional extras on cycle — " +
                        "they're the cost of admission.",
                    InsightTone.INFO, 30,
                )
            }
            Phase.PCT -> {
                out += Insight(
                    "pct_training", "🏋️", "Keep the weights heavy through PCT",
                    "Training load is the signal that tells your body to keep the new " +
                        "muscle while hormones normalize. Keep intensity; cut a set or two " +
                        "only if recovery visibly dips.",
                    InsightTone.PUSH, 75,
                )
                if (d.status.dayInPhase >= 24) out += Insight(
                    "bloodwork_post", "🩸", "Book post-cycle blood work",
                    "PCT ends in ${31 - d.status.dayInPhase} days. Schedule lipids, liver " +
                        "enzymes and total/free testosterone for ~2 weeks after the last " +
                        "Alpha-AF cap to see your true baseline.",
                    InsightTone.WARN, 85,
                )
            }
            Phase.RECOVERY -> out += Insight(
                "recovery_keep", "🧱", "Recovery is where gains consolidate",
                "Strength dipping slightly off-cycle is normal — hold your training and " +
                    "protein, and the muscle stays. Next cycle is earned with baseline " +
                    "blood work, not a calendar date.",
                InsightTone.INFO, 60,
            )
            Phase.MAINTENANCE -> out += Insight(
                "base_building", "🏗️", "Base-building block",
                "Off-cycle progress is the best predictor of on-cycle results. Add 5 lb " +
                    "to a lift or a rep to a set each week — boring consistency compounds.",
                InsightTone.INFO, 45,
            )
        }

        // --- Sleep ---
        if (d.sleepMisses7d >= 2) out += Insight(
            "sleep_debt", "😴", "${d.sleepMisses7d} missed bedtimes this week",
            "Most growth-hormone release and muscle repair happens in deep sleep. " +
                "A 5:30 AM session on short sleep is a cortisol workout — protect the " +
                "9 PM lights-out like a PR attempt.",
            InsightTone.WARN, 78,
        )

        // --- Creatine saturation ---
        if (d.creatineDays in 1..13) out += Insight(
            "creatine_sat", "🧂", "Creatine day ${d.creatineDays} — keep it daily",
            "5 g every day (rest days included) saturates muscle stores in ~2–3 weeks, " +
                "no loading phase needed. The +2–4 lb of water weight in your muscles is " +
                "normal and useful — it's not fat.",
            InsightTone.INFO, 42,
        )

        // --- Progressive overload vs last week ---
        if (!weekend && d.lastSameWeekdayVolumeLbs != null && d.lastSameWeekdayVolumeLbs > 0) {
            val last = d.lastSameWeekdayVolumeLbs
            if (d.todayVolumeLbs == 0.0) out += Insight(
                "beat_last", "⚔️", "Target: beat ${fmtInt(last)} lb",
                "That's your volume from last ${date.dayOfWeek.name.lowercase().replaceFirstChar { it.uppercase() }}. " +
                    "One extra rep or +5 lb on the bar is progressive overload doing its job.",
                InsightTone.PUSH, 65,
            )
        }

        // --- Streak flavor ---
        if (d.streak in 3..6) out += Insight(
            "streak_close", "🔥", "${7 - d.streak} days to Week Warrior",
            "Perfect days compound like interest — the streak bonus grows every day " +
                "and the 7-day badge pays +75 XP.",
            InsightTone.PUSH, 35,
        )

        return out
            .sortedWith(compareByDescending<Insight> { it.priority }.thenBy { rotation(it.id, date) })
            .take(3)
    }

    /**
     * Least-squares slope of weight over time, in lb/week. Needs at least two
     * entries spanning ≥7 days — anything shorter is water-weight noise.
     */
    fun weeklyTrendLbs(weights: List<Pair<Long, Double>>): Double? {
        if (weights.size < 2) return null
        val span = weights.last().first - weights.first().first
        if (span < 7) return null
        val n = weights.size.toDouble()
        val meanX = weights.sumOf { it.first.toDouble() } / n
        val meanY = weights.sumOf { it.second } / n
        val num = weights.sumOf { (x, y) -> (x - meanX) * (y - meanY) }
        val den = weights.sumOf { (x, _) -> (x - meanX) * (x - meanX) }
        if (den == 0.0) return null
        return num / den * 7.0
    }

    private fun mealsFor(gramsLeft: Int): String = when {
        gramsLeft > 90 -> "That's three feedings: chicken + rice twice and a double shake."
        gramsLeft > 50 -> "Two feedings cover it — e.g. 6 oz chicken and a double whey shake."
        else -> "One 6 oz chicken breast or a double shake closes it."
    }

    private fun rotation(id: String, date: LocalDate): Int =
        abs((id.hashCode() + date.toEpochDay()).toInt() % 97)

    private fun fmt(v: Double): String = "%.1f".format(v)
    private fun fmtInt(v: Double): String = "%,d".format(v.toLong())
}
