package http

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "your-module/internal/core/ports"
)

type HTTPHandler struct {
    service ports.Service
}

func NewHTTPHandler(service ports.Service) *HTTPHandler {
    return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetupRoutes(router *gin.Engine) {
    router.POST("/items", h.CreateItem)
    router.GET("/items/:id", h.GetItem)
}

func (h *HTTPHandler) CreateItem(c *gin.Context) {
    // ...handler implementation...
}

func (h *HTTPHandler) GetItem(c *gin.Context) {
    // ...handler implementation...
}
