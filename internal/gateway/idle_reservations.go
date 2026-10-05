package gateway

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	IdleReservationActive           = "active"
	IdleReservationPaused           = "paused"
	IdleReservationCompleted        = "completed"
	IdleReservationExpired          = "expired"
	IdleReservationNotifyOnce       = "once"
	IdleReservationNotifyContinuous = "continuous"
)

var (
	ErrInvalidIdleReservation  = errors.New("invalid_idle_reservation")
	ErrIdleReservationNotFound = errors.New("idle_reservation_not_found")
)

// IdleReservationFilters 描述用户希望被提醒的 GPU 条件。
type IdleReservationFilters struct {
	MinFreeVRAMGB         float64  `json:"minFreeVramGb"`
	MinFreeSystemMemoryGB float64  `json:"minFreeSystemMemoryGb"`
	GPUModel              string   `json:"gpuModel"`
	CPUModel              string   `json:"cpuModel"`
	MaxGPUUtilization     *float64 `json:"maxGpuUtilization"`
	ProcessPolicy         string   `json:"processPolicy"`
	NodeIDs               []int64  `json:"nodeIds"`
	IdleDurationMinutes   int      `json:"idleDurationMinutes"`
}

// IdleReservationInput 是创建提醒时的前端载荷。
type IdleReservationInput struct {
	Name           string                 `json:"name"`
	Filters        IdleReservationFilters `json:"filters"`
	NotifyMode     string                 `json:"notifyMode"`
	ExpiresInHours int                    `json:"expiresInHours"`
}

// IdleReservationDTO 是前后端共享的提醒数据。
type IdleReservationDTO struct {
	ID               uint                   `json:"id"`
	Name             string                 `json:"name"`
	Filters          IdleReservationFilters `json:"filters"`
	Status           string                 `json:"status"`
	NotifyMode       string                 `json:"notifyMode"`
	ExpiresAtUnix    *int64                 `json:"expiresAtUnix"`
	CurrentMatchKeys []string               `json:"currentMatchKeys"`
	CreatedAtUnix    int64                  `json:"createdAtUnix"`
}

type IdleReservationEvaluation struct {
	Reservation  IdleReservationDTO `json:"reservation"`
	NewMatchKeys []string           `json:"newMatchKeys"`
}

