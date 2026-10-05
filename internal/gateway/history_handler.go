package gateway

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NodeHistoryResponse struct {
	NodeID        int64                 `json:"nodeId"`
	FromUnix      int64                 `json:"fromUnix"`
	RetentionDays int                   `json:"retentionDays"`
	StepSeconds   int64                 `json:"stepSeconds"`
	Samples       []NodeHistorySnapshot `json:"samples"`
}

func (h *Handler) getNodeHistory(c *gin.Context) {
	if _, ok := currentAuthUser(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.store == nil || h.nodeStateService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "node_history_unavailable"})
		return
	}
	nodeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || nodeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_node_id"})
		return
	}
	if _, err := h.store.FindByID(nodeID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load_node_failed"})
		return
	}
	now := time.Now()
	fromUnix := now.Add(-24 * time.Hour).Unix()
	if rawFrom := c.Query("from"); rawFrom != "" {
		parsed, parseErr := strconv.ParseInt(rawFrom, 10, 64)
		if parseErr != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_history_range"})
			return
		}
		fromUnix = parsed
	}
	cutoff := now.Add(-defaultMetricHistoryRetentionDays * 24 * time.Hour).Unix()
	if fromUnix < cutoff {
		fromUnix = cutoff
	}
	if fromUnix > now.Unix() {
		fromUnix = now.Unix()
	}
	stepSeconds := int64(60)
	if rawStep := c.Query("step_seconds"); rawStep != "" {
		parsed, parseErr := strconv.ParseInt(rawStep, 10, 64)
		if parseErr != nil || parsed < 5 || parsed > int64(24*time.Hour/time.Second) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_history_resolution"})
			return
		}
		stepSeconds = parsed
	}
	aggregation := c.DefaultQuery("aggregation", HistoryAggregationAverage)
	if aggregation != HistoryAggregationAverage && aggregation != HistoryAggregationPeak {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_history_aggregation"})
		return
	}
	samples, err := h.nodeStateService.LoadNodeHistory(c.Request.Context(), nodeID, fromUnix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load_node_history_failed"})
		return
	}
	if stepSeconds > 0 {
		samples = DownsampleNodeHistory(samples, stepSeconds, aggregation)
	}
	c.JSON(http.StatusOK, NodeHistoryResponse{
		NodeID:        nodeID,
		FromUnix:      fromUnix,
		RetentionDays: defaultMetricHistoryRetentionDays,
		StepSeconds:   stepSeconds,
		Samples:       samples,
	})
}
