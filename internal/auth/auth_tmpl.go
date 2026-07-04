package auth

import (
	"github.com/Soltus/encv-go/internal/injector"
	"github.com/Soltus/encv-go/internal/routes"
)

const LoginPageTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>ENCV - Login</title>
    <style>
        :root { --bg-color: #f4f4f9; --text-color: #333; --container-bg: white; --input-border: #ccc; --btn-bg: #007BFF; --btn-hover-bg: #0056b3; }
        body.hope-ui-dark { --bg-color: #1a1a1a; --text-color: #e6edf3; --container-bg: #21262d; --input-border: #30363d; --btn-bg: #238636; --btn-hover-bg: #2ea043; }
        body { font-family: sans-serif; background-color: var(--bg-color); color: var(--text-color); display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; transition: background-color 0.3s, color 0.3s; }
        .login-container { background-color: var(--container-bg); padding: 2em; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); width: 100%; max-width: 400px; }
        h1 { text-align: center; margin-bottom: 1.5em; }
        .form-group { margin-bottom: 1em; }
        label { display: block; margin-bottom: 0.5em; }
        input[type="password"] { width: 100%; padding: 0.8em; border: 1px solid var(--input-border); border-radius: 4px; box-sizing: border-box; background-color: var(--bg-color); color: var(--text-color); }
        button { width: 100%; padding: 0.8em; border: none; border-radius: 4px; background-color: var(--btn-bg); color: white; font-size: 1em; cursor: pointer; transition: background-color 0.2s; }
        button:hover { background-color: var(--btn-hover-bg); }
        .error { color: #d9534f; text-align: center; margin-top: 1em; }
        .info { color: #6c757d; text-align: center; margin-top: 0.5em; font-size: 0.9em; }
    </style>
</head>
<body>
<div id="` + injector.InjectorID + `"></div>
    <div class="login-container">
        <h1>ENCV Login</h1>
        <form method="post" action="` + routes.Login + `">
            <div class="form-group">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required autofocus>
            </div>
            <button type="submit">Login</button>
            {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
            {{if .RedirectURL}}<div class="info">You will be redirected to: {{.RedirectURL}}</div>{{end}}
            {{if .RedirectURL}}<input type="hidden" name="redirect_url" value="{{.RedirectURL}}">{{end}}
        </form>
    </div>
</body>
</html>`
