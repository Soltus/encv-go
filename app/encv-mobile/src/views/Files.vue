<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button v-if="currentPath !== '/d'" @click="goUp">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button fill="clear" @click="openSideDrawer()">
            <ion-icon :icon="menuOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t('files.title') }}</ion-title>
      </ion-toolbar>
      <ion-toolbar v-if="currentPath !== '/d' && !isSearching">
        <div class="breadcrumb-scroll">
          <div class="breadcrumb">
            <span class="breadcrumb-item" @click="navigateTo('/d')">{{ t('files.mountRoot') || '挂载点' }}</span>
            <span v-for="(segment, index) in pathSegments" :key="index" class="breadcrumb-segment">
              <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
              <span class="breadcrumb-item" @click="navigateTo(segment.path)">{{ segment.name }}</span>
            </span>
          </div>
        </div>
      </ion-toolbar>
      <ion-toolbar>
        <!-- 🆕 2026-07-02 v2 重写：contenteditable div + span units（替换 ion-searchbar） -->
        <!-- 用户强反馈：之前 ion-searchbar + 外部 overlay div 是"在输入框外高亮"，不符合预期 -->
        <!-- 现在高亮在 input 内部（每个 token 都是 <span>），插入按钮插入 symbol span（&/｜/￢） -->
        <div class="search-input-wrapper">
          <div
            ref="queryInputRef"
            class="query-input"
            contenteditable="true"
            data-testid="search-input"
            :data-empty="!searchQuery"
            :placeholder="t('files.searchPlaceholder')"
            @input="onQueryInput($event)"
            @keydown="onQueryKeydown($event)"
            @focus="onQueryFocus"
            @blur="onQueryBlur"
            role="searchbox"
            aria-label="Search"
          ></div>
          <button
            v-if="searchQuery"
            class="query-clear-btn"
            type="button"
            :title="t('files.clear') || '清空'"
            @click="handleSearchClear"
          >×</button>
        </div>
        <!-- 🆕 2026-07-02 v2 插入操作符按钮行：点击即在光标位置插入 symbol span（不是字符串） -->
        <div v-if="searchQuery" class="search-insert-bar">
          <span class="insert-label">{{ t('files.insertOp') || '插入:' }}</span>
          <button class="insert-btn op-and" data-testid="btn-and" @click="insertOperator('AND')" type="button" :title="t('files.insertAndTitle')">＆</button>
          <button class="insert-btn op-or" data-testid="btn-or" @click="insertOperator('OR')" type="button" :title="t('files.insertOrTitle')">｜</button>
          <button class="insert-btn op-not" data-testid="btn-not" @click="insertOperator('NOT')" type="button" :title="t('files.insertNotTitle')">￢</button>
          <button class="insert-btn op-phrase" data-testid="btn-phrase" @click="insertOperator('__phrase_open__')" type="button" :title="t('files.insertPhraseTitle')">「」</button>
          <button class="insert-btn op-regex" data-testid="btn-regex" @click="insertOperator('__regex_prefix__')" type="button" :title="t('files.insertRegexTitle')">/ /</button>
        </div>
        <ion-toggle
          v-if="searchQuery"
          slot="end"
          v-model="searchFullText"
          @ionChange="handleSearchToggle"
          class="recursive-toggle"
        >
          {{ t('files.fullText') || '全文' }}
        </ion-toggle>
      </ion-toolbar>
    </ion-header>

    <ion-menu side="start" menu-id="plugin-menu" content-id="main-content">
      <ion-header>
        <ion-toolbar>
          <ion-title>插件分类</ion-title>
        </ion-toolbar>
      </ion-header>
      <ion-content>
        <ion-list>
          <ion-item button @click="exitPluginMode()" detail>
            <ion-icon :icon="folder" slot="start" color="primary"></ion-icon>
            <ion-label><h2>所有文件</h2><p>{{ currentPath || '/' }}</p></ion-label>
          </ion-item>
          <ion-list-header>文件类型</ion-list-header>
          <ion-item v-for="plugin in plugins" :key="plugin.name" button detail @click="openPluginView(plugin)">
            <ion-icon :icon="getPluginIcon(plugin)" slot="start" color="primary" />
            <ion-label>
              <h2>{{ plugin.name }}</h2>
              <p>{{ plugin.supportedExtensions?.length ?? 0 }} 种格式 · 容器 {{ plugin.containerExtension }}</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <ion-list v-if="tags.length > 0" style="margin-top: 16px">
          <ion-list-header>标签</ion-list-header>
          <ion-item v-for="tag in tags" :key="tag.name" button @click="handleTagFilter(tag.name)">
            <ion-icon :icon="pricetagOutline" slot="start" color="success" />
            <ion-label>
              <h2>{{ tag.name }}</h2>
              <p>{{ tag.count }} 个文件</p>
            </ion-label>
          </ion-item>
        </ion-list>
      </ion-content>
    </ion-menu>

    <ion-content id="main-content" ref="mainContentRef">
      <ion-refresher slot="fixed" @ionRefresh="handleRefresh" v-if="!searchQuery">
        <ion-refresher-content></ion-refresher-content>
      </ion-refresher>

      <!-- 🆕 v4 Bug1 修复：自动更新顶栏细 indicator（不阻塞老数据渲染，不清空列表） -->
      <div v-if="refreshing" class="files-refresh-bar" aria-live="polite">
        <ion-spinner name="dots" class="files-refresh-bar__spinner"></ion-spinner>
        <span class="files-refresh-bar__label">{{ t('files.autoRefreshing') }}</span>
      </div>

      <!-- 播放错误展示区域 -->
      <div v-if="playError" class="play-error-banner">
        <div class="play-error-header">
          <ion-icon :icon="alertCircle" color="danger"></ion-icon>
          <span class="play-error-file">{{ playErrorFile }}</span>
          <ion-button fill="clear" size="small" color="medium" @click="clearPlayError">
            <ion-icon :icon="close" slot="icon-only"></ion-icon>
          </ion-button>
        </div>
        <p class="play-error-message">{{ playError }}</p>
        <div v-if="playErrorDetail" class="play-error-detail-row">
          <ion-button fill="clear" size="small" color="medium" @click="togglePlayErrorDetail">
            {{ t('common.showDetail') }}
          </ion-button>
        </div>
      </div>

      <template v-if="(loading || isSearching || noPermission || !serverOnline || displayFiles.length === 0) && !selectedPlugin">
        <div v-if="loading || isSearching" class="loading-container">
          <!--
            搜索中 loading 三态区分（debug-discipline.md §1.6）：
              - 'available'   → spinner（旋转，索引完全可用）
              - 'degraded'    → dots（安静等待，部分降级）
              - 'unavailable' / 'unknown' → pulse（脉冲闪动，索引不可用，仅关键词）
          -->
          <ion-spinner v-if="isSearching && vectorSearchStatus === 'available'" name="crescent"></ion-spinner>
          <ion-spinner v-else-if="isSearching && vectorSearchStatus === 'degraded'" name="dots"></ion-spinner>
          <ion-spinner v-else-if="isSearching" name="lines" class="search-spinner-pulse"></ion-spinner>
          <ion-spinner v-else name="crescent"></ion-spinner>
          <p>{{ isSearching ? t('files.searching') : (connecting ? t('files.connecting') : t('files.loading')) }}</p>
        </div>
        <div v-else-if="noPermission" class="empty-state">
          <ion-icon :icon="lockClosed" class="empty-icon"></ion-icon>
          <h3>{{ t('files.noPermission') }}</h3>
          <p>{{ t('files.noPermissionDesc') }}</p>
          <ion-button @click="handleRequestStorage">
            <ion-icon :icon="folderOpen" slot="start"></ion-icon>
            {{ t('files.grantPermission') }}
          </ion-button>
        </div>
        <div v-else-if="!serverOnline" class="empty-state">
          <ion-icon :icon="cloudOffline" class="empty-icon"></ion-icon>
          <h3>{{ t('files.serverOffline') }}</h3>
          <p>{{ t('files.serverOfflineDesc') }}</p>
          <ion-button @click="retryConnection">
            <ion-icon :icon="refresh" slot="start"></ion-icon>
            {{ t('files.retry') }}
          </ion-button>
        </div>
        <div v-else class="empty-state">
          <ion-icon :icon="searchQuery ? search : folderOpen" class="empty-icon"></ion-icon>
          <h3>{{ searchQuery ? t('files.noSearchResults') : t('files.emptyDir') }}</h3>
          <p>{{ searchQuery ? t('files.noSearchResultsDesc') : t('files.emptyDirDesc') }}</p>

          <!-- 🆕 2026-07-03 搜索诊断卡片（仅搜索结果为空时显示）
                背景：安卓真机 FTS5 可能异常，但用户进不去 /settings/fulltext-index（classList bug）
                无法在那查看 FTS 状态。所以搜索界面直接显示诊断信息辅助排查。
                数据源：refreshSearchDiagnostics() 在 performSearch 空结果时自动调用 -->
          <div v-if="searchQuery && searchDiagnostics" class="search-diagnostics-card" data-testid="search-diagnostics-card">
            <div class="diag-card-header">
              <ion-icon :icon="alertCircle" class="diag-card-icon"></ion-icon>
              <span>搜索诊断</span>
              <span class="diag-card-time">{{ searchDiagnostics.searchedAt }}</span>
            </div>

            <div class="diag-grid">
              <div class="diag-item">
                <span class="diag-label">FTS5</span>
                <ion-badge
                  :color="searchDiagnostics.ftsAvailable ? 'success' : 'danger'"
                  data-testid="diag-fts-badge"
                >
                  {{ searchDiagnostics.ftsAvailable ? '可用' : '不可用' }}
                </ion-badge>
              </div>

              <div class="diag-item">
                <span class="diag-label">向量搜索</span>
                <ion-badge
                  :color="searchDiagnostics.vectorSearch === 'available' ? 'success' : (searchDiagnostics.vectorSearch === 'degraded' ? 'warning' : 'danger')"
                >
                  {{ searchDiagnostics.vectorSearch }}
                </ion-badge>
              </div>

              <div class="diag-item">
                <span class="diag-label">FTS 索引</span>
                <span class="diag-value" data-testid="diag-fts-index-size">{{ searchDiagnostics.ftsIndexSize ?? '-' }} 文件</span>
              </div>

              <div class="diag-item">
                <span class="diag-label">普通索引</span>
                <span class="diag-value">{{ searchDiagnostics.totalFiles ?? '-' }} 文件</span>
              </div>

              <div class="diag-item" v-if="searchDiagnostics.isIndexing">
                <span class="diag-label">状态</span>
                <ion-badge color="warning">
                  <ion-spinner name="dots" class="diag-spinner"></ion-spinner>
                  索引中
                </ion-badge>
              </div>

              <div class="diag-item" v-if="searchDiagnostics.ftsTokenizer">
                <span class="diag-label">分词器</span>
                <span class="diag-value mono">{{ searchDiagnostics.ftsTokenizer }}</span>
              </div>
            </div>

            <!-- FTS 不可用原因 / 错误信息 -->
            <div v-if="searchDiagnostics.ftsError || !searchDiagnostics.ftsAvailable" class="diag-error-row">
              <ion-icon :icon="warningOutline" class="diag-error-icon"></ion-icon>
              <span class="diag-error-text">
                {{ searchDiagnostics.ftsError || 'FTS5 未启用' }}
              </span>
            </div>

            <!-- 上次搜索元信息 -->
            <div v-if="searchDiagnostics.lastQuery" class="diag-meta-row">
              <span class="diag-meta-item">
                查询: <code>{{ searchDiagnostics.lastQuery }}</code>
              </span>
              <span class="diag-meta-item" v-if="searchDiagnostics.lastNormalCount !== undefined">
                普通匹配: {{ searchDiagnostics.lastNormalCount }}
              </span>
              <span class="diag-meta-item" v-if="searchDiagnostics.lastFtsCount !== undefined">
                FTS 匹配: {{ searchDiagnostics.lastFtsCount }}
              </span>
            </div>

            <!-- 操作按钮 -->
            <div class="diag-actions">
              <ion-button size="small" fill="outline" @click="retrySearch" data-testid="diag-retry-btn">
                <ion-icon :icon="refresh" slot="start"></ion-icon>
                重试
              </ion-button>
              <ion-button size="small" fill="outline" @click="refreshSearchDiagnostics()" data-testid="diag-refresh-btn">
                <ion-icon :icon="alertCircle" slot="start"></ion-icon>
                刷新状态
              </ion-button>
              <ion-button size="small" fill="outline" color="tertiary" @click="goFullTextIndexFromSearch" data-testid="diag-goto-index-btn">
                <ion-icon :icon="search" slot="start"></ion-icon>
                全文索引页
              </ion-button>
            </div>
          </div>
        </div>
      </template>

      <template v-else>
        <div v-if="selectedPlugin" class="plugin-view">
            <div class="plugin-header">
              <div class="plugin-header-top">
                <ion-button fill="clear" size="small" class="plugin-back-btn" @click="exitPluginMode()">
                  <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
                </ion-button>
                <span class="plugin-title">{{ selectedPlugin?.name }} 文件</span>
                <ion-segment v-model="pluginTab" value="source" class="plugin-segment">
                  <ion-segment-button value="source">未加密</ion-segment-button>
                  <ion-segment-button value="container">已加密</ion-segment-button>
                </ion-segment>
              </div>
              <ion-item button detail @click="showPluginFilters = !showPluginFilters" class="filter-toggle-item">
                <ion-icon :icon="filterOutline" slot="start"></ion-icon>
                <ion-label>筛选与排序</ion-label>
                <ion-badge v-if="activeFilterCount > 0" slot="end" color="primary">{{ activeFilterCount }}</ion-badge>
                <ion-badge slot="end" color="medium">{{ pluginSortLabel }}</ion-badge>
              </ion-item>
            </div>
            <div v-if="!pluginLoaded" class="loading-container">
              <ion-spinner name="crescent"></ion-spinner>
              <p>{{ t('files.loading') }}</p>
            </div>
            <template v-else>
            <div v-if="pluginFiles.length === 0" class="empty-state">
              <ion-icon :icon="folderOpen" class="empty-icon"></ion-icon>
              <h3>{{ t('files.emptyDir') }}</h3>
              <p>{{ t('settings.emptyPluginDesc', { name: selectedPlugin?.name ?? '' }) || '该类型下暂无文件' }}</p>
            </div>
            <template v-else>
            <ion-list v-if="showPluginFilters" :inset="true">
              <ion-item>
                <ion-label position="stacked">大小范围</ion-label>
                <div style="display:flex;gap:8px;align-items:center;width:100%">
                  <ion-input type="number" placeholder="最小"
                    :value="sizeFilterMin !== null ? String(sizeFilterMin) : ''"
                    @ionInput="sizeFilterMin = $event.detail.value ? Number($event.detail.value) : null">
                  </ion-input>
                  <span>~</span>
                  <ion-input type="number" placeholder="最大"
                    :value="sizeFilterMax !== null ? String(sizeFilterMax) : ''"
                    @ionInput="sizeFilterMax = $event.detail.value ? Number($event.detail.value) : null">
                  </ion-input>
                  <ion-button fill="clear" size="small" @click="sizeFilterMin=null;sizeFilterMax=null">
                    <ion-icon :icon="closeCircleOutline" slot="icon-only"></ion-icon>
                  </ion-button>
                </div>
                <div class="filter-chips">
                  <ion-chip v-for="p in SIZE_PRESETS" :key="p.label" :button="true" outline @click.stop="applySizePreset(p)">{{ p.label }}</ion-chip>
                </div>
              </ion-item>
              <ion-item>
                <ion-label position="stacked">修改时间</ion-label>
                <div style="display:flex;gap:8px;align-items:center;width:100%">
                  <ion-input type="date" placeholder="起始"
                    :value="timeFilterFrom || ''"
                    @ionInput="timeFilterFrom = ($event.detail.value as string) || null">
                  </ion-input>
                  <span>~</span>
                  <ion-input type="date" placeholder="结束"
                    :value="timeFilterTo || ''"
                    @ionInput="timeFilterTo = ($event.detail.value as string) || null">
                  </ion-input>
                  <ion-button fill="clear" size="small" @click="timeFilterFrom=null;timeFilterTo=null">
                    <ion-icon :icon="closeCircleOutline" slot="icon-only"></ion-icon>
                  </ion-button>
                </div>
                <div class="filter-chips">
                  <ion-chip v-for="p in TIME_PRESETS" :key="p.label" :button="true" outline @click.stop="applyTimePreset(p)">{{ p.label }}</ion-chip>
                </div>
              </ion-item>
              <ion-item>
                <ion-label position="stacked">排序方式</ion-label>
                <div class="filter-chips">
                  <ion-chip v-for="s in ['name', 'size', 'time'] as const" :key="s"
                    :button="true" :outline="pluginSortBy !== s" :color="pluginSortBy === s ? 'primary' : undefined"
                    @click.stop="pluginSortBy = s">
                    {{ s === 'name' ? '名称' : s === 'size' ? '大小' : '时间' }}
                  </ion-chip>
                </div>
                <div class="filter-chips" style="margin-top:4px">
                  <ion-chip :button="true" :outline="!!pluginSortDesc" :color="!pluginSortDesc ? 'primary' : undefined" @click.stop="pluginSortDesc = false">升序 ↑</ion-chip>
                  <ion-chip :button="true" :outline="!pluginSortDesc" :color="!!pluginSortDesc ? 'primary' : undefined" @click.stop="pluginSortDesc = true">降序 ↓</ion-chip>
                </div>
              </ion-item>
              <ion-item button @click="clearAllPluginFilters">
                <ion-icon :icon="closeCircleOutline" slot="start" color="danger"></ion-icon>
                <ion-label color="danger">清除所有筛选</ion-label>
              </ion-item>
            </ion-list>
            <ion-list :inset="true">
            <ion-item v-for="file in filteredPluginFiles" :key="file.path" button @click="handleFileClick(file)" v-longpress="() => handleLongPress(file)">
              <div slot="start" class="file-thumbnail-slot lazy-thumb-target" :data-file-path="file.path">
                <img
                  v-if="isImageFile(file) && thumbnailUrls[file.path]"
                  :src="thumbnailUrls[file.path]"
                  class="file-thumb"
                  loading="lazy"
                  @error="onThumbError(file.path)"
                />
                <ion-icon
                  v-else
                  :icon="getFileIcon(file)"
                  :color="getFileIconColor(file)"
                  :class="{ 'thumb-fallback': isImageFile(file) }"
                ></ion-icon>
              </div>
              <ion-label>
                <h2>{{ file.display_name || file.name }}</h2>
                <p v-if="!file.isDirectory && file.size">{{ formatFileSize(file.size) }}<span v-if="file.modified"> · {{ formatDateTime(file.modified) }}</span></p>
                <p v-else-if="file.isDirectory">{{ t('files.directory') }}</p>
                <div v-if="!file.isDirectory && file._tags && file._tags.length > 0" class="file-tag-chips">
                  <ion-chip v-for="tag in file._tags" :key="tag" size="small" color="tertiary" outline>{{ tag }}</ion-chip>
                </div>
              </ion-label>
              <ion-badge v-if="file.isEncrypted" color="warning" slot="end">
                ENCV
              </ion-badge>
            </ion-item>
            <ion-item v-if="filteredPluginFiles.length === 0">
              <ion-label>无匹配文件</ion-label>
            </ion-item>
          </ion-list>
          </template>
          </template>
        </div>

        <!--
          排序控件显示条件：
          - 非插件模式：始终显示
          - 搜索时：仅在有搜索结果时显示（避免空结果时显示无意义控件）
        -->
        <div v-if="!selectedPlugin && (!searchQuery || (searchQuery && displayFiles.length > 0))" class="main-sort-bar">
          <ion-item button detail @click="showMainSort = !showMainSort">
            <ion-icon :icon="swapVerticalOutline" slot="start"></ion-icon>
            <ion-label>{{ t('files.sortBy') || '排序' }}</ion-label>
            <ion-badge slot="end" color="medium">{{ mainSortLabel }}</ion-badge>
          </ion-item>
          <ion-list v-if="showMainSort" :inset="true">
            <ion-item>
              <ion-label position="stacked">{{ t('files.sortMethod') || '排序方式' }}</ion-label>
              <div class="filter-chips">
                <!--
                  排序选项：
                  - 搜索时：额外提供「相关度」选项（用后端返回的 hybrid score）
                    默认选中，且是降序（高相关度在前）
                  - 非搜索：name / size / time 三选一
                -->
                <ion-chip v-if="searchQuery && searchResults && searchResults.length > 0"
                  :button="true" :outline="sortBy !== 'relevance'" :color="sortBy === 'relevance' ? 'primary' : undefined"
                  @click.stop="setSortBy('relevance')">
                  {{ t('files.sortByRelevance') || '相关度' }}
                </ion-chip>
                <ion-chip v-for="s in (['name', 'size', 'time'] as const)" :key="s"
                  :button="true" :outline="sortBy !== s" :color="sortBy === s ? 'primary' : undefined"
                  @click.stop="setSortBy(s)">
                  {{ s === 'name' ? t('files.sortByName') || '名称' : s === 'size' ? t('files.sortBySize') || '大小' : t('files.sortByTime') || '时间' }}
                </ion-chip>
              </div>
              <!--
                相关度排序强制为降序（高相关度在前），不显示升降选择器。
                非相关度排序才显示升/降切换。
              -->
              <div v-if="sortBy !== 'relevance'" class="filter-chips" style="margin-top:4px">
                <ion-chip :button="true" :outline="!!sortDesc" :color="!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = false">{{ t('files.sortAsc') || '升序 ↑' }}</ion-chip>
                <ion-chip :button="true" :outline="!sortDesc" :color="!!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = true">{{ t('files.sortDesc') || '降序 ↓' }}</ion-chip>
              </div>
              <div v-else class="filter-chips" style="margin-top:4px">
                <ion-chip :button="false" :outline="false" color="primary">{{ t('files.sortDesc') || '降序 ↓' }}（{{ t('files.sortByRelevance') || '相关度' }}）</ion-chip>
              </div>
            </ion-item>
          </ion-list>
        </div>

        <!-- 🆕 2026-07-02 搜索模式提示（见 debug-discipline.md §3.5）：
             greedy 模式时显示徽章，告知用户结果是宽松匹配（可能不完全相关） -->
        <div v-if="searchQuery && searchResults && searchResults.length > 0 && searchMode === 'greedy'" class="search-mode-banner search-mode-greedy">
          <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
          <span>{{ t('files.searchModeGreedy', { defaultValue: '贪婪匹配：结果为语义近似，可能不完全相关' }) }}</span>
        </div>
        <div v-else-if="searchQuery && searchResults && searchResults.length > 0 && searchMode === 'combined'" class="search-mode-banner search-mode-combined">
          <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
          <span>{{ t('files.searchModeCombined', { defaultValue: '综合匹配：关键词 + 语义重排序' }) }}</span>
        </div>

        <!-- 🆕 2026-07-02 A6：FTS 失败降级 banner（不破坏现有结果） -->
        <div v-if="fulltextBanner" :class="['fulltext-banner', `fulltext-banner-${fulltextBanner.type}`]">
          <ion-icon :icon="warningOutline" class="fulltext-banner-icon"></ion-icon>
          <span class="fulltext-banner-msg">{{ fulltextBanner.message }}</span>
          <button class="fulltext-banner-dismiss" type="button" @click="dismissFulltextBanner">×</button>
        </div>

        <ion-list>
          <ion-item
            v-for="file in displayFiles"
            :key="file.path"
            @click="handleFileClick(file)"
            v-longpress="() => handleLongPress(file)"
            :data-highlight-path="file.path"
            :class="{ 'file-highlight': highlightedPath === file.path, 'greedy-match': searchMode === 'greedy' }"
          >
            <div slot="start" class="file-thumbnail-slot lazy-thumb-target" :data-file-path="file.path">
                <img
                  v-if="isImageFile(file) && thumbnailUrls[file.path]"
                  :src="thumbnailUrls[file.path]"
                  class="file-thumb"
                  loading="lazy"
                  @error="onThumbError(file.path)"
                />
                <ion-icon
                  v-else
                  :icon="getFileIcon(file)"
                  :color="getFileIconColor(file)"
                  :class="{ 'thumb-fallback': isImageFile(file) }"
                ></ion-icon>
            </div>
            <ion-label>
              <h2>{{ file.display_name || file.name }}</h2>
              <p v-if="searchQuery && !file.isDirectory" class="search-path">{{ file.path }}</p>
              <p v-if="!file.isDirectory && file.size">{{ formatFileSize(file.size) }}<span v-if="file.modified && !searchQuery"> · {{ formatDateTime(file.modified) }}</span></p>
              <!-- 🆕 2026-07-02 全文搜索命中预览框：snippet 高亮 + hitCount 计数 + 导航按钮 -->
              <div v-if="searchFullText && file.snippet" class="fulltext-preview">
                <div class="fulltext-preview-header">
                  <ion-badge color="primary" class="hit-count-badge">
                    {{ file.hitCount || 0 }} 命中
                  </ion-badge>
                  <span class="fulltext-preview-source">在内容中</span>
                </div>
                <div class="fulltext-snippet">
                  <template v-for="(part, idx) in renderSnippet(file.snippet)" :key="idx">
                    <mark v-if="part.highlight" class="snippet-highlight">{{ part.text }}</mark>
                    <span v-else>{{ part.text }}</span>
                  </template>
                </div>
              </div>
              <!-- 🆕 2026-06-15 multi-mount 适配：mount 伪 item 在根目录展示
                   driver badge + 真实 mount_path + resolved root_path
                   让用户能看到 "这是个 mount，不是普通目录" -->
              <p v-else-if="file.isDirectory && mountDriverOf(file)">
                <ion-badge color="primary" class="mount-driver-badge">{{ mountDriverOf(file) }}</ion-badge>
                <code class="mount-path-inline">{{ mountPathOf(file) }}</code>
                <span class="mount-root-inline">→ {{ mountRootOf(file) }}</span>
              </p>
              <p v-else-if="file.isDirectory">{{ t('files.directory') }}</p>
              <p v-for="sub in fileSubtitles[file.path]" :key="'sub-' + sub.text" class="real-name" :style="{ color: sub.color || 'var(--ion-color-danger)' }">{{ sub.text }}</p>
              <div v-if="!file.isDirectory && !searchQuery && file._tags && file._tags.length > 0" class="file-tag-chips">
                <ion-chip v-for="tag in file._tags" :key="tag" size="small" color="tertiary" outline>{{ tag }}</ion-chip>
              </div>
            </ion-label>
            <RelevanceBadge v-if="searchQuery && file.score" :score="file.score" slot="end" />
            <ion-badge v-if="file.isEncrypted" color="warning" slot="end">
              ENCV
            </ion-badge>
            <!-- 🆕 2026-07-02 A4：FTS 增强模式：FTS-only 命中（非普通搜索结果）的项打"全文"角标 -->
            <ion-badge v-if="file._fulltextHit" color="tertiary" slot="end" class="fulltext-hit-badge">
              全文
            </ion-badge>
            <ion-badge v-for="badge in fileBadges[file.path]" :key="'badge-' + badge.text" :color="badge.color" slot="end">
              {{ badge.text }}
            </ion-badge>
            <ion-button v-if="searchQuery" slot="end" fill="clear" class="open-folder-btn" @click.stop="openContainingFolder(file)">
              <ion-icon :icon="folderOpen" class="open-folder-icon" slot="icon-only"></ion-icon>
            </ion-button>
          </ion-item>
        </ion-list>
      </template>

      <ion-alert :is-open="showRenameDialog" header="重命名"
        :inputs="renameAlertInputs"
        :buttons="[
          { text: '取消', role: 'cancel' },
          { text: '确定', handler: onRenameConfirm }
        ]"
        @didDismiss="showRenameDialog = false" />
      <ion-modal :is-open="showTagDialog" @didDismiss="showTagDialog = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>标签管理</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showTagDialog = false">完成</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content>
          <div class="tag-editor-content">
            <div v-if="editingFileTags.length > 0" class="existing-tags">
              <ion-chip v-for="tag in editingFileTags" :key="tag" color="primary" outline>
                {{ tag }}
                <ion-icon :icon="closeCircle" @click="handleRemoveTag(tag)"></ion-icon>
              </ion-chip>
            </div>
            <p v-else class="no-tags-hint">暂无标签</p>
            <div class="tag-input-row">
              <InputWithHistory
                v-model="newTagInput"
                :label="t('files.newTag')"
                placeholder="输入新标签名"
                :icon="pricetagOutline"
                history-key="files.newTag"
                @keyup-enter="handleAddNewTag()"
              />
              <ion-button fill="solid" color="primary" @click="handleAddNewTag()" :disabled="!newTagInput.trim()">
                添加
              </ion-button>
            </div>
          </div>
        </ion-content>
      </ion-modal>
      <ion-alert :is-open="showMoveDialog" header="移动到"
        :inputs="[{ name: 'target', type: 'text', placeholder: '目标路径', value: moveTargetPath }]"
        :buttons="[
          { text: '取消', role: 'cancel' },
          { text: '移动', handler: (d: any) => { moveTargetPath = d.target; handleMove(selectedFile!); } }
        ]"
        @didDismiss="showMoveDialog = false" />

    </ion-content>

    <ion-fab vertical="bottom" horizontal="end" slot="fixed">
      <ion-fab-button @click="handleUpload">
        <ion-icon :icon="add" />
      </ion-fab-button>
    </ion-fab>

    <input
      ref="fileInputRef"
      type="file"
      multiple
      style="display: none"
      @change="handleFileSelected"
    />

  </ion-page>
