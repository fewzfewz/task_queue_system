sed -i 's|<a href="#features">Features</a>|<a href="/features">Features</a>|g' internal/api/handler/client_ui_handler.go
sed -i 's|<a href="/swagger/">Docs</a>|<a href="/docs">Docs</a>\n    <a href="/docs/sdk">Dev SDK</a>|g' internal/api/handler/client_ui_handler.go
