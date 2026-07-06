package com.musclequest.app.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.musclequest.app.MuscleQuestApp
import com.musclequest.app.data.AppDatabase
import com.musclequest.app.data.Completion
import com.musclequest.app.data.FoodEntry
import com.musclequest.app.data.Repository
import com.musclequest.app.data.Settings
import com.musclequest.app.data.SettingsStore
import com.musclequest.app.data.ToggleResult
import com.musclequest.app.data.WeightEntry
import com.musclequest.app.data.WorkoutSet
import com.musclequest.app.domain.Achievement
import com.musclequest.app.domain.Achievements
import com.musclequest.app.domain.ActivityLevel
import com.musclequest.app.domain.BodyProfile
import com.musclequest.app.domain.CycleEngine
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.DailyPlanner
import com.musclequest.app.domain.DayWorkout
import com.musclequest.app.domain.FoodPreset
import com.musclequest.app.domain.FoodPresets
import com.musclequest.app.domain.Insight
import com.musclequest.app.domain.InsightData
import com.musclequest.app.domain.InsightEngine
import com.musclequest.app.domain.LevelInfo
import com.musclequest.app.domain.MacroTargets
import com.musclequest.app.domain.NutritionEngine
import com.musclequest.app.domain.Phase
import com.musclequest.app.domain.PlannedTask
import com.musclequest.app.domain.StatsSnapshot
import com.musclequest.app.domain.WorkoutPlan
import com.musclequest.app.domain.Xp
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

data class TaskUi(val task: PlannedTask, val done: Boolean)

data class TodayUiState(
    val date: LocalDate = LocalDate.now(),
    val status: CycleStatus? = null,
    val tasks: List<TaskUi> = emptyList(),
    val perfectDay: Boolean = false,
    val streak: Int = 0,
    val totalXp: Long = 0,
    val level: LevelInfo = Xp.levelInfo(0),
    val xpEarnedToday: Int = 0,
)

/** Historical context for one exercise, for progressive-overload hints. */
data class ExerciseStats(
    val allTimeBestLbs: Double? = null,
    val lastTopSet: WorkoutSet? = null,
    val est1RmLbs: Double? = null,
)

data class WorkoutUiState(
    val workout: DayWorkout? = null,
    val sets: List<WorkoutSet> = emptyList(),
    val todayVolume: Double = 0.0,
    val workoutDone: Boolean = false,
    val stats: Map<String, ExerciseStats> = emptyMap(),
    /** Total volume from the same weekday last week, if any was logged. */
    val lastWeekVolume: Double? = null,
)

data class MacroTotals(
    val kcal: Int = 0,
    val proteinG: Int = 0,
    val carbsG: Int = 0,
    val fatG: Int = 0,
)

data class FuelUiState(
    val entries: List<FoodEntry> = emptyList(),
    val totals: MacroTotals = MacroTotals(),
    val targets: MacroTargets =
        NutritionEngine.targets(BodyProfile(31, 72, 170.0, true, ActivityLevel.MODERATE, 400)),
    val presets: List<FoodPreset> = FoodPresets.ALL,
    val proteinHit: Boolean = false,
)

data class ProgressUiState(
    val weights: List<WeightEntry> = emptyList(),
    val unlockedIds: Set<String> = emptySet(),
    val totalVolume: Long = 0,
    val prCount: Int = 0,
    val workoutsCompleted: Int = 0,
)

