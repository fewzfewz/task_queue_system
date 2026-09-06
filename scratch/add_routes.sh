sed -i '/mux.HandleFunc("GET \/", h.ServeLandingPage)/i \
	mux.HandleFunc("GET /features", h.ServeFeaturesPage)\
	mux.HandleFunc("GET /docs", h.ServeDocsPage)\
	mux.HandleFunc("GET /docs/sdk", h.ServeSDKPage)' internal/api/routes/routes.go
