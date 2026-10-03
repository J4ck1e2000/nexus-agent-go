package sshcollector

import (
	"math"
	"strconv"
	"strings"
)

// round1 保留 1 位小数，与旧 Agent 展示精度一致。
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// clampPercent 把百分比限制在 [0, 100]。
func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// parseIntField 解析整数字段，兼容小数输入。
func parseIntField(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if value, err := strconv.Atoi(raw); err == nil {
		return value, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}

// parseFloatField 解析浮点字段。
func parseFloatField(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}
