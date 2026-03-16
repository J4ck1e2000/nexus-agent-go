package agent

import (
	"context"
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"

	"nexus-agent-go/internal/model"
)

// gpuProcess 保存进程与显卡映射关系。
type gpuProcess struct {
	gpuIndex int
	vramMB   int
}

// collectGPU 通过 nvidia-smi 读取显卡指标。
func collectGPU(ctx context.Context) ([]model.GpuInfo, map[int]gpuProcess, error) {
	cmd := exec.CommandContext(
		ctx,
		"nvidia-smi",
		"--query-gpu=index,uuid,name,temperature.gpu,fan.speed,power.draw,utilization.gpu,memory.total,memory.used",
		"--format=csv,noheader,nounits",
	)

	out, err := cmd.Output()
	if err != nil {
		return []model.GpuInfo{}, map[int]gpuProcess{}, nil
	}

	csvReader := csv.NewReader(strings.NewReader(strings.TrimSpace(string(out))))
	records, err := csvReader.ReadAll()
	if err != nil {
		return []model.GpuInfo{}, map[int]gpuProcess{}, nil
	}

	gpus := make([]model.GpuInfo, 0, len(records))
	uuidToIndex := make(map[string]int, len(records))

	for _, rec := range records {
		if len(rec) < 9 {
			continue
		}

		idx := parseInt(rec[0])
		uuid := strings.TrimSpace(rec[1])
		name := strings.TrimSpace(rec[2])
		temp := parseInt(rec[3])
		fan := parseInt(rec[4])
		power := parseInt(rec[5])
		util := parseInt(rec[6])
		memTotalMB := parseFloat(rec[7])
		memUsedMB := parseFloat(rec[8])

		memTotalGB := round1(memTotalMB / 1024)
		memUsedGB := round1(memUsedMB / 1024)
		memUtil := 0.0
		if memTotalMB > 0 {
			memUtil = round1(memUsedMB / memTotalMB * 100)
		}

		uuidToIndex[uuid] = idx
		gpus = append(gpus, model.GpuInfo{
			ID:                idx,
			Name:              name,
			Temperature:       temp,
			FanSpeed:          fan,
			PowerDraw:         power,
			Utilization:       util,
			MemoryTotal:       memTotalGB,
			MemoryUsed:        memUsedGB,
			MemoryUtilization: memUtil,
		})
	}

	processes := collectGPUProcesses(ctx, uuidToIndex)
	return gpus, processes, nil
}

// collectGPUProcesses 读取 GPU 计算进程占用情况。
func collectGPUProcesses(ctx context.Context, uuidToIndex map[string]int) map[int]gpuProcess {
	processMap := map[int]gpuProcess{}

	cmd := exec.CommandContext(
		ctx,
		"nvidia-smi",
		"--query-compute-apps=pid,gpu_uuid,used_gpu_memory",
		"--format=csv,noheader,nounits",
	)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return processMap
	}

	csvReader := csv.NewReader(strings.NewReader(strings.TrimSpace(string(out))))
	records, err := csvReader.ReadAll()
	if err != nil {
		return processMap
	}

	for _, rec := range records {
		if len(rec) < 3 {
			continue
		}
		pid := parseInt(rec[0])
		uuid := strings.TrimSpace(rec[1])
		usedMB := parseInt(rec[2])
		gpuIdx, ok := uuidToIndex[uuid]
		if !ok {
			continue
		}
		processMap[pid] = gpuProcess{gpuIndex: gpuIdx, vramMB: usedMB}
	}
	return processMap
}

// parseInt 解析 nvidia-smi 中的数值字符串。
func parseInt(raw string) int {
	raw = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "%", ""), "W", ""))
	if strings.EqualFold(raw, "N/A") || strings.Contains(raw, "[") {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err == nil {
		return v
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

// parseFloat 解析浮点值并去掉单位。
func parseFloat(raw string) float64 {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "MiB", ""))
	if strings.EqualFold(raw, "N/A") || strings.Contains(raw, "[") {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}
