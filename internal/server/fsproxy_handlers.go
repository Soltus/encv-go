package server

import (
	"net/http/httptest"
	"strings"
	"text/template"

	"github.com/Soltus/encv-go/internal/auth"
	"github.com/Soltus/encv-go/internal/injector"
	"github.com/Soltus/encv-go/internal/routes"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleFSProxyGin(c *gin.Context) {
	path := c.Param("path")
	if path == "" {
		path = "/"
	}

	c.Request.Header.Set("X-Forwarded-Prefix", routes.FSProxy)

	rec := httptest.NewRecorder()
	s.servePath(rec, c.Request, path)

	body := rec.Body.Bytes()
	contentType := rec.Header().Get("Content-Type")

	if strings.Contains(contentType, "text/html") {
		isLoggedIn := false
		if s.jwtManager != nil {
			if token := auth.GetTokenFromCookie(c.Request); token != "" {
				if _, err := s.jwtManager.ValidateToken(token); err == nil {
					isLoggedIn = true
				}
			}
		}

		modifiedBody := injectAdminAssets(string(body), path, isLoggedIn)
		for k, v := range rec.Header() {
			c.Writer.Header()[k] = v
		}
		c.Data(rec.Code, contentType, []byte(modifiedBody))
	} else {
		for k, v := range rec.Header() {
			c.Writer.Header()[k] = v
		}
		c.Status(rec.Code)
		c.Writer.Write(body)
	}
}

func injectAdminAssets(htmlBody, currentPath string, isLoggedIn bool) string {
	userInjector := injector.NewUserInjector(routes.Logout)
	toolbarHTML := userInjector.GenerateFloatingToolbar(isLoggedIn)
	styleHTML := generateAdminUserAssets(isLoggedIn, currentPath)

	result := htmlBody

	if toolbarHTML != "" {
		emptyDiv := `<div id="` + injector.InjectorID + `"></div>`
		if idx := strings.Index(result, emptyDiv); idx != -1 {
			result = result[:idx] + `<div id="` + injector.InjectorID + `">` + toolbarHTML + `</div>` + result[idx+len(emptyDiv):]
		}
	}

	if styleHTML != "" {
		result = strings.Replace(result, "</head>", styleHTML+"</head>", 1)
	}

	return result
}

func generateAdminUserAssets(isLoggedIn bool, currentPath string) string {
	if !isLoggedIn {
		return ""
	}

	styleHTML := `
	<style>
		.actions-cell {
			text-align: center;
			width: 180px;
			white-space: nowrap;
		}
		.action-btn {
			padding: 4px 8px;
			font-size: 0.8em;
			color: var(--link-color);
			background-color: transparent;
			border: 1px solid var(--border-color);
			border-radius: 4px;
			cursor: pointer;
			transition: all 0.2s ease;
		}
		.action-btn:hover {
			background-color: var(--link-color);
			color: white;
		}
		.analyze-dialog {
			width: 80%;
			max-width: 900px;
			border: 1px solid var(--border-color, #ccc);
			border-radius: 8px;
			padding: 20px;
			background-color: var(--bg-color, #f9f9f9);
			color: var(--text-color, #333);
			box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
			position: fixed;
		}
		.dialog-close-btn {
			position: absolute;
			top: 15px;
			right: 15px;
			background: none;
			border: none;
			font-size: 24px;
			font-weight: bold;
			line-height: 1;
			color: #888;
			cursor: pointer;
			padding: 0;
			width: 30px;
			height: 30px;
			display: flex;
			align-items: center;
			justify-content: center;
			border-radius: 50%;
			transition: background-color 0.2s, color 0.2s;
		}
		.dialog-close-btn:hover {
			background-color: #e0e0e0;
			color: #333;
		}
		.dialog-content {
			background-color: #fff;
			border: 1px solid #ddd;
			padding: 15px;
			border-radius: 4px;
			max-height: 70vh;
			overflow-y: auto;
			margin-top: 10px;
		}
		.dialog-content pre {
			margin: 0;
			white-space: pre-wrap;
			word-wrap: break-word;
		}
		.dialog-content code {
			background-color: transparent;
			padding: 0;
			font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
		}
		body.hope-ui-dark .dialog-close-btn {
			color: #aaa;
		}
		body.hope-ui-dark .dialog-close-btn:hover {
			background-color: #444;
			color: #fff;
		}
		body.hope-ui-dark .dialog-content {
			background-color: #1e1e1e;
			border-color: #444;
		}
	</style>`

	jsScript := `
	<script>
		(function() {
			const currentPath = "` + template.JSEscapeString(currentPath) + `";
			document.addEventListener('DOMContentLoaded', function() {
				const table = document.querySelector('table');
				if (!table) return;

				const thead = table.querySelector('thead tr');
				const tbody = table.querySelector('tbody');

				if (thead && !thead.querySelector('.actions-cell')) {
					const actionsTh = document.createElement('th');
					actionsTh.textContent = 'Actions';
					actionsTh.className = 'actions-cell';
					thead.appendChild(actionsTh);
				}

				if (tbody) {
					const rows = tbody.querySelectorAll('tr');
					rows.forEach(row => {
						const firstCell = row.querySelector('td:first-child a');
						if (!firstCell || firstCell.textContent.trim() === '..') return;

						const fileName = firstCell.textContent.trim();
						const actionTd = document.createElement('td');
						actionTd.className = 'actions-cell';

						const renameBtn = document.createElement('button');
						renameBtn.textContent = 'Rename';
						renameBtn.className = 'action-btn';
						renameBtn.onclick = () => showRenameDialog(fileName);

						const analyzeBtn = document.createElement('button');
						analyzeBtn.textContent = 'Analyze';
						analyzeBtn.className = 'action-btn';
						analyzeBtn.style.marginLeft = '5px';
						analyzeBtn.onclick = () => showAnalyzeDialog(fileName, fileName);

						actionTd.appendChild(renameBtn);
						actionTd.appendChild(analyzeBtn);
						row.appendChild(actionTd);
					});
				}
			});

			function showAnalyzeDialog(baseName, fullOldPath) {
				let currentDirPath = currentPath;
				if (currentDirPath && !currentDirPath.endsWith('/')) {
					currentDirPath += '/';
				}
				const fullPathToSend = currentDirPath + fullOldPath;

				let dialog = document.getElementById('analyze-dialog');
				if (!dialog) {
					dialog = document.createElement('dialog');
					dialog.id = 'analyze-dialog';
					dialog.className = 'analyze-dialog';
					document.body.appendChild(dialog);
				}

				const updateDialogContent = (title, contentHtml) => {
					dialog.innerHTML = '<h2>' + title + '</h2>' + contentHtml;
					const closeBtn = document.createElement('button');
					closeBtn.innerHTML = '&times;';
					closeBtn.className = 'dialog-close-btn';
					closeBtn.onclick = () => dialog.close();
					dialog.appendChild(closeBtn);
				};

				updateDialogContent('Analyzing: ' + baseName, '<p>Please wait...</p><div class="dialog-content"></div>');
				dialog.showModal();

				fetch('/admin/file/analyze', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ path: fullPathToSend })
				})
				.then(response => {
					if (!response.ok) throw new Error('HTTP error! status: ' + response.status);
					return response.json();
				})
				.then(data => {
					if (data.code === 0) {
						updateDialogContent('Analysis Result for: ' + baseName, '<div class="dialog-content">' + data.data.htmlContent + '</div>');
					} else {
						throw new Error(data.message || 'An unknown error occurred.');
					}
				})
				.catch(error => {
					console.error('Analyze error:', error);
					updateDialogContent('Analysis Failed', '<div class="dialog-content" style="color: red;">Error: ' + error.message + '</div>');
				});
			}

			function showRenameDialog(fileName) {
				let currentDirPath = currentPath;
				if (currentDirPath && !currentDirPath.endsWith('/')) {
					currentDirPath += '/';
				}
				const fullOldPath = currentDirPath + fileName;

				const newName = prompt('Enter new name for "' + fileName + '":', fileName);
				if (newName && newName !== fileName) {
					const cleanedName = newName.replace(/[^\w\s.-]/g, '');
					if (!cleanedName) {
						alert('Error: The new name contains no valid characters.');
						return;
					}
					fetch('/admin/file/rename', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
						body: JSON.stringify({ oldPath: fullOldPath, newName: cleanedName })
					})
					.then(response => {
						if (!response.ok) throw new Error('HTTP error! status: ' + response.status);
						return response.json();
					})
					.then(data => {
						if (data.code === 0) {
							alert('Success: ' + data.message);
							location.reload();
						} else {
							throw new Error(data.message || 'An unknown error occurred.');
						}
					})
					.catch(error => {
						console.error('Rename error:', error);
						alert('Rename Failed: ' + error.message);
					});
				}
			}
		})();
	</script>`

	return styleHTML + jsScript
}
