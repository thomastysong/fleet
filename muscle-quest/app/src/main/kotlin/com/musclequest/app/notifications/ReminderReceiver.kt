package com.musclequest.app.notifications

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.musclequest.app.MainActivity
import com.musclequest.app.R

class ReminderReceiver : BroadcastReceiver() {

    companion object {
        const val EXTRA_TITLE = "title"
        const val EXTRA_MESSAGE = "message"
        const val EXTRA_ID = "id"
        const val CHANNEL_ID = "musclequest_reminders"
    }

    override fun onReceive(context: Context, intent: Intent) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            return
        }

        ensureChannel(context)

        val tapIntent = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(intent.getStringExtra(EXTRA_TITLE) ?: "MuscleQuest")
            .setContentText(intent.getStringExtra(EXTRA_MESSAGE) ?: "Time to check your plan.")
            .setStyle(NotificationCompat.BigTextStyle().bigText(intent.getStringExtra(EXTRA_MESSAGE)))
            .setContentIntent(tapIntent)
            .setAutoCancel(true)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .build()

        NotificationManagerCompat.from(context)
            .notify(intent.getIntExtra(EXTRA_ID, 0), notification)
    }

    private fun ensureChannel(context: Context) {
        val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ID,
                "Daily reminders",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply { description = "Dose, training, and sleep reminders" },
        )
    }
}
