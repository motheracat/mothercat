"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";

import { GlassSelect } from "@/components/workspace/glass-select";
import { DataTable, EmptyState, PageHeader, Panel } from "@/components/workspace/ui";
import {
  createDemoOpportunity,
  opportunityStoreEvent,
  readOpportunityDemoData,
  writeOpportunityDemoData,
  type Opportunity,
  type OpportunityDemoData,
} from "@/lib/opportunities/demo-store";

type OpportunityForm = Omit<Opportunity, "id" | "createdAt" | "status">;

type Props = {
  canPublish: boolean;
  canReviewApplications: boolean;
  canReviewSiwes: boolean;
};

const initialForm: OpportunityForm = {
  type: "job" as const,
  title: "",
  location: "",
  workMode: "",
  summary: "",
  description: "",
  requirements: "",
  deadline: "",
};

function displayType(type: Opportunity["type"]) {
  return type === "siwes" ? "SIWES / industrial training" : "Job";
}

export function OpportunityManagement({
  canPublish,
  canReviewApplications,
  canReviewSiwes,
}: Props) {
  const [data, setData] = useState<OpportunityDemoData | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [form, setForm] = useState(initialForm);

  const refresh = useCallback(() => {
    try {
      setData(readOpportunityDemoData());
      setLoadError(null);
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "Unable to load demo opportunities.");
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(refresh);
    window.addEventListener(opportunityStoreEvent, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(opportunityStoreEvent, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, [refresh]);

  function submitOpportunity(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canPublish || !data) return;
    setSaveError(null);
    setNotice(null);
    const newOpportunity = createDemoOpportunity({
      ...form,
      status: "draft",
    });
    const updated = { ...data, opportunities: [newOpportunity, ...data.opportunities] };
    try {
      writeOpportunityDemoData(updated);
      setData(updated);
      setForm(initialForm);
      setNotice("Opportunity saved as a draft in this browser.");
    } catch {
      setSaveError("Could not save this demo opportunity in browser storage.");
    }
  }

  function togglePublished(opportunity: Opportunity) {
    if (!canPublish || !data) return;
    const nextStatus: Opportunity["status"] = opportunity.status === "published" ? "draft" : "published";
    const updated = {
      ...data,
      opportunities: data.opportunities.map((item) =>
        item.id === opportunity.id ? { ...item, status: nextStatus } : item,
      ),
    };
    try {
      writeOpportunityDemoData(updated);
      setData(updated);
      setNotice(nextStatus === "published" ? "Opportunity published to the public listings." : "Opportunity moved back to drafts.");
      setSaveError(null);
    } catch {
      setSaveError("Could not update this demo opportunity in browser storage.");
    }
  }

  if (!canPublish && !canReviewApplications && !canReviewSiwes) {
    return (
      <>
        <PageHeader title="Opportunities" description="Manage job openings and student placements." />
        <p role="alert" className="error-banner">You do not have permission to manage opportunities or review applications.</p>
      </>
    );
  }

  return (
    <div className="grid gap-5 pb-6">
      <PageHeader
        title="Opportunity management"
        description="."
      />
      <div role="note" className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-5 text-amber-950">
        Demo mode: changes are stored only in this browser. Publishing and application uploads are not sent to a server.
      </div>
      {notice ? <p role="status" className="access-notice">{notice}</p> : null}
      {saveError ? <p role="alert" className="field-error">{saveError}</p> : null}
      {loadError ? (
        <div role="alert" className="error-banner">
          <p className="text-sm">{loadError}</p>
          <button type="button" className="button-secondary" onClick={refresh}>Retry</button>
        </div>
      ) : null}

      {canPublish ? (
        <Panel title="Create an opportunity">
          <form className="grid gap-4 p-4 sm:p-5" onSubmit={submitOpportunity}>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="workspace-field">
                <span className="field-label">Opportunity type</span>
                <GlassSelect
                  label="Opportunity type"
                  value={form.type}
                  placeholder="Job opening"
                  options={[
                    { value: "job", label: "Job opening" },
                    { value: "siwes", label: "SIWES / industrial training placement" },
                  ]}
                  onChange={(value) => setForm({ ...form, type: value as Opportunity["type"] })}
                />
              </label>
              <label className="workspace-field">
                <span className="field-label">Title</span>
                <input className="form-input" required maxLength={160} value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Location</span>
                <input className="form-input" required maxLength={120} value={form.location} onChange={(event) => setForm({ ...form, location: event.target.value })} placeholder="e.g. Lagos, Nigeria" />
              </label>
              <label className="workspace-field">
                <span className="field-label">Work arrangement</span>
                <input className="form-input" required maxLength={80} value={form.workMode} onChange={(event) => setForm({ ...form, workMode: event.target.value })} placeholder="e.g. Full-time or On-site" />
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Short summary</span>
                <textarea className="form-input form-textarea" required maxLength={400} rows={2} value={form.summary} onChange={(event) => setForm({ ...form, summary: event.target.value })} />
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Description</span>
                <textarea className="form-input form-textarea" required maxLength={8000} rows={4} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Requirements</span>
                <textarea className="form-input form-textarea" required maxLength={4000} rows={3} value={form.requirements} onChange={(event) => setForm({ ...form, requirements: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Application deadline</span>
                <input className="form-input" type="date" required value={form.deadline} onChange={(event) => setForm({ ...form, deadline: event.target.value })} />
              </label>
            </div>
            <div className="flex justify-end">
              <button type="submit" className="button-primary" disabled={!data}>Save as draft</button>
            </div>
          </form>
        </Panel>
      ) : null}

      <Panel title="Opportunities">
        {!data ? (
          <p role="status" className="p-5 text-sm text-[var(--muted)]">Loading demo opportunities…</p>
        ) : data.opportunities.length === 0 ? (
          <EmptyState>No opportunities have been created yet.</EmptyState>
        ) : (
          <DataTable columns={["Opportunity", "Type", "Deadline", "Status", ""]}>
            {data.opportunities.map((opportunity) => (
              <tr key={opportunity.id}>
                <td>
                  <span className="block font-medium">{opportunity.title}</span>
                  <span className="text-xs text-[var(--muted)]">{opportunity.location} · {opportunity.workMode}</span>
                </td>
                <td>{displayType(opportunity.type)}</td>
                <td className="whitespace-nowrap">{opportunity.deadline}</td>
                <td>{opportunity.status === "published" ? "Published" : "Draft"}</td>
                <td className="text-right">
                  {canPublish ? (
                    <button type="button" className="button-secondary" onClick={() => togglePublished(opportunity)}>
                      {opportunity.status === "published" ? "Unpublish" : "Publish"}
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </DataTable>
        )}
      </Panel>

      {(canReviewApplications || canReviewSiwes) ? (
        <Panel title="Demo applications">
          {!data ? (
            <p role="status" className="p-5 text-sm text-[var(--muted)]">Loading demo applications…</p>
          ) : !data.applications.some((application) =>
            application.type === "siwes" ? canReviewSiwes : canReviewApplications,
          ) ? (
            <EmptyState>Student submissions will appear here after they apply on the public opportunities page in this browser.</EmptyState>
          ) : (
            <DataTable columns={["Student", "Placement", "Institution", "Document", "Submitted"]}>
              {data.applications
                .filter((application) => application.type === "siwes" ? canReviewSiwes : canReviewApplications)
                .map((application) => (
                  <tr key={application.id}>
                    <td>
                      <span className="block font-medium">{application.fullName}</span>
                      <span className="text-xs text-[var(--muted)]">{application.email} · {application.phone}</span>
                    </td>
                    <td>{application.opportunityTitle}</td>
                    <td>
                      <span className="block">{application.institution || "—"}</span>
                      <span className="text-xs text-[var(--muted)]">{application.course}</span>
                    </td>
                    <td>
                      <span className="block">{application.fileName}</span>
                      <span className="text-xs text-[var(--muted)]">Demo file · {(application.fileSize / 1024).toFixed(0)} KB</span>
                    </td>
                    <td className="whitespace-nowrap">{new Date(application.submittedAt).toLocaleDateString()}</td>
                  </tr>
                ))}
            </DataTable>
          )}
        </Panel>
      ) : null}
    </div>
  );
}
