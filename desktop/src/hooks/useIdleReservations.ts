import { useCallback, useEffect, useRef, useState } from 'react';
import type { EnrichedNode } from '../lib/node-logic';
import { rankIdleGpuCandidates, type NodeHistoryById } from '../lib/idle-gpu';
import type {
  CreateIdleReservationPayload,
  IdleReservation,
} from '../../electron/types/ipc';
import type { NexusError } from '../../electron/types/ipc';
import type { Translator } from '../i18n';

export function useIdleReservations() {
  const [reservations, setReservations] = useState<IdleReservation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<NexusError | null>(null);

  const refresh = useCallback(async () => {
    const result = await window.nexus.idleReservations.list();
    if (result.ok) {
      setReservations(result.data);
      setError(null);
    } else {
      setError(result.error);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const create = useCallback(async (payload: CreateIdleReservationPayload) => {
    const result = await window.nexus.idleReservations.create(payload);
    if (!result.ok) {
      setError(result.error);
      return false;
    }
    setReservations((current) => [result.data, ...current]);
    setError(null);
    return true;
  }, []);

  const setStatus = useCallback(async (id: number, status: 'active' | 'paused') => {
    const result = await window.nexus.idleReservations.setStatus(id, status);
    if (!result.ok) {
      setError(result.error);
      return;
    }
    setReservations((current) => current.map((item) => item.id === id ? result.data : item));
  }, []);

  const remove = useCallback(async (id: number) => {
    const result = await window.nexus.idleReservations.remove(id);
    if (!result.ok) {
      setError(result.error);
      return false;
    }
    setReservations((current) => current.filter((item) => item.id !== id));
    return true;
  }, []);

  const applyEvaluation = useCallback((id: number, reservation: IdleReservation) => {
    setReservations((current) => current.map((item) => {
      if (item.id !== id || reservation.status === 'paused') return item;
      if (item.status === 'paused' && reservation.status === 'active') return item;
      return reservation;
    }));
  }, []);

  return { reservations, loading, error, refresh, create, setStatus, remove, applyEvaluation };
}

export function useIdleReservationMatcher(
  reservations: IdleReservation[],
  nodes: EnrichedNode[],
  historyByNode: NodeHistoryById,
  t: Translator,
  onEvaluation: (id: number, reservation: IdleReservation) => void,
) {
  const signatures = useRef(new Map<number, string>());
  const inFlight = useRef(new Set<number>());
  const nodeMap = new Map(nodes.map((node) => [node.id, node]));

  useEffect(() => {
    for (const reservation of reservations) {
      if (reservation.status !== 'active' || inFlight.current.has(reservation.id)) continue;
      const matching = rankIdleGpuCandidates(nodes, historyByNode, reservation.filters)
        .filter((candidate) => candidate.matches)
        .map((candidate) => candidate.key);
      const expired = reservation.expiresAtUnix !== null && reservation.expiresAtUnix <= Math.floor(Date.now() / 1000);
      // Expiry must be evaluated even when the candidate set stays unchanged.
      const signature = `${expired ? 'expired|' : ''}${matching.join('|')}`;
      if (signatures.current.get(reservation.id) === signature) continue;
      inFlight.current.add(reservation.id);
      void window.nexus.idleReservations.evaluate(reservation.id, matching).then(async (result) => {
        if (!result.ok) return;
        signatures.current.set(reservation.id, signature);
        onEvaluation(reservation.id, result.data.reservation);
        if (result.data.newMatchKeys.length > 0) {
          const labels = result.data.newMatchKeys.slice(0, 8).map((key) => {
            const [nodeId, gpuId] = key.split(':').map(Number);
            const node = nodeMap.get(nodeId);
            return node ? `${node.name} · GPU ${gpuId}` : key;
          });
          if (result.data.newMatchKeys.length > labels.length) labels.push(`+${result.data.newMatchKeys.length - labels.length}`);
          await window.nexus.notifications.show(
            t('idle.notificationTitle', { name: reservation.name }),
            t('idle.notificationBody', { matches: labels.join(', ') }),
          );
        }
      }).catch(() => {
        signatures.current.delete(reservation.id);
      }).finally(() => inFlight.current.delete(reservation.id));
    }
    for (const id of signatures.current.keys()) {
      if (!reservations.some((reservation) => reservation.id === id && reservation.status === 'active')) signatures.current.delete(id);
    }
  }, [historyByNode, nodes, onEvaluation, reservations, t]);
}
