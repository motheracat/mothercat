"use client";

import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";

export function Dialog({
  open,
  title,
  onClose,
  children,
  actions,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  actions?: ReactNode;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    else if (!open && dialog.open) dialog.close();
  }, [open]);

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleId}
      className="workspace-dialog"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="workspace-dialog-panel">
        <header className="workspace-dialog-header">
          <h2 id={titleId}>{title}</h2>
          <button type="button" className="button-ghost" onClick={onClose} aria-label="Close dialog">Close</button>
        </header>
        <div className="workspace-dialog-body">{children}</div>
        {actions ? <footer className="workspace-dialog-actions">{actions}</footer> : null}
      </div>
    </dialog>
  );
}
