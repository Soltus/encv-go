<template>
  <ion-page class="world-page">
    <ion-content :fullscreen="true" class="world-content">
      <div class="game-container">
        <div class="world-map">
          <div ref="phaserContainerRef" class="phaser-container"></div>

          <template v-if="!usePhaser || phaserHasError">
            <div class="map-grid">
              <div v-for="i in 48" :key="i" class="map-cell" :class="getCellClass(i)">
                <div class="cell-icon">{{ getCellIcon(i) }}</div>
              </div>
            </div>
            <div class="map-overlay">
              <div v-for="(npc, idx) in visibleNPCs" :key="npc.id"
                   class="npc-marker"
                   :style="getNPCPosition(idx)"
                   @click="selectNPC(npc)">
                <div class="npc-dot" :class="[npc.is_alive ? 'alive' : 'dead', getBehaviorClass(npc.current_behavior || 'idle')]"></div>
                <div class="npc-name">{{ npc.name }}</div>
              </div>
            </div>
          </template>

          <div v-if="phaserLoading && usePhaser && !phaserHasError" class="phaser-loading">
            <div class="loading-spinner"></div>
            <span class="loading-text">加载世界中...</span>
          </div>
        </div>

        <div class="top-bar">
          <div class="resource-group">
            <div class="resource-item game-resource" v-tooltip="t('simverse.diamond')">
              <span class="resource-icon">💎</span>
              <span class="resource-value" :key="playerDiamond">{{ playerDiamond }}</span>
            </div>
            <div class="resource-item game-resource" v-tooltip="t('simverse.gold')">
              <span class="resource-icon">🪙</span>
              <span class="resource-value" :key="playerGold">{{ playerGold }}</span>
            </div>
            <div class="resource-item game-resource" v-tooltip="t('simverse.stamina')">
              <span class="resource-icon">⚡</span>
              <span class="resource-value" :key="playerStamina">{{ playerStamina }}/120</span>
            </div>
            <div class="res-divider"></div>
            <div class="resource-item" v-tooltip="t('simverse.tick')">
              <span class="resource-icon">⏱️</span>
              <span class="resource-value">{{ worldState?.tick ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.population')">
              <span class="resource-icon">👥</span>
              <span class="resource-value">{{ worldState?.npc_count ?? 0 }}</span>
            </div>
          </div>
          <div class="top-actions">
            <button
              class="btn btn-circle w-[38px] h-[38px] p-0 backdrop-blur-md bg-[rgba(20,20,40,0.8)] border border-[rgba(139,92,246,0.25)] border-b-[3px] border-b-[rgba(139,92,246,0.35)] text-white text-sm leading-none hover:bg-[rgba(139,92,246,0.2)] hover:-translate-y-[1px] active:translate-y-[2px] transition-all duration-150 ease-[cubic-bezier(0.4,0,0.2,1)] play-btn"
              :class="{ running: worldState?.running }"
              @click="toggleRunning"
            >
              {{ worldState?.running ? "⏸" : "▶" }}
            </button>
            <button
              class="btn btn-circle w-[38px] h-[38px] p-0 backdrop-blur-md bg-[rgba(20,20,40,0.8)] border border-[rgba(139,92,246,0.25)] border-b-[3px] border-b-[rgba(139,92,246,0.35)] text-white text-sm leading-none hover:bg-[rgba(139,92,246,0.2)] hover:-translate-y-[1px] active:translate-y-[2px] transition-all duration-150 ease-[cubic-bezier(0.4,0,0.2,1)]"
              @click="stepOnce"
            >
              ⏭
            </button>
          </div>
        </div>

        <!-- 主世界底部主操作条：屏幕状态机 world 页专属导航，替代原先散落在两侧的浮动按钮 -->
        <!-- 转场已迁移至 GSAP（bottomBarRef watcher），不再使用 <transition> 包裹 -->
        <div v-if="screen === 'world'" ref="bottomBarRef" class="bottom-bar">
          <button class="bar-btn" :class="{ active: activePanel === 'npc' }" @click="openHudScene('focus')">
            <span class="bar-icon">🔍</span>
            <span class="bar-label">{{ t("simverse.focus") }}</span>
          </button>
          <button class="bar-btn" :class="{ active: activePanel === 'chronicles' }" @click="openDetailRoute('chronicles')">
            <span class="bar-icon">📖</span>
            <span class="bar-label">{{ t("simverse.chronicles") }}</span>
          </button>
          <button class="bar-btn" :class="{ active: activePanel === 'economy' }" @click="openDetailRoute('economy')">
            <span class="bar-icon">💰</span>
            <span class="bar-label">{{ t("simverse.economy") }}</span>
          </button>
          <button class="bar-btn" :class="{ active: isIntervene }" @click="openHudScene('intervene')">
            <span class="bar-icon">🎛️</span>
            <span class="bar-label">{{ t("simverse.intervene") }}</span>
          </button>
          <button class="bar-btn more" :class="{ active: bottomMoreOpen }" @click="toggleBottomMore">
            <span class="bar-icon">⋯</span>
            <span class="bar-label">{{ t("simverse.more") }}</span>
          </button>
          <button class="bar-btn" :class="{ active: activePanel === 'settings' }" @click="openDetailRoute('settings')">
            <span class="bar-icon">⚙️</span>
            <span class="bar-label">{{ t("simverse.settings") }}</span>
          </button>
        </div>

        <!-- 更多：二级功能（含 B 类体验桩）收拢于此，避免主操作条过载 -->
        <!-- 转场已迁移至 GSAP（morePopRef watcher），不再使用 <transition> 包裹 -->
        <div v-if="screen === 'world' && bottomMoreOpen" ref="morePopRef" class="more-pop" @click.self="toggleBottomMore">
          <div class="more-grid">
            <button class="more-item" @click="openDetailRoute('quest'); bottomMoreOpen = false">
              <span class="more-icon">📋</span><span class="more-label">任务</span>
            </button>
            <button class="more-item" @click="openDetailRoute('org'); bottomMoreOpen = false">
              <span class="more-icon">🏰</span><span class="more-label">{{ t("simverse.org") }}</span>
            </button>
            <button class="more-item" @click="openHudScene('character'); bottomMoreOpen = false">
              <span class="more-icon">🧑</span><span class="more-label">{{ t("simverse.character") }}</span>
            </button>
            <button class="more-item" @click="openDetailRoute('profile'); bottomMoreOpen = false">
              <span class="more-icon">👤</span><span class="more-label">{{ t("simverse.profile") }}</span>
            </button>
            <button class="more-item" @click="openDetailRoute('training'); bottomMoreOpen = false">
              <span class="more-icon">⚔️</span><span class="more-label">{{ t("simverse.training") }}</span>
            </button>
            <button class="more-item" @click="openDetailRoute('inventory'); bottomMoreOpen = false">
              <span class="more-icon">🎒</span><span class="more-label">背包</span>
            </button>
            <button class="more-item" @click="openDetailRoute('explore'); bottomMoreOpen = false">
              <span class="more-icon">🗺️</span><span class="more-label">{{ t("simverse.explore") }}</span>
            </button>
            <button class="more-item" @click="openDetailRoute('battle'); bottomMoreOpen = false">
              <span class="more-icon">🗡️</span><span class="more-label">{{ t("simverse.battle") }}</span>
            </button>
            <button class="more-item gacha" @click="openHudScene('gacha'); bottomMoreOpen = false">
              <span class="more-icon">✨</span><span class="more-label">{{ t("simverse.gacha") }}</span>
            </button>
          </div>
        </div>

        <div class="stats-bar">
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.perfTier") }}</span>
            <span class="stat-value">{{ worldState?.tier || "-" }}</span>
          </div>
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.focusNPCs") }}</span>
            <span class="stat-value">{{ worldState?.focus_count ?? 0 }}</span>
          </div>
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.cellCount") }}</span>
            <span class="stat-value">{{ worldState?.cell_count ?? 0 }}</span>
          </div>
        </div>

        <div class="event-ticker" v-if="recentEvents.length > 0" @click="openEventPage">
          <div class="ticker-icon">📜</div>
          <div class="ticker-content">
            <!-- 转场已迁移至 GSAP（recentEvents watcher），不再使用 <transition-group> -->
            <div class="ticker-list">
              <div v-for="ev in recentEvents.slice(0, 3)" :key="ev.id" class="ticker-item">
                <span class="event-dot" :class="'imp-' + ev.importance"></span>
                <span class="event-text">{{ ev.type_cn }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-if="activePanel" class="side-panel" :class="{ open: !!activePanel, 'panel-left': panelOnLeft }">
          <div class="panel-header">
            <span class="panel-title">{{ getPanelTitle(activePanel) }}</span>
            <button
              class="btn btn-circle btn-xs w-[28px] h-[28px] p-0 min-h-0 bg-white/[0.08] border-none text-white/60 hover:bg-white/[0.15] hover:text-white text-[11px] transition-all duration-200"
              @click="activePanel = null"
            >✕</button>
          </div>
          <div class="panel-content">
            <template v-if="activePanel === 'quest'">
              <div class="quest-panel">
                <div class="flex gap-1.5 p-3 bg-black/20 border-b border-white/10">
                  <button
                    class="flex-1 py-2 px-3 border-none rounded-lg text-white/60 text-[13px] font-medium cursor-pointer transition-all duration-200"
                    :class="questTab === 'daily' ? 'bg-gradient-to-br from-[var(--color-primary)] to-[#6366f1] text-white' : 'bg-white/[0.05]'"
                    @click="questTab = 'daily'"
                  >
                    📅 日常
                  </button>
                  <button
                    class="flex-1 py-2 px-3 border-none rounded-lg text-white/60 text-[13px] font-medium cursor-pointer transition-all duration-200"
                    :class="questTab === 'achieve' ? 'bg-gradient-to-br from-[var(--color-primary)] to-[#6366f1] text-white' : 'bg-white/[0.05]'"
                    @click="questTab = 'achieve'"
                  >
                    🏆 成就
                  </button>
                  <button
                    class="flex-1 py-2 px-3 border-none rounded-lg text-white/60 text-[13px] font-medium cursor-pointer transition-all duration-200"
                    :class="questTab === 'story' ? 'bg-gradient-to-br from-[var(--color-primary)] to-[#6366f1] text-white' : 'bg-white/[0.05]'"
                    @click="questTab = 'story'"
                  >
                    📖 剧情
                  </button>
                </div>
                <div class="quest-list">
                  <div v-for="q in filteredQuests" :key="q.id" class="quest-card"
                       :class="{ locked: q.status === 0, completable: q.progress >= q.goal && q.status === 1, claimed: q.status === 2 }">
                    <div class="quest-icon">{{ q.status === 0 ? '🔒' : q.icon }}</div>
                    <div class="quest-info">
                      <div class="quest-title">{{ q.title }}</div>
                      <div class="quest-desc">{{ q.desc }}</div>
                      <div class="flex items-center gap-2 mb-1.5">
                        <div class="flex-1 h-1.5 bg-white/10 rounded-sm overflow-hidden">
                          <div
                            class="h-full rounded-sm transition-[width] duration-300"
                            :class="q.progress >= q.goal && q.status === 1
                              ? 'bg-gradient-to-r from-[var(--color-success)] to-[#4ade80]'
                              : 'bg-gradient-to-r from-[var(--color-primary)] to-[#06b6d4]'"
                            :style="{ width: Math.min(100, (q.progress / q.goal) * 100) + '%' }"
                          ></div>
                        </div>
                        <span class="text-[11px] text-white/50 whitespace-nowrap">{{ q.progress }}/{{ q.goal }}</span>
                      </div>
                      <div class="quest-reward">
                        <span v-if="q.reward.diamond" class="reward-item">💎 {{ q.reward.diamond }}</span>
                        <span v-if="q.reward.gold" class="reward-item">🪙 {{ q.reward.gold }}</span>
                        <span v-if="q.reward.exp" class="reward-item">⭐ {{ q.reward.exp }}</span>
                      </div>
                    </div>
                    <button
                      v-if="q.progress >= q.goal && q.status === 1"
                      class="btn btn-primary btn-sm bg-gradient-to-br from-[var(--color-success)] to-[#16a34a] border-none text-white rounded-lg px-4 py-2 text-[13px] font-semibold flex-shrink-0 active:scale-95 transition-all duration-200"
                      @click="claimQuest(q.id)"
                    >
                      领取
                    </button>
                    <span v-else-if="q.status === 2" class="quest-claimed">✓</span>
                  </div>
                </div>
              </div>
            </template>

            <template v-if="activePanel === 'npc'">
              <div v-if="behaviorStats" class="behavior-stats-bar">
                <div class="stats-title">活动分布</div>
                <div class="behavior-chips">
                  <span v-for="(count, behavior) in behaviorStats.behavior_dist" :key="behavior"
                        class="badge badge-sm behavior-chip inline-flex items-center gap-1 bg-white/[0.05] border border-white/10 rounded-xl px-2 py-0.5 text-[10px]"
                        :class="behavior">
                    <span class="text-xs">{{ getBehaviorIcon(behavior as string) }}</span>
                    <span class="font-semibold text-white/80">{{ count }}</span>
                  </span>
                </div>
              </div>
              <div
                v-for="npc in npcList"
                :key="npc.id"
                class="card flex items-center gap-3 p-3 mb-2 bg-white/[0.04] rounded-xl border border-[rgba(139,92,246,0.1)] cursor-pointer transition-all duration-200 hover:bg-[rgba(139,92,246,0.1)] hover:border-[rgba(139,92,246,0.25)] hover:translate-x-0.5"
                @click="selectNPC(npc)"
              >
                <div class="w-10 h-10 rounded-full bg-gradient-to-br from-[var(--color-primary)] to-[#ec4899] flex items-center justify-center text-white font-bold text-base flex-shrink-0 border-2 border-white/10">{{ npc.name?.[0] || '?' }}</div>
                <div class="flex-1 min-w-0">
                  <div class="text-[13px] font-semibold text-white mb-1">{{ npc.name }}</div>
                  <div class="flex gap-1.5 items-center">
                    <span class="badge badge-sm bg-[rgba(139,92,246,0.2)] text-[#c4b5fd] text-[10px] px-1.5 py-0.5 rounded-md font-medium capitalize">{{ npc.profession }}</span>
                    <span class="badge badge-sm bg-[rgba(245,158,11,0.2)] text-[#fbbf24] text-[10px] px-1.5 py-0.5 rounded-md font-medium">Lv.{{ npc.level }}</span>
                  </div>
                  <div class="flex items-center gap-1 mt-1">
                    <span class="text-[11px]">{{ getBehaviorIcon(npc.current_behavior || 'idle') }}</span>
                    <span class="text-[10px] text-white/50">{{ npc.current_behavior_cn || '空闲' }}</span>
                  </div>
                </div>
                <div class="w-[50px] flex-shrink-0">
                  <div class="mini-hp-bar">
                    <div class="mini-hp-fill" :style="{ width: (npc.health / npc.max_health * 100) + '%' }"></div>
                  </div>
                </div>
              </div>
              <div v-if="npcList.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
              <ion-infinite-scroll v-if="hasMoreNPCs" @ionInfinite="loadMoreNPCs" threshold="100px">
                <ion-infinite-scroll-content></ion-infinite-scroll-content>
              </ion-infinite-scroll>
            </template>

            <template v-else-if="activePanel === 'chronicles'">
              <div v-for="ev in recentEvents" :key="ev.id" class="chronicle-card">
                <div class="chronicle-tick">Tick {{ ev.tick }}</div>
                <div class="chronicle-title">{{ ev.type_cn }}</div>
                <div class="chronicle-desc">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
              </div>
              <div v-if="recentEvents.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
            </template>

            <template v-else-if="activePanel === 'profile'">
              <div class="profile-card-panel">
                <div class="player-card-mini">
                  <div class="card-bg-mini"></div>
                  <div class="card-content-mini">
                    <div class="player-avatar-big">
                      <span class="avatar-icon-big">⚔️</span>
                    </div>
                    <div class="player-name">冒险者</div>
                    <div class="player-title">Lv.{{ playerLevel }} 勇者</div>
                  </div>
                </div>

                <div class="panel-section">
                  <div class="panel-section-title">{{ t("simverse.stats") }}</div>
                  <div class="stats-grid-compact">
                    <div class="stat-item-compact">
                      <span class="stat-label-comp">{{ t("simverse.hp") }}</span>
                      <span class="stat-val-comp">{{ playerHp }}/{{ playerMaxHp }}</span>
                      <div class="stat-bar-comp">
                        <div class="stat-fill-comp hp" :style="{ width: playerHpPercent + '%' }"></div>
                      </div>
                    </div>
                    <div class="stat-item-compact">
                      <span class="stat-label-comp">{{ t("simverse.attack") }}</span>
                      <span class="stat-val-comp">{{ playerAttack }}</span>
                    </div>
                    <div class="stat-item-compact">
                      <span class="stat-label-comp">{{ t("simverse.defense") }}</span>
                      <span class="stat-val-comp">{{ playerDefense }}</span>
                    </div>
                    <div class="stat-item-compact">
                      <span class="stat-label-comp">{{ t("simverse.exp") }}</span>
                      <span class="stat-val-comp">{{ playerExp }}/{{ expToNextLevel }}</span>
                      <div class="stat-bar-comp">
                        <div class="stat-fill-comp exp" :style="{ width: expPercent + '%' }"></div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="panel-section">
                  <div class="panel-section-title">{{ t("simverse.skills") }}</div>
                  <div class="skill-list-compact">
                    <div v-for="skill in playerSkills.slice(0, 4)" :key="skill.id" class="skill-item-comp">
                      <div class="skill-icon-comp">{{ skill.icon }}</div>
                      <div class="skill-info-comp">
                        <div class="skill-name-comp">{{ skill.name }}</div>
                        <div class="skill-level-comp">Lv.{{ skill.level }}</div>
                      </div>
                      <div class="skill-rarity-comp" :class="skill.rarity">{{ skill.rarity }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'training'">
              <div class="training-panel">
                <div class="training-stamina-bar">
                  <span class="stamina-icon">⚡</span>
                  <div class="stamina-bar-wrap">
                    <div class="stamina-bar-fill" :style="{ width: (playerStamina / 120 * 100) + '%' }"></div>
                  </div>
                  <span class="stamina-text">{{ playerStamina }}/120</span>
                </div>

                <div class="panel-section">
                  <div class="panel-section-title">{{ t("simverse.train") }}</div>
                  <div class="training-grid-small">
                    <div v-for="mode in trainingModes" :key="mode.id"
                         class="training-card-small"
                         :class="mode.type"
                         @click="doTraining(mode)">
                      <div class="training-icon-small">{{ mode.icon }}</div>
                      <div class="training-name-small">{{ mode.name }}</div>
                      <div class="training-cost-small">⚡{{ mode.staminaCost }}</div>
                    </div>
                  </div>
                </div>

                <div class="panel-section">
                  <div class="panel-section-title">{{ t("simverse.equipment") }}</div>
                  <div class="equipment-grid-small">
                    <div v-for="equip in equipmentSlots" :key="equip.slot" class="equip-slot-small">
                      <div class="equip-item-small" :class="{ empty: !equip.item }">
                        <span v-if="equip.item" class="equip-icon-small">{{ equip.item.icon }}</span>
                        <span v-else class="equip-empty-small">+</span>
                      </div>
                      <div class="equip-label-small">{{ equip.label }}</div>
                    </div>
                  </div>
                </div>

                <div class="panel-section">
                  <div class="panel-section-title">角色等级</div>
                  <div class="level-up-compact">
                    <div class="level-info-comp">
                      <span class="lv-text">Lv.{{ playerLevel }}</span>
                      <div class="exp-bar-small">
                        <div class="exp-fill-small" :style="{ width: expPercent + '%' }"></div>
                      </div>
                      <span class="exp-text-small">{{ playerExp }}/{{ expToNextLevel }}</span>
                    </div>
                    <button class="level-up-btn-small" :disabled="playerExp < expToNextLevel" @click="levelUp">
                      {{ t("simverse.upgrade") }}
                    </button>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'inventory'">
              <div class="inventory-panel">
                <div class="panel-section-title">背包 ({{ inventoryItems.length }}/50)</div>
                <div class="inventory-grid">
                  <div v-for="i in 20" :key="i" class="inv-slot" :class="{ filled: inventoryItems[i-1] }">
                    <span v-if="inventoryItems[i-1]" class="inv-icon">{{ inventoryItems[i-1].icon }}</span>
                    <span v-else class="inv-empty"></span>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'economy'">
              <div class="economy-panel">
                <div class="economy-tabs">
                  <button class="econ-tab" :class="{ active: econTab === 'prices' }" @click="econTab = 'prices'">
                    📊 物价行情
                  </button>
                  <button class="econ-tab" :class="{ active: econTab === 'ranking' }" @click="econTab = 'ranking'">
                    🏆 财富榜
                  </button>
                </div>

                <div v-if="econTab === 'prices'" class="prices-section">
                  <div class="econ-header">
                    <span class="econ-title">区域物价</span>
                    <span class="econ-region">区域 #1</span>
                  </div>
                  <div v-if="economyStats" class="price-list">
                    <div v-for="(price, name) in economyStats.prices" :key="name" class="price-item">
                      <div class="price-icon">{{ getResourceIcon(name as string) }}</div>
                      <div class="price-info">
                        <div class="price-name">{{ getResourceName(name as string) }}</div>
                        <div class="price-bar-row">
                          <div class="supply-bar">
                            <div class="supply-fill" :style="{ width: getSupplyPercent(name as string) + '%' }"></div>
                          </div>
                          <div class="demand-bar">
                            <div class="demand-fill" :style="{ width: getDemandPercent(name as string) + '%' }"></div>
                          </div>
                        </div>
                      </div>
                      <div class="price-value">
                        <span class="price-num">{{ price.toFixed(1) }}</span>
                        <span class="price-trend" :class="getPriceTrendClass(name as string)">
                          {{ getPriceTrend(name as string) }}
                        </span>
                      </div>
                    </div>
                  </div>
                  <div v-else class="loading-placeholder">
                    <div class="loading-spinner"></div>
                    <span>加载经济数据中...</span>
                  </div>
                </div>

                <div v-else-if="econTab === 'ranking'" class="ranking-section">
                  <div class="econ-header">
                    <span class="econ-title">NPC 财富榜</span>
                    <span class="econ-count">Top {{ wealthRankings.length }}</span>
                  </div>
                  <div class="ranking-list">
                    <div v-for="(npc, index) in wealthRankings" :key="npc.id" class="rank-item" :class="'rank-' + (index + 1)">
                      <div class="rank-badge">
                        <span v-if="index === 0">🥇</span>
                        <span v-else-if="index === 1">🥈</span>
                        <span v-else-if="index === 2">🥉</span>
                        <span v-else>{{ index + 1 }}</span>
                      </div>
                      <div class="rank-avatar">{{ npc.name?.[0] || '?' }}</div>
                      <div class="rank-info">
                        <div class="rank-name">{{ npc.name }}</div>
                        <div class="rank-prof">{{ npc.profession }} · Lv.{{ npc.level }}</div>
                      </div>
                      <div class="rank-wealth">
                        <span class="wealth-icon">💰</span>
                        <span class="wealth-num">{{ npc.wealth.toLocaleString() }}</span>
                      </div>
                    </div>
                  </div>
                  <div v-if="wealthRankings.length === 0" class="loading-placeholder">
                    <div class="loading-spinner"></div>
                    <span>加载排行榜中...</span>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'settings'">
              <div class="setting-section">
                <div class="section-title">{{ t("simverse.graphics") }}</div>
                <div class="option-block">
                  <div class="option-label">{{ t("simverse.frameRate") }}</div>
                  <ion-segment :value="String(fps)" @ionChange="onFpsChange" scrollable>
                    <ion-segment-button v-for="opt in FPS_OPTIONS" :key="opt" :value="String(opt)">
                      <ion-label>{{ opt }}</ion-label>
                    </ion-segment-button>
                  </ion-segment>
                </div>
                <div class="option-block">
                  <div class="option-label">{{ t("simverse.renderQuality") }}</div>
                  <ion-segment :value="quality" @ionChange="onQualityChange">
                    <ion-segment-button v-for="opt in QUALITY_OPTIONS" :key="opt.value" :value="opt.value">
                      <ion-label>{{ opt.label }}</ion-label>
                    </ion-segment-button>
                  </ion-segment>
                  <p class="option-hint">
                    {{ QUALITY_RESOLUTION[quality].width }} × {{ QUALITY_RESOLUTION[quality].height }}
                  </p>
                </div>
              </div>
              <div class="setting-section">
                <div class="section-title">{{ t("simverse.performanceTier") }}</div>
                <div class="tier-selector">
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'background' }"
                          @click="changeTier('background')">
                    <span class="tier-name">{{ t("simverse.tierBackground") }}</span>
                    <span class="tier-desc">低功耗</span>
                  </button>
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'foreground' }"
                          @click="changeTier('foreground')">
                    <span class="tier-name">{{ t("simverse.tierForeground") }}</span>
                    <span class="tier-desc">标准</span>
                  </button>
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'fg_idle' }"
                          @click="changeTier('fg_idle')">
                    <span class="tier-name">{{ t("simverse.tierIdle") }}</span>
                    <span class="tier-desc">高性能</span>
                  </button>
                </div>
              </div>
              <div v-if="worldConfig" class="setting-section">
                <div class="section-title">{{ t("simverse.configDetails") }}</div>
                <div class="config-list">
                  <div class="config-row">
                    <span>{{ t("simverse.eventRate") }}</span>
                    <span class="config-value">{{ worldConfig.event_rate_mul }}x</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.cacheSize") }}</span>
                    <span class="config-value">{{ worldConfig.cache_size }}</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.subSim") }}</span>
                    <span class="config-value">{{ worldConfig.sub_sim_active ? t("common.on") : t("common.off") }}</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.subSimDepth") }}</span>
                    <span class="config-value">{{ worldConfig.sub_sim_depth }}</span>
                  </div>
                </div>
              </div>
              <div class="setting-section danger-zone">
                <div class="section-title">{{ t("simverse.exitWorld") }}</div>
                <button class="exit-world-btn" @click="handleExitWorld">
                  <span class="exit-icon">🚪</span>
                  <span class="exit-text">{{ t("simverse.exitWorld") }}</span>
                </button>
              </div>
            </template>

            <template v-else-if="activePanel === 'gacha'">
              <div class="gacha-banner">
                <div class="banner-icon">🎲</div>
                <div class="banner-text">
                  <div class="banner-title">{{ t("simverse.gachaTitle") }}</div>
                  <div class="banner-desc">{{ t("simverse.gachaDesc") }}</div>
                </div>
              </div>
              <div class="gacha-actions">
                <button class="gacha-action-btn single" @click="doGacha(1)">
                  <span class="action-icon">🎴</span>
                  <span class="action-name">{{ t("simverse.singlePull") }}</span>
                  <span class="action-cost">100 💎</span>
                </button>
                <button class="gacha-action-btn ten" @click="doGacha(10)">
                  <span class="action-icon">🎴×10</span>
                  <span class="action-name">{{ t("simverse.tenPull") }}</span>
                  <span class="action-cost">900 💎</span>
                  <span class="action-badge">{{ t("simverse.guaranteedRare") }}</span>
                </button>
              </div>
              <div v-if="gachaResults.length > 0" class="gacha-results">
                <div class="results-header">{{ t("simverse.results") }}</div>
                <div class="results-grid">
                  <div v-for="(item, i) in gachaResults" :key="i" class="result-item" :class="item.rarity">
                    <div class="result-icon">{{ item.icon }}</div>
                    <div class="result-name">{{ item.name }}</div>
                    <div class="result-rarity">{{ item.rarity }}</div>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'explore'">
              <div class="explore-banner">
                <div class="banner-icon">🗺️</div>
                <div class="banner-text">
                  <div class="banner-title">{{ t("simverse.explore") }}</div>
                  <div class="banner-desc">{{ t("simverse.exploreDesc") }}</div>
                </div>
              </div>
              <div class="region-list">
                <div v-for="region in exploreRegions" :key="region.id"
                     class="region-card"
                     @click="enterRegion(region)">
                  <div class="region-icon">{{ region.icon }}</div>
                  <div class="region-info">
                    <div class="region-name">{{ region.name }}</div>
                    <div class="region-type">{{ region.typeName }}</div>
                  </div>
                  <div class="region-arrow">→</div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'battle'">
              <div class="battle-banner">
                <div class="banner-icon">⚔️</div>
                <div class="banner-text">
                  <div class="banner-title">{{ t("simverse.battle") }}</div>
                  <div class="banner-desc">{{ t("simverse.battleDesc") }}</div>
                </div>
              </div>
              <div class="battle-list">
                <div v-for="enemy in battleEnemies" :key="enemy.id"
                     class="enemy-card"
                     @click="startBattle(enemy)">
                  <div class="enemy-icon">{{ enemy.icon }}</div>
                  <div class="enemy-info">
                    <div class="enemy-name">{{ enemy.name }}</div>
                    <div class="enemy-level">Lv.{{ enemy.level }}</div>
                  </div>
                  <button class="fight-btn">{{ t("simverse.fight") }}</button>
                </div>
              </div>
            </template>

            <template v-else>
              <div class="empty-state">{{ t("simverse.comingSoon") }}</div>
            </template>
          </div>
        </div>

        <div ref="hudSceneContainer" class="hud-scene-container">
          <!-- 转场由 useSceneTransition.transitionToScene 处理，不再使用 <transition> 包裹 -->
          <div v-if="screen === 'focus' && selectedNPC" class="detail-modal" @click.self="backToWorld">
            <div class="detail-card">
            <div class="detail-header">
              <div class="detail-avatar" :style="focusBuild ? { background: `linear-gradient(135deg, ${ARCH_META[focusBuild.primary].colorCss}, #ec4899)` } : {}">
                {{ selectedNPC.name?.[0] }}
              </div>
              <div class="detail-info">
                <div class="detail-name">{{ selectedNPC.name }}</div>
                <div class="detail-meta">
                  {{ selectedNPC.species }} · {{ selectedNPC.gender }} · {{ selectedNPC.age }}{{ t("simverse.yearsOld") }}
                </div>
                <div v-if="focusBuild" class="detail-build">
                  <span class="build-chip" :style="{ background: ARCH_META[focusBuild.primary].colorCss }">
                    {{ ARCH_META[focusBuild.primary].emoji }} {{ ARCH_META[focusBuild.primary].name }}
                  </span>
                  <span class="build-synergy">★{{ focusBuild.synergy }}</span>
                </div>
              </div>
              <button class="detail-close" @click="backToWorld">✕</button>
            </div>

            <!-- 编队操作条：把焦点 NPC 编入/移出玩家编队（复用 simverse:squad 持久化） -->
            <div class="focus-actions">
              <button class="focus-action-btn" :class="{ active: inSquad }"
                      :disabled="!inSquad && squadIds.length >= MAX_SQUAD" @click="toggleSquad">
                {{ inSquad ? t("simverse.focus.removeFromSquad") : t("simverse.focus.addToSquad") }}
              </button>
              <span v-if="!inSquad && squadIds.length >= MAX_SQUAD" class="focus-action-hint">
                {{ t("simverse.focus.squadFull") }}
              </span>
            </div>

            <!-- 对象上下文标签：身份 / 时间线 / 关系 -->
            <ion-segment :value="focusTab" @ionChange="onFocusTabChange" class="focus-tabs" scrollable>
              <ion-segment-button value="identity">
                <ion-label>{{ t("simverse.focus.identity") }}</ion-label>
              </ion-segment-button>
              <ion-segment-button value="timeline">
                <ion-label>{{ t("simverse.focus.timeline") }}</ion-label>
              </ion-segment-button>
              <ion-segment-button value="relations">
                <ion-label>{{ t("simverse.focus.relations") }}</ion-label>
              </ion-segment-button>
            </ion-segment>

            <div class="detail-body">
              <!-- 身份：基础档案 -->
              <div v-if="focusTab === 'identity'" class="detail-grid">
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.profession") }}</span>
                  <span class="item-value">{{ selectedNPC.profession }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.level") }}</span>
                  <span class="item-value highlight">Lv.{{ selectedNPC.level }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.health") }}</span>
                  <span class="item-value success">{{ selectedNPC.health }} / {{ selectedNPC.max_health }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.energy") }}</span>
                  <span class="item-value warning">{{ selectedNPC.energy }} / {{ selectedNPC.max_energy }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.wealthTier") }}</span>
                  <span class="item-value">{{ selectedNPC.wealth_tier }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.socialTier") }}</span>
                  <span class="item-value">{{ selectedNPC.social_tier }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.lifeStage") }}</span>
                  <span class="item-value">{{ selectedNPC.life_stage }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.alive") }}</span>
                  <span class="item-value" :class="{ alive: selectedNPC.is_alive, dead: !selectedNPC.is_alive }">
                    {{ selectedNPC.is_alive ? t("common.on") : t("common.off") }}
                  </span>
                </div>
              </div>

              <!-- 时间线：该 NPC 的编年史事件流（接真实后端） -->
              <div v-else-if="focusTab === 'timeline'" class="focus-panel">
                <div v-if="focusLoading" class="empty-state">{{ t("simverse.loading") }}</div>
                <template v-else>
                  <div v-for="ev in npcChronicle" :key="ev.id" class="chrono-row" :class="'imp-' + ev.importance">
                    <div class="chrono-tick">Tick {{ ev.tick }}</div>
                    <div class="chrono-title">{{ ev.type_cn }}</div>
                    <div class="chrono-meta">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
                  </div>
                  <div v-if="npcChronicle.length === 0" class="empty-state">{{ t("simverse.focus.chronicleEmpty") }}</div>
                </template>
              </div>

              <!-- 关系：关系网（接真实后端 SocialGraph），点击跳转对方焦点页 -->
              <div v-else-if="focusTab === 'relations'" class="focus-panel">
                <div v-if="focusLoading" class="empty-state">{{ t("simverse.loading") }}</div>
                <template v-else>
                  <div v-for="rel in npcRelations" :key="rel.target_id" class="rel-row" @click="selectNPC(rel.target)">
                    <div class="rel-avatar">{{ rel.target?.name?.[0] }}</div>
                    <div class="rel-info">
                      <div class="rel-name">{{ rel.target?.name }}</div>
                      <div class="rel-type">{{ t("simverse.rel." + rel.rel_type) }}</div>
                    </div>
                    <div class="rel-affinity">
                      <span class="rel-affinity-val">{{ rel.affinity }}</span>
                      <span class="rel-affinity-label">{{ t("simverse.focus.affinity") }}</span>
                    </div>
                  </div>
                  <div v-if="npcRelations.length === 0" class="empty-state">{{ t("simverse.focus.relEmpty") }}</div>
                </template>
              </div>
            </div>
          </div>
          </div>

        <!-- 抽卡页：屏幕状态机 gacha 页，转场由 useSceneTransition.transitionToScene 处理 -->
        <div v-if="screen === 'gacha'" ref="gachaModalRef" class="gacha-modal-overlay" @click.self="closeGachaModal">
          <div class="gacha-modal-content">
            <button class="gacha-modal-close" @click="closeGachaModal">✕</button>
            <div class="gacha-banner-large">
              <div class="banner-bg"></div>
              <div class="banner-content">
                <div class="banner-icon-large">🎴</div>
                <div class="banner-title-large">{{ t("simverse.gachaTitle") }}</div>
                <div class="banner-desc-large">{{ t("simverse.gachaDesc") }}</div>
              </div>
              <div class="sparkle-layer">
                <span v-for="i in 12" :key="i" class="sparkle" :style="getSparkleStyle(i)">✦</span>
              </div>
            </div>

            <div class="gacha-pool-info">
              <div class="pool-rate">
                <span class="rate-label">SSR</span>
                <span class="rate-val">1%</span>
              </div>
              <div class="pool-rate">
                <span class="rate-label">SR</span>
                <span class="rate-val">8%</span>
              </div>
              <div class="pool-rate">
                <span class="rate-label">R</span>
                <span class="rate-val">30%</span>
              </div>
              <div class="pool-rate">
                <span class="rate-label">N</span>
                <span class="rate-val">61%</span>
              </div>
            </div>

            <div class="gacha-actions-large">
              <button class="gacha-big-btn single" :disabled="isGachaAnimating" @click="doGachaAnimation(1)">
                <span class="btn-icon-large">🎴</span>
                <span class="btn-name">{{ t("simverse.singlePull") }}</span>
                <span class="btn-cost">100 💎</span>
              </button>
              <button class="gacha-big-btn ten" :disabled="isGachaAnimating" @click="doGachaAnimation(10)">
                <span class="btn-icon-large">🎴×10</span>
                <span class="btn-name">{{ t("simverse.tenPull") }}</span>
                <span class="btn-cost">900 💎</span>
                <span class="btn-badge">{{ t("simverse.guaranteedRare") }}</span>
              </button>
            </div>

            <div v-if="gachaHistory.length > 0" class="gacha-history">
              <div class="history-title">最近召唤</div>
              <div class="history-list">
                <div v-for="(item, i) in gachaHistory.slice(0, 6)" :key="i"
                     class="history-item"
                     :class="item.rarity">
                  <span class="hist-icon">{{ item.icon }}</span>
                  <span class="hist-name">{{ item.name }}</span>
                  <span class="hist-rarity">{{ item.rarity }}</span>
                </div>
              </div>
            </div>

            <!-- 转场已迁移至 GSAP（gachaFlashRef watcher），不再使用 <transition> 包裹 -->
            <div v-if="isGachaAnimating" ref="gachaFlashRef" class="gacha-animation-overlay">
              <div class="gacha-cards-container" :class="{ reveal: gachaRevealed }">
                <div v-for="(item, i) in gachaAnimResults" :key="i"
                     class="gacha-card-anim"
                     :class="[item.rarity, { revealed: gachaRevealed }]"
                     :style="{ animationDelay: (i * 0.1) + 's' }">
                  <div class="card-inner">
                    <div class="card-front">
                      <span class="card-back-icon">✦</span>
                    </div>
                    <div class="card-back">
                      <span class="gacha-item-icon">{{ item.icon }}</span>
                      <span class="gacha-item-name">{{ item.name }}</span>
                      <span class="gacha-item-rarity" :class="item.rarity">{{ item.rarity }}</span>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="gachaRevealed" class="gacha-skip-btn" @click="finishGachaAnimation">
                点击继续
              </div>
            </div>
          </div>
        </div>

        <!-- 事件页：屏幕状态机 event 页，重大编年史事件全屏呈现，替代“一页不变” -->
        <!-- 转场由 useSceneTransition.transitionToScene 处理，不再使用 <transition> 包裹 -->
        <div v-if="screen === 'event'" class="event-page">
          <div class="event-page-header">
            <button
              class="btn btn-sm bg-[rgba(139,92,246,0.18)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-3.5 py-2 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
              @click="backToWorld"
            >← {{ t("simverse.back") }}</button>
            <span class="event-page-title">📜 {{ t("simverse.chronicles") }}</span>
          </div>
          <div class="event-feed">
            <div v-for="ev in recentEvents" :key="ev.id" class="event-row" :class="'imp-' + ev.importance">
              <div class="event-row-tick">Tick {{ ev.tick }}</div>
              <div class="event-row-title">{{ ev.type_cn }}</div>
              <div class="event-row-meta">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
            </div>
            <div v-if="recentEvents.length === 0" class="empty-state">{{ t("simverse.noData") }}</div>
          </div>
        </div>

        <!-- 干预页：屏幕状态机 intervene 页，上帝视角的世界控制，全部接真实后端能力 -->
        <!-- 转场由 useSceneTransition.transitionToScene 处理，不再使用 <transition> 包裹 -->
        <div v-if="screen === 'intervene'" class="intervene-page">
          <div class="event-page-header">
            <button
              class="btn btn-sm bg-[rgba(139,92,246,0.18)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-3.5 py-2 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
              @click="backToWorld"
            >← {{ t("simverse.back") }}</button>
            <span class="event-page-title">🎛️ {{ t("simverse.intervene") }}</span>
          </div>
          <div class="intervene-body">
            <section class="card bg-white/[0.04] border border-[rgba(139,92,246,0.18)] rounded-2xl p-4">
              <div class="card-title text-[13px] font-bold text-white/85 mb-3">{{ t("simverse.timeControl") }}</div>
              <div class="ctrl-row">
                <button
                  class="btn btn-sm primary bg-[rgba(34,197,94,0.2)] border border-[rgba(34,197,94,0.45)] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
                  @click="toggleRunning"
                >
                  {{ worldState?.running ? "⏸ " + t("simverse.pause") : "▶ " + t("simverse.resume") }}
                </button>
                <button
                  class="btn btn-sm bg-[rgba(139,92,246,0.16)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
                  @click="stepOnce"
                >⏭ {{ t("simverse.step") }}</button>
              </div>
              <div class="ctrl-row">
                <span class="ctrl-hint">{{ t("simverse.fastForward") }}</span>
                <input class="ff-input" type="number" min="1" max="200" v-model.number="ffSteps" />
                <span class="ctrl-hint">{{ t("simverse.steps") }}</span>
                <button
                  class="btn btn-sm bg-[rgba(139,92,246,0.16)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
                  @click="fastForward"
                >⏩</button>
              </div>
            </section>
            <section class="card bg-white/[0.04] border border-[rgba(139,92,246,0.18)] rounded-2xl p-4">
              <div class="card-title text-[13px] font-bold text-white/85 mb-3">{{ t("simverse.worldSnapshot") }}</div>
              <div class="ctrl-row">
                <button
                  class="btn btn-sm bg-[rgba(139,92,246,0.16)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
                  @click="doSave"
                >💾 {{ t("simverse.saveNow") }}</button>
                <button
                  class="btn btn-sm bg-[rgba(139,92,246,0.16)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
                  @click="doLoad"
                >📂 {{ t("simverse.loadSave") }}</button>
              </div>
              <div v-if="saveMsg" class="ctrl-msg">{{ saveMsg }}</div>
            </section>
          </div>
        </div>

        <!-- 化身页：屏幕状态机 character 页，B 体验（后端角色系统未接入，诚实标注为占位） -->
        <!-- 转场由 useSceneTransition.transitionToScene 处理，不再使用 <transition> 包裹 -->
        <div v-if="screen === 'character'" class="character-page">
          <div class="event-page-header">
            <button
              class="btn btn-sm bg-[rgba(139,92,246,0.18)] border border-[rgba(139,92,246,0.3)] text-white rounded-xl px-3.5 py-2 text-sm hover:bg-[rgba(139,92,246,0.28)] active:scale-95 transition-all duration-150"
              @click="backToWorld"
            >← {{ t("simverse.back") }}</button>
            <span class="event-page-title">🧑 {{ t("simverse.character") }}</span>
          </div>
          <div class="character-body">
            <div class="stub-banner">⚠️ {{ t("simverse.bStubHint") }}</div>
            <div class="char-card">
              <div class="char-avatar">⚔️</div>
              <div class="char-name">冒险者 · Lv.{{ playerLevel }}</div>
              <div class="char-res">
                <span>💎 {{ playerDiamond }}</span>
                <span>🪙 {{ playerGold }}</span>
                <span>⚡ {{ playerStamina }}/120</span>
              </div>
              <div class="char-skills">
                <span v-for="s in playerSkills.slice(0, 4)" :key="s.id" class="char-skill">{{ s.icon }} {{ s.name }}</span>
              </div>
            </div>
          </div>
        </div>
        </div>
    </div>
  </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { IonInfiniteScroll, IonInfiniteScrollContent, IonLabel, IonSegment, IonSegmentButton } from "@ionic/vue";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useSimverseAnimations } from "@/composables/useSimverseAnimations";
import { usePhaserWorld } from "@/composables/usePhaserWorld";
import { useSceneTransition } from "@/composables/useSceneTransition";
import { type SimverseChronicleEvent, type SimverseNPC, type SimverseRelation, useSimverse } from "@/composables/useSimverse";
import { type RenderFps, type RenderQuality, useWorldRenderSettings } from "@/composables/useWorldRenderSettings";
import { ARCH_META, deriveBuildFromNPC } from "@/game/builds";
import {
  closeWorld,
  hideSystemUI,
  isNativePluginMode,
  lockScreenOrientation,
  showSystemUI,
  unlockScreenOrientation,
} from "@/plugins/SimVerse";

const router = useRouter();

// HUD 场景过渡：用 GSAP Flip 做 world ↔ focus/event/intervene/character/gacha 的镜头转场
const { transitionToScene, recordState, playTransition, reverseTransition } = useSceneTransition();
const hudSceneContainer = ref<HTMLElement | null>(null);

// GSAP 动画：通过 useSimverseAnimations composable 集中托管（Task 1.3 / 1.4 / 1.5）
// 详见 composables/useSimverseAnimations.ts —— 替换 @keyframes + <transition> + 游戏级动效

const { t } = useI18n();
const {
  worldState,
  worldConfig,
  isRunning,
  currentTick,
  loadWorldState,
  loadWorldConfig,
  setPerformanceTier,
  controlWorld,
  saveWorld,
  loadWorld,
  loadNPCList,
  loadChronicleWorld,
  loadBehaviorStats,
  loadBehaviorList,
  loadChronicleNPC,
  loadNPCRelations,
  init,
  cleanup,
} = useSimverse();

// 画面设置（帧率/等效渲染）接入实时世界：改动经 useWorldRenderSettings 的
// localStorage + `simverse:render-settings` 事件，由 usePhaserWorld 实时应用到运行中的 Phaser 游戏。
const { fps, quality, FPS_OPTIONS, QUALITY_OPTIONS, QUALITY_RESOLUTION } = useWorldRenderSettings();
function onFpsChange(ev: any) {
  fps.value = Number(ev.detail.value) as RenderFps;
}
function onQualityChange(ev: any) {
  quality.value = ev.detail.value as RenderQuality;
}

const npcList = ref<SimverseNPC[]>([]);
const npcPage = ref(1);
const hasMoreNPCs = ref(true);
const recentEvents = ref<SimverseChronicleEvent[]>([]);
const activePanel = ref<string | null>(null);
const selectedNPC = ref<SimverseNPC | null>(null);
// 焦点页（屏幕状态机 focus 页）上下文：身份/时间线/关系 + 编队状态。
// 升级自"小卡片浮层"——进入后镜头推近对象（Phaser centerOnNPC），页面切换为对象上下文页。
const focusTab = ref<"identity" | "timeline" | "relations">("identity");
const npcChronicle = ref<SimverseChronicleEvent[]>([]);
const npcRelations = ref<SimverseRelation[]>([]);
const focusLoading = ref(false);
const SQUAD_KEY = "simverse:squad";
const MAX_SQUAD = 6;
const squadIds = ref<number[]>(loadSquadIds());
const inSquad = computed(() => !!selectedNPC.value && squadIds.value.includes(selectedNPC.value.id));
// 焦点对象的流派（确定性派生，复用 P14 builds.ts），用于身份徽章
const focusBuild = computed(() => (selectedNPC.value ? deriveBuildFromNPC(selectedNPC.value) : null));

function loadSquadIds(): number[] {
  try {
    const v = JSON.parse(localStorage.getItem(SQUAD_KEY) || "[]");
    return Array.isArray(v) ? v.filter((x: unknown) => typeof x === "number").slice(0, MAX_SQUAD) : [];
  } catch {
    return [];
  }
}
function persistSquadIds() {
  try {
    localStorage.setItem(SQUAD_KEY, JSON.stringify(squadIds.value));
  } catch (e) {
    console.warn("[simverse] squad persist failed:", e);
  }
}
function toggleSquad() {
  if (!selectedNPC.value) return;
  const id = selectedNPC.value.id;
  if (squadIds.value.includes(id)) {
    squadIds.value = squadIds.value.filter(x => x !== id);
  } else {
    if (squadIds.value.length >= MAX_SQUAD) return;
    squadIds.value = [...squadIds.value, id];
  }
  persistSquadIds();
}
// 屏幕状态机：横屏世界由多页组成、会互相切换（不再是一页永远不变）。
// world=主世界 / focus=焦点对象页 / event=事件页 / intervene=干预页 / character=化身页(B) / gacha=抽卡页
type ScreenState =
  | 'world'        // 主世界
  | 'focus'        // NPC 焦点 HUD 场景
  | 'event'        // 事件流 HUD 场景
  | 'intervene'    // 干预 HUD 场景
  | 'character'    // 化身 HUD 场景
  | 'gacha'        // 抽卡 HUD 场景（从 modal 升级为场景）
const screen = ref<ScreenState>("world");
// 独立 computed，避免模板在 v-if="screen === 'world'" 块内将 screen 收窄为 "world" 而误报比较无交集
const isIntervene = computed(() => screen.value === "intervene");
const bottomMoreOpen = ref(false);
function toggleBottomMore() {
  bottomMoreOpen.value = !bottomMoreOpen.value;
}

// 事件页：从主世界进入全屏编年史事件流（屏幕状态机 event 页），用 GSAP Flip 过渡
async function openEventPage() {
  if (recentEvents.value.length === 0) await loadEvents();
  await openHudScene('event');
}

// 干预页：现在通过 openHudScene('intervene') 从底部操作条直接调用
const ffSteps = ref(10);
const saveMsg = ref("");
async function fastForward() {
  for (let i = 0; i < ffSteps.value; i++) {
    await controlWorld("step");
  }
  await refreshState();
}
async function doSave() {
  await saveWorld();
  saveMsg.value = t("simverse.saveSuccess");
}
async function doLoad() {
  await loadWorld();
  await refreshState();
  saveMsg.value = t("simverse.loadSuccess");
}

// 化身页：现在通过 openHudScene('character') 从"更多"面板直接调用
const gachaResults = ref<{ name: string; icon: string; rarity: string }[]>([]);
const behaviorStats = ref<{ total_npcs: number; alive_npcs: number; behavior_dist: Record<string, number> } | null>(null);

const econTab = ref<"prices" | "ranking">("prices");
const economyStats = ref<{
  region_id: number;
  prices: Record<string, number>;
  supply: Record<string, number>;
  demand: Record<string, number>;
  trade_volume: number;
} | null>(null);
const wealthRankings = ref<
  {
    id: number;
    name: string;
    level: number;
    profession: string;
    wealth: number;
    rank: number;
  }[]
>([]);

const questTab = ref<"daily" | "achieve" | "story">("daily");
const questList = ref<
  {
    id: string;
    type: number;
    title: string;
    desc: string;
    icon: string;
    goal: number;
    progress: number;
    reward: { diamond: number; gold: number; exp: number; icon: string };
    status: number;
    sort_order: number;
  }[]
>([]);
const questCompletable = ref(0);

const questTypeMap: Record<string, number> = { daily: 0, achieve: 1, story: 2 };
const filteredQuests = computed(() => {
  const targetType = questTypeMap[questTab.value];
  return questList.value
    .filter(q => q.type === targetType || (questTab.value === "achieve" && q.type === 3))
    .sort((a, b) => a.sort_order - b.sort_order);
});

const isGachaAnimating = ref(false);
const gachaRevealed = ref(false);
const gachaAnimResults = ref<{ name: string; icon: string; rarity: string }[]>([]);
const gachaHistory = ref<{ name: string; icon: string; rarity: string }[]>([]);

const playerLevel = ref(1);
const playerExp = ref(0);
const playerHp = ref(100);
const playerMaxHp = ref(100);
const playerAttack = ref(15);
const playerDefense = ref(10);
const playerDiamond = ref(2500);
const playerGold = ref(50000);
const playerStamina = ref(120);

const playerSkills = ref([
  { id: 1, name: "烈焰斩", icon: "🔥", level: 3, rarity: "SR" },
  { id: 2, name: "铁壁防御", icon: "🛡️", level: 2, rarity: "R" },
  { id: 3, name: "疾风步", icon: "💨", level: 1, rarity: "N" },
  { id: 4, name: "治愈术", icon: "💚", level: 1, rarity: "SR" },
]);

const trainingModes = ref([
  { id: "strength", name: "力量训练", icon: "💪", effect: "攻击 +2", staminaCost: 10, type: "attack" },
  { id: "defense", name: "防御训练", icon: "🛡️", effect: "防御 +2", staminaCost: 10, type: "defense" },
  { id: "endurance", name: "耐力训练", icon: "❤️", effect: "生命 +10", staminaCost: 15, type: "hp" },
  { id: "meditation", name: "冥想修炼", icon: "🧘", effect: "经验 +50", staminaCost: 20, type: "exp" },
]);

const equipmentSlots = ref([
  { slot: "weapon", label: "武器", item: { name: "新手剑", icon: "⚔️" } },
  { slot: "armor", label: "护甲", item: { name: "布甲", icon: "🎽" } },
  { slot: "accessory", label: "饰品", item: null },
  { slot: "rune", label: "符文", item: null },
]);

const inventoryItems = ref([
  { id: 1, name: "生命药水", icon: "🧪", count: 5 },
  { id: 2, name: "攻击宝石", icon: "💎", count: 3 },
  { id: 3, name: "经验卷轴", icon: "📜", count: 2 },
  { id: 4, name: "强化石", icon: "🪨", count: 10 },
  { id: 5, name: "幸运草", icon: "🍀", count: 1 },
]);

const expToNextLevel = computed(() => playerLevel.value * 100);
const expPercent = computed(() => Math.min(100, (playerExp.value / expToNextLevel.value) * 100));
const playerHpPercent = computed(() => (playerHp.value / playerMaxHp.value) * 100);

// 抽卡：现在通过 openHudScene('gacha') 从"更多"面板直接调用（gacha 已从 modal 升级为 HUD 场景）
async function closeGachaModal() {
  if (isGachaAnimating.value) return;
  await openHudScene('world');
}

function getSparkleStyle(index: number) {
  const angle = (index / 12) * 360;
  const delay = index * 0.1;
  return {
    transform: `rotate(${angle}deg) translateY(-60px)`,
    animationDelay: `${delay}s`,
  };
}

function doGachaAnimation(count: number) {
  if (isGachaAnimating.value) return;

  const cost = count === 1 ? 100 : 900;
  if (playerDiamond.value < cost) return;

  playerDiamond.value -= cost;
  isGachaAnimating.value = true;
  gachaRevealed.value = false;

  const results: { name: string; icon: string; rarity: string }[] = [];
  const pool = [
    { name: "普通村民", icon: "👤", rarity: "N", weight: 61 },
    { name: "熟练工匠", icon: "🔨", rarity: "R", weight: 30 },
    { name: "精英战士", icon: "⚔️", rarity: "SR", weight: 8 },
    { name: "传奇英雄", icon: "👑", rarity: "SSR", weight: 1 },
  ];

  for (let i = 0; i < count; i++) {
    const isGuaranteed = count === 10 && i === 9;
    let rarity: string;

    if (isGuaranteed) {
      rarity = pool.filter(p => ["SR", "SSR"].includes(p.rarity))[Math.floor(Math.random() * 2)].rarity;
    } else {
      const total = pool.reduce((s, p) => s + p.weight, 0);
      let rand = Math.random() * total;
      rarity = pool[0].rarity;
      for (const item of pool) {
        rand -= item.weight;
        if (rand <= 0) {
          rarity = item.rarity;
          break;
        }
      }
    }

    const matched = pool.find(p => p.rarity === rarity)!;
    results.push({ ...matched });
  }

  gachaAnimResults.value = results;
  gachaResults.value = results;

  setTimeout(() => {
    gachaRevealed.value = true;
    recordQuestAction("gacha");
    // Task 1.5: 抽卡揭晓时触发游戏级动效（光柱 + 屏幕震动 + 粒子）
    nextTick(() => {
      playGachaLightBeam();
      shakeScreen(6, 0.4);
      const overlay = document.querySelector(".gacha-animation-overlay");
      if (overlay instanceof HTMLElement) {
        spawnParticles(overlay, 16, "#8b5cf6");
      }
    });
  }, 1500);
}

function finishGachaAnimation() {
  gachaHistory.value = [...gachaAnimResults.value, ...gachaHistory.value].slice(0, 20);
  isGachaAnimating.value = false;
  gachaRevealed.value = false;
}

function doTraining(mode: { id: string; staminaCost: number; type: string; effect: string }) {
  if (playerStamina.value < mode.staminaCost) return;

  playerStamina.value -= mode.staminaCost;
  playerExp.value += 30;

  switch (mode.type) {
    case "attack":
      playerAttack.value += 1;
      break;
    case "defense":
      playerDefense.value += 1;
      break;
    case "hp":
      playerMaxHp.value += 5;
      playerHp.value += 5;
      break;
    case "exp":
      playerExp.value += 20;
      break;
  }

  while (playerExp.value >= expToNextLevel.value) {
    playerExp.value -= expToNextLevel.value;
    levelUp();
  }
}

function levelUp() {
  playerLevel.value++;
  playerMaxHp.value += 20;
  playerHp.value = playerMaxHp.value;
  playerAttack.value += 3;
  playerDefense.value += 2;
}

const exploreRegions = ref([
  { id: "town", name: "星光镇", typeName: "城镇", type: "town", icon: "🏘️" },
  { id: "forest", name: "幽暗森林", typeName: "森林", type: "forest", icon: "🌲" },
  { id: "mountain", name: "巨岩山脉", typeName: "山脉", type: "mountain", icon: "⛰️" },
  { id: "dungeon", name: "深渊地牢", typeName: "地牢", type: "dungeon", icon: "🏚️" },
  { id: "plains", name: "辽阔平原", typeName: "平原", type: "plains", icon: "🌾" },
]);

const battleEnemies = ref([
  { id: "slime", name: "史莱姆", level: 1, icon: "🟢", enemyEmoji: "🟢" },
  { id: "goblin", name: "哥布林", level: 3, icon: "👺", enemyEmoji: "👺" },
  { id: "skeleton", name: "骷髅兵", level: 5, icon: "💀", enemyEmoji: "💀" },
  { id: "wolf", name: "魔狼", level: 7, icon: "🐺", enemyEmoji: "🐺" },
  { id: "dragon", name: "幼龙", level: 15, icon: "🐉", enemyEmoji: "🐉" },
]);

let pollInterval: number | null = null;
let behaviorPollInterval: number | null = null;

const visibleNPCs = computed(() => npcList.value.slice(0, 12));

const phaserContainerRef = ref<HTMLElement | null>(null);
const usePhaser = ref(true);
const phaserLoading = ref(true);
const phaserHasError = ref(false);

const phaserWorld = usePhaserWorld();

phaserWorld.onNPCClick(npc => {
  selectNPC(npc);
});

async function initPhaser() {
  if (!usePhaser.value || !phaserContainerRef.value) return;

  try {
    phaserLoading.value = true;
    phaserHasError.value = false;

    phaserWorld.setGameContainer(phaserContainerRef.value);
    const success = await phaserWorld.initPhaser(12345);

    if (success) {
      const checkReady = setInterval(() => {
        if (phaserWorld.isReady.value) {
          clearInterval(checkReady);
          phaserLoading.value = false;
          if (npcList.value.length > 0) {
            phaserWorld.setNPCs(npcList.value);
            loadNPCBehaviors();
          }
        }
      }, 100);

      setTimeout(() => {
        clearInterval(checkReady);
        if (!phaserWorld.isReady.value) {
          console.warn("[SimverseWorld] Phaser init timeout, fallback to DOM mode");
          phaserHasError.value = true;
          phaserLoading.value = false;
        }
      }, 10000);
    } else {
      phaserHasError.value = true;
      phaserLoading.value = false;
      usePhaser.value = false;
    }
  } catch (e) {
    console.warn("[SimverseWorld] Phaser init failed, fallback to DOM mode:", e);
    phaserHasError.value = true;
    phaserLoading.value = false;
    usePhaser.value = false;
  }
}

watch(
  npcList,
  newList => {
    if (usePhaser.value && phaserWorld.isReady.value) {
      phaserWorld.setNPCs(newList);
      loadNPCBehaviors();
    }
  },
  { deep: true }
);

const panelOnLeft = computed(() => {
  const leftPanels = ["quest", "npc", "org", "economy", "chronicles"];
  return leftPanels.includes(activePanel.value || "");
});

const cellTypes = ["forest", "mountain", "water", "plain", "village", "city", "desert"];
const cellIcons: Record<string, string> = {
  forest: "🌲",
  mountain: "⛰️",
  water: "🌊",
  plain: "🌾",
  village: "🏘️",
  city: "🏙️",
  desert: "🏜️",
};

function getCellClass(i: number): string {
  const seed = (i * 7 + 13) % cellTypes.length;
  return cellTypes[seed];
}

function getCellIcon(i: number): string {
  const seed = (i * 7 + 13) % cellTypes.length;
  return cellIcons[cellTypes[seed]];
}

function getNPCPosition(idx: number): Record<string, string> {
  const cols = 8;
  const col = idx % cols;
  const row = Math.floor(idx / cols);
  return {
    left: `${12.5 + col * 12 + (idx % 3) * 3}%`,
    top: `${15 + row * 18 + (idx % 2) * 5}%`,
  };
}

const behaviorIcons: Record<string, string> = {
  idle: "😴",
  work: "💼",
  rest: "😌",
  eat: "🍽️",
  sleep: "💤",
  socialize: "💬",
  explore: "🚶",
  trade: "💰",
};

function getBehaviorIcon(behavior: string): string {
  return behaviorIcons[behavior] || "❓";
}

function getBehaviorClass(behavior: string): string {
  return `beh-${behavior}`;
}

async function refreshState() {
  await Promise.all([loadWorldState(), loadWorldConfig()]);
}

async function toggleRunning() {
  if (!worldState.value) return;
  const action = worldState.value.running ? "pause" : "resume";
  await controlWorld(action);
  await refreshState();
}

async function stepOnce() {
  await controlWorld("step");
  await refreshState();
}

async function changeTier(tier: string) {
  const result = await setPerformanceTier(tier as any);
  if (result) {
    await refreshState();
  }
}

async function loadNPCs() {
  const result = await loadNPCList(npcPage.value, 20);
  if (result) {
    if (npcPage.value === 1) {
      npcList.value = result.items;
    } else {
      npcList.value = [...npcList.value, ...result.items];
    }
    hasMoreNPCs.value = npcList.value.length < result.total;
  }
}

async function loadMoreNPCs(ev: any) {
  npcPage.value++;
  await loadNPCs();
  ev.target.complete();
}

async function loadEvents() {
  const data = await loadChronicleWorld(0, 20);
  recentEvents.value = data?.items || [];
}

async function loadBehaviorStatsData() {
  const data = await loadBehaviorStats();
  if (data) {
    behaviorStats.value = data;
  }
}

async function loadNPCBehaviors() {
  if (!usePhaser.value || !phaserWorld.isReady.value) return;
  const data = await loadBehaviorList(1, 200);
  if (!data) return;
  const map = new Map<number, string>();
  for (const b of data.items) {
    if (b.current_behavior_cn) map.set(b.npc_id, b.current_behavior_cn);
  }
  phaserWorld.setNPCBehaviors(map);
}

// 进入焦点页：屏幕状态机 world → focus，镜头推近对象 + 加载对象上下文（身份/时间线/关系）
// 用 GSAP Flip 做焦点 HUD 场景过渡；selectNPC 仍保留本地 focus 状态用于 panel→fullscreen 转场
async function selectNPC(npc: SimverseNPC) {
  selectedNPC.value = npc;
  focusTab.value = "identity";
  // 镜头推近：Phaser 相机居中并放大到该 NPC，构成"world → focus"的镜头转场
  if (usePhaser.value && phaserWorld.isReady.value) {
    phaserWorld.centerOnNPC(npc.id);
  }
  recordQuestAction("view_npc");
  await openHudScene('focus');
  await loadFocusContext(npc.id);
}

// 跳转 NPC 详情独立路由（深度页面，从 focus HUD 场景进入完整 NPC 详情）
function viewNPCDetail(id: number | string) {
  openDetailRoute('npc', { id: String(id) });
}

// 加载焦点对象的编年史时间线与关系网（接真实后端，非桩）
async function loadFocusContext(id: number) {
  focusLoading.value = true;
  try {
    const [chronicle, rels] = await Promise.all([loadChronicleNPC(id, 30), loadNPCRelations(id)]);
    npcChronicle.value = chronicle?.items ?? [];
    npcRelations.value = rels?.relations ?? [];
  } catch (e) {
    console.warn("Failed to load focus context:", e);
    npcChronicle.value = [];
    npcRelations.value = [];
  } finally {
    focusLoading.value = false;
  }
}

// 从焦点页返回主世界（屏幕状态机回退）：GSAP 驱动的镜头复位（居中世界中心 + 缩放回 1.0）
// 用 GSAP Flip 做逆向转场，让回退也有过渡动画
async function backToWorld() {
  selectedNPC.value = null;
  if (usePhaser.value && phaserWorld.isReady.value) {
    phaserWorld.returnToWorldView();
  }
  await openHudScene('world');
}

// 焦点页标签切换（身份/时间线/关系）
function onFocusTabChange(ev: CustomEvent) {
  focusTab.value = ev.detail.value as "identity" | "timeline" | "relations";
}

// HUD 场景切换：focus/gacha/intervene/character/event 保留在单页面内，用 GSAP Flip 过渡
async function openHudScene(target: ScreenState) {
  if (!hudSceneContainer.value) {
    screen.value = target;
    return;
  }
  await transitionToScene(
    hudSceneContainer.value,
    () => { screen.value = target; },
    { duration: 0.45, ease: 'power3.inOut' },
  );
}

// 详情路由：导航到独立路由（NPC/编年史/经济/组织/区域/任务/训练/背包/探索/战斗/化身资料/设置）
function openDetailRoute(name: string, params?: Record<string, string>) {
  const routeMap: Record<string, string> = {
    npc: '/world/npc/:id',
    chronicles: '/world/chronicles',
    economy: '/world/economy',
    quest: '/world/quest/detail',
    org: '/tabs/orgs',
    training: '/world/training',
    inventory: '/world/inventory',
    explore: '/world/explore',
    battle: '/world/battle',
    profile: '/world/profile',
    settings: '/world/settings',
  };
  const path = routeMap[name];
  if (!path) {
    console.warn(`[SimverseWorld] unknown panel: ${name}`);
    return;
  }
  // 处理 params（如 NPC id）
  let finalPath = path;
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      finalPath = finalPath.replace(`:${key}?`, value).replace(`:${key}`, value);
    }
  }
  router.push(finalPath);
}

// 兼容旧调用：根据 panel 名字自动分流（HUD 场景 vs 详情路由）
function openPanel(name: string) {
  const hudScenes = new Set<string>(['focus', 'event', 'intervene', 'character', 'gacha']);
  if (hudScenes.has(name)) {
    openHudScene(name as ScreenState);
  } else {
    openDetailRoute(name);
  }
}

async function loadEconomyData() {
  try {
    const [statsRes, rankRes] = await Promise.all([
      fetch("/api/simverse/economy/stats").then(r => r.json()),
      fetch("/api/simverse/economy/wealth-rank?count=20").then(r => r.json()),
    ]);
    economyStats.value = statsRes;
    wealthRankings.value = rankRes.items || [];
  } catch (e) {
    console.warn("Failed to load economy data:", e);
  }
}

async function loadQuestData() {
  try {
    const res = await fetch("/api/simverse/quest/list");
    const data = await res.json();
    questList.value = data.quests || [];
    questCompletable.value = data.completable || 0;
  } catch (e) {
    console.warn("Failed to load quest data:", e);
  }
}

async function recordQuestAction(action: string) {
  try {
    await fetch("/api/simverse/quest/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action }),
    });
    if (activePanel.value === "quest") {
      loadQuestData();
    }
  } catch (e) {
    console.warn("Failed to record quest action:", e);
  }
}

async function claimQuest(questId: string) {
  try {
    const res = await fetch("/api/simverse/quest/claim", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ quest_id: questId }),
    });
    const data = await res.json();
    if (data.success) {
      const r = data.reward;
      if (r.diamond) playerDiamond.value += r.diamond;
      if (r.gold) playerGold.value += r.gold;
      if (r.exp) playerExp.value += r.exp;
      loadQuestData();
    }
  } catch (e) {
    console.warn("Failed to claim quest:", e);
  }
}

function getResourceIcon(name: string): string {
  const icons: Record<string, string> = {
    Food: "🍞",
    Wood: "🪵",
    Stone: "🪨",
    Iron: "⚙️",
    Gold: "💰",
    Cloth: "🧵",
    Potion: "🧪",
    ManaCrystal: "💎",
    Herb: "🌿",
    Leather: "👜",
  };
  return icons[name] || "📦";
}

function getResourceName(name: string): string {
  const names: Record<string, string> = {
    Food: "食物",
    Wood: "木材",
    Stone: "石料",
    Iron: "铁矿",
    Gold: "金币",
    Cloth: "布匹",
    Potion: "药水",
    ManaCrystal: "魔晶",
    Herb: "草药",
    Leather: "皮革",
  };
  return names[name] || name;
}

function getSupplyPercent(name: string): number {
  if (!economyStats.value) return 50;
  const supply = economyStats.value.supply[name] || 100;
  return Math.min(100, Math.max(0, (supply / 200) * 100));
}

function getDemandPercent(name: string): number {
  if (!economyStats.value) return 50;
  const demand = economyStats.value.demand[name] || 100;
  return Math.min(100, Math.max(0, (demand / 200) * 100));
}

function getPriceTrend(name: string): string {
  if (!economyStats.value) return "→";
  const supply = economyStats.value.supply[name] || 100;
  const demand = economyStats.value.demand[name] || 100;
  const ratio = demand / supply;
  if (ratio > 1.1) return "↑";
  if (ratio < 0.9) return "↓";
  return "→";
}

function getPriceTrendClass(name: string): string {
  if (!economyStats.value) return "trend-stable";
  const supply = economyStats.value.supply[name] || 100;
  const demand = economyStats.value.demand[name] || 100;
  const ratio = demand / supply;
  if (ratio > 1.1) return "trend-up";
  if (ratio < 0.9) return "trend-down";
  return "trend-stable";
}

function getPanelTitle(panel: string): string {
  const titles: Record<string, string> = {
    npc: t("simverse.npc"),
    org: t("simverse.org"),
    profile: t("simverse.profileTitle"),
    training: t("simverse.trainingTitle"),
    inventory: "背包",
    chronicles: t("simverse.chronicles"),
    settings: t("simverse.settings"),
    economy: t("simverse.economy"),
    quest: "任务",
    intervention: t("simverse.intervention"),
    debug: t("simverse.debug"),
  };
  return titles[panel] || panel;
}

function doGacha(count: number) {
  const results: { name: string; icon: string; rarity: string }[] = [];
  const pool = [
    { name: "普通村民", icon: "👤", rarity: "N", weight: 60 },
    { name: "熟练工匠", icon: "🔨", rarity: "R", weight: 25 },
    { name: "精英战士", icon: "⚔️", rarity: "SR", weight: 10 },
    { name: "传奇英雄", icon: "👑", rarity: "SSR", weight: 4 },
    { name: "神话存在", icon: "🌠", rarity: "UR", weight: 1 },
  ];

  for (let i = 0; i < count; i++) {
    const isGuaranteed = count === 10 && i === 9;
    let rarity: string;

    if (isGuaranteed) {
      rarity = pool.filter(p => ["SR", "SSR", "UR"].includes(p.rarity))[Math.floor(Math.random() * 3)].rarity;
    } else {
      const total = pool.reduce((s, p) => s + p.weight, 0);
      let rand = Math.random() * total;
      rarity = pool[0].rarity;
      for (const item of pool) {
        rand -= item.weight;
        if (rand <= 0) {
          rarity = item.rarity;
          break;
        }
      }
    }

    const matched = pool.filter(p => p.rarity === rarity)[0];
    results.push({ ...matched });
  }

  gachaResults.value = results;
  activePanel.value = "gacha";
}

function enterRegion(region: { id: string; name: string; type: string }) {
  activePanel.value = null;
  if (usePhaser.value && phaserWorld.isReady.value) {
    phaserWorld.enterRegion({
      regionId: region.id,
      regionName: region.name,
      regionType: region.type,
    });
  }
}

function startBattle(enemy: { id: string; name: string; level: number; enemyEmoji: string }) {
  activePanel.value = null;
  if (usePhaser.value && phaserWorld.isReady.value) {
    phaserWorld.startBattle({
      enemyName: enemy.name,
      enemyLevel: enemy.level,
      enemyEmoji: enemy.enemyEmoji,
      playerName: "勇者",
      playerLevel: 1,
      playerEmoji: "⚔️",
    });
  }
}

async function handleExitWorld() {
  try {
    if (isNativePluginMode()) {
      await showSystemUI();
      await unlockScreenOrientation();
      await closeWorld();
    } else {
      router.push("/tabs/home");
    }
  } catch (e) {
    console.warn("[SimverseWorld] Exit world failed:", e);
    if (!isNativePluginMode()) {
      router.push("/tabs/home");
    }
  }
}

function startPolling() {
  pollInterval = window.setInterval(() => {
    refreshState();
  }, 3000);
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
  if (behaviorPollInterval) {
    clearInterval(behaviorPollInterval);
    behaviorPollInterval = null;
  }
}

// 行为实时刷新：让 NPC 行为气泡随世界演化更新
function startBehaviorPolling() {
  stopBehaviorPolling();
  behaviorPollInterval = window.setInterval(() => {
    if (usePhaser.value && phaserWorld.isReady.value) {
      loadNPCBehaviors();
    }
  }, 5000);
}

function stopBehaviorPolling() {
  if (behaviorPollInterval) {
    clearInterval(behaviorPollInterval);
    behaviorPollInterval = null;
  }
}

// 元素引用：用于 GSAP 触发入场/退场动画（Task 1.4）
const bottomBarRef = ref<HTMLElement | null>(null);
const morePopRef = ref<HTMLElement | null>(null);
const gachaModalRef = ref<HTMLElement | null>(null);
const gachaFlashRef = ref<HTMLElement | null>(null);

// Task 1.3 / 1.4 / 1.5：GSAP 动画通过 useSimverseAnimations composable 集中托管
// 详见 composables/useSimverseAnimations.ts —— 替换 @keyframes + <transition> + 游戏级动效
const {
  playGachaLightBeam,
  shakeScreen,
  spawnParticles,
  startAmbientLoops,
  stopAll: stopAllAnimations,
} = useSimverseAnimations({
  bottomBarRef,
  morePopRef,
  gachaFlashRef,
  phaserLoading,
  usePhaser,
  phaserHasError,
  worldState,
  screen,
  bottomMoreOpen,
  isGachaAnimating,
  recentEvents,
  playerDiamond,
  playerGold,
  playerStamina,
  activePanel,
  econTab,
  economyStats,
});

onMounted(async () => {
  if (isNativePluginMode()) {
    lockScreenOrientation("landscape-primary").catch(e => {
      console.warn("[SimverseWorld] Lock orientation failed:", e);
    });
    hideSystemUI().catch(e => {
      console.warn("[SimverseWorld] Hide system UI failed:", e);
    });
  } else {
    try {
      if (document.documentElement.requestFullscreen) {
        await document.documentElement.requestFullscreen();
      }
    } catch (e) {
      console.warn("[SimverseWorld] requestFullscreen failed:", e);
    }
  }
  await init();
  await refreshState();
  await loadNPCs();
  startPolling();

  await nextTick();
  initPhaser();
  startBehaviorPolling();

  // 启动常驻 GSAP 循环动画（bgDrift + npcPulse，替换 @keyframes）
  startAmbientLoops();
});

onUnmounted(() => {
  stopPolling();
  stopBehaviorPolling();
  cleanup();
  // 清理所有 GSAP tween，防止内存泄漏（由 useSimverseAnimations composable 托管）
  stopAllAnimations();
  if (isNativePluginMode()) {
    unlockScreenOrientation().catch(() => {});
    showSystemUI().catch(() => {});
  } else {
    try {
      if (document.exitFullscreen && document.fullscreenElement) {
        document.exitFullscreen();
      }
    } catch (e) {
      console.warn("[SimverseWorld] exitFullscreen failed:", e);
    }
  }
});
</script>

<style scoped lang="scss" src="./simverse-world/SimverseWorld.scss"></style>
