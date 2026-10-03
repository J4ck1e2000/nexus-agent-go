package sshcollector

import (
	"encoding/csv"
	"fmt"
	"strings"
)

// parseGPUQueryOutput 解析 nvidia-smi --query-gpu 输出：
// index,uuid,name,temperature.gpu,fan.speed,power.draw,utilization.gpu,
// memory.total,memory.used（csv,noheader,nounits）。
// 解析失败的行跳过；输出为空（无 GPU / nvidia-smi 缺失）返回空切片。
func parseGPUQueryOutput(lines []string) []RawGPU {
	gpus := make([]RawGPU, 0, len(lines))

	for _, record := range parseCSVLines(lines) {
		if len(record) < 9 {
			continue
		}

		index, err := strconvInt(record[0])
		if err != nil {
			continue
		}

		gpus = append(gpus, RawGPU{
			Index:         index,
			UUID:          strings.TrimSpace(record[1]),
			Name:          strings.TrimSpace(record[2]),
			Temperature:   parseSMIInt(record[3]),
			FanSpeed:      parseSMIInt(record[4]),
			PowerDraw:     parseSMIInt(record[5]),
			Utilization:   parseSMIInt(record[6]),
			MemoryTotalMB: parseSMIFloat(record[7]),
			MemoryUsedMB:  parseSMIFloat(record[8]),
		})
	}
	return gpus
}

// parseGPUProcessOutput 解析 nvidia-smi --query-compute-apps 输出：
// pid,gpu_uuid,used_gpu_memory（csv,noheader,nounits）。
func parseGPUProcessOutput(lines []string) []RawGPUProcess {
	processes := make([]RawGPUProcess, 0, len(lines))

	for _, record := range parseCSVLines(lines) {
		if len(record) < 3 {
			continue
		}
		pid, err := strconvInt(record[0])
		if err != nil {
			continue
		}
		vram, err := strconvInt(record[2])
		if err != nil {
			continue
		}
		processes = append(processes, RawGPUProcess{
			PID:    pid,
			UUID:   strings.TrimSpace(record[1]),
			VRAMMB: vram,
		})
	}
	return processes
}

// parseCSVLines 用 csv 语义解析行（GPU 名称含逗号时会被正确处理）。
func parseCSVLines(lines []string) [][]string {
	var content strings.Builder
	for _, line := range lines {
		content.WriteString(line)
		content.WriteString("\n")
	}
	if strings.TrimSpace(content.String()) == "" {
		return nil
	}

	reader := csv.NewReader(strings.NewReader(content.String()))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		// 整体解析失败时退化为按行拆分，尽量保留可用数据。
		fallback := make([][]string, 0, len(lines))
		for _, line := range nonEmptyLines(lines) {
			fallback = append(fallback, strings.Split(line, ","))
		}
		return fallback
	}
	return records
}

// parseSMIInt 解析 nvidia-smi 数值字段，N/A / 非法值按 0 处理。
func parseSMIInt(raw string) int {
	value, err := strconvInt(raw)
	if err != nil {
		return 0
	}
	return value
}

// parseSMIFloat 解析 nvidia-smi 浮点字段，N/A / 非法值按 0 处理。
func parseSMIFloat(raw string) float64 {
	value, err := strconvFloat(raw)
	if err != nil {
		return 0
	}
	return value
}

func strconvInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "N/A") || strings.Contains(raw, "[") {
		return 0, fmt.Errorf("not a number: %q", raw)
	}
	return parseIntField(raw)
}

func strconvFloat(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "N/A") || strings.Contains(raw, "[") {
		return 0, fmt.Errorf("not a number: %q", raw)
	}
	return parseFloatField(raw)
}
