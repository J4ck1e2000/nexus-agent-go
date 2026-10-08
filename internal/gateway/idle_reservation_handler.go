package gateway

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) registerIdleReservationRoutes(authorized *gin.RouterGroup) {
	if h == nil || h.store == nil {
		return
	}
	authorized.GET("/idle-reservations", h.getIdleReservations)
	authorized.POST("/idle-reservations", h.postIdleReservation)
	authorized.PATCH("/idle-reservations/:id", h.patchIdleReservation)
	authorized.POST("/idle-reservations/:id/evaluate", h.postIdleReservationEvaluation)
	authorized.DELETE("/idle-reservations/:id", h.deleteIdleReservation)
}

func (h *Handler) getIdleReservations(c *gin.Context) {
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	items, err := listIdleReservations(h.store.db, user.ID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load_idle_reservations_failed"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) postIdleReservation(c *gin.Context) {
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var input IdleReservationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidIdleReservation.Error()})
		return
	}
	item, err := createIdleReservation(h.store.db, user.ID, input, time.Now())
	if errors.Is(err, ErrInvalidIdleReservation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_idle_reservation_failed"})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) patchIdleReservation(c *gin.Context) {
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, ok := parseReservationID(c)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidIdleReservation.Error()})
		return
	}
	item, err := setIdleReservationStatus(h.store.db, user.ID, id, input.Status)
	if errors.Is(err, ErrInvalidIdleReservation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrIdleReservationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_idle_reservation_failed"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) postIdleReservationEvaluation(c *gin.Context) {
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, ok := parseReservationID(c)
	if !ok {
		return
	}
	var input struct {
		MatchingKeys []string `json:"matchingKeys"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || len(input.MatchingKeys) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidIdleReservation.Error()})
		return
	}
	result, err := evaluateIdleReservation(h.store.db, user.ID, id, input.MatchingKeys, time.Now())
	if errors.Is(err, ErrIdleReservationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidIdleReservation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "evaluate_idle_reservation_failed"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) deleteIdleReservation(c *gin.Context) {
	user, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, ok := parseReservationID(c)
	if !ok {
		return
	}
	if err := deleteIdleReservation(h.store.db, user.ID, id); errors.Is(err, ErrIdleReservationNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_idle_reservation_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func parseReservationID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidIdleReservation.Error()})
		return 0, false
	}
	return uint(value), true
}
