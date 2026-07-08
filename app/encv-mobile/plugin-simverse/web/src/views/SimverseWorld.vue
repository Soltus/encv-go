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
            <div class="resource-item" v-tooltip="t('simverse.tick')">
              <span class="resource-icon">⏱️</span>
              <span class="resource-value">{{ worldState?.tick ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.population')">
              <span class="resource-icon">👥</span>
              <span class="resource-value">{{ worldState?.npc_count ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.brains')">
              <span class="resource-icon">🧠</span>
              <span class="resource-value">{{ worldState?.brain_count ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.memory')">
              <span class="resource-icon">💾</span>
              <span class="resource-value">{{ (worldState?.total_mb ?? 0).toFixed(1) }}M</span>
            </div>
          </div>
          <div class="top-actions">
            <button class="game-btn play-btn" :class="{ running: worldState?.running }" @click="toggleRunning">
              <span class="btn-icon">{{ worldState?.running ? "⏸" : "▶" }}</span>
            </button>
            <button class="game-btn" @click="stepOnce">
              <span class="btn-icon">⏭</span>
            </button>
            <button class="game-btn" @click="refreshState">
              <span class="btn-icon">↻</span>
            </button>
          </div>
        </div>

        <div class="left-menu">
          <button class="menu-btn" :class="{ active: activePanel === 'npc' }" @click="openPanel('npc')">
            <span class="menu-icon">👤</span>
            <span class="menu-label">{{ t("simverse.npc") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'org' }" @click="openPanel('org')">
            <span class="menu-icon">🏰</span>
            <span class="menu-label">{{ t("simverse.org") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'chronicles' }" @click="openPanel('chronicles')">
            <span class="menu-icon">📖</span>
            <span class="menu-label">{{ t("simverse.chronicles") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'economy' }" @click="openPanel('economy')">
            <span class="menu-icon">💰</span>
            <span class="menu-label">{{ t("simverse.economy") }}</span>
          </button>
          <div class="menu-divider"></div>
          <button class="menu-btn explore-btn" @click="openPanel('explore')">
            <span class="menu-icon">🗺️</span>
            <span class="menu-label">{{ t("simverse.explore") }}</span>
          </button>
          <button class="menu-btn battle-btn" @click="openPanel('battle')">
            <span class="menu-icon">⚔️</span>
            <span class="menu-label">{{ t("simverse.battle") }}</span>
          </button>
          <div class="menu-divider"></div>
          <button class="menu-btn gacha-btn" @click="openPanel('gacha')">
            <span class="menu-icon sparkle">✨</span>
            <span class="menu-label">{{ t("simverse.gacha") }}</span>
          </button>
        </div>

        <div class="right-menu">
          <button class="menu-btn" :class="{ active: activePanel === 'settings' }" @click="openPanel('settings')">
            <span class="menu-icon">⚙️</span>
            <span class="menu-label">{{ t("simverse.settings") }}</span>
          </button>
          <button class="menu-btn" @click="openPanel('intervention')">
            <span class="menu-icon">⚡</span>
            <span class="menu-label">{{ t("simverse.intervention") }}</span>
          </button>
          <button class="menu-btn" @click="openPanel('debug')">
            <span class="menu-icon">🔧</span>
            <span class="menu-label">{{ t("simverse.debug") }}</span>
          </button>
          <div class="menu-divider"></div>
          <button class="menu-btn exit-btn" @click="handleExitWorld">
            <span class="menu-icon">🚪</span>
            <span class="menu-label">{{ t("simverse.exitWorld") }}</span>
          </button>
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

        <div class="event-ticker" v-if="recentEvents.length > 0">
          <div class="ticker-icon">📜</div>
          <div class="ticker-content">
            <transition-group name="ticker" tag="div" class="ticker-list">
              <div v-for="ev in recentEvents.slice(0, 3)" :key="ev.id" class="ticker-item">
                <span class="event-dot" :class="'imp-' + ev.importance"></span>
                <span class="event-text">{{ ev.type_cn }}</span>
              </div>
            </transition-group>
          </div>
        </div>

        <div v-if="activePanel" class="side-panel" :class="{ open: !!activePanel, 'panel-left': panelOnLeft }">
          <div class="panel-header">
            <span class="panel-title">{{ getPanelTitle(activePanel) }}</span>
            <button class="panel-close-btn" @click="activePanel = null">✕</button>
          </div>
          <div class="panel-content">
            <template v-if="activePanel === 'npc'">
              <div v-if="behaviorStats" class="behavior-stats-bar">
                <div class="stats-title">活动分布</div>
                <div class="behavior-chips">
                  <span v-for="(count, behavior) in behaviorStats.behavior_dist" :key="behavior"
                        class="behavior-chip" :class="behavior">
                    <span class="chip-icon">{{ getBehaviorIcon(behavior as string) }}</span>
                    <span class="chip-count">{{ count }}</span>
                  </span>
                </div>
              </div>
              <div v-for="npc in npcList" :key="npc.id" class="list-card" @click="selectNPC(npc)">
                <div class="card-avatar">{{ npc.name?.[0] || '?' }}</div>
                <div class="card-info">
                  <div class="card-title">{{ npc.name }}</div>
                  <div class="card-subtitle">
                    <span class="prof-tag">{{ npc.profession }}</span>
                    <span class="level-tag">Lv.{{ npc.level }}</span>
                  </div>
                  <div class="card-behavior">
                    <span class="behavior-icon">{{ getBehaviorIcon(npc.current_behavior || 'idle') }}</span>
                    <span class="behavior-text">{{ npc.current_behavior_cn || '空闲' }}</span>
                  </div>
                </div>
                <div class="card-action">
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

            <template v-else-if="activePanel === 'settings'">
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

        <div v-if="selectedNPC" class="detail-modal" @click.self="selectedNPC = null">
          <div class="detail-card">
            <div class="detail-header">
              <div class="detail-avatar">{{ selectedNPC.name?.[0] }}</div>
              <div class="detail-info">
                <div class="detail-name">{{ selectedNPC.name }}</div>
                <div class="detail-meta">
                  {{ selectedNPC.species }} · {{ selectedNPC.gender }} · {{ selectedNPC.age }}岁
                </div>
              </div>
              <button class="detail-close" @click="selectedNPC = null">✕</button>
            </div>
            <div class="detail-body">
              <div class="detail-grid">
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
            </div>
          </div>
        </div>

        <div class="bottom-nav">
          <button class="nav-item" :class="{ active: activeSubPage === 'home' }" @click="openSubPage('home')">
            <span class="nav-icon">🏠</span>
            <span class="nav-label">主页</span>
          </button>
          <button class="nav-item" :class="{ active: activeSubPage === 'profile' }" @click="openSubPage('profile')">
            <span class="nav-icon">👤</span>
            <span class="nav-label">{{ t("simverse.profile") }}</span>
          </button>
          <button class="nav-item gacha-nav" @click="openSubPage('gacha')">
            <span class="nav-icon sparkle">✨</span>
            <span class="nav-label">{{ t("simverse.gacha") }}</span>
          </button>
          <button class="nav-item" :class="{ active: activeSubPage === 'training' }" @click="openSubPage('training')">
            <span class="nav-icon">⚔️</span>
            <span class="nav-label">{{ t("simverse.training") }}</span>
          </button>
          <button class="nav-item" :class="{ active: activeSubPage === 'settings' }" @click="openSubPage('settings')">
            <span class="nav-icon">⚙️</span>
            <span class="nav-label">{{ t("simverse.settings") }}</span>
          </button>
        </div>

        <transition name="page-slide">
          <div v-if="activeSubPage && activeSubPage !== 'home'" class="full-screen-page" :key="activeSubPage">
            <template v-if="activeSubPage === 'profile'">
              <div class="profile-page">
                <div class="page-header">
                  <button class="back-btn" @click="closeSubPage()">
                    <span>←</span>
                  </button>
                  <span class="page-title">{{ t("simverse.profileTitle") }}</span>
                  <div class="header-spacer"></div>
                </div>
                <div class="page-content">
                  <div class="player-card">
                    <div class="card-bg"></div>
                    <div class="card-content">
                      <div class="player-avatar">
                        <span class="avatar-icon">⚔️</span>
                      </div>
                      <div class="player-name">冒险者</div>
                      <div class="player-title">Lv.{{ playerLevel }} 勇者</div>
                      <div class="player-stats-row">
                        <div class="mini-stat">
                          <span class="mini-stat-icon">💎</span>
                          <span class="mini-stat-val">{{ playerDiamond }}</span>
                        </div>
                        <div class="mini-stat">
                          <span class="mini-stat-icon">🪙</span>
                          <span class="mini-stat-val">{{ playerGold }}</span>
                        </div>
                        <div class="mini-stat">
                          <span class="mini-stat-icon">⚡</span>
                          <span class="mini-stat-val">{{ playerStamina }}/120</span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="section-card">
                    <div class="section-title">{{ t("simverse.stats") }}</div>
                    <div class="stats-grid">
                      <div class="stat-item">
                        <div class="stat-icon hp">❤️</div>
                        <div class="stat-info">
                          <div class="stat-name">{{ t("simverse.hp") }}</div>
                          <div class="stat-bar">
                            <div class="stat-fill hp-fill" :style="{ width: playerHpPercent + '%' }"></div>
                          </div>
                          <div class="stat-val">{{ playerHp }} / {{ playerMaxHp }}</div>
                        </div>
                      </div>
                      <div class="stat-item">
                        <div class="stat-icon atk">⚔️</div>
                        <div class="stat-info">
                          <div class="stat-name">{{ t("simverse.attack") }}</div>
                          <div class="stat-val big">{{ playerAttack }}</div>
                        </div>
                      </div>
                      <div class="stat-item">
                        <div class="stat-icon def">🛡️</div>
                        <div class="stat-info">
                          <div class="stat-name">{{ t("simverse.defense") }}</div>
                          <div class="stat-val big">{{ playerDefense }}</div>
                        </div>
                      </div>
                      <div class="stat-item">
                        <div class="stat-icon exp">⭐</div>
                        <div class="stat-info">
                          <div class="stat-name">{{ t("simverse.exp") }}</div>
                          <div class="stat-bar">
                            <div class="stat-fill exp-fill" :style="{ width: expPercent + '%' }"></div>
                          </div>
                          <div class="stat-val">{{ playerExp }} / {{ expToNextLevel }}</div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="section-card">
                    <div class="section-title">{{ t("simverse.skills") }}</div>
                    <div class="skill-list">
                      <div v-for="skill in playerSkills" :key="skill.id" class="skill-item">
                        <div class="skill-icon">{{ skill.icon }}</div>
                        <div class="skill-info">
                          <div class="skill-name">{{ skill.name }}</div>
                          <div class="skill-level">Lv.{{ skill.level }}</div>
                        </div>
                        <div class="skill-rarity" :class="skill.rarity">{{ skill.rarity }}</div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activeSubPage === 'gacha'">
              <div class="gacha-page">
                <div class="page-header">
                  <button class="back-btn" @click="closeSubPage()">
                    <span>←</span>
                  </button>
                  <span class="page-title">{{ t("simverse.gachaTitle") }}</span>
                  <div class="header-spacer"></div>
                </div>
                <div class="page-content">
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
                </div>

                <transition name="gacha-flash">
                  <div v-if="isGachaAnimating" class="gacha-animation-overlay">
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
                </transition>
              </div>
            </template>

            <template v-else-if="activeSubPage === 'training'">
              <div class="training-page">
                <div class="page-header">
                  <button class="back-btn" @click="closeSubPage()">
                    <span>←</span>
                  </button>
                  <span class="page-title">{{ t("simverse.trainingTitle") }}</span>
                  <div class="header-spacer"></div>
                </div>
                <div class="page-content">
                  <div class="training-stamina-bar">
                    <span class="stamina-icon">⚡</span>
                    <div class="stamina-bar-wrap">
                      <div class="stamina-bar-fill" :style="{ width: (playerStamina / 120 * 100) + '%' }"></div>
                    </div>
                    <span class="stamina-text">{{ playerStamina }}/120</span>
                  </div>

                  <div class="training-section">
                    <div class="section-title">{{ t("simverse.train") }}</div>
                    <div class="training-grid">
                      <div v-for="mode in trainingModes" :key="mode.id"
                           class="training-card"
                           :class="mode.type"
                           @click="doTraining(mode)">
                        <div class="training-icon">{{ mode.icon }}</div>
                        <div class="training-name">{{ mode.name }}</div>
                        <div class="training-effect">{{ mode.effect }}</div>
                        <div class="training-cost">
                          <span class="cost-stamina">⚡ {{ mode.staminaCost }}</span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="training-section">
                    <div class="section-title">{{ t("simverse.equipment") }}</div>
                    <div class="equipment-grid">
                      <div v-for="equip in equipmentSlots" :key="equip.slot" class="equip-slot">
                        <div class="equip-slot-label">{{ equip.label }}</div>
                        <div class="equip-item" :class="{ empty: !equip.item }">
                          <span v-if="equip.item" class="equip-icon">{{ equip.item.icon }}</span>
                          <span v-else class="equip-empty">+</span>
                        </div>
                        <div v-if="equip.item" class="equip-name">{{ equip.item.name }}</div>
                        <div v-else class="equip-name empty">未装备</div>
                      </div>
                    </div>
                  </div>

                  <div class="training-section">
                    <div class="section-title">角色等级</div>
                    <div class="level-up-section">
                      <div class="level-info">
                        <span class="current-level">Lv.{{ playerLevel }}</span>
                        <div class="exp-bar-large">
                          <div class="exp-fill-large" :style="{ width: expPercent + '%' }"></div>
                        </div>
                        <span class="exp-text">{{ playerExp }} / {{ expToNextLevel }}</span>
                      </div>
                      <button class="level-up-btn" :disabled="playerExp < expToNextLevel" @click="levelUp">
                        {{ t("simverse.upgrade") }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activeSubPage === 'settings'">
              <div class="settings-page">
                <div class="page-header">
                  <button class="back-btn" @click="closeSubPage()">
                    <span>←</span>
                  </button>
                  <span class="page-title">{{ t("simverse.settings") }}</span>
                  <div class="header-spacer"></div>
                </div>
                <div class="page-content">
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
                </div>
              </div>
            </template>
          </div>
        </transition>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseNPC, type SimverseChronicleEvent } from "@/composables/useSimverse";
import { usePhaserWorld } from "@/composables/usePhaserWorld";
import { IonInfiniteScroll, IonInfiniteScrollContent } from "@ionic/vue";
import { lockScreenOrientation, unlockScreenOrientation, closeWorld, isNativePluginMode } from "@/plugins/SimVerse";

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
  loadNPCList,
  loadChronicleWorld,
  loadBehaviorStats,
  init,
  cleanup,
} = useSimverse();

const npcList = ref<SimverseNPC[]>([]);
const npcPage = ref(1);
const hasMoreNPCs = ref(true);
const recentEvents = ref<SimverseChronicleEvent[]>([]);
const activePanel = ref<string | null>(null);
const selectedNPC = ref<SimverseNPC | null>(null);
const gachaResults = ref<{ name: string; icon: string; rarity: string }[]>([]);
const behaviorStats = ref<{ total_npcs: number; alive_npcs: number; behavior_dist: Record<string, number> } | null>(null);

const activeSubPage = ref<string>('home');
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
  { id: 1, name: '烈焰斩', icon: '🔥', level: 3, rarity: 'SR' },
  { id: 2, name: '铁壁防御', icon: '🛡️', level: 2, rarity: 'R' },
  { id: 3, name: '疾风步', icon: '💨', level: 1, rarity: 'N' },
  { id: 4, name: '治愈术', icon: '💚', level: 1, rarity: 'SR' },
]);

const trainingModes = ref([
  { id: 'strength', name: '力量训练', icon: '💪', effect: '攻击 +2', staminaCost: 10, type: 'attack' },
  { id: 'defense', name: '防御训练', icon: '🛡️', effect: '防御 +2', staminaCost: 10, type: 'defense' },
  { id: 'endurance', name: '耐力训练', icon: '❤️', effect: '生命 +10', staminaCost: 15, type: 'hp' },
  { id: 'meditation', name: '冥想修炼', icon: '🧘', effect: '经验 +50', staminaCost: 20, type: 'exp' },
]);

const equipmentSlots = ref([
  { slot: 'weapon', label: '武器', item: { name: '新手剑', icon: '⚔️' } },
  { slot: 'armor', label: '护甲', item: { name: '布甲', icon: '🎽' } },
  { slot: 'accessory', label: '饰品', item: null },
  { slot: 'rune', label: '符文', item: null },
]);

const expToNextLevel = computed(() => playerLevel.value * 100);
const expPercent = computed(() => Math.min(100, (playerExp.value / expToNextLevel.value) * 100));
const playerHpPercent = computed(() => (playerHp.value / playerMaxHp.value) * 100);

function openSubPage(name: string) {
  activeSubPage.value = name;
}

function closeSubPage() {
  activeSubPage.value = 'home';
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
    { name: '普通村民', icon: '👤', rarity: 'N', weight: 61 },
    { name: '熟练工匠', icon: '🔨', rarity: 'R', weight: 30 },
    { name: '精英战士', icon: '⚔️', rarity: 'SR', weight: 8 },
    { name: '传奇英雄', icon: '👑', rarity: 'SSR', weight: 1 },
  ];

  for (let i = 0; i < count; i++) {
    const isGuaranteed = count === 10 && i === 9;
    let rarity: string;

    if (isGuaranteed) {
      rarity = pool.filter(p => ['SR', 'SSR'].includes(p.rarity))[
        Math.floor(Math.random() * 2)
      ].rarity;
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
    case 'attack':
      playerAttack.value += 1;
      break;
    case 'defense':
      playerDefense.value += 1;
      break;
    case 'hp':
      playerMaxHp.value += 5;
      playerHp.value += 5;
      break;
    case 'exp':
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

const visibleNPCs = computed(() => npcList.value.slice(0, 12));

const phaserContainerRef = ref<HTMLElement | null>(null);
const usePhaser = ref(true);
const phaserLoading = ref(true);
const phaserHasError = ref(false);

const phaserWorld = usePhaserWorld();

phaserWorld.onNPCClick((npc) => {
  selectedNPC.value = npc;
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

watch(npcList, (newList) => {
  if (usePhaser.value && phaserWorld.isReady.value) {
    phaserWorld.setNPCs(newList);
  }
}, { deep: true });

const panelOnLeft = computed(() => {
  const leftPanels = ['npc', 'org', 'economy', 'chronicles'];
  return leftPanels.includes(activePanel.value || '');
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
  idle: '😴',
  work: '💼',
  rest: '😌',
  eat: '🍽️',
  sleep: '💤',
  socialize: '💬',
  explore: '🚶',
  trade: '💰',
};

function getBehaviorIcon(behavior: string): string {
  return behaviorIcons[behavior] || '❓';
}

function getBehaviorClass(behavior: string): string {
  return `beh-${behavior}`;
}

async function refreshState() {
  await Promise.all([
    loadWorldState(),
    loadWorldConfig(),
  ]);
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

function selectNPC(npc: SimverseNPC) {
  selectedNPC.value = npc;
}

function openPanel(name: string) {
  if (activePanel.value === name) {
    activePanel.value = null;
  } else {
    activePanel.value = name;
    if (name === "npc" && npcList.value.length === 0) {
      loadNPCs();
    }
    if (name === "npc") {
      loadBehaviorStatsData();
    }
    if (name === "chronicles" && recentEvents.value.length === 0) {
      loadEvents();
    }
    if (name === "settings" && !worldConfig.value) {
      refreshState();
    }
  }
}

function getPanelTitle(panel: string): string {
  const titles: Record<string, string> = {
    npc: t("simverse.npc"),
    org: t("simverse.org"),
    gacha: t("simverse.gacha"),
    chronicles: t("simverse.chronicles"),
    settings: t("simverse.settings"),
    economy: t("simverse.economy"),
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
      rarity = pool.filter((p) => ["SR", "SSR", "UR"].includes(p.rarity))[
        Math.floor(Math.random() * 3)
      ].rarity;
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

    const matched = pool.filter((p) => p.rarity === rarity)[0];
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
      await unlockScreenOrientation();
      await closeWorld();
    } else {
      window.history.back();
    }
  } catch (e) {
    console.warn("[SimverseWorld] Exit world failed:", e);
    if (!isNativePluginMode()) {
      window.history.back();
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
}

onMounted(async () => {
  if (isNativePluginMode()) {
    lockScreenOrientation("landscape-primary").catch((e) => {
      console.warn("[SimverseWorld] Lock orientation failed:", e);
    });
    try {
      const { StatusBar } = await import("@capacitor/status-bar");
      await StatusBar.hide();
    } catch (e) {
      console.warn("[SimverseWorld] Hide status bar failed:", e);
    }
  }
  await init();
  await refreshState();
  await loadNPCs();
  startPolling();

  await nextTick();
  initPhaser();
});

onUnmounted(() => {
  stopPolling();
  cleanup();
  if (isNativePluginMode()) {
    unlockScreenOrientation().catch(() => {});
    import("@capacitor/status-bar").then(({ StatusBar, Style }) => {
      StatusBar.show().catch(() => {});
      StatusBar.setStyle({ style: Style.Default }).catch(() => {});
    }).catch(() => {});
  }
});
</script>

<style scoped>
.world-page {
  --background: #0a0a1a;
}

.world-content {
  --background: #0a0a1a;
  --padding-top: 0;
  --padding-bottom: 0;
  --offset-top: 0;
  --offset-bottom: 0;
}

.world-content :deep(.inner-scroll) {
  height: 100%;
  padding: 0 !important;
  overflow: hidden;
}

.game-container {
  position: relative;
  width: 100%;
  height: 100dvh;
  height: 100vh;
  overflow: hidden;
  box-sizing: border-box;
  background: 
    radial-gradient(ellipse at 30% 20%, rgba(124, 58, 237, 0.15) 0%, transparent 50%),
    radial-gradient(ellipse at 70% 80%, rgba(236, 72, 153, 0.1) 0%, transparent 50%),
    linear-gradient(180deg, #12122a 0%, #0a0a1a 100%);
}

.world-map {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  padding: 60px 100px 70px;
}

.phaser-container {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}

.phaser-container :deep(canvas) {
  display: block;
  width: 100% !important;
  height: 100% !important;
}

.phaser-loading {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  background: rgba(10, 10, 26, 0.9);
  z-index: 5;
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(139, 92, 246, 0.2);
  border-top-color: #8b5cf6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.loading-text {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.6);
}

.map-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  grid-template-rows: repeat(6, 1fr);
  gap: 4px;
  height: 100%;
  opacity: 0.5;
}

.map-cell {
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  transition: all 0.3s;
}

.map-cell.forest { background: rgba(34, 197, 94, 0.12); }
.map-cell.mountain { background: rgba(107, 114, 128, 0.18); }
.map-cell.water { background: rgba(59, 130, 246, 0.12); }
.map-cell.plain { background: rgba(234, 179, 8, 0.08); }
.map-cell.village { background: rgba(139, 92, 246, 0.12); }
.map-cell.city { background: rgba(236, 72, 153, 0.12); }
.map-cell.desert { background: rgba(249, 115, 22, 0.08); }

.map-overlay {
  position: absolute;
  top: 60px;
  left: 100px;
  right: 100px;
  bottom: 70px;
  pointer-events: none;
}

.npc-marker {
  position: absolute;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  pointer-events: auto;
  cursor: pointer;
  transition: transform 0.2s;
}

.npc-marker:hover {
  transform: translate(-50%, -50%) scale(1.2);
  z-index: 10;
}

.npc-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #6b7280;
  border: 2px solid rgba(255, 255, 255, 0.5);
  box-shadow: 0 0 8px rgba(0, 0, 0, 0.5);
}

.npc-dot.alive {
  background: #22c55e;
  animation: pulse 2s ease-in-out infinite;
}

.npc-dot.dead {
  background: #6b7280;
  animation: none;
}

.npc-dot.beh-work { background: #3b82f6; box-shadow: 0 0 8px rgba(59, 130, 246, 0.6); }
.npc-dot.beh-sleep { background: #8b5cf6; box-shadow: 0 0 8px rgba(139, 92, 246, 0.6); }
.npc-dot.beh-eat { background: #f97316; box-shadow: 0 0 8px rgba(249, 115, 22, 0.6); }
.npc-dot.beh-socialize { background: #ec4899; box-shadow: 0 0 8px rgba(236, 72, 153, 0.6); }
.npc-dot.beh-explore { background: #22c55e; box-shadow: 0 0 8px rgba(34, 197, 94, 0.6); }
.npc-dot.beh-trade { background: #eab308; box-shadow: 0 0 8px rgba(234, 179, 8, 0.6); }
.npc-dot.beh-rest { background: #6b7280; box-shadow: 0 0 6px rgba(107, 114, 128, 0.5); }
.npc-dot.beh-idle { background: #4b5563; box-shadow: 0 0 4px rgba(75, 85, 99, 0.4); }

@keyframes pulse {
  0%, 100% { box-shadow: 0 0 4px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 12px rgba(34, 197, 94, 0.8); }
}

.npc-name {
  font-size: 9px;
  color: rgba(255, 255, 255, 0.7);
  white-space: nowrap;
  margin-top: 2px;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.8);
}

.top-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.7) 0%, transparent 100%);
  z-index: 10;
}

.resource-group {
  display: flex;
  gap: 8px;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  padding: 6px 12px;
  border-radius: 20px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-bottom: 3px solid rgba(139, 92, 246, 0.3);
}

.resource-icon {
  font-size: 14px;
}

.resource-value {
  font-size: 13px;
  font-weight: 700;
  color: #fff;
  font-variant-numeric: tabular-nums;
}

.top-actions {
  display: flex;
  gap: 6px;
}

.game-btn {
  width: 38px;
  height: 38px;
  border-radius: 50%;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.25);
  border-bottom: 3px solid rgba(139, 92, 246, 0.35);
  color: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
  padding: 0;
}

.game-btn:hover {
  background: rgba(139, 92, 246, 0.2);
  transform: translateY(-1px);
}

.game-btn:active {
  transform: translateY(2px);
  border-bottom-width: 1px;
}

.game-btn .btn-icon {
  font-size: 14px;
  line-height: 1;
}

.game-btn.play-btn.running {
  background: rgba(34, 197, 94, 0.2);
  border-color: rgba(34, 197, 94, 0.4);
  border-bottom-color: rgba(34, 197, 94, 0.5);
}

.left-menu,
.right-menu {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 10;
  padding: 10px 8px;
}

.left-menu {
  left: 10px;
}

.right-menu {
  right: 10px;
}

.menu-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-bottom: 3px solid rgba(139, 92, 246, 0.3);
  border-radius: 12px;
  cursor: pointer;
  padding: 10px 8px;
  min-width: 56px;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
}

.menu-btn:hover {
  background: rgba(139, 92, 246, 0.2);
  transform: translateY(-1px);
}

.menu-btn:active {
  transform: translateY(2px);
  border-bottom-width: 1px;
}

.menu-btn.active {
  background: rgba(139, 92, 246, 0.3);
  border-color: rgba(139, 92, 246, 0.6);
  border-bottom-color: rgba(139, 92, 246, 0.7);
}

.menu-icon {
  font-size: 22px;
  filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.3));
  line-height: 1;
}

.menu-icon.sparkle {
  animation: sparkle 2s ease-in-out infinite;
}

@keyframes sparkle {
  0%, 100% {
    transform: scale(1);
    filter: drop-shadow(0 2px 4px rgba(255, 215, 0, 0.3));
  }
  50% {
    transform: scale(1.1);
    filter: drop-shadow(0 2px 8px rgba(255, 215, 0, 0.6));
  }
}

.menu-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.menu-btn.gacha-btn .menu-label {
  color: #ffd700;
  font-weight: 600;
}

.menu-btn.exit-btn .menu-label {
  color: #ef4444;
}

.menu-divider {
  width: 40px;
  height: 1px;
  background: rgba(139, 92, 246, 0.2);
  margin: 4px auto;
}

.stats-bar {
  position: absolute;
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 8px;
  z-index: 9;
}

.stat-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(20, 20, 40, 0.7);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 16px;
  padding: 5px 12px;
}

.stat-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
}

.stat-value {
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  font-variant-numeric: tabular-nums;
}

.event-ticker {
  position: absolute;
  top: 58px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-radius: 20px;
  padding: 6px 14px;
  z-index: 9;
  max-width: 400px;
}

.ticker-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.ticker-content {
  overflow: hidden;
  flex: 1;
  min-width: 0;
}

.ticker-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ticker-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ticker-enter-active,
.ticker-leave-active {
  transition: all 0.3s ease;
}

.ticker-enter-from {
  opacity: 0;
  transform: translateY(-10px);
}

.ticker-leave-to {
  opacity: 0;
  transform: translateY(10px);
}

.event-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #4ade80;
  flex-shrink: 0;
}

.event-dot.warning { background: #f59e0b; }
.event-dot.danger { background: #ef4444; }

.event-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.side-panel {
  position: absolute;
  top: 50px;
  bottom: 60px;
  width: 300px;
  background: rgba(15, 15, 35, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  z-index: 15;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  right: 80px;
  transform: translateX(20px);
  opacity: 0;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none;
}

.side-panel.panel-left {
  left: 80px;
  right: auto;
  transform: translateX(-20px);
}

.side-panel.open {
  transform: translateX(0);
  opacity: 1;
  pointer-events: auto;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(139, 92, 246, 0.1);
  flex-shrink: 0;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.1) 0%, rgba(236, 72, 153, 0.05) 100%);
}

.panel-title {
  font-size: 15px;
  font-weight: 700;
  color: #fff;
}

.panel-close-btn {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.08);
  border: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.panel-close-btn:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.panel-content::-webkit-scrollbar {
  width: 5px;
}

.panel-content::-webkit-scrollbar-track {
  background: transparent;
}

.panel-content::-webkit-scrollbar-thumb {
  background: rgba(139, 92, 246, 0.25);
  border-radius: 3px;
}

.list-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 12px;
  margin-bottom: 8px;
  border: 1px solid rgba(139, 92, 246, 0.1);
  transition: all 0.2s;
  cursor: pointer;
}

.list-card:hover {
  background: rgba(139, 92, 246, 0.1);
  border-color: rgba(139, 92, 246, 0.25);
  transform: translateX(2px);
}

.card-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 16px;
  flex-shrink: 0;
  border: 2px solid rgba(255, 255, 255, 0.1);
}

.card-info {
  flex: 1;
  min-width: 0;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 4px;
}

.card-subtitle {
  display: flex;
  gap: 6px;
  align-items: center;
}

.prof-tag,
.level-tag {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 6px;
  font-weight: 500;
}

.prof-tag {
  background: rgba(139, 92, 246, 0.2);
  color: #c4b5fd;
  text-transform: capitalize;
}

.level-tag {
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
}

.card-behavior {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
}

.behavior-icon {
  font-size: 11px;
}

.behavior-text {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
}

.behavior-stats-bar {
  background: rgba(139, 92, 246, 0.1);
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 12px;
}

.stats-title {
  font-size: 11px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 8px;
}

.behavior-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.behavior-chip {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  padding: 3px 8px;
  font-size: 10px;
}

.behavior-chip .chip-icon {
  font-size: 12px;
}

.behavior-chip .chip-count {
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
}

.behavior-chip.work { background: rgba(59, 130, 246, 0.15); border-color: rgba(59, 130, 246, 0.3); }
.behavior-chip.sleep { background: rgba(139, 92, 246, 0.15); border-color: rgba(139, 92, 246, 0.3); }
.behavior-chip.eat { background: rgba(249, 115, 22, 0.15); border-color: rgba(249, 115, 22, 0.3); }
.behavior-chip.socialize { background: rgba(236, 72, 153, 0.15); border-color: rgba(236, 72, 153, 0.3); }
.behavior-chip.explore { background: rgba(34, 197, 94, 0.15); border-color: rgba(34, 197, 94, 0.3); }
.behavior-chip.trade { background: rgba(234, 179, 8, 0.15); border-color: rgba(234, 179, 8, 0.3); }
.behavior-chip.rest { background: rgba(107, 114, 128, 0.15); border-color: rgba(107, 114, 128, 0.3); }
.behavior-chip.idle { background: rgba(107, 114, 128, 0.1); border-color: rgba(107, 114, 128, 0.2); }

.card-action {
  width: 50px;
  flex-shrink: 0;
}

.mini-hp-bar {
  width: 100%;
  height: 5px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  overflow: hidden;
}

.mini-hp-fill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #4ade80);
  border-radius: 3px;
  transition: width 0.3s;
}

.empty-state {
  text-align: center;
  padding: 30px 16px;
  color: rgba(255, 255, 255, 0.3);
  font-size: 12px;
}

.chronicle-card {
  padding: 12px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 12px;
  margin-bottom: 8px;
  border-left: 3px solid #8b5cf6;
  border: 1px solid rgba(139, 92, 246, 0.1);
  border-left: 3px solid #8b5cf6;
}

.chronicle-tick {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  margin-bottom: 4px;
  font-variant-numeric: tabular-nums;
}

.chronicle-title {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 4px;
}

.chronicle-desc {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.4;
}

.setting-section {
  margin-bottom: 20px;
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 10px;
  padding-left: 2px;
  letter-spacing: 0.3px;
}

.tier-selector {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tier-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.tier-option:hover {
  background: rgba(139, 92, 246, 0.1);
  border-color: rgba(139, 92, 246, 0.35);
}

.tier-option.active {
  background: rgba(139, 92, 246, 0.2);
  border-color: rgba(139, 92, 246, 0.6);
  color: #fff;
}

.tier-name {
  font-weight: 600;
}

.tier-desc {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
}

.tier-option.active .tier-desc {
  color: rgba(255, 255, 255, 0.6);
}

.config-list {
  background: rgba(255, 255, 255, 0.03);
  border-radius: 10px;
  border: 1px solid rgba(139, 92, 246, 0.1);
  overflow: hidden;
}

.config-row {
  display: flex;
  justify-content: space-between;
  padding: 10px 14px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(139, 92, 246, 0.05);
}

.config-row:last-child {
  border-bottom: none;
}

.config-value {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.gacha-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.25), rgba(236, 72, 153, 0.2));
  border-radius: 14px;
  padding: 16px;
  border: 1px solid rgba(139, 92, 246, 0.35);
  margin-bottom: 14px;
}

.banner-icon {
  font-size: 36px;
  flex-shrink: 0;
}

.banner-text {
  flex: 1;
  min-width: 0;
}

.banner-title {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 2px;
}

.banner-desc {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.4;
}

.gacha-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 14px;
}

.gacha-action-btn {
  position: relative;
  padding: 14px 16px;
  border-radius: 12px;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
  border-bottom: 4px solid rgba(0, 0, 0, 0.2);
}

.gacha-action-btn.single {
  background: linear-gradient(135deg, #3b82f6, #6366f1);
}

.gacha-action-btn.ten {
  background: linear-gradient(135deg, #f59e0b, #ef4444);
}

.gacha-action-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.1);
}

.gacha-action-btn:active {
  transform: translateY(3px);
  border-bottom-width: 1px;
}

.action-icon {
  font-size: 24px;
  flex-shrink: 0;
}

.action-name {
  flex: 1;
  text-align: left;
  font-size: 14px;
  font-weight: 700;
  color: #fff;
}

.action-cost {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.95);
  font-weight: 600;
}

.action-badge {
  position: absolute;
  top: -6px;
  right: 12px;
  background: #ffd700;
  color: #1a1a1a;
  font-size: 9px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 8px;
}

.gacha-results {
  margin-top: 4px;
}

.results-header {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 10px;
}

.results-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
}

.result-item {
  aspect-ratio: 3 / 4;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(139, 92, 246, 0.15);
  animation: cardReveal 0.4s ease-out;
  padding: 6px 4px;
}

@keyframes cardReveal {
  from { opacity: 0; transform: scale(0.8) rotateY(90deg); }
  to { opacity: 1; transform: scale(1) rotateY(0); }
}

.result-item .result-icon { font-size: 22px; }

.result-item .result-name {
  font-size: 8px;
  color: rgba(255, 255, 255, 0.6);
  text-align: center;
  line-height: 1.2;
}

.result-item .result-rarity {
  font-size: 9px;
  font-weight: 700;
}

.result-item.N .result-rarity { color: #9ca3af; }
.result-item.R .result-rarity { color: #3b82f6; }
.result-item.SR .result-rarity { color: #a78bfa; }
.result-item.SSR .result-rarity { color: #fbbf24; }
.result-item.UR .result-rarity { color: #f87171; }

.detail-modal {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
  backdrop-filter: blur(4px);
}

.detail-card {
  width: 85%;
  max-width: 360px;
  background: rgba(15, 15, 35, 0.98);
  border-radius: 18px;
  border: 1px solid rgba(139, 92, 246, 0.25);
  overflow: hidden;
  animation: modalIn 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes modalIn {
  from {
    opacity: 0;
    transform: scale(0.9) translateY(20px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.detail-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.2), rgba(236, 72, 153, 0.15));
  border-bottom: 1px solid rgba(139, 92, 246, 0.1);
  position: relative;
}

.detail-avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 24px;
  flex-shrink: 0;
  border: 3px solid rgba(255, 255, 255, 0.15);
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-name {
  font-size: 20px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 4px;
}

.detail-meta {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.detail-close {
  position: absolute;
  top: 14px;
  right: 14px;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  border: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.detail-close:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.detail-body {
  padding: 16px 18px 18px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px 16px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
}

.item-value {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.item-value.highlight {
  color: #fbbf24;
  font-weight: 600;
}

.item-value.success {
  color: #4ade80;
}

.item-value.warning {
  color: #fbbf24;
}

.item-value.alive { color: #4ade80; }
.item-value.dead { color: #f87171; }

.explore-banner,
.battle-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  margin-bottom: 16px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.2), rgba(236, 72, 153, 0.2));
  border-radius: 12px;
  border: 1px solid rgba(99, 102, 241, 0.3);
}

.battle-banner {
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.2), rgba(249, 115, 22, 0.2));
  border-color: rgba(239, 68, 68, 0.3);
}

.explore-banner .banner-icon,
.battle-banner .banner-icon {
  font-size: 36px;
}

.explore-banner .banner-title,
.battle-banner .banner-title {
  font-size: 18px;
  font-weight: 600;
  color: #ffffff;
  margin-bottom: 2px;
}

.explore-banner .banner-desc,
.battle-banner .banner-desc {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
}

.region-list,
.battle-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.region-card,
.enemy-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.region-card:hover,
.enemy-card:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(99, 102, 241, 0.5);
  transform: translateX(4px);
}

.enemy-card:hover {
  border-color: rgba(239, 68, 68, 0.5);
}

.region-icon,
.enemy-icon {
  font-size: 32px;
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(99, 102, 241, 0.2);
  border-radius: 10px;
}

.enemy-icon {
  background: rgba(239, 68, 68, 0.2);
}

.region-info,
.enemy-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.region-name,
.enemy-name {
  font-size: 15px;
  font-weight: 600;
  color: #ffffff;
}

.region-type,
.enemy-level {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.enemy-level {
  color: #fbbf24;
  font-weight: 600;
}

.region-arrow {
  font-size: 20px;
  color: rgba(255, 255, 255, 0.4);
}

.fight-btn {
  padding: 8px 16px;
  background: linear-gradient(135deg, #ef4444, #f97316);
  color: #ffffff;
  border: none;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.2s ease;
}

.fight-btn:hover {
  transform: scale(1.05);
}

.explore-btn {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.3), rgba(139, 92, 246, 0.3));
  border: 1px solid rgba(99, 102, 241, 0.4);
}

.explore-btn.active {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.5), rgba(139, 92, 246, 0.5));
  box-shadow: 0 0 20px rgba(99, 102, 241, 0.4);
}

.battle-btn {
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.3), rgba(249, 115, 22, 0.3));
  border: 1px solid rgba(239, 68, 68, 0.4);
}

.battle-btn.active {
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.5), rgba(249, 115, 22, 0.5));
  box-shadow: 0 0 20px rgba(239, 68, 68, 0.4);
}

.bottom-nav {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 70px;
  display: flex;
  justify-content: space-around;
  align-items: center;
  background: linear-gradient(180deg, transparent 0%, rgba(10, 10, 26, 0.95) 30%);
  backdrop-filter: blur(20px);
  border-top: 1px solid rgba(139, 92, 246, 0.2);
  z-index: 15;
  padding-bottom: 8px;
}

.nav-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: none;
  color: rgba(255, 255, 255, 0.6);
  padding: 6px 12px;
  cursor: pointer;
  transition: all 0.2s ease;
  position: relative;
}

.nav-item.active {
  color: #8b5cf6;
}

.nav-item.active .nav-icon {
  transform: scale(1.15);
}

.nav-icon {
  font-size: 22px;
  transition: transform 0.2s ease;
}

.nav-label {
  font-size: 10px;
  font-weight: 500;
}

.gacha-nav .nav-icon {
  animation: sparkle-pulse 2s ease-in-out infinite;
}

@keyframes sparkle-pulse {
  0%, 100% { filter: brightness(1); }
  50% { filter: brightness(1.5) drop-shadow(0 0 8px #fbbf24); }
}

.full-screen-page {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: linear-gradient(180deg, #0f0f2a 0%, #0a0a1a 100%);
  z-index: 20;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.page-slide-enter-active,
.page-slide-leave-active {
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.page-slide-enter-from {
  transform: translateY(100%);
}

.page-slide-leave-to {
  transform: translateY(100%);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  background: rgba(15, 15, 42, 0.95);
  backdrop-filter: blur(20px);
  border-bottom: 1px solid rgba(139, 92, 246, 0.15);
  flex-shrink: 0;
}

.back-btn {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(139, 92, 246, 0.15);
  border: 1px solid rgba(139, 92, 246, 0.25);
  color: #fff;
  font-size: 18px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
}

.back-btn:hover {
  background: rgba(139, 92, 246, 0.3);
  transform: translateX(-2px);
}

.page-title {
  font-size: 17px;
  font-weight: 700;
  color: #fff;
}

.header-spacer {
  width: 36px;
}

.page-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.profile-page {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.player-card {
  position: relative;
  border-radius: 16px;
  overflow: hidden;
  margin-bottom: 16px;
}

.card-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: linear-gradient(135deg, #1e1b4b 0%, #312e81 50%, #4c1d95 100%);
}

.card-bg::before {
  content: '';
  position: absolute;
  top: -50%;
  right: -20%;
  width: 300px;
  height: 300px;
  background: radial-gradient(circle, rgba(139, 92, 246, 0.3) 0%, transparent 70%);
  border-radius: 50%;
}

.card-content {
  position: relative;
  z-index: 1;
  padding: 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.player-avatar {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  border: 3px solid rgba(255, 255, 255, 0.2);
  box-shadow: 0 0 30px rgba(139, 92, 246, 0.5);
}

.avatar-icon {
  font-size: 40px;
}

.player-name {
  font-size: 22px;
  font-weight: 700;
  color: #fff;
}

.player-title {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.7);
}

.player-stats-row {
  display: flex;
  gap: 16px;
  margin-top: 8px;
}

.mini-stat {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(0, 0, 0, 0.25);
  padding: 6px 12px;
  border-radius: 20px;
}

.mini-stat-icon {
  font-size: 14px;
}

.mini-stat-val {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
}

.section-card {
  background: rgba(20, 20, 50, 0.6);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 12px;
  padding: 16px;
  margin-bottom: 16px;
}

.section-title {
  font-size: 15px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 12px;
}

.stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
}

.stat-icon.hp { background: rgba(239, 68, 68, 0.2); }
.stat-icon.atk { background: rgba(249, 115, 22, 0.2); }
.stat-icon.def { background: rgba(59, 130, 246, 0.2); }
.stat-icon.exp { background: rgba(234, 179, 8, 0.2); }

.stat-info {
  flex: 1;
  min-width: 0;
}

.stat-name {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 2px;
}

.stat-bar {
  height: 6px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  overflow: hidden;
}

.stat-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}

.hp-fill { background: linear-gradient(90deg, #ef4444, #f87171); }
.exp-fill { background: linear-gradient(90deg, #eab308, #fde047); }

.stat-val {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
  margin-top: 2px;
}

.stat-val.big {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
  margin-top: 0;
}

.skill-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.skill-item {
  display: flex;
  align-items: center;
  gap: 12px;
  background: rgba(0, 0, 0, 0.2);
  padding: 10px 12px;
  border-radius: 10px;
}

.skill-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: rgba(139, 92, 246, 0.15);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.skill-info {
  flex: 1;
}

.skill-name {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
}

.skill-level {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
}

.skill-rarity {
  font-size: 10px;
  font-weight: 700;
  padding: 3px 8px;
  border-radius: 4px;
}

.skill-rarity.SR { background: rgba(234, 179, 8, 0.2); color: #eab308; }
.skill-rarity.R { background: rgba(59, 130, 246, 0.2); color: #3b82f6; }
.skill-rarity.N { background: rgba(156, 163, 175, 0.2); color: #9ca3af; }

.gacha-page {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
}

.gacha-banner-large {
  position: relative;
  height: 180px;
  border-radius: 16px;
  overflow: hidden;
  margin-bottom: 16px;
}

.banner-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: linear-gradient(135deg, #581c87 0%, #be185d 50%, #ea580c 100%);
}

.banner-bg::after {
  content: '';
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, transparent 0%, rgba(0, 0, 0, 0.4) 100%);
}

.banner-content {
  position: relative;
  z-index: 2;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.banner-icon-large {
  font-size: 48px;
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-8px); }
}

.banner-title-large {
  font-size: 24px;
  font-weight: 800;
  color: #fff;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.5);
}

.banner-desc-large {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.8);
}

.sparkle-layer {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  z-index: 1;
}

.sparkle {
  position: absolute;
  color: #fde68a;
  font-size: 14px;
  animation: sparkle-rotate 4s linear infinite;
  transform-origin: center;
}

@keyframes sparkle-rotate {
  from { opacity: 0.3; }
  50% { opacity: 1; }
  to { opacity: 0.3; }
}

.gacha-pool-info {
  display: flex;
  justify-content: space-around;
  background: rgba(20, 20, 50, 0.6);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 12px;
  padding: 12px;
  margin-bottom: 16px;
}

.pool-rate {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.rate-label {
  font-size: 11px;
  font-weight: 700;
  background: linear-gradient(135deg, #eab308, #f97316);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.rate-val {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
}

.gacha-actions-large {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
}

.gacha-big-btn {
  flex: 1;
  border: none;
  border-radius: 14px;
  padding: 16px 12px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  transition: all 0.2s ease;
  position: relative;
}

.gacha-big-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.gacha-big-btn.single {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.2), rgba(139, 92, 246, 0.2));
  border: 2px solid rgba(59, 130, 246, 0.4);
}

.gacha-big-btn.ten {
  background: linear-gradient(135deg, rgba(234, 179, 8, 0.2), rgba(249, 115, 22, 0.2));
  border: 2px solid rgba(234, 179, 8, 0.4);
}

.gacha-big-btn:not(:disabled):hover {
  transform: translateY(-2px);
}

.gacha-big-btn:not(:disabled):active {
  transform: translateY(1px);
}

.btn-icon-large {
  font-size: 28px;
}

.btn-name {
  font-size: 13px;
  font-weight: 700;
  color: #fff;
}

.btn-cost {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
}

.btn-badge {
  position: absolute;
  top: -8px;
  right: 8px;
  background: linear-gradient(135deg, #ef4444, #f97316);
  color: #fff;
  font-size: 9px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 8px;
}

.gacha-history {
  background: rgba(20, 20, 50, 0.6);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 12px;
  padding: 12px;
}

.history-title {
  font-size: 13px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 10px;
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.history-item {
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(0, 0, 0, 0.2);
  padding: 8px 10px;
  border-radius: 8px;
}

.hist-icon {
  font-size: 18px;
}

.hist-name {
  flex: 1;
  font-size: 12px;
  color: #fff;
}

.hist-rarity {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 4px;
}

.hist-rarity.SSR { background: rgba(239, 68, 68, 0.2); color: #ef4444; }
.hist-rarity.SR { background: rgba(234, 179, 8, 0.2); color: #eab308; }
.hist-rarity.R { background: rgba(59, 130, 246, 0.2); color: #3b82f6; }
.hist-rarity.N { background: rgba(156, 163, 175, 0.2); color: #9ca3af; }

.gacha-animation-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.9);
  z-index: 50;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.gacha-flash-enter-active,
.gacha-flash-leave-active {
  transition: opacity 0.3s ease;
}

.gacha-flash-enter-from,
.gacha-flash-leave-to {
  opacity: 0;
}

.gacha-cards-container {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: center;
  max-width: 360px;
  padding: 20px;
}

.gacha-card-anim {
  width: 80px;
  height: 110px;
  perspective: 1000px;
}

.gacha-card-anim.revealed {
  animation: card-pop 0.5s ease forwards;
}

@keyframes card-pop {
  0% { transform: scale(1); }
  50% { transform: scale(1.1); }
  100% { transform: scale(1); }
}

.card-inner {
  position: relative;
  width: 100%;
  height: 100%;
  transform-style: preserve-3d;
  transition: transform 0.8s cubic-bezier(0.4, 0, 0.2, 1);
}

.gacha-card-anim.revealed .card-inner {
  transform: rotateY(180deg);
}

.card-front,
.card-back {
  position: absolute;
  inset: 0;
  backface-visibility: hidden;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.card-front {
  background: linear-gradient(135deg, #1e1b4b, #312e81);
  border: 2px solid rgba(139, 92, 246, 0.5);
}

.card-back-icon {
  font-size: 36px;
  color: rgba(139, 92, 246, 0.6);
}

.card-back {
  transform: rotateY(180deg);
  background: linear-gradient(180deg, rgba(20, 20, 50, 0.95), rgba(30, 27, 75, 0.95));
  padding: 8px;
  text-align: center;
}

.gacha-card-anim.SSR .card-back {
  border: 2px solid #ef4444;
  box-shadow: 0 0 20px rgba(239, 68, 68, 0.5);
}

.gacha-card-anim.SR .card-back {
  border: 2px solid #eab308;
  box-shadow: 0 0 15px rgba(234, 179, 8, 0.4);
}

.gacha-card-anim.R .card-back {
  border: 2px solid #3b82f6;
}

.gacha-item-icon {
  font-size: 30px;
}

.gacha-item-name {
  font-size: 10px;
  color: #fff;
  font-weight: 600;
}

.gacha-item-rarity {
  font-size: 9px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 3px;
}

.gacha-item-rarity.SSR { background: rgba(239, 68, 68, 0.3); color: #f87171; }
.gacha-item-rarity.SR { background: rgba(234, 179, 8, 0.3); color: #fde047; }
.gacha-item-rarity.R { background: rgba(59, 130, 246, 0.3); color: #93c5fd; }
.gacha-item-rarity.N { background: rgba(156, 163, 175, 0.3); color: #d1d5db; }

.gacha-skip-btn {
  margin-top: 20px;
  padding: 10px 30px;
  background: rgba(139, 92, 246, 0.2);
  border: 1px solid rgba(139, 92, 246, 0.4);
  border-radius: 20px;
  color: #fff;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.gacha-skip-btn:hover {
  background: rgba(139, 92, 246, 0.4);
}

.training-page {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.training-stamina-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(20, 20, 50, 0.6);
  border: 1px solid rgba(234, 179, 8, 0.2);
  border-radius: 12px;
  padding: 12px 16px;
  margin-bottom: 16px;
}

.stamina-icon {
  font-size: 18px;
}

.stamina-bar-wrap {
  flex: 1;
  height: 12px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  overflow: hidden;
}

.stamina-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #eab308, #fde047);
  border-radius: 6px;
  transition: width 0.3s ease;
}

.stamina-text {
  font-size: 13px;
  font-weight: 600;
  color: #fde047;
  min-width: 60px;
  text-align: right;
}

.training-section {
  margin-bottom: 20px;
}

.training-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.training-card {
  background: rgba(20, 20, 50, 0.6);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 12px;
  padding: 16px;
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.training-card:hover {
  transform: translateY(-2px);
  border-color: rgba(139, 92, 246, 0.4);
}

.training-card:active {
  transform: translateY(0);
}

.training-icon {
  font-size: 32px;
}

.training-name {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
}

.training-effect {
  font-size: 11px;
  color: rgba(34, 197, 94, 0.9);
}

.training-cost {
  font-size: 11px;
  color: rgba(234, 179, 8, 0.9);
}

.equipment-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}

.equip-slot {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.equip-slot-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
}

.equip-item {
  width: 50px;
  height: 50px;
  border-radius: 12px;
  background: rgba(139, 92, 246, 0.1);
  border: 2px solid rgba(139, 92, 246, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
}

.equip-item.empty {
  border-style: dashed;
  border-color: rgba(255, 255, 255, 0.2);
}

.equip-icon {
  font-size: 24px;
}

.equip-empty {
  font-size: 20px;
  color: rgba(255, 255, 255, 0.2);
}

.equip-name {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.7);
  text-align: center;
}

.equip-name.empty {
  color: rgba(255, 255, 255, 0.3);
}

.level-up-section {
  display: flex;
  align-items: center;
  gap: 16px;
}

.level-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.current-level {
  font-size: 18px;
  font-weight: 700;
  color: #8b5cf6;
}

.exp-bar-large {
  height: 10px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 5px;
  overflow: hidden;
}

.exp-fill-large {
  height: 100%;
  background: linear-gradient(90deg, #eab308, #fde047);
  border-radius: 5px;
  transition: width 0.3s ease;
}

.exp-text {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
}

.level-up-btn {
  padding: 12px 24px;
  background: linear-gradient(135deg, #8b5cf6, #6366f1);
  border: none;
  border-radius: 25px;
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.level-up-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.level-up-btn:not(:disabled):hover {
  transform: scale(1.05);
  box-shadow: 0 0 20px rgba(139, 92, 246, 0.5);
}

.settings-page {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}
</style>
