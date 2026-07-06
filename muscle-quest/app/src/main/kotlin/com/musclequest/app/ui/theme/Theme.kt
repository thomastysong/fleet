package com.musclequest.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.sp

// Electric-gym palette: charcoal iron, volt green, ember orange.
val Volt = Color(0xFFB6F34C)
val VoltDark = Color(0xFF7CB332)
val Ember = Color(0xFFFF8A3D)
val Iron900 = Color(0xFF15171B)
val Iron800 = Color(0xFF1E2126)
val Iron700 = Color(0xFF2A2E35)
val Iron100 = Color(0xFFE8EAED)
val Danger = Color(0xFFEF5350)

private val DarkColors = darkColorScheme(
    primary = Volt,
    onPrimary = Color(0xFF1A2400),
    secondary = Ember,
    onSecondary = Color(0xFF2B1400),
    background = Iron900,
    onBackground = Iron100,
    surface = Iron800,
    onSurface = Iron100,
    surfaceVariant = Iron700,
    onSurfaceVariant = Color(0xFFB9BEC7),
    error = Danger,
)

private val LightColors = lightColorScheme(
    primary = VoltDark,
    onPrimary = Color.White,
    secondary = Ember,
    onSecondary = Color.White,
    background = Color(0xFFF7F8F5),
    onBackground = Color(0xFF191C16),
    surface = Color.White,
    onSurface = Color(0xFF191C16),
)

private val AppTypography = Typography(
    headlineMedium = TextStyle(fontWeight = FontWeight.Black, fontSize = 28.sp, letterSpacing = (-0.5).sp),
    titleLarge = TextStyle(fontWeight = FontWeight.Bold, fontSize = 20.sp),
    titleMedium = TextStyle(fontWeight = FontWeight.Bold, fontSize = 16.sp),
    bodyMedium = TextStyle(fontSize = 14.sp, lineHeight = 20.sp),
    labelSmall = TextStyle(fontSize = 11.sp, fontWeight = FontWeight.Medium, letterSpacing = 0.5.sp),
)

@Composable
fun MuscleQuestTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        typography = AppTypography,
        content = content,
    )
}
