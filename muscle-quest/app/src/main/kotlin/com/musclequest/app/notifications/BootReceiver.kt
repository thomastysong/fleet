package com.musclequest.app.notifications

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.musclequest.app.MuscleQuestApp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Re-arms the reminder alarms after a reboot, and re-anchors their local-time
 * triggers after clock or timezone changes (DST included) so a 4:30 AM dose
 * reminder stays 4:30 AM wall-clock.
 */
class BootReceiver : BroadcastReceiver() {

    private val actions = setOf(
        Intent.ACTION_BOOT_COMPLETED,
        Intent.ACTION_TIMEZONE_CHANGED,
        Intent.ACTION_TIME_CHANGED,
    )

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action !in actions) return
        val pending = goAsync()
        CoroutineScope(Dispatchers.Default).launch {
            try {
                MuscleQuestApp.syncReminders(context.applicationContext)
            } finally {
                pending.finish()
            }
        }
    }
}
