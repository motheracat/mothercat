"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { EmptyState, ErrorBanner, PageHeader, Panel, StatusPill } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import { hasPermission } from "@/lib/auth/permissions";
import {
  formatOrganizationError,
  getStaffImport,
  submitStaffImport,
} from "@/lib/organization/api";
import type { StaffImportBatch } from "@/lib/organization/api";

const maxUploadBytes = 5 * 1024 * 1024;
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export function StaffImportForm() {
  const router = useRouter();
  const { session, devOpen, notify } = useWorkspace();
  const canImport = !devOpen && hasPermission(session, "staff.import");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (!file) {
      setError("Choose a CSV file to upload.");
      return;
    }
    if (!file.name.toLowerCase().endsWith(".csv")) {
      setError("Choose a file with a .csv extension.");
      return;
    }
    if (file.size > maxUploadBytes) {
      setError("This file exceeds the 5 MiB upload limit.");
      return;
    }

    setSubmitting(true);
    try {
      const result = await submitStaffImport(file);
      if (result.validationFailed) {
        notify(
          `Import validation failed with ${result.batch.result.error_count} row issue${result.batch.result.error_count === 1 ? "" : "s"}.${result.requestId ? ` Reference: ${result.requestId}` : ""}`,
          "error",
        );
      } else {
        notify(`${result.batch.result.imported_rows} staff ${result.batch.result.imported_rows === 1 ? "profile" : "profiles"} imported.`);
      }
      router.push(`/staff/imports/${encodeURIComponent(result.batch.id)}`);
    } catch (submissionError) {
      const message = formatOrganizationError(submissionError, "Unable to import this CSV.");
      setError(message);
      notify(message, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <>
      <PageHeader title="Staff imports" description="Import staff profiles from a validated CSV file." />
      {devOpen ? <p className="field-error mb-4">Development navigation only. Organization data is not connected in this mode.</p> : null}
      {!canImport && !devOpen ? <p role="alert" className="field-error mb-4">Your account does not have permission to import staff.</p> : null}
      {canImport ? (
        <Panel title="Upload CSV">
          <div className="p-5 sm:p-6">
            <p className="mb-4 text-sm leading-6 text-[var(--muted)]">
              Files must be UTF-8 CSV with <code>staff_id</code> and <code>full_name</code> headers.
              Optional headers are <code>work_email</code>, <code>job_title</code>, and <code>department_id</code>.
              Imports are all-or-nothing and create pending profiles only.
            </p>
            <form className="workspace-form max-w-2xl" onSubmit={(event) => void handleSubmit(event)}>
              <label className="workspace-field">
                <span className="field-label">CSV file</span>
                <input
                  className="form-input form-file"
                  type="file"
                  accept=".csv,text/csv"
                  required
                  onChange={(event) => {
                    setFile(event.target.files?.[0] ?? null);
                    setError(null);
                  }}
                />
                <span className="text-xs text-[var(--muted)]">Maximum 5 MiB and 5,000 data rows.</span>
              </label>
              {file ? <p className="text-sm text-[var(--muted)]">{file.name} · {(file.size / 1024).toFixed(1)} KiB</p> : null}
              {error ? <p role="alert" className="field-error">{error}</p> : null}
              <button type="submit" className="button-primary" disabled={submitting}>
                {submitting ? "Importing…" : "Import staff"}
              </button>
              {submitting ? <p role="status" aria-live="polite" className="text-sm text-[var(--muted)]">The import is being validated and processed. Keep this page open.</p> : null}
            </form>
            <p className="mt-6 text-xs leading-5 text-[var(--muted)]">
              Example header: <code>staff_id,full_name,work_email,job_title,department_id</code>
            </p>
          </div>
        </Panel>
      ) : null}
      <div className="mt-4"><Link href="/staff" className="button-secondary">Back to staff directory</Link></div>
    </>
  );
}

export function StaffImportDetails({ importId }: { importId: string }) {
  const { session, devOpen } = useWorkspace();
  const canImport = !devOpen && hasPermission(session, "staff.import");
  const [batch, setBatch] = useState<StaffImportBatch | null>(null);
  const [loadState, setLoadState] = useState<{ key: string; error: string | null } | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const validImportId = uuidPattern.test(importId);
  const requestKey = `${importId}|${refreshKey}`;
  const loading = canImport && validImportId && loadState?.key !== requestKey;
  const error = !validImportId
    ? "The import ID is invalid."
    : loadState?.key === requestKey
      ? loadState.error
      : null;

  useEffect(() => {
    if (!canImport || !validImportId) return;
    const controller = new AbortController();
    void getStaffImport(importId, controller.signal)
      .then(({ import: result }) => {
        setBatch(result);
        setLoadState({ key: requestKey, error: null });
      })
      .catch((requestError: unknown) => {
        if (!controller.signal.aborted) {
          setLoadState({ key: requestKey, error: formatOrganizationError(requestError, "Unable to load this import result.") });
        }
      });
    return () => controller.abort();
  }, [canImport, importId, refreshKey, requestKey, validImportId]);

  return (
    <>
      <PageHeader title="Import result" description="Review the result of a staff CSV import." />
      {devOpen ? <p className="field-error mb-4">Development navigation only. Organization data is not connected in this mode.</p> : null}
      {!canImport && !devOpen ? <p role="alert" className="field-error mb-4">Your account does not have permission to view staff imports.</p> : null}
      {canImport ? (
        <>
          {error ? <div className="mb-4"><ErrorBanner message={error} onRetry={() => setRefreshKey((value) => value + 1)} /></div> : null}
          {loading ? <p role="status" aria-live="polite" className="py-5 text-sm text-[var(--muted)]">Loading import result…</p> : null}
          {batch?.id === importId ? (
            <Panel title="Batch summary" action={<StatusPill label={batch.status} tone={batch.status === "completed" ? "success" : "danger"} />}>
              <div className="p-5 sm:p-6">
                <dl className="import-summary">
                  <div><dt>Import ID</dt><dd className="font-mono text-xs">{batch.id}</dd></div>
                  <div><dt>Rows processed</dt><dd>{batch.result.total_rows}</dd></div>
                  <div><dt>Profiles imported</dt><dd>{batch.result.imported_rows}</dd></div>
                  <div><dt>Issues</dt><dd>{batch.result.error_count}</dd></div>
                  <div><dt>Created</dt><dd>{new Date(batch.created_at).toLocaleString()}</dd></div>
                </dl>
                {batch.result.errors?.length ? (
                  <div className="mt-6 overflow-auto">
                    <h3 className="mb-2 text-sm font-semibold">Rows to review</h3>
                    <table className="data-table">
                      <caption className="sr-only">Staff import validation issues</caption>
                      <thead><tr><th scope="col" className="h-10">CSV row</th><th scope="col" className="h-10">Staff ID</th><th scope="col" className="h-10">Issue</th></tr></thead>
                      <tbody>{batch.result.errors.map((issue, index) => (
                        <tr key={`${issue.row}-${issue.code}-${index}`}>
                          <td>{issue.row}</td>
                          <td>{issue.staff_id || "—"}</td>
                          <td className="whitespace-normal">{issue.message}</td>
                        </tr>
                      ))}</tbody>
                    </table>
                    {batch.result.errors_truncated ? <p className="mt-2 text-sm text-[var(--muted)]">Some issues were omitted. Correct the listed rows and retry the complete CSV.</p> : null}
                  </div>
                ) : batch.status === "completed" ? (
                  <p className="mt-4 text-sm text-[var(--success-text)]">All rows were imported successfully.</p>
                ) : (
                  <EmptyState>The import failed without any row details.</EmptyState>
                )}
              </div>
            </Panel>
          ) : !loading && !error ? <EmptyState>Import result not available.</EmptyState> : null}
        </>
      ) : null}
      <div className="mt-4 flex flex-wrap gap-2">
        <Link href="/staff/imports" className="button-primary">Import another CSV</Link>
        <Link href="/staff" className="button-secondary">Back to staff directory</Link>
      </div>
    </>
  );
}
