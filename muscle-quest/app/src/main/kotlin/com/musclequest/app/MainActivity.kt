package com.musclequest.app

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Checklist
import androidx.compose.material.icons.filled.EmojiEvents
import androidx.compose.material.icons.filled.FitnessCenter
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.core.content.ContextCompat
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.musclequest.app.ui.MainViewModel
import com.musclequest.app.ui.screens.CycleScreen
import com.musclequest.app.ui.screens.ProgressScreen
import com.musclequest.app.ui.screens.SettingsScreen
import com.musclequest.app.ui.screens.TodayScreen
import com.musclequest.app.ui.screens.WorkoutScreen
import com.musclequest.app.ui.theme.MuscleQuestTheme
import kotlinx.coroutines.flow.collectLatest

private data class Tab(val route: String, val label: String, val icon: ImageVector)

private val TABS = listOf(
    Tab("today", "Today", Icons.Filled.Checklist),
    Tab("train", "Train", Icons.Filled.FitnessCenter),
    Tab("cycle", "Cycle", Icons.Filled.CalendarMonth),
    Tab("progress", "Progress", Icons.Filled.EmojiEvents),
    Tab("settings", "Settings", Icons.Filled.Settings),
)

class MainActivity : ComponentActivity() {

    private val viewModel: MainViewModel by viewModels()

    private val notificationPermission =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        requestNotificationPermissionIfNeeded()

        setContent {
            MuscleQuestTheme {
                val navController = rememberNavController()
                val snackbarHostState = remember { SnackbarHostState() }

                LaunchedEffect(Unit) {
                    // collectLatest: a new event preempts the one on screen, so
                    // bulk check-offs don't queue a minute of stale toasts.
                    viewModel.events.collectLatest { snackbarHostState.showSnackbar(it) }
                }

                Scaffold(
                    snackbarHost = { SnackbarHost(snackbarHostState) },
                    bottomBar = {
                        val backStack by navController.currentBackStackEntryAsState()
                        val currentRoute = backStack?.destination?.route
                        NavigationBar {
                            TABS.forEach { tab ->
                                NavigationBarItem(
                                    selected = currentRoute == tab.route,
                                    onClick = {
                                        navController.navigate(tab.route) {
                                            popUpTo(navController.graph.startDestinationId) { saveState = true }
                                            launchSingleTop = true
                                            restoreState = true
                                        }
                                    },
                                    icon = { Icon(tab.icon, contentDescription = tab.label) },
                                    label = { Text(tab.label) },
                                )
                            }
                        }
                    },
                ) { innerPadding ->
                    val todayState by viewModel.todayState.collectAsState()
                    val workoutState by viewModel.workoutState.collectAsState()
                    val progressState by viewModel.progressState.collectAsState()
                    val settings by viewModel.settings.collectAsState()

                    NavHost(
                        navController = navController,
                        startDestination = "today",
                        modifier = Modifier.padding(innerPadding),
                    ) {
                        composable("today") {
                            TodayScreen(state = todayState, onToggle = viewModel::toggleTask)
                        }
                        composable("train") {
                            WorkoutScreen(
                                state = workoutState,
                                onLogSet = viewModel::logSet,
                                onDeleteSet = viewModel::deleteSet,
                                onFinishWorkout = {
                                    // completeTask is check-only, so a double-tap
                                    // racing this stale snapshot can't un-check.
                                    todayState.tasks
                                        .firstOrNull { it.task.id == "workout" }
                                        ?.let(viewModel::completeTask)
                                },
                            )
                        }
                        composable("cycle") {
                            CycleScreen(settings = settings, status = todayState.status)
                        }
                        composable("progress") {
                            ProgressScreen(
                                progress = progressState,
                                today = todayState,
                                onLogWeight = viewModel::logWeight,
                            )
                        }
                        composable("settings") {
                            SettingsScreen(
                                settings = settings,
                                onSetCycleStart = viewModel::setCycleStart,
                                onSetActiveWeeks = viewModel::setActiveWeeks,
                                onSetReminders = { enabled ->
                                    // Re-prompt when turning reminders on after an
                                    // earlier denial; otherwise the alarms fire into
                                    // a void with the toggle claiming otherwise.
                                    if (enabled) requestNotificationPermissionIfNeeded()
                                    viewModel.setRemindersEnabled(enabled)
                                },
                            )
                        }
                    }
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        viewModel.refreshToday()
    }

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}
