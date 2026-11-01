package com.ryderdavid.nacre.ui.common

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.Row
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.GoalOptions

@Composable
fun GoalOptionsPicker(value: GoalOptions, onChange: (GoalOptions) -> Unit, enabled: Boolean = true) {
    Column {
        GoalCheck("Use a goal to work until acceptance criteria are met and:", value.enabled, enabled) { onChange(value.copy(enabled = it)) }
        Column(Modifier.padding(start = 24.dp)) {
            GoalCheck("committed", value.committed, enabled && value.enabled) { onChange(value.withCommitted(it)) }
            GoalCheck("pushed", value.pushed, enabled && value.enabled) { onChange(value.withPushed(it)) }
        }
    }
}

@Composable
private fun GoalCheck(label: String, checked: Boolean, enabled: Boolean, onChange: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().toggleable(value = checked, enabled = enabled, role = Role.Checkbox, onValueChange = onChange).padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = null, enabled = enabled)
        Text(label, style = MaterialTheme.typography.bodyMedium, color = if (enabled) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 12.dp))
    }
}
