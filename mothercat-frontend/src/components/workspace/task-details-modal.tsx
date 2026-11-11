"use client";

import type { ReactNode } from "react";

import { GlassModal } from "@/components/workspace/glass-modal";

export function TaskDetailsModal({
  open,
  title,
  headerMeta,
  onClose,
  children,
  footer,
}: {
  open: boolean;
  title: string;
  headerMeta?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <GlassModal
      open={open}
      title={title}
      subtitle="Task details"
      titleMeta={headerMeta}
      onClose={onClose}
      footer={footer}
    >
      {children}
    </GlassModal>
  );
}
