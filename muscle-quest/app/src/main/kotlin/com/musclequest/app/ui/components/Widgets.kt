package com.musclequest.app.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.musclequest.app.domain.InsightTone
import com.musclequest.app.domain.TaskCategory
import com.musclequest.app.ui.theme.Ember
import com.musclequest.app.ui.theme.MQ

/** Animated circular progress ring with arbitrary center content. */
@Composable
fun ProgressRing(
    progress: Float,
    modifier: Modifier = Modifier,
    size: Dp = 72.dp,
    stroke: Dp = 8.dp,
    color: Color = MaterialTheme.colorScheme.primary,
    track: Color = MaterialTheme.colorScheme.surfaceVariant,
    content: @Composable () -> Unit = {},
) {
    val animated by animateFloatAsState(
        targetValue = progress.coerceIn(0f, 1f),
        animationSpec = tween(700),
        label = "ring",
    )
    Box(modifier.size(size), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(size)) {
            val strokePx = stroke.toPx()
            val arcSize = Size(this.size.width - strokePx, this.size.height - strokePx)
            val topLeft = Offset(strokePx / 2, strokePx / 2)
            drawArc(
                color = track,
                startAngle = -90f,
                sweepAngle = 360f,
                useCenter = false,
                topLeft = topLeft,
                size = arcSize,
                style = Stroke(strokePx, cap = StrokeCap.Round),
            )
            if (animated > 0f) {
                drawArc(
                    color = color,
                    startAngle = -90f,
                    sweepAngle = 360f * animated,
                    useCenter = false,
                    topLeft = topLeft,
                    size = arcSize,
                    style = Stroke(strokePx, cap = StrokeCap.Round),
                )
            }
        }
        content()
    }
}

/** Labeled macro bar: name left, "value / target unit" right, animated fill. */
@Composable
fun MacroBar(
    label: String,
    value: Int,
    target: Int,
    unit: String,
    color: Color,
    modifier: Modifier = Modifier,
) {
    val animated by animateFloatAsState(
        targetValue = if (target <= 0) 0f else (value.toFloat() / target).coerceIn(0f, 1f),
        animationSpec = tween(600),
        label = "macro",
    )
    Column(modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                label,
                style = MaterialTheme.typography.labelSmall,
                fontWeight = FontWeight.Bold,
                modifier = Modifier.weight(1f),
            )
            Text(
                "$value / $target $unit",
                style = MaterialTheme.typography.labelSmall,
                color = if (value >= target && target > 0) color else MaterialTheme.colorScheme.onSurfaceVariant,
                fontWeight = if (value >= target && target > 0) FontWeight.Bold else FontWeight.Medium,
            )
        }
        Spacer(Modifier.height(3.dp))
        Box(
            Modifier
                .fillMaxWidth()
                .height(8.dp)
                .background(MaterialTheme.colorScheme.surfaceVariant, CircleShape),
        ) {
            Box(
                Modifier
                    .fillMaxWidth(animated)
                    .height(8.dp)
                    .background(color, CircleShape),
            )
        }
    }
}

/** Consistent color coding for the five task categories across screens. */
@Composable
fun categoryColor(category: TaskCategory): Color = when (category) {
    TaskCategory.DOSE -> MQ.dose
    TaskCategory.TRAINING -> MaterialTheme.colorScheme.primary
    TaskCategory.NUTRITION -> MQ.nutrition
    TaskCategory.HYDRATION -> MQ.hydration
    TaskCategory.RECOVERY -> MQ.recovery
}

@Composable
fun toneColor(tone: InsightTone): Color = when (tone) {
    InsightTone.PUSH -> MaterialTheme.colorScheme.primary
    InsightTone.WARN -> Ember
    InsightTone.INFO -> MQ.hydration
}