@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModel(app: Application) : AndroidViewModel(app) {

    private val repo = Repository(AppDatabase.get(app))
    private val settingsStore = SettingsStore(app)

    /** Bumped on resume so "today" flows recompute after midnight. */
    private val today = MutableStateFlow(LocalDate.now())

    /** One-shot celebration messages (XP toasts, PRs, achievement unlocks). */
    private val _events = Channel<String>(Channel.BUFFERED)
    val events: Flow<String> = _events.receiveAsFlow()

    val settings: StateFlow<Settings?> = settingsStore.settings
        .stateIn(viewModelScope, SharingStarted.Eagerly, null)

    private val statusFlow = combine(today, settingsStore.settings) { date, s ->
        Triple(date, s, CycleEngine.status(date, s.cycleStart, s.activeWeeks))
    }

    // 366-day window so computeStreak's year-long walk never runs out of data.
    private val recentCompletions = today.flatMapLatest { date ->
        repo.completionDao.since(date.minusDays(366).toEpochDay())
    }

    val todayState: StateFlow<TodayUiState> = combine(
        statusFlow,
        recentCompletions,
        repo.completionDao.totalXp(),
    ) { (date, s, status), recent, totalXp ->
        val epochDay = date.toEpochDay()
        val tasks = DailyPlanner.tasksFor(date, status)
        val todayRows = recent.filter { it.epochDay == epochDay }
        val doneIds = todayRows.map { it.taskId }.toSet()
        val perfect = tasks.all { it.id in doneIds }
        val streak = computeStreak(date, s, recent, includeToday = perfect)
        TodayUiState(
            date = date,
            status = status,
            tasks = tasks.map { TaskUi(it, it.id in doneIds) },
            perfectDay = perfect,
            streak = streak,
            totalXp = totalXp,
            level = Xp.levelInfo(totalXp),
            xpEarnedToday = todayRows.sumOf { it.xp },
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, TodayUiState())

    private val todaySets = today.flatMapLatest { date -> repo.workoutSetDao.forDay(date.toEpochDay()) }

    private val exerciseStats = combine(today, todaySets) { date, _ ->
        val workout = WorkoutPlan.forDay(date.dayOfWeek)
            ?: return@combine emptyMap<String, ExerciseStats>()
        val names = workout.exercises.map { it.name }
        val history = repo.workoutSetDao.historyFor(names, date.toEpochDay()).filter { it.reps > 0 }
        names.associateWith { name ->
            val h = history.filter { it.exercise == name }
            val lastDay = h.maxOfOrNull { it.epochDay }
            ExerciseStats(
                allTimeBestLbs = h.maxOfOrNull { it.weightLbs },
                lastTopSet = h.filter { it.epochDay == lastDay }.maxByOrNull { it.weightLbs },
                est1RmLbs = h.mapNotNull { NutritionEngine.estimate1Rm(it.weightLbs, it.reps) }.maxOrNull(),
            )
        }
    }

    val workoutState: StateFlow<WorkoutUiState> = combine(
        today, todaySets, todayState, exerciseStats,
    ) { date, sets, t, stats ->
        WorkoutUiState(
            workout = WorkoutPlan.forDay(date.dayOfWeek),
            sets = sets,
            todayVolume = sets.sumOf { it.weightLbs * it.reps },
            workoutDone = t.tasks.any { it.task.id == "workout" && it.done },
            stats = stats,
            lastWeekVolume = repo.workoutSetDao.volumeForDay(date.toEpochDay() - 7).takeIf { it > 0 },
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, WorkoutUiState())

    private val latestWeight = repo.weightDao.all().map { it.lastOrNull()?.weightLbs }

    /** Live science targets — recomputed whenever profile or weight changes. */
    val targetsState: StateFlow<MacroTargets> = combine(settingsStore.settings, latestWeight) { s, w ->
        NutritionEngine.targets(s.profile(w))
    }.stateIn(viewModelScope, SharingStarted.Eagerly, FuelUiState().targets)

    private val todayFood = today.flatMapLatest { repo.foodDao.forDay(it.toEpochDay()) }

    val fuelState: StateFlow<FuelUiState> = combine(todayFood, targetsState) { entries, targets ->
        val totals = MacroTotals(
            kcal = entries.sumOf { it.kcal },
            proteinG = entries.sumOf { it.proteinG },
            carbsG = entries.sumOf { it.carbsG },
            fatG = entries.sumOf { it.fatG },
        )
        FuelUiState(
            entries = entries,
            totals = totals,
            targets = targets,
            presets = FoodPresets.ALL,
            proteinHit = targets.proteinG > 0 && totals.proteinG >= targets.proteinG,
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, FuelUiState())

    /** Top-3 coach cards for today, recomputed as the day's data changes. */
    val insights: StateFlow<List<Insight>> = combine(
        todayState, fuelState, repo.weightDao.all(), recentCompletions, todaySets,
    ) { t, f, weightsAll, recent, sets ->
        val status = t.status ?: return@combine emptyList()
        val epoch = t.date.toEpochDay()
        val byDay = recent.groupBy({ it.epochDay }, { it.taskId })
        // Only days that have activity at all count as trackable misses.
        val sleepMisses = (1..7).count { d ->
            byDay[epoch - d]?.let { "sleep" !in it } == true
        }
        val creatineDays = recent
            .filter { it.taskId == "pwo_shake" || it.taskId == "creatine_rest" }
            .map { it.epochDay }.distinct().size
        InsightEngine.insightsFor(
            t.date,
            InsightData(
                status = status,
                weights = weightsAll.filter { it.epochDay >= epoch - 30 }
                    .map { it.epochDay to it.weightLbs },
                currentWeightLbs = weightsAll.lastOrNull()?.weightLbs
                    ?: (f.targets.proteinG / 1.2), // protein target is 1.2 g/lb, so this recovers profile weight
                proteinTodayG = f.totals.proteinG,
                kcalToday = f.totals.kcal,
                targets = f.targets,
                foodLoggedToday = f.entries.isNotEmpty(),
                sleepMisses7d = sleepMisses,
                creatineDays = creatineDays,
                todayVolumeLbs = sets.sumOf { it.weightLbs * it.reps },
                lastSameWeekdayVolumeLbs = repo.workoutSetDao.volumeForDay(epoch - 7).takeIf { it > 0 },
                streak = t.streak,
            ),
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    val progressState: StateFlow<ProgressUiState> = combine(
        repo.weightDao.all(),
        repo.achievementDao.all(),
        repo.workoutSetDao.totalVolume(),
        repo.workoutSetDao.prCount(),
        repo.completionDao.workoutCount(),
    ) { weights, unlocked, volume, prs, workouts ->
        ProgressUiState(
            weights = weights,
            unlockedIds = unlocked.map { it.achievementId }.toSet(),
            totalVolume = volume.toLong(),
            prCount = prs,
            workoutsCompleted = workouts,
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, ProgressUiState())

    init {
        watchAchievements()
    }

    fun refreshToday() {
        today.value = LocalDate.now()
    }

    fun toggleTask(taskUi: TaskUi) {
        val t = todayState.value
        viewModelScope.launch {
            val result = repo.toggleTask(
                date = t.date,
                task = taskUi.task,
                plannedTasks = t.tasks.map { it.task },
                streakBeforeToday = if (t.perfectDay) (t.streak - 1).coerceAtLeast(0) else t.streak,
                nowMillis = System.currentTimeMillis(),
            )
            announce(result, taskUi)
        }
    }

    /** Check-only variant for buttons like "Finish workout": never un-checks. */
    fun completeTask(taskUi: TaskUi) {
        val t = todayState.value
        viewModelScope.launch {
            val result = repo.completeTask(
                date = t.date,
                task = taskUi.task,
                plannedTasks = t.tasks.map { it.task },
                streakBeforeToday = if (t.perfectDay) (t.streak - 1).coerceAtLeast(0) else t.streak,
                nowMillis = System.currentTimeMillis(),
            ) ?: return@launch
            announce(result, taskUi)
        }
    }

    private suspend fun announce(result: ToggleResult, taskUi: TaskUi) {
        when (result) {
            ToggleResult.CHECKED_PERFECT ->
                _events.send("PERFECT DAY! +${taskUi.task.xp} XP + bonus 🏆")
            ToggleResult.CHECKED ->
                _events.send("+${taskUi.task.xp} XP — ${taskUi.task.title}")
            ToggleResult.UNCHECKED -> Unit
        }
    }

    fun logSet(exercise: String, weightLbs: Double, reps: Int) {
        viewModelScope.launch {
            val pr = repo.logSet(today.value, exercise, weightLbs, reps, System.currentTimeMillis())
            _events.send(
                if (pr) {
                    "NEW PR on $exercise — ${weightLbs.trimZeros()} lb! +${DailyPlanner.PR_XP} XP 🎉"
                } else {
                    "Set logged: $exercise ${weightLbs.trimZeros()} lb × $reps"
                },
            )
        }
    }

    fun deleteSet(set: WorkoutSet) {
        viewModelScope.launch { repo.deleteSet(set) }
    }

    fun logWeight(weightLbs: Double) {
        viewModelScope.launch {
            repo.logWeight(today.value, weightLbs)
            _events.send("Body weight logged: ${weightLbs.trimZeros()} lb")
        }
    }

    fun logFood(preset: FoodPreset) =
        logCustomFood(preset.name, preset.kcal, preset.proteinG, preset.carbsG, preset.fatG)

    fun logCustomFood(name: String, kcal: Int, proteinG: Int, carbsG: Int, fatG: Int) {
        viewModelScope.launch {
            val entry = FoodEntry(
                epochDay = today.value.toEpochDay(),
                name = name,
                kcal = kcal,
                proteinG = proteinG,
                carbsG = carbsG,
                fatG = fatG,
                loggedAtMillis = System.currentTimeMillis(),
            )
            val hitTarget = repo.logFood(entry, fuelState.value.targets.proteinG)
            _events.send(
                if (hitTarget) {
                    "🎯 Protein target hit! +${DailyPlanner.FUEL_PROTEIN_XP} XP"
                } else {
                    "Logged: $name"
                },
            )
        }
    }

    fun deleteFood(entry: FoodEntry) {
        viewModelScope.launch { repo.deleteFood(entry, fuelState.value.targets.proteinG) }
    }

    fun setAge(years: Int) = viewModelScope.launch { settingsStore.setAge(years) }
    fun setHeightInches(inches: Int) = viewModelScope.launch { settingsStore.setHeightInches(inches) }
    fun setIsMale(male: Boolean) = viewModelScope.launch { settingsStore.setIsMale(male) }
    fun setActivity(level: ActivityLevel) = viewModelScope.launch { settingsStore.setActivity(level) }
    fun setSurplus(kcal: Int) = viewModelScope.launch { settingsStore.setSurplus(kcal) }

    // Cycle changes move phase boundaries, so reminder alarms are re-derived
    // after every write (sequentially, to avoid racing the DataStore read).
    fun setCycleStart(date: LocalDate) = viewModelScope.launch {
        settingsStore.setCycleStart(date)
        MuscleQuestApp.syncReminders(getApplication())
    }

    fun setActiveWeeks(weeks: Int) = viewModelScope.launch {
        settingsStore.setActiveWeeks(weeks)
        MuscleQuestApp.syncReminders(getApplication())
    }

    fun setRemindersEnabled(enabled: Boolean) = viewModelScope.launch {
        settingsStore.setRemindersEnabled(enabled)
        MuscleQuestApp.syncReminders(getApplication())
    }

    /**
     * Streak = consecutive perfect days ending yesterday (plus today when
     * [includeToday]). A perfect day means every planned task for that date
     * was checked off.
     */
    private fun computeStreak(
        date: LocalDate,
        settings: Settings,
        recent: List<Completion>,
        includeToday: Boolean,
    ): Int {
        val byDay = recent.groupBy({ it.epochDay }, { it.taskId })
        var streak = if (includeToday) 1 else 0
        var d = date.minusDays(1)
        while (streak <= 365) {
            val status = CycleEngine.status(d, settings.cycleStart, settings.activeWeeks)
            val planned = DailyPlanner.tasksFor(d, status).map { it.id }
            val done = byDay[d.toEpochDay()].orEmpty().toSet()
            if (planned.all { it in done }) {
                streak++
                d = d.minusDays(1)
            } else {
                break
            }
        }
        return streak
    }

    private fun watchAchievements() {
        viewModelScope.launch {
            // Guards against re-announcing an unlock while progressState is
            // still catching up to the freshly inserted row.
            val announced = mutableSetOf<String>()
            combine(todayState, progressState) { t, p -> t to p }
                .collect { (t, p) ->
                    val status = t.status ?: return@collect
                    val started = status.daysUntilStart == 0
                    val pastActive = started && status.phase != Phase.ON_CYCLE
                    val pastPct = started &&
                        (status.phase == Phase.RECOVERY || status.phase == Phase.MAINTENANCE)
                    val zone = ZoneId.systemDefault()
                    val sixAm = LocalTime.of(6, 0)
                    val earlyWorkouts = repo.completionDao.workoutCompletionTimes().count {
                        Instant.ofEpochMilli(it).atZone(zone).toLocalTime().isBefore(sixAm)
                    }
                    val stats = StatsSnapshot(
                        workoutsCompleted = p.workoutsCompleted,
                        earlyWorkouts = earlyWorkouts,
                        currentStreak = t.streak,
                        bothDoseDays = repo.completionDao.bothDoseDays().first(),
                        prCount = p.prCount,
                        totalVolumeLbs = p.totalVolume,
                        waterGoalDays = repo.completionDao.waterGoalDays().first(),
                        level = t.level.level,
                        phase = status.phase,
                        // Only counts when they actually trained during the cycle,
                        // so a fresh install dated in the past doesn't auto-unlock.
                        cycleCompleted = pastActive && p.workoutsCompleted > 0,
                        pctCompleted = pastPct && p.workoutsCompleted > 0,
                        proteinTargetDays =
                            repo.completionDao.countForTask(DailyPlanner.FUEL_PROTEIN_TASK_ID),
                        foodLoggedDays = repo.foodDao.daysLogged().first(),
                    )
                    // Unlock state is read from the DAO, not the progressState
                    // snapshot: at startup that StateFlow still holds its empty
                    // default and would re-award everything already unlocked.
                    val unlockedNow = repo.achievementDao.all().first()
                        .map { it.achievementId }
                        .toSet()
                    val fresh = Achievements.earned(stats)
                        .filter { it.id !in unlockedNow && announced.add(it.id) }
                    fresh.forEach { a ->
                        if (repo.unlock(a, today.value, System.currentTimeMillis())) {
                            _events.send("${a.emoji} Achievement unlocked: ${a.title}! +${a.xpReward} XP")
                        }
                    }
                }
        }
    }
}

private fun Double.trimZeros(): String =
    if (this == this.toLong().toDouble()) this.toLong().toString() else "%.1f".format(this)
