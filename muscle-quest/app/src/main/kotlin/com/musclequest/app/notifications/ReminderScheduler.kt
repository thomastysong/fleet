package com.musclequest.app.notifications

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import java.util.Calendar

/**
 * Three fixed daily reminders, scheduled with inexact repeating alarms (no
 * special permissions needed; a few minutes of drift doesn't matter here).
 */
object ReminderScheduler {

    data class Reminder(val requestCode: Int, val hour: Int, val minute: Int, val title: String, val message: String)

    val REMINDERS = listOf(
        Reminder(
            1001, 4, 30,
            "Rise & dose 💊",
            "AM andro dose on an empty stomach, 16 oz water. Gym in 60 minutes.",
        ),
        Reminder(
            1002, 16, 30,
            "PM dose window ⏰",
            "Empty stomach? Take the PM dose now — dinner in 45 minutes.",
        ),
        Reminder(
            1003, 20, 45,
            "Wind down 😴",
            "Evening shake, screens off. Lights out at 9 — that's where the muscle gets built.",
        ),
    )

    fun scheduleDaily(context: Context) {
        val alarmManager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
        REMINDERS.forEach { reminder ->
            alarmManager.setInexactRepeating(
                AlarmManager.RTC_WAKEUP,
                nextTrigger(reminder.hour, reminder.minute),
                AlarmManager.INTERVAL_DAY,
                pendingIntent(context, reminder),
            )
        }
    }

    fun cancelAll(context: Context) {
        val alarmManager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
        REMINDERS.forEach { alarmManager.cancel(pendingIntent(context, it)) }
    }

    private fun pendingIntent(context: Context, reminder: Reminder): PendingIntent {
        val intent = Intent(context, ReminderReceiver::class.java).apply {
            putExtra(ReminderReceiver.EXTRA_TITLE, reminder.title)
            putExtra(ReminderReceiver.EXTRA_MESSAGE, reminder.message)
            putExtra(ReminderReceiver.EXTRA_ID, reminder.requestCode)
        }
        return PendingIntent.getBroadcast(
            context,
            reminder.requestCode,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun nextTrigger(hour: Int, minute: Int): Long {
        val cal = Calendar.getInstance().apply {
            set(Calendar.HOUR_OF_DAY, hour)
            set(Calendar.MINUTE, minute)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
            if (timeInMillis <= System.currentTimeMillis()) {
                add(Calendar.DAY_OF_YEAR, 1)
            }
        }
        return cal.timeInMillis
    }
}
