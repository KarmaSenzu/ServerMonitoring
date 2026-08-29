package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/generator"
)

// GeneratorHandler exposes the /generator/* endpoints. These are pure
// templating functions that never execute commands; the rendered output
// is returned to the operator as a string they can paste themselves.
type GeneratorHandler struct {
	App *app.App
}

// NewGeneratorHandler constructs a GeneratorHandler.
func NewGeneratorHandler(a *app.App) *GeneratorHandler {
	return &GeneratorHandler{App: a}
}

// Register mounts the generator routes. Caller wraps with auth.
func (h *GeneratorHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/generator/docker", h.docker)
	rg.POST("/generator/pm2", h.pm2)
	rg.POST("/generator/compose", h.compose)
	rg.POST("/generator/nginx", h.nginx)
}

func (h *GeneratorHandler) docker(c *gin.Context) {
	var opts generator.DockerRunOpts
	if err := c.ShouldBindJSON(&opts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	cmd, err := generator.DockerRun(opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"command": cmd})
}

func (h *GeneratorHandler) pm2(c *gin.Context) {
	var opts generator.PM2Opts
	if err := c.ShouldBindJSON(&opts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	cmd, err := generator.PM2Start(opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"command": cmd})
}

func (h *GeneratorHandler) compose(c *gin.Context) {
	var opts generator.ComposeOpts
	if err := c.ShouldBindJSON(&opts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	yamlText, err := generator.DockerCompose(opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"yaml": yamlText})
}

func (h *GeneratorHandler) nginx(c *gin.Context) {
	var opts generator.NginxOpts
	if err := c.ShouldBindJSON(&opts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	cfg, err := generator.NginxReverseProxy(opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": cfg})
}
