package com.musclequest.app.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.musclequest.app.data.AppDatabase
import com.musclequest.app.data.Completion
import com.musclequest.app.data.Repository
import com.musclequest.app.data.Settings
import com.musclequest.app.data.SettingsStore
import com.musclequest.app.data.ToggleResult
import com.musclequest.app.data.WeightEntry
import com.musclequest.app.data.WorkoutSet
import com.musclequest.app.domain.Achievement
import com.musclequest.app.domain.Achievements
import com.musclequest.app.domain.CycleEngine
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.DailyPlanner
import com.musclequest.app.domain.DayWorkout
import com.musclequest.app.domain.LevelInfo
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

data class WorkoutUiState(
    val workout: DayWorkout? = null,
    val sets: List<WorkoutSet> = emptyList(),
    val todayVolume: Double = 0.0,
    val workoutDone: Boolean = false,
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

    val workoutState: StateFlow<WorkoutUiState> = combine(today, todaySets, todayState) { date, sets, t ->
        WorkoutUiState(
            workout = WorkoutPlan.forDay(date.dayOfWeek),
            sets = sets,
            todayVolume = sets.sumOf { it.weightLbs * it.reps },
            workoutDone = t.tasks.any { it.task.id == "workout" && it.done },
        )
    }.stateIn(viewModelScope, SharingStarted.Eagerly, WorkoutUiState())

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
            when (result) {
                ToggleResult.CHECKED_PERFECT ->
                    _events.send("PERFECT DAY! +${taskUi.task.xp} XP + bonus 🏆")
                ToggleResult.CHECKED ->
                    _events.send("+${taskUi.task.xp} XP — ${taskUi.task.title}")
                ToggleResult.UNCHECKED -> Unit
            }
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

    fun deleteSet(id: Long) {
        viewModelScope.launch { repo.workoutSetDao.delete(id) }
    }

    fun logWeight(weightLbs: Double) {
        viewModelScope.launch {
            repo.logWeight(today.value, weightLbs)
            _events.send("Body weight logged: ${weightLbs.trimZeros()} lb")
        }
    }

    fun setCycleStart(date: LocalDate) = viewModelScope.launch { settingsStore.setCycleStart(date) }
    fun setActiveWeeks(weeks: Int) = viewModelScope.launch { settingsStore.setActiveWeeks(weeks) }
    fun setRemindersEnabled(enabled: Boolean) = viewModelScope.launch { settingsStore.setRemindersEnabled(enabled) }

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
                    )
                    val fresh = Achievements.earned(stats)
                        .filter { it.id !in p.unlockedIds && announced.add(it.id) }
                    fresh.forEach { a ->
                        repo.unlock(a, today.value, System.currentTimeMillis())
                        _events.send("${a.emoji} Achievement unlocked: ${a.title}! +${a.xpReward} XP")
                    }
                }
        }
    }
}

private fun Double.trimZeros(): String =
    if (this == this.toLong().toDouble()) this.toLong().toString() else "%.1f".format(this)
