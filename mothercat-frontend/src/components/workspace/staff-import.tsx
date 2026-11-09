"use client";

import { useState, type FormEvent } from "react";
import { FileUp, Search } from "lucide-react";
import Link from "next/link";

import { PageHeader, Panel } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import {
  formatOrganizationError,
  getStaffImport,
  OrganizationApiError,
  parseStaffImport,
  uploadStaffCsv,
  type StaffImport,
} from "@/lib/organization/api";

const maxFileSize = 5 * 1024 * 1024;

function ImportResult({ result }: { result: StaffImport }) {
  const isComplete = result.status === "completed" && result.result.error_count === 0;
  return (
    <section className="mt-5 rounded-lg border border-[var(--border)] bg-[var(--page-bg)] p-4" aria-labelledby="import-result-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 id="import-result-title" className="font-semibold">{result.status === "completed" ? "Import complete" : "Import validation failed"}</h3>
          <p className="mt-1 break-all text-xs text-[var(--muted)]">Import ID: {result.id}</p>
        </div>
        <span className={`status-pill ${isComplete ? "status-success" : "status-warning"}`}>{result.status}</span>
      </div>
      <dl className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div><dt className="text-xs text-[var(--muted)]">Rows checked</dt><dd className="mt-1 text-lg font-semibold">{result.result.total_rows}</dd></div>
        <div><dt className="text-xs text-[var(--muted)]">Imported</dt><dd className="mt-1 text-lg font-semibold">{result.result.imported_rows}</dd></div>
        <div><dt className="text-xs text-[var(--muted)]">Issues</dt><dd className="mt-1 text-lg font-semibold">{result.result.error_count}</dd></div>
      </dl>
      {result.result.errors && result.result.errors.length > 0 ? (
        <div className="mt-4">
          <h4 className="text-sm font-semibold">Rows to review</h4>
          <ul className="mt-2 space-y-2">
            {result.result.errors.map((issue, index) => (
              <li key={`${issue.row}-${issue.code}-${index}`} className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950">
                <span className="font-medium">Row {issue.row}{issue.staff_id ? ` · ID:${issue.staff_id}` : ""}:</span> {issue.message}
              </li>
            ))}
          </ul>
          {result.result.errors_truncated ? <p className="mt-2 text-xs text-[var(--muted)]">Some row issues were omitted. The service reported {result.result.error_count} issue(s) in total.</p> : null}
        </div>
      ) : result.result.error_count > 0 ? (
        <p className="mt-3 text-sm text-[var(--warning-text)]">The service reported {result.result.error_count} issue(s), but did not return row details.</p>
      ) : null}
    </section>
  );
}

export function StaffImport({ canImport }: { canImport: boolean }) {
  const { notify } = useWorkspace();
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<StaffImport | null>(null);
  const [uploading, setUploading] = useState(false);
  const [importId, setImportId] = useState("");
  const [lookupBusy, setLookupBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);

  function chooseFile(nextFile: File | null) {
    setFile(nextFile);
    setFileError(null);
    setError(null);
    if (!nextFile) return;
    if (!nextFile.name.toLowerCase().endsWith(".csv")) {
      setFileError("Choose a CSV file.");
    } else if (nextFile.size > maxFileSize) {
      setFileError("The CSV file must be 5 MiB or smaller.");
    }
  }

  async function submitUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file || fileError) return;
    setUploading(true);
    setError(null);
    setResult(null);
    try {
      const imported = await uploadStaffCsv(file);
      setResult(imported);
      setImportId(imported.id);
      if (imported.status === "completed" && imported.result.error_count === 0) {
        notify("Staff CSV import completed.");
      } else {
        notify(`Import validation found ${imported.result.error_count} issue(s). Review the results below.`, "warning");
      }
    } catch (requestError) {
      let failedImport: StaffImport | null = null;
      if (requestError instanceof OrganizationApiError) {
        failedImport = parseStaffImport(requestError.payload);
        if (failedImport) {
          setResult(failedImport);
          setImportId(failedImport.id);
        }
      }
      const message = formatOrganizationError(requestError, "Unable to import this CSV.");
      setError(message);
      notify(message, failedImport ? "warning" : "error");
    } finally {
      setUploading(false);
    }
  }

  async function lookup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const id = importId.trim();
    if (!id) return;
    setLookupBusy(true);
    setError(null);
    try {
      setResult(await getStaffImport(id));
      notify("Import result loaded.", "info");
    } catch (requestError) {
      const message = formatOrganizationError(requestError, "Unable to retrieve this import result.");
      setError(message);
      notify(message, "error");
    } finally {
      setLookupBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Staff imports"
        description="."
        actions={<Link href="/staff" className="button-secondary">Back to staff directory</Link>}
      />
      {!canImport ? (
        <div role="alert" className="error-banner"><p className="text-sm">You do not have permission to import staff records.</p></div>
      ) : (
        <>
          {error ? <div role="alert" className="error-banner mb-4"><p className="text-sm">{error}</p></div> : null}
          <Panel title="Upload staff CSV">
            <div className="p-5">
              <p className="max-w-3xl text-sm leading-6 text-[var(--muted)]">
                Upload a CSV with required <code>staff_id</code> and <code>full_name</code> columns. Optional columns are <code>work_email</code>, <code>job_title</code>, and <code>department_id</code>. The import is all-or-nothing; new profiles start as pending.
              </p>
              <div className="mt-4 rounded-lg border border-blue-100 bg-blue-50 px-4 py-3 text-sm text-blue-950">
                Staff numbers must be 4 or 5 digits without the display-only ID: prefix. Maximum file size is 5 MiB and maximum data rows is 5,000.
              </div>
              <form className="mt-5" onSubmit={(event) => void submitUpload(event)}>
                <label className="block text-sm font-medium">
                  CSV file
                  <input
                    className="form-input mt-1 !h-auto py-2"
                    type="file"
                    accept=".csv,text/csv"
                    onChange={(event) => chooseFile(event.target.files?.[0] ?? null)}
                    required
                  />
                </label>
                {fileError ? <p role="alert" className="mt-2 text-sm text-[var(--danger-text)]">{fileError}</p> : null}
                <button type="submit" className="button-primary mt-4" disabled={!file || Boolean(fileError) || uploading}>
                  <FileUp size={16} aria-hidden="true" />
                  {uploading ? "Uploading and validating…" : "Upload and validate"}
                </button>
                {uploading ? <p role="status" className="mt-3 text-sm text-[var(--muted)]">Processing the CSV. This may take a moment for larger files.</p> : null}
              </form>
              {result ? <ImportResult result={result} /> : null}
            </div>
          </Panel>

          <div className="mt-5">
            <Panel title="Look up an import result">
              <div className="p-5">
                <p className="text-sm text-[var(--muted)]">Retrieve a previously returned import by its UUID.</p>
                <form className="mt-4 flex flex-wrap items-end gap-3" onSubmit={(event) => void lookup(event)}>
                  <label className="min-w-[220px] flex-1 text-sm font-medium">
                    Import ID
                    <input className="form-input mt-1" value={importId} onChange={(event) => setImportId(event.target.value)} />
                  </label>
                  <button type="submit" className="button-secondary" disabled={!importId.trim() || lookupBusy}>
                    <Search size={16} aria-hidden="true" />
                    {lookupBusy ? "Looking up…" : "Look up"}
                  </button>
                </form>
              </div>
            </Panel>
          </div>
        </>
      )}
    </>
  );
}