func normalizeIdleReservationInput(input IdleReservationInput) (IdleReservationInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 64 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	if math.IsNaN(input.Filters.MinFreeVRAMGB) || math.IsInf(input.Filters.MinFreeVRAMGB, 0) || input.Filters.MinFreeVRAMGB < 0 || input.Filters.MinFreeVRAMGB > 1024 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	if math.IsNaN(input.Filters.MinFreeSystemMemoryGB) || math.IsInf(input.Filters.MinFreeSystemMemoryGB, 0) || input.Filters.MinFreeSystemMemoryGB < 0 || input.Filters.MinFreeSystemMemoryGB > 16384 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	input.Filters.GPUModel = strings.TrimSpace(input.Filters.GPUModel)
	input.Filters.CPUModel = strings.TrimSpace(input.Filters.CPUModel)
	if len(input.Filters.GPUModel) > 128 || len(input.Filters.CPUModel) > 256 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	if input.Filters.MaxGPUUtilization == nil {
		input.Filters.MaxGPUUtilization = floatPointer(10)
	}
	if math.IsNaN(*input.Filters.MaxGPUUtilization) || math.IsInf(*input.Filters.MaxGPUUtilization, 0) || *input.Filters.MaxGPUUtilization < 0 || *input.Filters.MaxGPUUtilization > 100 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	if input.Filters.ProcessPolicy == "" {
		input.Filters.ProcessPolicy = "any"
	}
	if input.Filters.ProcessPolicy != "any" && input.Filters.ProcessPolicy != "emptyOnly" {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	switch input.Filters.IdleDurationMinutes {
	case 0, 5, 10, 30, 60:
	default:
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	if len(input.Filters.NodeIDs) > 100 {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	nodeIDs := make([]int64, 0, len(input.Filters.NodeIDs))
	seen := make(map[int64]struct{}, len(input.Filters.NodeIDs))
	for _, id := range input.Filters.NodeIDs {
		if id <= 0 {
			return IdleReservationInput{}, ErrInvalidIdleReservation
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		nodeIDs = append(nodeIDs, id)
	}
	sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })
	input.Filters.NodeIDs = nodeIDs
	if input.NotifyMode == "" {
		input.NotifyMode = IdleReservationNotifyOnce
	}
	if input.NotifyMode != IdleReservationNotifyOnce && input.NotifyMode != IdleReservationNotifyContinuous {
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	switch input.ExpiresInHours {
	case 0, 1, 4, 8, 24, 72:
	default:
		return IdleReservationInput{}, ErrInvalidIdleReservation
	}
	return input, nil
}

func reservationToDTO(row GPUReservation) (IdleReservationDTO, error) {
	var filters IdleReservationFilters
	if err := json.Unmarshal([]byte(row.FiltersJSON), &filters); err != nil {
		return IdleReservationDTO{}, err
	}
	if filters.MaxGPUUtilization == nil {
		filters.MaxGPUUtilization = floatPointer(10)
	}
	var currentMatches []string
	if err := json.Unmarshal([]byte(row.CurrentMatchKeysJSON), &currentMatches); err != nil {
		currentMatches = []string{}
	}
	if currentMatches == nil {
		currentMatches = []string{}
	}
	var expiresAt *int64
	if row.ExpiresAt != nil {
		unix := row.ExpiresAt.Unix()
		expiresAt = &unix
	}
	return IdleReservationDTO{
		ID:               row.ID,
		Name:             row.Name,
		Filters:          filters,
		Status:           row.Status,
		NotifyMode:       row.NotifyMode,
		ExpiresAtUnix:    expiresAt,
		CurrentMatchKeys: currentMatches,
		CreatedAtUnix:    row.CreatedAt.Unix(),
	}, nil
}

func listIdleReservations(db *gorm.DB, userID uint, now time.Time) ([]IdleReservationDTO, error) {
	if db == nil || userID == 0 {
		return nil, errors.New("idle reservation store not initialized")
	}
	if err := db.Model(&GPUReservation{}).
		Where("user_id = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", userID, []string{IdleReservationActive, IdleReservationPaused}, now).
		Updates(map[string]any{"status": IdleReservationExpired, "current_match_keys_json": "[]"}).Error; err != nil {
		return nil, err
	}
	var rows []GPUReservation
	if err := db.Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]IdleReservationDTO, 0, len(rows))
	for _, row := range rows {
		item, err := reservationToDTO(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func createIdleReservation(db *gorm.DB, userID uint, raw IdleReservationInput, now time.Time) (IdleReservationDTO, error) {
	if db == nil || userID == 0 {
		return IdleReservationDTO{}, errors.New("idle reservation store not initialized")
	}
	input, err := normalizeIdleReservationInput(raw)
	if err != nil {
		return IdleReservationDTO{}, err
	}
	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return IdleReservationDTO{}, err
	}
	expiresAt := (*time.Time)(nil)
	if input.ExpiresInHours > 0 {
		value := now.Add(time.Duration(input.ExpiresInHours) * time.Hour)
		expiresAt = &value
	}
	row := GPUReservation{
		UserID:               userID,
		Name:                 input.Name,
		FiltersJSON:          string(filtersJSON),
		Status:               IdleReservationActive,
		NotifyMode:           input.NotifyMode,
		ExpiresAt:            expiresAt,
		CurrentMatchKeysJSON: "[]",
	}
	if err := db.Create(&row).Error; err != nil {
		return IdleReservationDTO{}, err
	}
	return reservationToDTO(row)
}

func setIdleReservationStatus(db *gorm.DB, userID, id uint, status string) (IdleReservationDTO, error) {
	if status != IdleReservationActive && status != IdleReservationPaused {
		return IdleReservationDTO{}, ErrInvalidIdleReservation
	}
	if db == nil || userID == 0 {
		return IdleReservationDTO{}, errors.New("idle reservation store not initialized")
	}
	var result IdleReservationDTO
	err := db.Transaction(func(tx *gorm.DB) error {
		var row GPUReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIdleReservationNotFound
			}
			return err
		}
		if row.Status != IdleReservationCompleted && row.Status != IdleReservationExpired {
			row.Status = status
			if row.ExpiresAt != nil && !time.Now().Before(*row.ExpiresAt) {
				row.Status = IdleReservationExpired
			}
			if row.Status == IdleReservationPaused || row.Status == IdleReservationExpired {
				row.CurrentMatchKeysJSON = "[]"
			}
			if err := tx.Model(&GPUReservation{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{
				"status": row.Status, "current_match_keys_json": row.CurrentMatchKeysJSON,
			}).Error; err != nil {
				return err
			}
		}
		dto, err := reservationToDTO(row)
		if err != nil {
			return err
		}
		result = dto
		return nil
	})
	return result, err
}

func deleteIdleReservation(db *gorm.DB, userID, id uint) error {
	result := db.Where("id = ? AND user_id = ?", id, userID).Delete(&GPUReservation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrIdleReservationNotFound
	}
	return nil
}

func evaluateIdleReservation(db *gorm.DB, userID, id uint, matchingKeys []string, now time.Time) (IdleReservationEvaluation, error) {
	if len(matchingKeys) > 4096 {
		return IdleReservationEvaluation{}, ErrInvalidIdleReservation
	}
	for _, key := range matchingKeys {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			return IdleReservationEvaluation{}, ErrInvalidIdleReservation
		}
		nodeID, nodeErr := strconv.ParseInt(parts[0], 10, 64)
		gpuID, gpuErr := strconv.Atoi(parts[1])
		if nodeErr != nil || nodeID <= 0 || gpuErr != nil || gpuID < 0 {
			return IdleReservationEvaluation{}, ErrInvalidIdleReservation
		}
	}
	matchingKeys = uniqueSortedStrings(matchingKeys)
	if db == nil || userID == 0 {
		return IdleReservationEvaluation{}, errors.New("idle reservation store not initialized")
	}
	var result IdleReservationEvaluation
	err := db.Transaction(func(tx *gorm.DB) error {
		var row GPUReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIdleReservationNotFound
			}
			return err
		}
		var current []string
		if err := json.Unmarshal([]byte(row.CurrentMatchKeysJSON), &current); err != nil {
			current = []string{}
		}
		currentSet := make(map[string]struct{}, len(current))
		for _, key := range current {
			currentSet[key] = struct{}{}
		}
		newMatches := make([]string, 0)
		if row.Status == IdleReservationActive {
			for _, key := range matchingKeys {
				if _, exists := currentSet[key]; !exists {
					newMatches = append(newMatches, key)
				}
			}
		} else {
			matchingKeys = current
		}
		if (row.Status == IdleReservationActive || row.Status == IdleReservationPaused) && row.ExpiresAt != nil && !now.Before(*row.ExpiresAt) {
			row.Status = IdleReservationExpired
			matchingKeys = []string{}
			newMatches = []string{}
		}
		if row.Status == IdleReservationActive && row.NotifyMode == IdleReservationNotifyOnce && len(newMatches) > 0 {
			row.Status = IdleReservationCompleted
		}
		encodedMatches, err := json.Marshal(matchingKeys)
		if err != nil {
			return err
		}
		if err := tx.Model(&GPUReservation{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{
			"status":                  row.Status,
			"current_match_keys_json": string(encodedMatches),
		}).Error; err != nil {
			return err
		}
		dto, err := reservationToDTO(row)
		if err != nil {
			return err
		}
		dto.CurrentMatchKeys = matchingKeys
		result = IdleReservationEvaluation{Reservation: dto, NewMatchKeys: newMatches}
		return nil
	})
	return result, err
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 64 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
