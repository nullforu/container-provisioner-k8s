package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"smctf/internal/stack"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *stack.Service
}

func New(svc *stack.Service) *Handler {
	return &Handler{svc: svc}
}

type createStackRequest struct {
	PodSpec    string           `json:"pod_spec"`
	TargetPort []stack.PortSpec `json:"target_port"`
}

func (h *Handler) CreateStack(c *gin.Context) {
	var req createStackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(fmt.Errorf("bind create stack request: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	st, err := h.svc.Create(c.Request.Context(), stack.CreateInput{
		PodSpecYML:  req.PodSpec,
		TargetPorts: req.TargetPort,
	})

	if err != nil {
		h.writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, st)
}

func (h *Handler) GetStack(c *gin.Context) {
	stackID := c.Param("stack_id")
	st, err := h.svc.GetDetails(c.Request.Context(), stackID)
	if err != nil {
		h.writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, st)
}

func (h *Handler) GetStackStatusSummary(c *gin.Context) {
	stackID := c.Param("stack_id")
	statusSummary, err := h.svc.GetStatusSummary(c.Request.Context(), stackID)
	if err != nil {
		h.writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, statusSummary)
}

func (h *Handler) DeleteStack(c *gin.Context) {
	stackID := c.Param("stack_id")
	if err := h.svc.Delete(c.Request.Context(), stackID); err != nil {
		h.writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true, "stack_id": stackID})
}

func (h *Handler) ListStacks(c *gin.Context) {
	items, err := h.svc.ListAll(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"stacks": items})
}

func (h *Handler) GetStats(c *gin.Context) {
	stats, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	_ = c.Error(err)

	switch {
	case errors.Is(err, stack.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, stack.ErrInvalidInput), errors.Is(err, stack.ErrPodSpecInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, stack.ErrNoAvailableNodePort), errors.Is(err, stack.ErrClusterSaturated):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
