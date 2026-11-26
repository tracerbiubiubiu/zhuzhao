package handler

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed reference_assets_page/index.html
var referenceAssetsHTML []byte

//go:embed reference_assets_page/app.js
var referenceAssetsScript []byte

//go:embed reference_assets_page/app.css
var referenceAssetsStyles []byte

const referenceAssetsPageCSP = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src data:; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"

// ReferenceAssetsPage serves the same-origin browser entry point for the synthetic directory.
func ReferenceAssetsPage(c *gin.Context) {
	c.Header("Content-Security-Policy", referenceAssetsPageCSP)
	writeReferenceAssetsPage(c, "text/html; charset=utf-8", referenceAssetsHTML)
}

// ReferenceAssetsPageScript serves the fixed script for the directory page.
func ReferenceAssetsPageScript(c *gin.Context) {
	writeReferenceAssetsPage(c, "text/javascript; charset=utf-8", referenceAssetsScript)
}

// ReferenceAssetsPageStyles serves the fixed stylesheet for the directory page.
func ReferenceAssetsPageStyles(c *gin.Context) {
	writeReferenceAssetsPage(c, "text/css; charset=utf-8", referenceAssetsStyles)
}

func writeReferenceAssetsPage(c *gin.Context, contentType string, body []byte) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, contentType, body)
}
