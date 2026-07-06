package com.musclequest.app.notifications

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.musclequest.app.MuscleQuestApp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/** Re-registers the daily reminder alarms after a device reboot. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
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
