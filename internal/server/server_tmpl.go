package server

import "github.com/Soltus/encv-go/internal/injector"

var tmpl_dynamic_files = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>ENCV File Server - {{.CurrentPath}}</title>
    <style>
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

        /* --- 全局与布局 --- */
        body {
            font-family: sans-serif;
            background-color: var(--bg-color);
            color: var(--text-color);
            margin: 2em;
            transition: background-color 0.3s ease, color 0.3s ease;
        }
        
        h1 { color: var(--text-color); }

        .breadcrumb {
            margin-bottom: 1em;
            font-size: 1.1em;
        }

        .breadcrumb a {
            color: var(--link-color);
            text-decoration: none;
        }

        .breadcrumb a:hover {
            text-decoration: underline;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 1em;
            box-shadow: 0 2px 4px rgba(0,0,0,0.05);
        }

        th, td {
            padding: 12px 15px;
            border: 1px solid var(--border-color);
            text-align: left;
        }

        th {
            background-color: var(--header-bg-color);
            color: var(--header-text-color);
        }

        tr:nth-child(even) {
            background-color: var(--table-even-bg-color);
        }
        
        /* 【关键修复】更新悬停样式 */
        tr:hover {
            background-color: var(--hover-bg-color); /* 使用新的变量 */
            cursor: pointer;
        }

        a {
            text-decoration: none;
            color: var(--link-color);
        }

        a:hover {
            color: var(--link-hover-color);
            text-decoration: underline;
        }

        .dir-tag {
            color: var(--dir-tag-color);
            font-weight: bold;
        }

        .container-tag {
            color: var(--container-tag-color);
            font-weight: bold;
        }

        /* --- 悬浮工具栏 --- */
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
            border: 1px solid var(--border-color);
            background-color: var(--toolbar-btn-bg);
            color: var(--text-color);
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
            background-color: var(--link-color);
            color: #fff;
            border-color: var(--link-color);
        }

        .toolbar-btn svg {
            width: 20px;
            height: 20px;
            fill: currentColor;
        }
    </style>
</head>
<body>
<div id="` + injector.InjectorID + `"></div>

    <h1>ENCV File Server</h1>
    <div class="breadcrumb">
        <a href="{{.RootPath}}">Root</a> / {{range .Ancestors}}<a href="{{.Path}}">{{.Name}}</a> / {{end}}
    </div>
    <table>
        <thead>
            <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Size</th>
                <th>Modified</th>
            </tr>
        </thead>
        <tbody>
            {{if .NotRoot}}<tr><td><a href="{{.ParentPath}}">..</a></td><td>Directory</td><td>-</td><td>-</td></tr>{{end}}
            {{range .Files}}
            <tr>
                <td><a href="{{.Path}}">{{.Name}}</a></td>
                <td>
                    {{if .IsDir}}<span class="dir-tag">Directory</span>
                    {{else if .IsContainer}}<span class="container-tag">ENCV Container</span>
                    {{else}}File
                    {{end}}
                </td>
                <td>{{if .IsDir}}-{{else}}{{.HumanSize}}{{end}}</td>
                <td>{{.ModTime.Format "2006-01-02 15:04:05"}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

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
                themeToggle.querySelector('svg[style*="none"]').style.display = 'block'; // Show moon
                themeToggle.querySelector('svg:not([style*="none"])').style.display = 'none'; // Hide sun
            } else {
                body.classList.remove('hope-ui-dark');
                themeToggle.querySelector('svg[style*="none"]').style.display = 'none'; // Hide moon
                themeToggle.querySelector('svg:not([style*="none"])').style.display = 'block'; // Show sun
            }
        }

        function applyWrap() {
            if (isWrapping) {
                table.style.whiteSpace = 'normal';
                wrapToggle.classList.add('active');
            } else {
                table.style.whiteSpace = 'nowrap';
                wrapToggle.classList.remove('active');
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
        themeToggle.addEventListener('click', toggleTheme);
        wrapToggle.addEventListener('click', toggleWrap);

        // --- 初始化 ---
        // 页面加载时应用保存的主题和换行状态
        applyTheme();
        applyWrap();
    </script>
</body>
</html>`