</template>
<script setup lang="ts">
// Files.vue 重构后只剩 thin script：调用 useFilesView() composable + 必要 imports。
// 原 1565 行 script 逻辑已全部抽到 ./useFilesView.ts。
//
// template 内直接使用的 state/handler 都在此解构到局部变量，
// Vue 3 <script setup> 自动暴露顶层 binding 给 template，所以 template 用法保持不变。

import { useFilesView } from "./useFilesView";

const {
  // i18n + composable re-exposed values
  t,
  vectorSearchStatus,
  thumbnailUrls,
  onThumbError,
  sortBy,
  sortDesc,
  mainSortLabel,
  setSortBy,
  showMainSort,
  // connection / loading state
  serverOnline,
  noPermission,
  loading,
  refreshing,
  connecting,
  // file list + sort state
  fileBadges,
  fileSubtitles,
  displayFiles,
  // path / navigation
  currentPath,
  pathSegments,
  navigateTo,
  goUp,
  highlightedPath,
  mainContentRef,
  openContainingFolder,
  // search state
  searchQuery,
  searchFullText,
  searchResults,
  isSearching,
  searchMode,
  // 🆕 A3 contenteditable ref + handlers
  queryInputRef,
  onQueryInput,
  onQueryKeydown,
  // 🆕 A6 FTS 降级 banner
  fulltextBanner,
  dismissFulltextBanner,
  // 🆕 2026-07-03 搜索空结果诊断
  searchDiagnostics,
  refreshSearchDiagnostics,
  retrySearch,
  goFullTextIndexFromSearch,
  // 🆕 旧 handleSearchInput 不再需要（onQueryInput 自动触发）
  handleSearchClear,
  handleSearchToggle,
  insertOperator,
  renderSnippet,
  // play error state
  playError,
  playErrorDetail,
  playErrorFile,
  clearPlayError,
  togglePlayErrorDetail,
  // plugin view state
  selectedPlugin,
  pluginTab,
  pluginFiles,
  pluginLoaded,
  showPluginFilters,
  sizeFilterMin,
  sizeFilterMax,
  timeFilterFrom,
  timeFilterTo,
  pluginSortBy,
  pluginSortDesc,
  pluginSortLabel,
  activeFilterCount,
  filteredPluginFiles,
  applySizePreset,
  applyTimePreset,
  clearAllPluginFilters,
  SIZE_PRESETS,
  TIME_PRESETS,
  // dialogs + selected
  showRenameDialog,
  showTagDialog,
  showMoveDialog,
  selectedFile,
  moveTargetPath,
  editingFileTags,
  newTagInput,
  renameAlertInputs,
  // menu / drawer
  plugins,
  tags,
  openSideDrawer,
  exitPluginMode,
  openPluginView,
  getPluginIcon,
  handleTagFilter,
  // file actions
  handleRefresh,
  retryConnection,
  handleRequestStorage,
  handleFileClick,
  handleLongPress,
  onRenameConfirm,
  handleMove,
  handleAddNewTag,
  handleRemoveTag,
  handleUpload,
  handleFileSelected,
  fileInputRef,
  // multi-mount 字段访问器
  mountDriverOf,
  mountPathOf,
  mountRootOf,
  // icons (template 用)
  add,
} = useFilesView();

