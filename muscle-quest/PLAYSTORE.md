# Getting MuscleQuest onto Google Play — the human checklist

Everything the code can't do for you. The technical build steps live in
[README.md](README.md#publishing-to-google-play); this file is the
people/paperwork side, in order. Budget: **$25 one-time** and roughly
**3–4 weeks** end to end (most of it is Google-mandated waiting).

## Phase 0 — Accounts & identity (Day 1, ~1 hour)

- [ ] **Google Play developer account** at [play.google.com/console/signup](https://play.google.com/console/signup)
      using your Google account. Choose **Personal** account type.
- [ ] Pay the **$25 one-time registration fee** (credit card).
- [ ] **Identity verification** — Google requires a government ID, a
      verified address, and phone/email verification for personal accounts.
      Your legal name (or a verified developer name) appears publicly on the
      listing. Allow up to a few days for verification to clear.

## Phase 1 — Things only you can create (Day 1–2, ~2 hours)

- [ ] **Upload keystore** — run the `keytool` command from the README, then
      create `keystore.properties`. Store the keystore file and both passwords
      in a password manager. Losing an upload key is recoverable via Play
      support; losing it *and* not enrolling in Play App Signing is not.
      (Enroll in **Play App Signing** when creating the app — it's the default
      and lets Google hold the final signing key.)
- [ ] **Privacy policy URL** — required for every app, even with zero data
      collection. A one-page policy hosted anywhere public works
      (GitHub Pages is fine): "MuscleQuest stores all data locally on your
      device, has no network access, and collects nothing."
- [ ] **Store listing copy** — app title (≤30 chars), short description
      (≤80 chars), full description (≤4,000 chars).
- [ ] **Graphics** —
      - 512×512 PNG app icon
      - 1024×500 feature graphic
      - At least 2 phone screenshots (take them from the emulator:
        `adb exec-out screencap -p > shot.png`)

## Phase 2 — Console setup (Day 2–3, ~2 hours of form-filling)

- [ ] Create the app in Play Console (name, default language, App/Free).
- [ ] Upload the `.aab` from `./gradlew bundleRelease` to a **closed testing**
      track.
- [ ] **Data safety form** — declare *no data collected, no data shared*
      (true for this app: Room + DataStore on-device only, no INTERNET
      permission).
- [ ] **Content rating questionnaire** (IARC) — answer honestly; the app
      references dietary/hormonal supplements, so expect a teen+ rating.
- [ ] **Declarations** — ads (none), target audience (adults; do NOT tick
      any child-directed audience), health/fitness category declarations if
      prompted. Keep the in-app health notice — the app *tracks a personal
      protocol*, it does not sell supplements or give medical advice, which
      keeps it clear of the dangerous-products policy.
- [ ] Select countries, set price (free).

## Phase 3 — The mandatory closed test (14+ days of waiting)

New **personal** developer accounts cannot publish straight to production:

- [ ] Recruit **at least 12 testers** (friends, gym buddies — they just need
      Google accounts) and add their emails to the closed-test list, or use a
      Google Group.
- [ ] Each tester opts in via your test link and **installs the app**.
- [ ] Testers must stay opted in **continuously for 14 days**.
- [ ] Use the time: watch for crashes in Play Console → Android Vitals, and
      actually live with the app through week 1–2 of your cycle.

## Phase 4 — Production (Day ~17+)

- [ ] Apply for **production access** in Play Console (short questionnaire
      about your testing).
- [ ] Once granted, promote the release to **Production**.
- [ ] First-time app review typically takes **up to 7 days** (often faster).
- [ ] After approval: the app is live. For every update, bump
      `versionCode`/`versionName`, rebuild the bundle, upload.

## Gotchas that bite first-time publishers

1. **Don't lose the keystore/passwords** — see Phase 1.
2. **Developer name & email are public** on the listing.
3. **Target API level** — Play requires new apps to target a recent API level;
   this app targets **API 36**, which satisfies the 2026 requirement.
4. **Inactivity** — Google may close dev accounts with no activity; publish or
   at least log in periodically.
5. **The 14-day test is continuous** — if testers drop below the threshold,
   the clock can reset. Over-recruit (15–20 people).
