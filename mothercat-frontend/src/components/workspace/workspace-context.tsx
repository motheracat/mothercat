"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";

import type { AuthSession } from "@/lib/auth/types";

type ToastTone = "success" | "error";
type ToastNotice = {
  id: number;
  message: string;
  tone: ToastTone;
};
type WorkspaceContextValue = {
  session: AuthSession | null;
  devOpen: boolean;
  notify: (message: string, tone?: ToastTone) => void;
};

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function useWorkspace() {
  const context = useContext(WorkspaceContext);
  if (!context) {
    throw new Error("Workspace controls must be used inside WorkspaceShell.");
  }
  return context;
}

export function WorkspaceContextProvider({
  children,
  session,
  devOpen,
}: {
  children: React.ReactNode;
  session: AuthSession | null;
  devOpen: boolean;
}) {
  const [notice, setNotice] = useState<ToastNotice | null>(null);
  const notify = useCallback((message: string, tone: ToastTone = "success") => {
    setNotice({ id: Date.now(), message, tone });
  }, []);

  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(null), 6000);
    return () => window.clearTimeout(timeout);
  }, [notice]);

  return (
    <WorkspaceContext.Provider value={{ session, devOpen, notify }}>
      {children}
      {notice ? (
        <div className="workspace-toast-region">
          <div
            key={notice.id}
            role={notice.tone === "error" ? "alert" : "status"}
            className={`workspace-toast workspace-toast-${notice.tone}`}
          >
            <p>{notice.message}</p>
            <button type="button" aria-label="Dismiss notification" onClick={() => setNotice(null)}>Dismiss</button>
          </div>
        </div>
      ) : null}
    </WorkspaceContext.Provider>
  );
}
