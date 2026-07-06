package com.musclequest.app

import android.app.Application
import android.content.Context
import com.musclequest.app.data.SettingsStore
import com.musclequest.app.notifications.ReminderScheduler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class MuscleQuestApp : Application() {

    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onCreate() {
        super.onCreate()
        appScope.launch {
            SettingsStore(this@MuscleQuestApp).ensureInitialized()
            syncReminders(this@MuscleQuestApp)
        }
    }

    companion object {
        /** Arms or disarms the daily reminder alarms based on the saved settings. */
        suspend fun syncReminders(context: Context) {
            val settings = SettingsStore(context).settings.first()
            if (settings.remindersEnabled) {
                ReminderScheduler.scheduleNext(context, settings)
            } else {
                ReminderScheduler.cancelAll(context)
            }
        }
    }
}
