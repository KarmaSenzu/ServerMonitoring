package httpx

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// frontendFS holds the embedded React production build.
//
// IMPORTANT: The build script (scripts/build.sh) MUST copy frontend/dist/
// into backend-go/internal/httpx/dist/ before running go build, because
// go:embed cannot reference parent directories (../ is forbidden).
//
// Build process:
//   1. cd frontend && npm run build  (creates frontend/dist/)
//   2. mkdir -p backend-go/internal/httpx/dist
//   3. cp -r frontend/dist/* backend-go/internal/httpx/dist/
//   4. cd backend-go && go build -o vpsdash cmd/api/main.go
//
// The dist/ directory is .gitignored (it's build output).
//
//go:embed all:dist
var frontendFS embed.FS

// ServeFrontend returns a gin handler that serves the embedded React SPA.
// It handles two cases:
//   1. Static assets (JS/CSS/images): served directly with cache headers
//   2. Client-side routes: fallback to index.html for SPA routing
//
// Routing logic matches Vite dev server behavior (see frontend/vite.config.js):
// - HTML requests (Accept: text/html, no XHR headers) to unknown paths → index.html
// - API requests (JSON/XHR) to unknown paths → 404 (API handler precedence)
func ServeFrontend() gin.HandlerFunc {
	// Strip the "dist" prefix so the embedded FS root points directly
	// at the built assets (index.html, assets/, etc.)
	stripped, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		// This should never happen unless the embed path is wrong.
		panic("frontend: failed to strip embed prefix: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(stripped))

	return func(c *gin.Context) {
		urlPath := c.Request.URL.Path

		// Check if the requested path exists as a static file.
		// For SPAs, we need to handle two cases:
		//   - /assets/main-abc123.js → serve the file
		//   - /servers/123 (HTML request) → serve index.html (client-side route)
		//   - /servers (API request) → let API handler return JSON/404
		cleanPath := path.Clean(urlPath)
		if cleanPath == "/" {
			cleanPath = "/index.html"
		}

		// Try to open the file. If it exists, serve it.
		if f, err := stripped.Open(strings.TrimPrefix(cleanPath, "/")); err == nil {
			_ = f.Close()
			// File exists: serve it with appropriate cache headers.
			if strings.HasPrefix(urlPath, "/assets/") {
				// Vite fingerprints assets (main-abc123.js), cache aggressively.
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				// index.html and other non-fingerprinted files should not be
				// cached (otherwise updates won't be seen).
				c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// File doesn't exist. Distinguish between:
		// - HTML navigation (browser address bar) → serve index.html (SPA fallback)
		// - API/XHR request → let it 404 (API handlers already ran, this is truly 404)
		//
		// This matches Vite's isHtmlRequest logic (see frontend/vite.config.js).
		if isHTMLRequest(c) {
			// Browser navigation to unknown path (e.g., /servers/123).
			// Serve index.html so React Router can handle the route.
			c.Request.URL.Path = "/index.html"
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Content-Type", "text/html; charset=utf-8")
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// Not an HTML request and file doesn't exist → this is a genuine 404.
		// If an API route should have matched, it already did (routes are
		// registered before this middleware runs). Fall through to gin's 404.
		c.Next()
	}
}

// isHTMLRequest checks if the request is a browser navigation (expecting HTML)
// rather than an API/XHR request (expecting JSON). This matches the logic in
// frontend/vite.config.js's isHtmlRequest function.
func isHTMLRequest(c *gin.Context) bool {
	// Only GET requests navigate (POST/PUT/DELETE are always API calls)
	if c.Request.Method != "GET" {
		return false
	}

	// Check Accept header for text/html
	accept := c.GetHeader("Accept")
	if !strings.Contains(accept, "text/html") {
		return false
	}

	// XHR requests typically set X-Requested-With header
	if c.GetHeader("X-Requested-With") != "" {
		return false
	}

	// fetch() API requests often omit text/html from Accept, but if we got
	// here it means Accept includes text/html, so this is likely a browser
	// navigation (address bar, link click, history back/forward).
	return true
}

// HasEmbeddedFrontend checks if the frontend was embedded at compile time.
// This allows the binary to detect whether it was built with frontend assets
// or not, and print a helpful message if someone tries to access the UI
// without having built the frontend first.
func HasEmbeddedFrontend() bool {
	entries, err := frontendFS.ReadDir("dist")
	if err != nil {
		return false
	}
	// A valid Vite build always has index.html.
	for _, e := range entries {
		if e.Name() == "index.html" {
			return true
		}
	}
	return false
}
