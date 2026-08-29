import { useCallback, useEffect, useRef, useState } from "react";
import { asApiClientError, type ApiClientError, type ApiResponse } from "../../../api/httpClient";
import {
  fetchHealth,
  fetchReadiness,
  type HealthResponse,
  type ReadinessResponse,
} from "../../../api/systemApi";

export type ProbeLoadState<T> =
  | { phase: "loading" }
  | {
      phase: "success";
      response: ApiResponse<T>;
    }
  | {
      phase: "error";
      error: ApiClientError;
    };

export interface SystemStatusState {
  health: ProbeLoadState<HealthResponse>;
  readiness: ProbeLoadState<ReadinessResponse>;
  completedAt?: string;
}

export interface UseSystemStatusResult {
  state: SystemStatusState;
  refresh: () => void;
}

function createLoadingState(): SystemStatusState {
  return {
    health: { phase: "loading" },
    readiness: { phase: "loading" },
  };
}

export function useSystemStatus(): UseSystemStatusResult {
  const [state, setState] = useState<SystemStatusState>(createLoadingState);
  const generation = useRef(0);
  const activeController = useRef<AbortController | null>(null);

  const refresh = useCallback(() => {
    generation.current += 1;
    const currentGeneration = generation.current;
    activeController.current?.abort();

    const controller = new AbortController();
    activeController.current = controller;
    setState(createLoadingState());

    const healthRequest = fetchHealth({ signal: controller.signal });
    const readinessRequest = fetchReadiness({ signal: controller.signal });

    void healthRequest.then(
      (response) => {
        if (generation.current === currentGeneration && !controller.signal.aborted) {
          setState((current) => ({
            ...current,
            health: { phase: "success", response },
          }));
        }
      },
      (error: unknown) => {
        if (generation.current === currentGeneration && !controller.signal.aborted) {
          setState((current) => ({
            ...current,
            health: { phase: "error", error: asApiClientError(error) },
          }));
        }
      },
    );

    void readinessRequest.then(
      (response) => {
        if (generation.current === currentGeneration && !controller.signal.aborted) {
          setState((current) => ({
            ...current,
            readiness: { phase: "success", response },
          }));
        }
      },
      (error: unknown) => {
        if (generation.current === currentGeneration && !controller.signal.aborted) {
          setState((current) => ({
            ...current,
            readiness: { phase: "error", error: asApiClientError(error) },
          }));
        }
      },
    );

    void Promise.allSettled([healthRequest, readinessRequest]).then(() => {
      if (generation.current === currentGeneration && !controller.signal.aborted) {
        setState((current) => ({
          ...current,
          completedAt: new Date().toISOString(),
        }));
      }
    });
  }, []);

  useEffect(() => {
    refresh();
    return () => {
      generation.current += 1;
      activeController.current?.abort();
    };
  }, [refresh]);

  return { state, refresh };
}
