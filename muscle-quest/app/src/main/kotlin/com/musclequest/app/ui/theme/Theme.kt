package com.musclequest.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Electric-gym palette: charcoal iron, volt green, ember orange.
val Volt = Color(0xFFB6F34C)
val VoltDark = Color(0xFF6FA82B)
val Ember = Color(0xFFFF8A3D)
val Iron900 = Color(0xFF15171B)
val Iron800 = Color(0xFF1E2126)
val Iron700 = Color(0xFF2A2E35)
val Iron100 = Color(0xFFE8EAED)
val Danger = Color(0xFFEF5350)

/** Category + accent colors that read on both light and dark surfaces. */
object MQ {
    val dose = Color(0xFF9E7BFF)
    val nutrition = Color(0xFFE8A33D)
    val hydration = Color(0xFF42A5F5)
    val recovery = Color(0xFF7986CB)
    val gold = Color(0xFFF2C94C)
}

private val DarkColors = darkColorScheme(
    primary = Volt,
    onPrimary = Color(0xFF1A2400),
    primaryContainer = Color(0xFF3A4D14),
    onPrimaryContainer = Volt,
    secondary = Ember,
    onSecondary = Color(0xFF2B1400),
    secondaryContainer = Color(0xFF4A2A10),
    onSecondaryContainer = Color(0xFFFFC59E),
    tertiary = MQ.hydration,
    onTertiary = Color(0xFF00263A),
    background = Iron900,
    onBackground = Iron100,
    surface = Iron800,
    onSurface = Iron100,
    surfaceVariant = Iron700,
    onSurfaceVariant = Color(0xFFB9BEC7),
    outlineVariant = Color(0xFF3A3F47),
    error = Danger,
)

private val LightColors = lightColorScheme(
    primary = VoltDark,
    onPrimary = Color.White,
    primaryContainer = Color(0xFFE4F5C8),
    onPrimaryContainer = Color(0xFF2A3D08),
    secondary = Ember,
    onSecondary = Color.White,
    secondaryContainer = Color(0xFFFFE0CC),
    onSecondaryContainer = Color(0xFF5A2E0A),
    tertiary = Color(0xFF1E88E5),
    onTertiary = Color.White,
    background = Color(0xFFF7F8F5),
    onBackground = Color(0xFF191C16),
    surface = Color.White,
    onSurface = Color(0xFF191C16),
    surfaceVariant = Color(0xFFEDEFE8),
    onSurfaceVariant = Color(0xFF5B6055),
    outlineVariant = Color(0xFFDCDFD6),
)

private val AppTypography = Typography(
    displaySmall = TextStyle(fontWeight = FontWeight.Black, fontSize = 38.sp, letterSpacing = (-1).sp),
    headlineMedium = TextStyle(fontWeight = FontWeight.Black, fontSize = 28.sp, letterSpacing = (-0.5).sp),
    headlineSmall = TextStyle(fontWeight = FontWeight.ExtraBold, fontSize = 22.sp, letterSpacing = (-0.3).sp),
    titleLarge = TextStyle(fontWeight = FontWeight.Bold, fontSize = 20.sp),
    titleMedium = TextStyle(fontWeight = FontWeight.Bold, fontSize = 16.sp),
    titleSmall = TextStyle(fontWeight = FontWeight.Bold, fontSize = 14.sp),
    bodyMedium = TextStyle(fontSize = 14.sp, lineHeight = 20.sp),
    bodySmall = TextStyle(fontSize = 12.sp, lineHeight = 17.sp),
    labelMedium = TextStyle(fontSize = 12.sp, fontWeight = FontWeight.SemiBold, letterSpacing = 0.3.sp),
    labelSmall = TextStyle(fontSize = 11.sp, fontWeight = FontWeight.Medium, letterSpacing = 0.5.sp),
)

private val AppShapes = Shapes(
    small = RoundedCornerShape(10.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(22.dp),
)

@Composable
fun MuscleQuestTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        typography = AppTypography,
        shapes = AppShapes,
        content = content,
    )
}
