import { useCallback, useEffect, useRef, useState } from "react";

import { asApiClientError, type ApiClientError } from "../api/httpClient";
import {
  createSession,
  readCurrentSession,
  revokeCurrentSession,
  type CreateSessionInput,
  type SessionSnapshot,
} from "../api/sessionApi";

export type AnonymousReason = "signed-out" | "session-ended" | "revocation-indeterminate";

export type SessionBoundaryState =
  | { phase: "checking" }
  | { phase: "anonymous"; reason?: AnonymousReason }
  | { phase: "authenticated"; session: SessionSnapshot }
  | { phase: "unavailable"; error: ApiClientError };

export type LoginActionState =
  | { phase: "idle" }
  | { phase: "submitting" }
  | { phase: "error"; error: ApiClientError };

export type LogoutActionState =
  | { phase: "idle" }
  | { phase: "logging-out" }
  | { phase: "error"; error: ApiClientError };

export interface UseSessionBoundaryResult {
  sessionState: SessionBoundaryState;
  loginState: LoginActionState;
  logoutState: LogoutActionState;
  retryCurrentSession: () => void;
  signIn: (input: CreateSessionInput) => Promise<boolean>;
  signOut: () => Promise<boolean>;
}

function isUnauthenticated(error: ApiClientError): boolean {
  return error.kind === "http" && error.status === 401 && error.code === "unauthenticated";
}

function isRevocationIndeterminate(error: ApiClientError): boolean {
  return (
    error.kind === "http" &&
    error.status === 503 &&
    error.code === "session_revocation_indeterminate"
  );
}

export function useSessionBoundary(): UseSessionBoundaryResult {
  const [sessionState, setSessionState] = useState<SessionBoundaryState>({ phase: "checking" });
  const [loginState, setLoginState] = useState<LoginActionState>({ phase: "idle" });
  const [logoutState, setLogoutState] = useState<LogoutActionState>({ phase: "idle" });

  const sessionStateRef = useRef(sessionState);
  const checkGeneration = useRef(0);
  const actionGeneration = useRef(0);
  const activeCheck = useRef<AbortController | null>(null);
  const activeAction = useRef<AbortController | null>(null);

  const publishSessionState = useCallback((next: SessionBoundaryState) => {
    sessionStateRef.current = next;
    setSessionState(next);
  }, []);

  const retryCurrentSession = useCallback(() => {
    if (activeAction.current !== null) {
      return;
    }

    checkGeneration.current += 1;
    const currentGeneration = checkGeneration.current;
    activeCheck.current?.abort();

    const controller = new AbortController();
    activeCheck.current = controller;
    publishSessionState({ phase: "checking" });
    setLoginState({ phase: "idle" });
    setLogoutState({ phase: "idle" });

    void readCurrentSession({ signal: controller.signal }).then(
      (response) => {
        if (checkGeneration.current === currentGeneration && !controller.signal.aborted) {
          publishSessionState({ phase: "authenticated", session: response.data });
          activeCheck.current = null;
        }
      },
      (cause: unknown) => {
        if (checkGeneration.current !== currentGeneration || controller.signal.aborted) {
          return;
        }

        const error = asApiClientError(cause);
        publishSessionState(
          isUnauthenticated(error) ? { phase: "anonymous" } : { phase: "unavailable", error },
        );
        activeCheck.current = null;
      },
    );
  }, [publishSessionState]);

  const signIn = useCallback(
    async (input: CreateSessionInput): Promise<boolean> => {
      if (activeAction.current !== null || sessionStateRef.current.phase !== "anonymous") {
        return false;
      }

      actionGeneration.current += 1;
      const currentGeneration = actionGeneration.current;
      const controller = new AbortController();
      activeAction.current = controller;
      setLoginState({ phase: "submitting" });
      setLogoutState({ phase: "idle" });

      try {
        const response = await createSession(input, { signal: controller.signal });
        if (actionGeneration.current !== currentGeneration || controller.signal.aborted) {
          return false;
        }

        checkGeneration.current += 1;
        activeCheck.current?.abort();
        activeCheck.current = null;
        publishSessionState({ phase: "authenticated", session: response.data });
        setLoginState({ phase: "idle" });
        return true;
      } catch (cause: unknown) {
        if (actionGeneration.current !== currentGeneration || controller.signal.aborted) {
          return false;
        }

        setLoginState({ phase: "error", error: asApiClientError(cause) });
        return false;
      } finally {
        if (actionGeneration.current === currentGeneration) {
          activeAction.current = null;
        }
      }
    },
    [publishSessionState],
  );

  const signOut = useCallback(async (): Promise<boolean> => {
    const current = sessionStateRef.current;
    if (activeAction.current !== null || current.phase !== "authenticated") {
      return false;
    }

    // Starting a mutating action establishes a new session-state generation.
    // A late read must never restore the credentials that logout is revoking.
    checkGeneration.current += 1;
    activeCheck.current?.abort();
    activeCheck.current = null;

    actionGeneration.current += 1;
    const currentGeneration = actionGeneration.current;
    const controller = new AbortController();
    activeAction.current = controller;
    setLogoutState({ phase: "logging-out" });
    setLoginState({ phase: "idle" });

    try {
      await revokeCurrentSession(current.session.csrfToken, { signal: controller.signal });
      if (actionGeneration.current !== currentGeneration || controller.signal.aborted) {
        return false;
      }

      publishSessionState({ phase: "anonymous", reason: "signed-out" });
      setLogoutState({ phase: "idle" });
      return true;
    } catch (cause: unknown) {
      if (actionGeneration.current !== currentGeneration || controller.signal.aborted) {
        return false;
      }

      const error = asApiClientError(cause);
      if (isUnauthenticated(error)) {
        publishSessionState({ phase: "anonymous", reason: "session-ended" });
        setLogoutState({ phase: "idle" });
        return true;
      }
      if (isRevocationIndeterminate(error)) {
        publishSessionState({ phase: "anonymous", reason: "revocation-indeterminate" });
        setLogoutState({ phase: "idle" });
        // The browser credential was cleared, but a caller must not interpret the
        // return value as proof that the server-side token was revoked.
        return false;
      }

      // For ordinary failures the browser credential may still be valid. Keep the
      // snapshot and CSRF token in component memory so the user can explicitly retry.
      setLogoutState({ phase: "error", error });
      return false;
    } finally {
      if (actionGeneration.current === currentGeneration) {
        activeAction.current = null;
      }
    }
  }, [publishSessionState]);

  useEffect(() => {
    retryCurrentSession();
    return () => {
      checkGeneration.current += 1;
      actionGeneration.current += 1;
      activeCheck.current?.abort();
      activeAction.current?.abort();
      activeCheck.current = null;
      activeAction.current = null;
    };
  }, [retryCurrentSession]);

  return {
    sessionState,
    loginState,
    logoutState,
    retryCurrentSession,
    signIn,
    signOut,
  };
}
