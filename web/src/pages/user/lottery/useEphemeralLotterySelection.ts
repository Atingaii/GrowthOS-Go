import { useCallback, useEffect, useRef, useState } from "react";
import { asApiClientError, type ApiClientError, type ApiResponse } from "../../../api/httpClient";
import {
  requestEphemeralSelection,
  type EphemeralSelectionResponse,
} from "../../../api/lotteryApi";

export type EphemeralSelectionState =
  | { phase: "idle" }
  | { phase: "selecting"; strategyId: string }
  | {
      phase: "success";
      strategyId: string;
      response: ApiResponse<EphemeralSelectionResponse>;
    }
  | { phase: "error"; strategyId: string; error: ApiClientError };

export interface UseEphemeralLotterySelectionResult {
  state: EphemeralSelectionState;
  select: (strategyId: string) => void;
  clear: () => void;
}

export function useEphemeralLotterySelection(): UseEphemeralLotterySelectionResult {
  const [state, setState] = useState<EphemeralSelectionState>({ phase: "idle" });
  const generation = useRef(0);
  const activeController = useRef<AbortController | null>(null);

  const select = useCallback((strategyId: string) => {
    if (activeController.current !== null) {
      return;
    }

    generation.current += 1;
    const currentGeneration = generation.current;
    const controller = new AbortController();
    activeController.current = controller;
    setState({ phase: "selecting", strategyId });

    let request: Promise<ApiResponse<EphemeralSelectionResponse>>;
    try {
      request = requestEphemeralSelection(strategyId, { signal: controller.signal });
    } catch (error: unknown) {
      activeController.current = null;
      setState({ phase: "error", strategyId, error: asApiClientError(error) });
      return;
    }

    void request.then(
      (response) => {
        if (generation.current !== currentGeneration || controller.signal.aborted) {
          return;
        }
        activeController.current = null;
        setState({ phase: "success", strategyId, response });
      },
      (error: unknown) => {
        if (generation.current !== currentGeneration || controller.signal.aborted) {
          return;
        }

        activeController.current = null;
        const clientError = asApiClientError(error);
        if (clientError.kind === "cancelled") {
          setState({ phase: "idle" });
          return;
        }
        setState({ phase: "error", strategyId, error: clientError });
      },
    );
  }, []);

  const clear = useCallback(() => {
    generation.current += 1;
    activeController.current?.abort();
    activeController.current = null;
    setState({ phase: "idle" });
  }, []);

  useEffect(
    () => () => {
      generation.current += 1;
      activeController.current?.abort();
      activeController.current = null;
    },
    [],
  );

  return { state, select, clear };
}
