"use client";

import { X } from "lucide-react";
import { useEffect, useId, useRef } from "react";
import type { ReactNode, RefObject } from "react";

export function GlassModal({
  open,
  title,
  subtitle,
  titleMeta,
  onClose,
  children,
  footer,
  dialogClassName = "",
  panelClassName = "",
  returnFocusRef,
}: {
  open: boolean;
  title: string;
  subtitle: string;
  titleMeta?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  dialogClassName?: string;
  panelClassName?: string;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    let closeTimer: number | undefined;
    if (open) {
      dialog.removeAttribute("data-closing");
      if (!dialog.open) {
        previouslyFocusedRef.current = returnFocusRef?.current
          ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
        dialog.showModal();
      }
      const previousOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = previousOverflow;
      };
    }

    if (dialog.open) {
      dialog.dataset.closing = "true";
      closeTimer = window.setTimeout(() => {
        dialog.close();
        const focusTarget = returnFocusRef?.current ?? previouslyFocusedRef.current;
        if (focusTarget?.isConnected) focusTarget.focus();
        previouslyFocusedRef.current = null;
      }, 180);
    }
    return () => {
      if (closeTimer !== undefined) window.clearTimeout(closeTimer);
    };
  }, [open, returnFocusRef]);

  return (
    <dialog
      ref={dialogRef}
      className={`task-details-dialog ${dialogClassName}`.trim()}
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className={`task-details-glass ${panelClassName}`.trim()}>
        <header className="task-details-header">
          <div className="min-w-0">
            <h2 id={titleId}>{title}</h2>
            <p>{subtitle}</p>
            {titleMeta ? <div className="task-details-header-meta">{titleMeta}</div> : null}
          </div>
          <button type="button" className="task-details-close" aria-label="Close dialog" onClick={onClose}>
            <X size={19} aria-hidden="true" />
          </button>
        </header>
        <div className="task-details-body">{children}</div>
        {footer ? <footer className="task-details-footer">{footer}</footer> : null}
      </div>
    </dialog>
  );
}
