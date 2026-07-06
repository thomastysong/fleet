# MuscleQuest 💪⚡

A gamified muscle-building companion for one specific mission: run a full
supplement cycle + PCT with perfect consistency, train Mon–Fri at 5:30 AM,
eat big, and get paid in XP for every box you check.

**The plan itself lives in [SCHEDULE.md](SCHEDULE.md).** The app turns that
plan into a daily game.

## Features

- **Today** — the day's full checklist (doses, meals, training, water, sleep),
  generated from your cycle phase and day of week. Every check-off pays XP;
  finishing everything pays a perfect-day bonus that scales with your streak 🔥.
  Plus a **daily coach**: up to three science-based insight cards computed from
  your actual data (weight trend vs the lean-bulk band, protein gap, sleep
  debt, creatine saturation, phase-specific guidance, blood-work timing).
- **Fuel** — calorie & macro tracking with targets computed from *your* body
  (Mifflin-St Jeor BMR × activity + surplus; protein 1.2 g/lb, fat 0.35 g/lb,
  carbs fill the rest), live-updated as your logged weight changes. One-tap
  presets for the rice/chicken/steak/whey plan, custom entries, and +20 XP
  the moment the day's protein target is banked.
- **Train** — the 5-day split with per-exercise set logging, automatic PR
  detection (+30 XP), progressive-overload cues (all-time best, last session's
  top set, estimated 1RM), a per-exercise **rest timer**, and a
  beat-last-week volume target.
- **Cycle** — a visual timeline: active weeks → 30-day PCT → 8-week recovery →
  next-cycle eligibility, with real dates, live progress, blood-work
  checkpoints, and the non-negotiable rules.
- **Progress** — level + rank (Rookie → Living Legend), body-weight chart with
  least-squares trend line vs your lean-bulk band (0.25–0.5% BW/week),
  lifetime volume, and 13 unlockable achievements.
- **Reminders** — dose + wake-up, PM dose window, and wind-down notifications,
  scheduled as exact alarms (Doze-proof), phase-aware (Alpha-AF copy during
  PCT, no dose prompts off-cycle) and weekend-shifted. Survives reboots and
  timezone changes; toggleable in Settings.
- **Phase-aware** — on PCT day 1 the andro dose tasks disappear and
  Alpha-AF (3 caps with first meal) appears, automatically. Weekends shift
  the schedule later and swap training for active recovery.

Everything is stored locally (Room + DataStore). No account, no network, no
data collection.

## Tech stack

Kotlin · Jetpack Compose (Material 3) · Room (KSP) · DataStore ·
Navigation-Compose · AlarmManager reminders. Single-module app,
min SDK 26, target SDK 36.

```
app/src/main/kotlin/com/musclequest/app/
├── domain/        # Pure logic: CycleEngine, DailyPlanner, WorkoutPlan, Xp, Achievements
├── data/          # Room entities/DAOs, DataStore settings, Repository (XP ledger)
├── ui/            # MainViewModel + Compose screens (Today/Train/Cycle/Progress/Settings)
└── notifications/ # AlarmManager scheduler + receivers
```

The domain layer is pure JVM and covered by unit tests
(`./gradlew test`).

## Build & run

Requires JDK 17+ and the Android SDK (Android Studio installs both).

```bash
cd muscle-quest
./gradlew assembleDebug          # debug APK
./gradlew test                   # domain unit tests
./gradlew installDebug           # install to a connected device
```

> Verified: `assembleDebug` + all unit tests pass with AGP 8.13.2 /
> Gradle 8.13 / JDK 17, and the app has been smoke-tested end to end on an
> Android 16 (API 36) Pixel 7 emulator.

## Publishing to Google Play

> The complete human-side checklist (account setup, identity verification,
> the mandatory closed test, listing assets, timelines) lives in
> [PLAYSTORE.md](PLAYSTORE.md). The steps below are just the technical core.

1. **Create a signing key** (once, keep it safe):
   ```bash
   keytool -genkey -v -keystore musclequest-release.keystore \
     -alias musclequest -keyalg RSA -keysize 2048 -validity 10000
   ```
2. **Configure signing** — create `keystore.properties` (git-ignored):
   ```properties
   storeFile=/path/to/musclequest-release.keystore
   storePassword=...
   keyAlias=musclequest
   keyPassword=...
   ```
   and wire it into a `signingConfigs.release` block in `app/build.gradle.kts`.
3. **Build the bundle:** `./gradlew bundleRelease` → `app/build/outputs/bundle/release/app-release.aab`
4. **Play Console** (one-time $25 developer fee):
   - Create the app, upload the `.aab` to an internal testing track first.
   - **Data safety form:** the app collects nothing and has no network access —
     declare "No data collected or shared."
   - **Content rating questionnaire:** note that the app references dietary
     supplements; expect an adult/parental-guidance rating.
   - Add store listing (title, description, screenshots from a device or
     emulator, 512×512 icon, feature graphic) and a privacy policy URL.
   - **Closed test (mandatory for new personal accounts):** run a closed
     testing release with at least 12 testers opted in continuously for
     14 days, then apply for production access — internal testing alone
     does not unlock the production track. Organization accounts are exempt.
   - Promote to production once access is granted.
5. Bump `versionCode`/`versionName` in `app/build.gradle.kts` for each release.

## Health notice

This app schedules and tracks a hormonal supplement protocol the user has
chosen; it is a checklist, not medical advice. Baseline and post-cycle blood
work and a physician's sign-off are strongly recommended. If you fork this
for distribution, keep the in-app health notices intact.
