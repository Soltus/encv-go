// internal/admin/injector/user_injector.go
package injector

import (
	"fmt"
	"strings"
)

const (
	InjectorID = "encv-content-injection-point"
)

// UserInjector 用户状态注入器
type UserInjector struct {
	logoutRoute string
}

// NewUserInjector 创建用户注入器
func NewUserInjector(logoutRoute string) *UserInjector {
	return &UserInjector{
		logoutRoute: logoutRoute,
	}
}

// GenerateFloatingToolbar 生成完整的悬浮工具栏
func (ui *UserInjector) GenerateFloatingToolbar(isLoggedIn bool) string {
	// log.Printf("GenerateFloatingToolbar isLoggedIn: %v", isLoggedIn)

	// 生成用户状态部分（仅在登录时显示）
	userStatusHTML := ""
	if isLoggedIn {
		userStatusHTML = fmt.Sprintf(
			`<span class="user-status"><span class="status-text">Logged In</span><a href="%s" class="logout-btn">Logout</a></span>`,
			ui.logoutRoute,
		)
	}

	// 使用字符串拼接而不是 fmt.Sprintf
	var sb strings.Builder

	sb.WriteString(`
	<div class="floating-toolbar">
		<button class="toolbar-btn" id="theme-toggle" aria-label="Toggle Theme">
			<!-- Moon icon for dark mode -->
			<svg viewBox="0 0 24 24" aria-hidden="true" style="display: none;"><path d="M9 2c-1.05 0-2.05.16-3 .46 4.06 1.27 7 5.06 7 9.54 0 4.48-2.94 8.27-7 9.54.95.3 1.95.46 3 .46 5.52 0 10-4.48 10-10S14.52 2 9 2z"/></svg>
			<!-- Sun icon for light mode -->
			<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 7c-2.76 0-5 2.24-5 5s2.24 5 5 5 5-2.24 5-5-2.24-5-5-5zM2 13h2c.55 0 1-.45 1-1s-.45-1-1-1H2c-.55 0-1 .45-1 1s.45 1 1 1zm18 0h2c.55 0 1-.45 1-1s-.45-1-1-1h-2c-.55 0-1 .45-1 1s.45 1 1 1zM11 2v2c0 .55.45 1 1 1s1-.45 1-1V2c0-.55-.45-1-1-1s-1 .45-1 1zm0 18v2c0 .55.45 1 1 1s1-.45 1-1v-2c0-.55-.45-1-1-1s-1 .45-1 1zM5.99 4.58c-.39-.39-1.03-.39-1.41 0-.39.39-.39 1.03 0 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41L5.99 4.58zm12.37 12.37c-.39-.39-1.03-.39-1.41 0-.39.39-.39 1.03 0 1.41l1.06 1.06c.39.39 1.03.39 1.41 0 .39-.39.39-1.03 0-1.41l-1.06-1.06zm1.06-10.96c.39-.39.39-1.03 0-1.41-.39-.39-1.03-.39-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06zM7.05 18.36c.39-.39.39-1.03 0-1.41-.39-.39-1.03-.39-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06z"/></svg>
		</button>
		<button class="toolbar-btn active" id="wrap-toggle" aria-label="Toggle Word Wrap">
			<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19h6v-2H4v2zM20 5H4v2h16V5zm-3 6H4v2h13.25c1.1 0 2 .9 2 2s-.9 2-2 2H15v-2l-3 3 3 3v-2h2c2.21 0 4-1.79 4-4s-1.79-4-4-4z"/></svg>
		</button>`)
	sb.WriteString(userStatusHTML)
	sb.WriteString(`</div>`)
	sb.WriteString(`<style>
		/* --- 悬浮工具栏样式 --- */
		.floating-toolbar {
			position: fixed;
			top: 1.5em;
			right: 1.5em;
			display: flex;
			gap: 0.5em;
			z-index: 1000;
		}

		.toolbar-btn {
			width: 40px;
			height: 40px;
			border-radius: 50%;
			border: 1px solid var(--border-color, #ddd);
			background-color: var(--toolbar-btn-bg, rgba(255, 255, 255, 0.8));
			color: var(--text-color, #333);
			cursor: pointer;
			display: flex;
			align-items: center;
			justify-content: center;
			transition: all 0.2s ease;
			box-shadow: 0 2px 8px rgba(0,0,0,0.1);
		}

		.toolbar-btn:hover {
			transform: scale(1.1);
			box-shadow: 0 4px 12px rgba(0,0,0,0.15);
		}
		
		.toolbar-btn.active {
			background-color: var(--link-color, #007BFF);
			color: #fff;
			border-color: var(--link-color, #007BFF);
		}

		.toolbar-btn svg {
			width: 20px;
			height: 20px;
			fill: currentColor;
		}
		
		/* --- 用户状态样式 --- */
		.user-status {
			display: inline-flex;
			align-items: center;
			padding: 0.3em 0.8em;
			background-color: var(--toolbar-btn-bg, rgba(255, 255, 255, 0.8));
			border-radius: 12px;
			border: 1px solid var(--border-color, #ddd);
			box-shadow: 0 2px 8px rgba(0,0,0,0.1);
			font-size: 0.9em;
			white-space: nowrap;
		}
		.user-status .status-text {
			color: var(--muted-text-color, #586069);
			margin-right: 0.5em;
		}
		.user-status .logout-btn {
			color: var(--link-color, #007BFF);
			text-decoration: none;
			font-weight: bold;
		}
		.user-status .logout-btn:hover {
			text-decoration: underline;
		}
			 /* --- CSS 变量定义 --- */
        :root {
            --bg-color: #f4f4f9;
            --text-color: #333;
            --muted-text-color: #586069;
            --border-color: #ddd;
            --header-bg-color: #4c86afff;
            --header-text-color: white;
            --link-color: #007BFF;
            --link-hover-color: #0056b3;
            --table-even-bg-color: #f2f2f2;
            --dir-tag-color: #007BFF;
            --container-tag-color: #d9534f;
            --selection-bg: rgba(46, 170, 220, 0.3);
            --toolbar-btn-bg: rgba(255, 255, 255, 0.8);
            /* 【新增】定义悬停背景色 */
            --hover-bg-color: #e9e9f3; /* 亮色主题下的悬停背景 */
        }

        body.hope-ui-dark {
            --bg-color: #1a1a1a;
            --text-color: #e6edf3;
            --muted-text-color: #8b949e;
            --border-color: #30363d;
            --header-bg-color: #0a233bff;
            --header-text-color: #ffffff;
            --link-color: #58a6ff;
            --link-hover-color: #79c0ff;
            --table-even-bg-color: #161b22;
            --dir-tag-color: #58a6ff;
            --container-tag-color: #f85149;
            --selection-bg: rgba(46, 170, 220, 0.4);
            --toolbar-btn-bg: rgba(33, 38, 45, 0.8);
            /* 【新增】定义悬停背景色 */
            --hover-bg-color: #2c2c2c; /* 暗色主题下的悬停背景 */
        }
	</style>
	
	<script>
		// --- 状态管理 ---
		const themeKey = 'encv-theme';
		const wrapKey = 'encv-wrap';
		
		// 从 localStorage 加载状态，如果没有则使用默认值
		let isDark = localStorage.getItem(themeKey) === 'true';
		let isWrapping = localStorage.getItem(wrapKey) !== 'false'; // 默认为 true

		// --- DOM 元素 ---
		const themeToggle = document.getElementById('theme-toggle');
		const wrapToggle = document.getElementById('wrap-toggle');
		const body = document.body;
		const table = document.querySelector('table');

		// --- 功能函数 ---
		function applyTheme() {
			if (isDark) {
				body.classList.add('hope-ui-dark');
				if (themeToggle) {
					themeToggle.querySelector('svg[style*="none"]').style.display = 'block'; // Show moon
					themeToggle.querySelector('svg:not([style*="none"])').style.display = 'none'; // Hide sun
				}
			} else {
				body.classList.remove('hope-ui-dark');
				if (themeToggle) {
					themeToggle.querySelector('svg[style*="none"]').style.display = 'none'; // Hide moon
					themeToggle.querySelector('svg:not([style*="none"])').style.display = 'block'; // Show sun
				}
			}
		}

		function applyWrap() {
			if (isWrapping) {
				if (table) table.style.whiteSpace = 'normal';
				if (wrapToggle) wrapToggle.classList.add('active');
			} else {
				if (table) table.style.whiteSpace = 'nowrap';
				if (wrapToggle) wrapToggle.classList.remove('active');
			}
		}

		function toggleTheme() {
			isDark = !isDark;
			localStorage.setItem(themeKey, isDark);
			applyTheme();
		}

		function toggleWrap() {
			isWrapping = !isWrapping;
			localStorage.setItem(wrapKey, isWrapping);
			applyWrap();
		}

		// --- 事件监听 ---
		if (themeToggle) themeToggle.addEventListener('click', toggleTheme);
		if (wrapToggle) wrapToggle.addEventListener('click', toggleWrap);

		// --- 初始化 ---
		// 页面加载时应用保存的主题和换行状态
		applyTheme();
		applyWrap();
	</script>`)

	return sb.String()
}
