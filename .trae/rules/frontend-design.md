# 前端设计规范

## 任务卡片错误展示规范（重要！）

### 参考实现：Tasks.vue

任务卡片（加密/解密任务）的错误展示遵循以下规范：

#### 1. 错误信息分层展示

```vue
<!-- 密码错误特殊处理 -->
<p v-if="isPasswordError(task)" class="task-error password-error">
  <ion-icon :icon="lockClosed"></ion-icon>
  {{ t('tasks.passwordErrorHint') }}
</p>

<!-- 普通错误 -->
<p v-else-if="task.error" class="task-error">{{ task.error }}</p>

<!-- 错误详情（可展开） -->
<div v-if="task.errorDetail && task.errorDetail !== task.error" class="error-detail-row">
  <p class="task-error-detail" @click="toggleErrorDetail(task.id)">
    {{ showErrorDetail[task.id] ? t('tasks.hideDetail') : t('tasks.showDetail') }}
  </p>
  <ion-button fill="clear" size="small" color="medium" @click="copyErrorDetail(task)">
    <ion-icon :icon="copiedTaskId === task.id ? checkmarkCircle : copyOutline" slot="icon-only"></ion-icon>
  </ion-button>
</div>
<pre v-if="showErrorDetail[task.id] && task.errorDetail" class="error-detail-pre">{{ task.errorDetail }}</pre>
```

#### 2. 警告信息展示

```vue
<!-- 警告行（可点击展开） -->
<div v-if="task.warning" class="task-warning" @click="toggleWarningDetail(task)">
  <ion-icon :icon="warningOutline" class="warning-icon"></ion-icon>
  <span class="task-warning-text">{{ task.warning }}</span>
</div>
<div v-if="expandedWarningDetail === task.id && task.warningDetail" class="task-warning-detail">
  <pre>{{ formatWarningDetail(task.warningDetail) }}</pre>
</div>
```

#### 3. 状态徽章

```vue
<ion-badge :color="getStatusColor(task.status)" class="status-badge">
  {{ getStatusLabel(task.status) }}
</ion-badge>
```

#### 4. 进度展示

```vue
<!-- 运行中进度 -->
<div v-if="task.status === 'running' || task.status === 'cancelling'" class="progress-section">
  <ion-progress-bar :value="task.progress / 100"></ion-progress-bar>
  <div class="progress-detail">
    <span v-if="task.phase" class="phase-label">{{ getPhaseLabel(task.phase) }}</span>
    <span class="progress-percent">{{ task.progress }}%</span>
    <span v-if="task.speed" class="speed-label">{{ task.speed }}</span>
    <span v-if="task.eta" class="eta-label">{{ t('tasks.eta') }} {{ task.eta }}</span>
  </div>
</div>

<!-- 完成信息 -->
<div v-if="task.status === 'completed'" class="completed-info">
  <ion-icon :icon="checkmarkCircle" color="success" class="completed-icon"></ion-icon>
  <span class="completed-text">{{ t('tasks.phaseCompleted') }}</span>
</div>
```

### 设计原则

1. **错误信息可见**：错误直接显示在卡片内，不隐藏
2. **错误详情可展开**：点击展开完整错误堆栈，不默认展开（避免卡片过长）
3. **错误详情可复制**：提供复制按钮，方便用户反馈问题
4. **特殊错误特殊处理**：密码错误显示特殊提示（`password-error` 样式）
5. **警告与错误分离**：警告（warning）是任务完成但有问题的提示，错误（error）是任务失败
6. **状态徽章颜色语义**：queued=medium, running=primary, completed=success, failed=danger

### CSS 样式规范

```css
/* 错误信息 */
.task-error {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
}

/* 密码错误特殊样式 */
.password-error {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  padding: 6px 10px;
  border-radius: 6px;
  border-left: 3px solid var(--ion-color-danger);
}

/* 错误详情展开按钮 */
.task-error-detail {
  color: var(--ion-color-medium);
  font-size: 11px;
  cursor: pointer;
}

/* 错误详情内容 */
.error-detail-pre {
  background: var(--ion-color-light);
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}

/* 警告信息 */
.task-warning {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  background: rgba(255, 152, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  color: #e65100;
}
```

---

## 选项项状态徽章规范

### ion-select-option 状态展示

**正确做法**：在 `ion-item` 末尾使用 `ion-badge`

```vue
<ion-item>
  <ion-select>
    <ion-select-option value="mpv-plugin" :disabled="status !== 'ready'">MPV 播放器</ion-select-option>
  </ion-select>
  <ion-badge v-if="status !== 'ready'" slot="end" color="warning">未安装</ion-badge>
  <ion-badge v-if="status === 'ready'" slot="end" color="success">✓</ion-badge>
</ion-item>
```

**错误做法**：在 `ion-select-option` 内部使用 `<span>`（action-sheet 模式下不显示）

---

## 铁律关联

1. **严禁 Toast 提示**：错误信息通过卡片内持久性 UI 元素显示
2. **严禁自动 fallback**：任务失败后显示错误，用户主动选择重试或放弃
3. **饱和调试**：错误信息完整展示，可展开详情，可复制