// 🆕 2026-07-02 v2 简化：不需要 phraseInsertion 常量（直接调 insertSymbol('__phrase_open__')）
// 占位：保留空的占位 hooks（focus/blur 事件，可后续加视觉反馈）
function _onQueryFocus() {
  // 占位：input 聚焦时可以让插入按钮行高亮
}
function _onQueryBlur() {
  // 占位：input 失焦时不立即清空（用户可能想看高亮结果）
}
</script>

<style scoped>
/* 🆕 2026-07-02 全文搜索命中预览框 */
.fulltext-preview {
  margin-top: 4px;
  padding: 6px 8px;
  background: var(--ion-color-light-shade, #f4f4f4);
  border-left: 3px solid var(--ion-color-primary, #4f8cff);
  border-radius: 4px;
  font-size: 0.85em;
  line-height: 1.4;
}

/* 🆕 2026-07-02 搜索语法高亮：符号 overlay（在搜索框下方展示输入） */
.search-syntax-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 12px;
  border-top: 1px solid var(--ion-color-light-shade, #e0e0e0);
  background: var(--ion-color-light-tint, #fafafa);
  min-height: 32px;
  align-items: center;
  font-size: 0.85em;
  line-height: 1.4;
  color: var(--ion-text-color, #333);
}

.syntax-token-text {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  color: var(--ion-text-color, #333);
}

.syntax-token-text.syntax-word {
  color: var(--ion-text-color, #333);
}

.syntax-token-text.syntax-op {
  font-weight: 700;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--ion-color-warning-tint, #fff4d6);
  color: var(--ion-color-warning-shade, #b07a00);
}

.syntax-token-text.syntax-or {
  font-weight: 700;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--ion-color-primary-tint, #d6e6ff);
  color: var(--ion-color-primary-shade, #2962cc);
}

.syntax-token-text.syntax-not {
  font-weight: 700;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--ion-color-danger-tint, #ffd6d6);
  color: var(--ion-color-danger-shade, #b00020);
}

.syntax-token-text.syntax-phrase {
  font-style: italic;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--ion-color-success-tint, #d6f5d6);
  color: var(--ion-color-success-shade, #1b6b1b);
}

.syntax-token-text.syntax-regex {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--ion-color-tertiary-tint, #e6d6f5);
  color: var(--ion-color-tertiary-shade, #6b3aa0);
}

/* 🆕 2026-07-02 插入操作符按钮行（搜索框内操作） */
.search-insert-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px 12px;
  border-top: 1px solid var(--ion-color-light-shade, #e0e0e0);
  background: var(--ion-color-light, #ffffff);
  align-items: center;
  min-height: 36px;
}

.search-insert-bar .insert-label {
  font-size: 0.75em;
  color: var(--ion-color-medium, #666);
  margin-right: 4px;
}

.search-insert-bar .insert-btn {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 0.85em;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  background: var(--ion-color-light-tint, #fafafa);
  cursor: pointer;
  transition: background 0.15s;
}

.search-insert-bar .insert-btn:hover {
  background: var(--ion-color-light-shade, #f0f0f0);
}

.search-insert-bar .insert-btn.op-and { color: var(--ion-color-warning-shade, #b07a00); }
.search-insert-bar .insert-btn.op-or { color: var(--ion-color-primary-shade, #2962cc); }
.search-insert-bar .insert-btn.op-not { color: var(--ion-color-danger-shade, #b00020); }
.search-insert-bar .insert-btn.op-phrase { color: var(--ion-color-success-shade, #1b6b1b); }
.search-insert-bar .insert-btn.op-regex { color: var(--ion-color-tertiary-shade, #6b3aa0); }

/* 🆕 2026-07-02 A3：contenteditable 搜索输入框 + span units（替换 ion-searchbar） */
.search-input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}

.query-input {
  flex: 1;
  min-height: 32px;
  padding: 4px 8px;
  font-size: 0.95em;
  line-height: 1.4;
  border: 1px solid var(--ion-color-light-shade, #d0d0d0);
  border-radius: 4px;
  background: var(--ion-color-light, #ffffff);
  outline: none;
  white-space: pre-wrap;
  word-break: break-word;
  cursor: text;
  user-select: text;
}

.query-input:focus {
  border-color: var(--ion-color-primary, #4f8cff);
  box-shadow: 0 0 0 2px var(--ion-color-primary-tint, #d6e4ff);
}

/* 空内容时显示 placeholder（用 :empty + ::before） */
.query-input:empty::before,
.query-input[data-empty="true"]::before {
  content: attr(placeholder);
  color: var(--ion-color-medium, #999);
  pointer-events: none;
}

/* 输入框内的 span unit 样式 */
.query-input :deep(.syntax-text-span) {
  color: var(--ion-text-color, #333);
}

.query-input :deep(.syntax-op-span) {
  font-weight: 700;
  padding: 0 4px;
  border-radius: 3px;
  cursor: default;
  user-select: none;
}

.query-input :deep(.syntax-op-span.syntax-op) {
  background: var(--ion-color-warning-tint, #fff4d6);
  color: var(--ion-color-warning-shade, #b07a00);
}

.query-input :deep(.syntax-op-span.syntax-or) {
  background: var(--ion-color-primary-tint, #d6e4ff);
  color: var(--ion-color-primary-shade, #2962cc);
}

.query-input :deep(.syntax-op-span.syntax-not) {
  background: var(--ion-color-danger-tint, #fbd6d6);
  color: var(--ion-color-danger-shade, #b00020);
}

.query-input :deep(.syntax-op-span.syntax-phrase) {
  background: var(--ion-color-success-tint, #d6f5d6);
  color: var(--ion-color-success-shade, #1b6b1b);
  font-style: italic;
}

.query-input :deep(.syntax-op-span.syntax-regex) {
  background: var(--ion-color-tertiary-tint, #e6d6f5);
  color: var(--ion-color-tertiary-shade, #6b3aa0);
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 0.9em;
}

.query-clear-btn {
  position: absolute;
  right: 18px;
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 24px;
  border: none;
  background: var(--ion-color-medium, #999);
  color: white;
  border-radius: 50%;
  font-size: 1.1em;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.query-clear-btn:hover {
  background: var(--ion-color-medium-shade, #666);
}

/* 🆕 2026-07-02 A6：FTS 失败降级 banner（不破坏现有结果） */
.fulltext-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin: 8px 12px;
  border-radius: 6px;
  font-size: 0.85em;
  line-height: 1.4;
}

.fulltext-banner-unavailable {
  background: var(--ion-color-warning-tint, #fff4d6);
  border-left: 4px solid var(--ion-color-warning, #ffc409);
  color: var(--ion-color-warning-shade, #b07a00);
}

.fulltext-banner-error {
  background: var(--ion-color-danger-tint, #fbd6d6);
  border-left: 4px solid var(--ion-color-danger, #eb445a);
  color: var(--ion-color-danger-shade, #b00020);
}

.fulltext-banner-icon {
  font-size: 1.2em;
  flex-shrink: 0;
}

.fulltext-banner-msg {
  flex: 1;
}

.fulltext-banner-dismiss {
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: inherit;
  font-size: 1.2em;
  line-height: 1;
  cursor: pointer;
  padding: 0;
}

.fulltext-banner-dismiss:hover {
  opacity: 0.7;
}

/* 🆕 2026-07-02 A4：FTS 命中角标（merge 后非普通结果的项） */
.fulltext-hit-badge {
  font-size: 0.7em;
  font-weight: 700;
  margin-left: 4px;
}

/* 🆕 2026-07-03 从 git history (commit 08c5b63) 恢复 Files.vue 拆分时丢失的样式
 *   背景：Files.vue script 拆到 useFilesView.ts 时 <style> 块丢失了 40+ 个 class。
 *   之前只在 2026-07-02 补了 loading-container / empty-state，其余仍丢失。
 *   本次完整恢复（面包屑 / 长按菜单 action sheet / 文件缩略图 / 标签 / plugin 视图等）。
 */

/* === v3 2026-06-18 Task 8：route.query.highlight 驱动的文件高亮 === */
.file-highlight {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  box-shadow: inset 3px 0 0 var(--ion-color-primary);
  transition: background 0.2s ease, box-shadow 0.2s ease;
}

/* === 播放错误展示区域 === */
.play-error-banner {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-left: 3px solid var(--ion-color-danger);
  border-radius: 6px;
  margin: 8px 12px;
  padding: 10px 12px;
}
.play-error-header {
  display: flex;
  align-items: center;
  gap: 8px;
}
.play-error-file {
  font-weight: 500;
  color: var(--ion-color-danger);
  font-size: 14px;
}
.play-error-message {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
  margin-bottom: 0;
}
.play-error-detail-row {
  margin-top: 6px;
}

/* === v4 Bug1 修复：自动更新顶栏细 indicator === */
.files-refresh-bar {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  background: var(--ion-color-primary-tint);
  color: var(--ion-color-primary-contrast);
  font-size: 11px;
  font-weight: 500;
  border-bottom: 1px solid var(--ion-color-primary-shade);
  animation: files-refresh-bar-enter 0.15s ease-out;
}
.files-refresh-bar__spinner {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
}
.files-refresh-bar__label {
  white-space: nowrap;
}
@keyframes files-refresh-bar-enter {
  from { transform: translateY(-100%); opacity: 0; }
  to { transform: translateY(0); opacity: 1; }
}

/* === 面包屑 === */
.breadcrumb-scroll {
  --background: transparent;
}
.breadcrumb {
  display: flex;
  align-items: center;
  padding: 0 16px;
  white-space: nowrap;
}
.breadcrumb-item {
  cursor: pointer;
  color: var(--ion-color-primary);
  font-size: 14px;
}
.breadcrumb-item:hover {
  text-decoration: underline;
}
.breadcrumb-sep {
  font-size: 14px;
  margin: 0 4px;
  color: var(--encv-text-secondary);
}
.breadcrumb-segment {
  display: flex;
  align-items: center;
}

/* === 搜索框旁的全文搜索开关 === */
.recursive-toggle {
  margin-right: 8px;
  font-size: 12px;
}

/* === 搜索路径提示 === */
.search-path {
  font-size: 11px;
  color: var(--encv-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* === 2026-07-02 搜索模式提示横幅 === */
.search-mode-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 12px 4px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.4;
}
.search-mode-banner .search-mode-icon {
  font-size: 16px;
  flex-shrink: 0;
}
.search-mode-greedy {
  background: rgba(var(--ion-color-warning-rgb), 0.12);
  border: 1px dashed var(--ion-color-warning);
  color: var(--ion-color-warning-shade);
}
.search-mode-combined {
  background: rgba(var(--ion-color-tertiary-rgb), 0.10);
  border: 1px solid var(--ion-color-tertiary);
  color: var(--ion-color-tertiary-shade);
}
/* greedy 模式下，结果项加左侧橙色虚线条 */
.greedy-match {
  --background: rgba(var(--ion-color-warning-rgb), 0.06);
  box-shadow: inset 3px 0 0 var(--ion-color-warning);
}
@media (prefers-color-scheme: dark) {
  .search-mode-greedy {
    color: var(--ion-color-warning-tint);
  }
  .search-mode-combined {
    color: var(--ion-color-tertiary-tint);
  }
}

/* === 打开文件夹按钮 === */
.open-folder-btn {
  --padding-start: 8px;
  --padding-end: 8px;
  min-width: 44px;
  min-height: 44px;
  margin: 0;
}
.open-folder-icon {
  font-size: 20px;
  color: var(--ion-color-primary);
}

/* === Tag 编辑器 === */
.tag-editor-content {
  padding: 16px;
}
.existing-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.no-tags-hint {
  color: var(--ion-text-secondary);
  font-size: 14px;
  margin-bottom: 16px;
}
.tag-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.tag-input-row .input-with-history {
  flex: 1;
}
.tag-input-row .input-with-history ion-input {
  --padding-start: 12px;
}
.file-tag-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
  contain: content;
}

/* === 文件缩略图 === */
.file-thumbnail-slot {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.file-thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 8px;
}
.thumb-fallback {
  opacity: 0.4;
}
ion-item {
  contain: layout style;
}

/* === 筛选 chips === */
.filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.filter-chips ion-chip {
  font-size: 12px;
  --padding-start: 8px;
  --padding-end: 8px;
  cursor: pointer;
}

/* === Plugin 视图 === */
.plugin-header {
  padding: 0 4px;
}
.plugin-header-top {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 4px 4px;
}
.plugin-back-btn {
  --padding-start: 4px;
  --padding-end: 4px;
  --padding-top: 4px;
  --padding-bottom: 4px;
  min-width: 32px;
  height: 32px;
  flex-shrink: 0;
}
.plugin-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ion-color-dark);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 1;
  min-width: 0;
}
.plugin-segment {
  flex-shrink: 0;
  --segment-height: 28px;
  margin: 0;
}
.plugin-segment ion-segment-button {
  --padding-start: 10px;
  --padding-end: 10px;
  font-size: 12px;
  min-height: 28px;
  line-height: 1;
}
.filter-toggle-item {
  --padding-start: 12px;
  --padding-end: 12px;
  --min-height: 40px;
}
.main-sort-bar {
  padding: 0 4px;
}

/* === 真实文件名（alist 加密解码后）=== */
.real-name {
  color: var(--ion-color-danger);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* === 2026-06-15 multi-mount 适配：mount 伪 item 样式 === */
.mount-driver-badge {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10px;
  text-transform: lowercase;
  margin-right: 6px;
  vertical-align: middle;
}
.mount-path-inline {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  padding: 1px 4px;
  border-radius: 3px;
}
.mount-root-inline {
  font-size: 10px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  margin-left: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

/* ===== 长按菜单分区样式 (action-sheet overlay，需 :global) ===== */
:global(.file-action-sheet .action-button) {
  padding: 12px 16px;
  font-size: 15px;
}
:global(.file-action-sheet .action-section_view),
:global(.file-action-sheet .action-section-view) {
  --color: var(--ion-color-primary);
}
:global(.file-action-sheet .action-section_view .action-button-icon),
:global(.file-action-sheet .action-section-view .action-button-icon) {
  color: var(--ion-color-primary) !important;
}
:global(.file-action-sheet .action-section-crypto) {
  --color: var(--ion-color-warning);
}
:global(.file-action-sheet .action-section-crypto .action-button-icon) {
  color: #e6a000 !important;
}
:global(.file-action-sheet .action-section-manage) {
  --color: var(--ion-color-medium);
}
:global(.file-action-sheet .action-section-manage .action-button-icon) {
  color: var(--ion-color-medium-shade) !important;
}
:global(.file-action-sheet .action-section-danger) {
  --color: var(--ion-color-danger);
}
:global(.file-action-sheet .action-section-danger .action-button-icon) {
  color: var(--ion-color-danger) !important;
}

/* 🆕 2026-07-02 修复 loading 样式丢失 */
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  min-height: 200px;
  color: var(--ion-text-color, #333);
}

.loading-container ion-spinner {
  width: 40px;
  height: 40px;
  margin-bottom: 12px;
  color: var(--ion-color-primary, #4f8cff);
}

.loading-container p {
  margin: 0;
  font-size: 0.9em;
  color: var(--ion-color-medium, #666);
}

.search-spinner-pulse {
  animation: search-spinner-pulse 1.2s ease-in-out infinite;
}

@keyframes search-spinner-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}

/* 🆕 2026-07-02 修复 empty-state 样式丢失 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  min-height: 200px;
  color: var(--ion-text-color, #333);
}

.empty-state h3 {
  margin: 12px 0 6px;
  font-size: 1.1em;
  font-weight: 600;
  color: var(--ion-text-color, #333);
}

.empty-state p {
  margin: 0 0 16px;
  font-size: 0.9em;
  color: var(--ion-color-medium, #666);
  max-width: 320px;
}

.empty-state .empty-icon {
  font-size: 64px;
  color: var(--ion-color-medium, #999);
  margin-bottom: 8px;
}

/* 🆕 2026-07-03 搜索诊断卡片样式（搜索结果为空时显示 FTS/索引状态辅助排查） */
.search-diagnostics-card {
  width: 100%;
  max-width: 560px;
  margin: 16px auto 0;
  padding: 14px 16px;
  border-radius: 12px;
  background: rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.06);
  border: 1px solid rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.18);
  text-align: left;
  font-size: 13px;
}
body.dark .search-diagnostics-card {
  background: rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.12);
  border-color: rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.3);
}

.diag-card-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  font-size: 13.5px;
  margin-bottom: 10px;
  color: var(--ion-color-warning, #ff9800);
}
.diag-card-icon { font-size: 16px; }
.diag-card-time {
  margin-left: auto;
  font-size: 11px;
  font-weight: 400;
  color: var(--ion-color-medium, #999);
  font-family: 'SFMono-Regular', Consolas, monospace;
}

.diag-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px 12px;
  margin-bottom: 10px;
}
.diag-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px 10px;
  border-radius: 8px;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.5);
  border: 1px solid rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.1);
}
body.dark .diag-item {
  background: rgba(var(--ion-background-color-rgb, 30, 30, 30), 0.5);
}
.diag-label {
  font-size: 11px;
  color: var(--ion-color-medium, #666);
  font-weight: 500;
}
.diag-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color, #333);
}
.diag-value.mono {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 11.5px;
  word-break: break-all;
}
.diag-spinner {
  width: 12px;
  height: 12px;
  margin-right: 4px;
  vertical-align: middle;
}

.diag-error-row {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 8px 10px;
  margin-bottom: 8px;
  border-radius: 6px;
  background: rgba(var(--ion-color-danger-rgb, 240, 60, 60), 0.08);
  border-left: 3px solid var(--ion-color-danger, #f53d3d);
}
.diag-error-icon {
  font-size: 14px;
  color: var(--ion-color-danger, #f53d3d);
  flex-shrink: 0;
  margin-top: 1px;
}
.diag-error-text {
  font-size: 12px;
  color: var(--ion-color-danger, #f53d3d);
  font-family: 'SFMono-Regular', Consolas, monospace;
  word-break: break-all;
  line-height: 1.4;
}

.diag-meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 6px 0;
  margin-bottom: 8px;
  border-top: 1px dashed rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.15);
  font-size: 11.5px;
  color: var(--ion-color-medium, #666);
}
.diag-meta-item code {
  font-family: 'SFMono-Regular', Consolas, monospace;
  background: rgba(var(--ion-color-medium-rgb, 100, 100, 100), 0.1);
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 11px;
}

.diag-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 4px;
}
.diag-actions ion-button {
  --padding-start: 8px;
  --padding-end: 12px;
  font-size: 12px;
  height: 32px;
}
.diag-actions ion-button ion-icon {
  font-size: 14px;
  margin-right: 2px;
}


.fulltext-preview-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.hit-count-badge {
  font-size: 0.75em;
  padding: 2px 6px;
}

.fulltext-preview-source {
  color: var(--ion-color-medium, #666);
  font-size: 0.8em;
}

.fulltext-snippet {
  word-break: break-word;
  white-space: pre-wrap;
  color: var(--ion-text-color, #333);
}

.fulltext-snippet .snippet-highlight {
  background: var(--ion-color-primary-tint, #cce0ff);
  color: var(--ion-color-primary-shade, #2962cc);
  font-weight: 600;
  padding: 0 2px;
  border-radius: 2px;
}

/* 暗黑模式适配 */
@media (prefers-color-scheme: dark) {
  .fulltext-preview {
    background: rgba(255, 255, 255, 0.05);
  }
  .fulltext-snippet {
    color: var(--ion-text-color, #ddd);
  }
  .fulltext-snippet .snippet-highlight {
    background: rgba(79, 140, 255, 0.3);
    color: #cce0ff;
  }
}
</style>

