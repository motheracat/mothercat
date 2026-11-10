import Link from "next/link";
import type { ReactNode } from "react";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <h1 className="sr-only">{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {actions ? <div className="page-header-actions flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}

export function Panel({
  title,
  action,
  children,
  className = "",
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`surface-card ${className}`}>
      <div className="surface-card-header">
        <h2 className="surface-title">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}

export function EmptyState({
  children,
  action,
}: {
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <p>{children}</p>
      {action ? <div className="mt-4 flex justify-center gap-2">{action}</div> : null}
    </div>
  );
}

export function LoadingState({ rows = 4 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Loading" aria-busy="true" className="space-y-4 p-5">
      {Array.from({ length: rows }, (_, index) => (
        <div key={index} className="flex items-center gap-4">
          <div className="skeleton h-8 w-8 shrink-0 rounded-full" />
          <div className="flex-1 space-y-2">
            <div className="skeleton w-1/3" />
            <div className="skeleton w-1/5" />
          </div>
          <div className="skeleton hidden w-1/5 sm:block" />
        </div>
      ))}
    </div>
  );
}

export function LoadingTable({ columns, rows = 5 }: { columns: number; rows?: number }) {
  return (
    <div role="status" aria-label="Loading records" aria-busy="true" className="p-5">
      <div className="space-y-4">
        {Array.from({ length: rows }, (_, row) => (
          <div key={row} className="grid items-center gap-4" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
            {Array.from({ length: columns }, (_, column) => (
              <div key={column} className={`skeleton ${column === 0 ? "w-3/4" : "w-full"}`} />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export function WorkspacePageSkeleton() {
  return (
    <div aria-label="Loading page" aria-busy="true" className="workspace-page-skeleton">
      <div className="page-header">
        <div className="min-w-0 flex-1 space-y-3">
          <div className="skeleton h-7 w-48 max-w-full" />
          <div className="skeleton w-72 max-w-full" />
        </div>
        <div className="skeleton h-10 w-32 shrink-0" />
      </div>
      <section className="surface-card">
        <div className="surface-card-header">
          <div className="skeleton w-36" />
          <div className="skeleton w-24" />
        </div>
        <LoadingTable columns={4} rows={6} />
      </section>
    </div>
  );
}

export function ErrorBanner({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div role="alert" className="error-banner">
      <p className="text-sm">{message}</p>
      <button type="button" className="button-secondary shrink-0" onClick={onRetry}>Retry</button>
    </div>
  );
}

export function StatCard({
  label,
  value,
  href,
  delta,
}: {
  label: string;
  value: string;
  href: string;
  delta?: string;
}) {
  return (
    <Link href={href} className="surface-card dashboard-stat-card">
      <p className="dashboard-stat-label">{label}</p>
      <p className="dashboard-stat-value">{value}</p>
      {delta ? <p className="dashboard-stat-delta">{delta}</p> : null}
    </Link>
  );
}

export function StatusPill({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: "success" | "warning" | "danger" | "neutral" | "info";
}) {
  return <span className={`status-pill status-${tone}`}>{label}</span>;
}

export function Toolbar({ children }: { children: ReactNode }) {
  return <div className="mb-3 flex flex-wrap items-end gap-3">{children}</div>;
}

export function TableFrame({ children }: { children: ReactNode }) {
  return <div className="max-h-[min(60vh,640px)] overflow-auto">{children}</div>;
}

export function DataTable({
  columns,
  children,
  empty,
}: {
  columns: string[];
  children?: ReactNode;
  empty?: ReactNode;
}) {
  return (
    <TableFrame>
      <table className="data-table">
        <thead>
          <tr>{columns.map((column) => <th key={column} scope="col" className="h-10">{column}</th>)}</tr>
        </thead>
        <tbody>
          {children ?? (
            <tr>
              <td colSpan={columns.length} className="!h-auto">
                {empty}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </TableFrame>
  );
}
