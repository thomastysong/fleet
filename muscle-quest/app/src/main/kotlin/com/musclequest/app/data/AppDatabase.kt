package com.musclequest.app.data

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

@Database(
    entities = [
        Completion::class, WorkoutSet::class, WeightEntry::class,
        UnlockedAchievement::class, FoodEntry::class,
    ],
    version = 2,
    exportSchema = false,
)
abstract class AppDatabase : RoomDatabase() {
    abstract fun completionDao(): CompletionDao
    abstract fun workoutSetDao(): WorkoutSetDao
    abstract fun weightDao(): WeightDao
    abstract fun achievementDao(): AchievementDao
    abstract fun foodDao(): FoodDao

    companion object {
        @Volatile
        private var instance: AppDatabase? = null

        /** v1 → v2: adds the food log. */
        private val MIGRATION_1_2 = object : Migration(1, 2) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL(
                    "CREATE TABLE IF NOT EXISTS `food_entries` (" +
                        "`id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, " +
                        "`epochDay` INTEGER NOT NULL, " +
                        "`name` TEXT NOT NULL, " +
                        "`kcal` INTEGER NOT NULL, " +
                        "`proteinG` INTEGER NOT NULL, " +
                        "`carbsG` INTEGER NOT NULL, " +
                        "`fatG` INTEGER NOT NULL, " +
                        "`loggedAtMillis` INTEGER NOT NULL)",
                )
                db.execSQL(
                    "CREATE INDEX IF NOT EXISTS `index_food_entries_epochDay` " +
                        "ON `food_entries` (`epochDay`)",
                )
            }
        }

        fun get(context: Context): AppDatabase = instance ?: synchronized(this) {
            instance ?: Room.databaseBuilder(
                context.applicationContext,
                AppDatabase::class.java,
                "musclequest.db",
            ).addMigrations(MIGRATION_1_2).build().also { instance = it }
        }
    }
}
