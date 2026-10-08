import { useEffect, useRef } from 'react';
import type { EnrichedNode } from '../lib/node-logic';
import { isIdleGpuMetric } from '../lib/idle-gpu';
import type { NotificationPreferences } from '../lib/notification-preferences';
import type { Translator } from '../i18n';
import { loadLocalTerminalProfile } from '../lib/terminal-profile';

export function useSystemAlerts(nodes: EnrichedNode[], preferences: NotificationPreferences, t: Translator, userId: number | null): void {
  const previousSamples = useRef(new Map<number, EnrichedNode>());
  const conditionsSince = useRef(new Map<string, number>());
  const activeConditions = useRef(new Set<string>());

  useEffect(() => {
    const resetConditions = (nodeId: number) => {
      for (const key of conditionsSince.current.keys()) {
        if (key.split(':')[1] === String(nodeId)) conditionsSince.current.delete(key);
      }
      for (const key of activeConditions.current) {
        if (key.split(':')[1] === String(nodeId)) activeConditions.current.delete(key);
      }
    };
    const presentIds = new Set(nodes.map((node) => node.id));
    for (const nodeId of previousSamples.current.keys()) {
      if (!presentIds.has(nodeId)) { previousSamples.current.delete(nodeId); resetConditions(nodeId); }
    }
    const notify = (title: string, body: string) => {
      void window.nexus.notifications.show(title, body);
    };
    for (const node of nodes) {
      const previous = previousSamples.current.get(node.id);
      const sampleTimestamp = node.online ? node.collectedAtUnix : node.lastPolledAtUnix;
      const previousTimestamp = previous ? (previous.online ? previous.collectedAtUnix : previous.lastPolledAtUnix) : 0;
      if (!sampleTimestamp || sampleTimestamp <= previousTimestamp) continue;
      previousSamples.current.set(node.id, node);

      if (!preferences.enabled) {
        conditionsSince.current.clear();
        activeConditions.current.clear();
        continue;
      }
      const nodeName = node.name || node.hostDisplay;
      if (previous?.online && !node.online && preferences.nodeOffline) {
        notify(t('alerts.nodeOfflineTitle', { node: nodeName }), t('alerts.nodeOfflineBody'));
      }
      if (!node.online || !node.data || Date.now() / 1000 - node.collectedAtUnix > 180) {
        resetConditions(node.id);
        continue;
      }
      // An offline interval or a collection gap cannot count toward sustained
      // idle/load. Begin a new observation window after reconnecting.
      if (previous && (!previous.online || node.collectedAtUnix - previous.collectedAtUnix > 180)) resetConditions(node.id);

      for (const gpu of node.data.gpus) {
        const key = `${node.id}:${gpu.id}`;
        const temperatureKey = `temperature:${key}`;
        if (preferences.gpuTemperature && gpu.temperature >= preferences.temperatureThresholdCelsius) {
          if (!activeConditions.current.has(temperatureKey)) {
            notify(t('alerts.temperatureTitle', { node: nodeName }), t('alerts.temperatureBody', { gpu: gpu.id, temperature: gpu.temperature }));
            activeConditions.current.add(temperatureKey);
          }
        } else {
          activeConditions.current.delete(temperatureKey);
        }

        const idleKey = `idle:${key}`;
        const hasActiveGpuProcess = node.data.processes.some((process) =>
          process.gpu_index === gpu.id && (process.vram_used_mb ?? 0) >= 100,
        );
        const idle = isIdleGpuMetric(gpu.utilization, gpu.memory_used, gpu.memory_total) && !hasActiveGpuProcess;
        if (preferences.idleGpu && idle) {
          const firstIdleAt = conditionsSince.current.get(idleKey) ?? node.collectedAtUnix;
          conditionsSince.current.set(idleKey, firstIdleAt);
          if (!activeConditions.current.has(idleKey) && node.collectedAtUnix - firstIdleAt >= preferences.idleAfterMinutes * 60) {
            notify(t('alerts.idleTitle', { node: nodeName }), t('alerts.idleBody', { gpu: gpu.id, minutes: preferences.idleAfterMinutes }));
            activeConditions.current.add(idleKey);
          }
        } else {
          conditionsSince.current.delete(idleKey);
          activeConditions.current.delete(idleKey);
        }

        const loadKey = `load:${key}`;
        if (preferences.sustainedGpuLoad && gpu.utilization >= 95) {
          const firstHighAt = conditionsSince.current.get(loadKey) ?? node.collectedAtUnix;
          conditionsSince.current.set(loadKey, firstHighAt);
          if (!activeConditions.current.has(loadKey) && node.collectedAtUnix - firstHighAt >= preferences.sustainedLoadMinutes * 60) {
            notify(t('alerts.loadTitle', { node: nodeName }), t('alerts.loadBody', { gpu: gpu.id, minutes: preferences.sustainedLoadMinutes }));
            activeConditions.current.add(loadKey);
          }
        } else {
          conditionsSince.current.delete(loadKey);
          activeConditions.current.delete(loadKey);
        }

        const previousGpu = previous?.data?.gpus.find((item) => item.id === gpu.id);
        if (preferences.memoryReleased && previousGpu) {
          const previousFree = Math.max(0, previousGpu.memory_total - previousGpu.memory_used);
          const currentFree = Math.max(0, gpu.memory_total - gpu.memory_used);
          if (previousFree < preferences.releasedVramGb && currentFree >= preferences.releasedVramGb) {
            notify(t('alerts.memoryTitle', { node: nodeName }), t('alerts.memoryBody', { gpu: gpu.id, amount: currentFree.toFixed(1) }));
          }
        }
      }

      const remoteUsername = userId === null ? '' : loadLocalTerminalProfile(userId, node.id)?.sshUser.trim() ?? '';
      if (preferences.processEnded && remoteUsername && previous?.data) {
        const currentPids = new Set(node.data.processes.map((process) => process.pid));
        const ended = previous.data.processes.filter((process) => process.user === remoteUsername && process.gpu_index !== null && process.gpu_index !== undefined && !currentPids.has(process.pid)).length;
        if (ended > 0) notify(t('alerts.processEndedTitle', { node: nodeName }), t('alerts.processEndedBody', { count: ended }));
      }
    }
  }, [nodes, preferences, t, userId]);
}
