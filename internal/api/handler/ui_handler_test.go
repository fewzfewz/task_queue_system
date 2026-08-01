package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func extractInlineScript(html string) string {
	const open = "<script>"
	const close = "</script>"
	start := strings.Index(html, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	end := strings.Index(html[start:], close)
	if end < 0 {
		return ""
	}
	return html[start : start+end]
}

func TestAppHTMLInlineScriptSyntax(t *testing.T) {
	script := extractInlineScript(appHTML)
	if script == "" {
		t.Fatal("appHTML has no inline script block")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", "--check", path).CombinedOutput()
	if err != nil {
		t.Fatalf("embedded UI script syntax error: %v\n%s", err, out)
	}
}

func TestLoginPageInlineScriptSyntax(t *testing.T) {
	script := extractInlineScript(loginPageHTML)
	if script == "" {
		t.Fatal("loginPageHTML has no inline script block")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "login.js")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", "--check", path).CombinedOutput()
	if err != nil {
		t.Fatalf("login page script syntax error: %v\n%s", err, out)
	}
}
