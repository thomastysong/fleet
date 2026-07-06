package com.musclequest.app.notifications

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import com.musclequest.app.data.Settings
import com.musclequest.app.domain.CycleEngine
import com.musclequest.app.domain.CycleStatus
import com.musclequest.app.domain.Phase
import java.time.DayOfWeek
import java.time.LocalDate
import java.time.ZoneId

/**
 * Three daily reminder slots, armed as one-shot alarms: inexact repeating
 * alarms are deferred for hours by Doze — fatal for a 4:30 AM dose window.
 * Each slot is computed for the date it will actually fire on, so the text
 * follows the cycle phase (Alpha-AF instead of andro during PCT, nothing
 * misleading off-cycle) and the time follows the weekend shift.
 * [ReminderReceiver] re-arms every slot after each firing, and app launch,
 * boot, and clock/timezone changes re-anchor local time.
 */
object ReminderScheduler {

    enum class Slot(val requestCode: Int) { AM(1001), PM(1002), WIND_DOWN(1003) }

    private data class Spec(val hour: Int, val minute: Int, val title: String, val message: String)

    /** PM dose pauses off-cycle, so a slot's next applicable day can be ahead. */
    private const val LOOKAHEAD_DAYS = 14

    fun scheduleNext(context: Context, settings: Settings) {
        val alarmManager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
        val zone = ZoneId.systemDefault()
        val now = System.currentTimeMillis()
        Slot.entries.forEach { slot ->
            var scheduled = false
            var date = LocalDate.now()
            repeat(LOOKAHEAD_DAYS) {
                if (!scheduled) {
                    val status = CycleEngine.status(date, settings.cycleStart, settings.activeWeeks)
                    val spec = specFor(slot, date, status)
                    if (spec != null) {
                        val triggerAt = date.atTime(spec.hour, spec.minute).atZone(zone).toInstant().toEpochMilli()
                        if (triggerAt > now) {
                            setAlarm(alarmManager, triggerAt, pendingIntent(context, slot, spec))
                            scheduled = true
                        }
                    }
                    date = date.plusDays(1)
                }
            }
            if (!scheduled) alarmManager.cancel(pendingIntent(context, slot, null))
        }
    }

    fun cancelAll(context: Context) {
        val alarmManager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
        Slot.entries.forEach { alarmManager.cancel(pendingIntent(context, it, null)) }
    }

    private fun setAlarm(alarmManager: AlarmManager, triggerAt: Long, pi: PendingIntent) {
        val canExact = Build.VERSION.SDK_INT < Build.VERSION_CODES.S || alarmManager.canScheduleExactAlarms()
        if (canExact) {
            alarmManager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAt, pi)
        } else {
            // Fires during Doze too, just with bounded (~15 min) slack.
            alarmManager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAt, pi)
        }
    }

    private fun specFor(slot: Slot, date: LocalDate, status: CycleStatus): Spec? {
        val weekend = date.dayOfWeek == DayOfWeek.SATURDAY || date.dayOfWeek == DayOfWeek.SUNDAY
        return when (slot) {
            Slot.AM -> when (status.phase) {
                Phase.ON_CYCLE -> Spec(
                    if (weekend) 6 else 4, 30,
                    "Rise & dose 💊",
                    "AM andro dose on an empty stomach, 16 oz water." +
                        if (weekend) "" else " Gym in 60 minutes.",
                )
                Phase.PCT -> Spec(
                    if (weekend) 9 else 7, 15,
                    "Alpha-AF with breakfast 💊",
                    "3 capsules with your first meal — every day of PCT, no exceptions.",
                )
                else -> if (weekend) {
                    null
                } else {
                    Spec(4, 30, "Rise & grind 🏋️", "Wake-up call — gym in 60 minutes.")
                }
            }
            Slot.PM -> if (status.phase == Phase.ON_CYCLE) {
                Spec(
                    if (weekend) 18 else 16, 30,
                    "PM dose window ⏰",
                    "Empty stomach? Take the PM dose now — dinner in 45 minutes.",
                )
            } else {
                null
            }
            Slot.WIND_DOWN -> Spec(
                if (weekend) 21 else 20, 45,
                "Wind down 😴",
                "Evening shake, screens off. Lights out at ${if (weekend) 10 else 9} — " +
                    "that's where the muscle gets built.",
            )
        }
    }

    private fun pendingIntent(context: Context, slot: Slot, spec: Spec?): PendingIntent {
        val intent = Intent(context, ReminderReceiver::class.java).apply {
            putExtra(ReminderReceiver.EXTRA_ID, slot.requestCode)
            if (spec != null) {
                putExtra(ReminderReceiver.EXTRA_TITLE, spec.title)
                putExtra(ReminderReceiver.EXTRA_MESSAGE, spec.message)
            }
        }
        return PendingIntent.getBroadcast(
            context,
            slot.requestCode,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }
}
