"use client";

import { AlertCircle, CheckCircle2, Info, TriangleAlert, X } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

import type { AuthSession } from "@/lib/auth/types";

type ToastTone = "success" | "error" | "warning" | "info";
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
  const noticeId = useRef(0);
  const notify = useCallback((message: string, tone: ToastTone = "success") => {
    setNotice({ id: ++noticeId.current, message, tone });
  }, []);

  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(null), notice.tone === "error" ? 8000 : 5000);
    return () => window.clearTimeout(timeout);
  }, [notice]);

  const ToastIcon = notice?.tone === "success"
    ? CheckCircle2
    : notice?.tone === "error"
      ? AlertCircle
      : notice?.tone === "warning"
        ? TriangleAlert
        : Info;

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
            <ToastIcon className="workspace-toast-icon" size={20} aria-hidden="true" />
            <div className="workspace-toast-content">
              <p className="workspace-toast-title">
                {notice.tone === "success" ? "Success" : notice.tone === "error" ? "Action failed" : notice.tone === "warning" ? "Needs attention" : "Information"}
              </p>
              <p className="workspace-toast-message">{notice.message}</p>
            </div>
            <button type="button" aria-label="Dismiss notification" onClick={() => setNotice(null)}>
              <X size={16} aria-hidden="true" />
            </button>
          </div>
        </div>
      ) : null}
    </WorkspaceContext.Provider>
  );
}
