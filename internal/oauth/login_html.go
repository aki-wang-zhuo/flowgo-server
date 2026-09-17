package oauth

import (
	"html"
	"strings"

	"github.com/flowgo/flowgo/api/types"
)

type loginStrings struct {
	title       string
	subtitle    string
	username    string
	password    string
	submit      string
	badCreds    string
	mcpDisabled string
}

func loginCopy(locale string) loginStrings {
	if types.NormalizeLocale(locale) == types.LocaleEnUS {
		return loginStrings{
			title:       "Authorize FlowGo MCP",
			subtitle:    "Sign in to allow this client to use FlowGo tools.",
			username:    "Username",
			password:    "Password",
			submit:      "Authorize",
			badCreds:    "Invalid username or password.",
			mcpDisabled: "MCP is disabled for this account. Enable it in FlowGo settings.",
		}
	}
	return loginStrings{
		title:       "授权 FlowGo MCP",
		subtitle:    "登录后将允许该客户端调用 FlowGo 工具。",
		username:    "用户名",
		password:    "密码",
		submit:      "授权",
		badCreds:    "用户名或密码错误。",
		mcpDisabled: "该账号已关闭 MCP，请先在 FlowGo 设置中开启。",
	}
}

func renderLoginHTML(aq authorizeQuery) string {
	return renderLoginHTMLWithError(aq, "")
}

func renderLoginHTMLWithError(aq authorizeQuery, errMsg string) string {
	c := loginCopy(aq.Locale)
	lang := "zh-CN"
	if types.NormalizeLocale(aq.Locale) == types.LocaleEnUS {
		lang = "en-US"
	}
	errBlock := ""
	if errMsg != "" {
		errBlock = `<p class="err">` + html.EscapeString(errMsg) + `</p>`
	}
	scope := aq.Scope
	if strings.TrimSpace(scope) == "" {
		scope = DefaultScope
	}
	return `<!DOCTYPE html>
<html lang="` + lang + `">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>` + html.EscapeString(c.title) + `</title>
<style>
  :root { color-scheme: light; }
  body { margin:0; font-family: "Segoe UI", system-ui, sans-serif; background: #f4f6f8; color:#1f2937; }
  .wrap { min-height:100vh; display:flex; align-items:center; justify-content:center; padding:24px; }
  .card { width:100%; max-width:400px; background:#fff; border:1px solid #e5e7eb; border-radius:12px; padding:28px 24px; box-shadow: 0 8px 24px rgba(15,23,42,.06); }
  h1 { margin:0 0 8px; font-size:1.25rem; }
  .sub { margin:0 0 20px; color:#6b7280; font-size:.9rem; line-height:1.45; }
  label { display:block; font-size:.8rem; color:#4b5563; margin:0 0 6px; }
  input { width:100%; box-sizing:border-box; border:1px solid #d1d5db; border-radius:8px; padding:10px 12px; font-size:.95rem; margin-bottom:14px; }
  button { width:100%; border:0; border-radius:8px; padding:11px 14px; background:#0f766e; color:#fff; font-weight:600; cursor:pointer; }
  button:hover { background:#0d9488; }
  .err { background:#fef2f2; color:#b91c1c; border:1px solid #fecaca; border-radius:8px; padding:10px 12px; font-size:.85rem; margin:0 0 14px; }
</style>
</head>
<body>
<div class="wrap"><div class="card">
  <h1>` + html.EscapeString(c.title) + `</h1>
  <p class="sub">` + html.EscapeString(c.subtitle) + `</p>
  ` + errBlock + `
  <form method="post" action="/oauth/authorize">
    <input type="hidden" name="response_type" value="` + html.EscapeString(aq.ResponseType) + `"/>
    <input type="hidden" name="client_id" value="` + html.EscapeString(aq.ClientID) + `"/>
    <input type="hidden" name="redirect_uri" value="` + html.EscapeString(aq.RedirectURI) + `"/>
    <input type="hidden" name="scope" value="` + html.EscapeString(scope) + `"/>
    <input type="hidden" name="state" value="` + html.EscapeString(aq.State) + `"/>
    <input type="hidden" name="code_challenge" value="` + html.EscapeString(aq.CodeChallenge) + `"/>
    <input type="hidden" name="code_challenge_method" value="` + html.EscapeString(aq.CodeChallengeMethod) + `"/>
    <input type="hidden" name="resource" value="` + html.EscapeString(aq.Resource) + `"/>
    <input type="hidden" name="lang" value="` + html.EscapeString(aq.Locale) + `"/>
    <label for="username">` + html.EscapeString(c.username) + `</label>
    <input id="username" name="username" autocomplete="username" required autofocus/>
    <label for="password">` + html.EscapeString(c.password) + `</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required/>
    <button type="submit">` + html.EscapeString(c.submit) + `</button>
  </form>
</div></div>
</body>
</html>`
}
