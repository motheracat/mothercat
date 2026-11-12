"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";

import { Dialog } from "@/components/workspace/dialog";
import { GlassSelect } from "@/components/workspace/glass-select";
import { DataTable, EmptyState, PageHeader, Panel } from "@/components/workspace/ui";
import {
  changeOpportunityStatus,
  createOpportunity,
  downloadSiwesDocument,
  formatOpportunityError,
  getSiwesApplication,
  listManagedOpportunities,
  listSiwesApplications,
  updateOpportunity,
  updateSiwesApplication,
  type Opportunity,
  type OpportunityInput,
  type OpportunityStatus,
  type OpportunityType,
  type SiwesApplication,
  type SiwesApplicationStatus,
} from "@/lib/opportunities/api";

type Props = {
  canPublish: boolean;
  canReviewApplications: boolean;
  canReviewSiwes: boolean;
};

type OpportunityForm = {
  type: OpportunityType;
  title: string;
  location: string;
  work_mode: string;
  summary: string;
  description: string;
  requirements: string;
  application_deadline: string;
};

const emptyForm: OpportunityForm = {
  type: "job",
  title: "",
  location: "",
  work_mode: "",
  summary: "",
  description: "",
  requirements: "",
  application_deadline: "",
};

const applicationStatuses: SiwesApplicationStatus[] = [
  "submitted",
  "under_review",
  "shortlisted",
  "accepted",
  "rejected",
];
const applicationStatusLabels: Record<SiwesApplicationStatus, string> = {
  submitted: "Submitted",
  under_review: "Under review",
  shortlisted: "Shortlisted",
  accepted: "Accepted",
  rejected: "Rejected",
};

const validNextApplicationStatuses: Record<SiwesApplicationStatus, SiwesApplicationStatus[]> = {
  submitted: ["under_review", "rejected"],
  under_review: ["shortlisted", "accepted", "rejected"],
  shortlisted: ["accepted", "rejected"],
  accepted: [],
  rejected: [],
};
const opportunityPageSize = 25;
const applicationPageSize = 25;

function displayType(type: OpportunityType) {
  return type === "siwes" ? "SIWES / industrial training" : "Job";
}

function displayStatus(status: OpportunityStatus) {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatDate(value: string | null) {
  if (!value) return "No deadline";
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function opportunityForm(item?: Opportunity): OpportunityForm {
  return item ? {
    type: item.type,
    title: item.title,
    location: item.location,
    work_mode: item.work_mode,
    summary: item.summary,
    description: item.description,
    requirements: item.requirements,
    application_deadline: item.application_deadline ?? "",
  } : emptyForm;
}

function toOpportunityInput(form: OpportunityForm): OpportunityInput {
  return {
    ...form,
    application_deadline: form.application_deadline || null,
  };
}

function downloadBlob(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob);
  const link = window.document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.style.display = "none";
  window.document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1_000);
}

export function OpportunityManagement({
  canPublish,
  canReviewApplications,
  canReviewSiwes,
}: Props) {
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [opportunitiesLoading, setOpportunitiesLoading] = useState(true);
  const [applications, setApplications] = useState<SiwesApplication[]>([]);
  const [opportunityTotal, setOpportunityTotal] = useState(0);
  const [applicationTotal, setApplicationTotal] = useState(0);
  const [opportunityOffset, setOpportunityOffset] = useState(0);
  const [applicationOffset, setApplicationOffset] = useState(0);
  const [applicationsLoading, setApplicationsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [applicationLoadError, setApplicationLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [filterType, setFilterType] = useState<OpportunityType | "">("");
  const [filterStatus, setFilterStatus] = useState<OpportunityStatus | "">("");
  const [applicationFilter, setApplicationFilter] = useState<SiwesApplicationStatus | "">("");
  const [form, setForm] = useState<OpportunityForm>(emptyForm);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Opportunity | null>(null);
  const [saving, setSaving] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [selectedApplicationId, setSelectedApplicationId] = useState<string | null>(null);
  const [selectedApplication, setSelectedApplication] = useState<SiwesApplication | null>(null);
  const [applicationLoading, setApplicationLoading] = useState(false);
  const [applicationError, setApplicationError] = useState<string | null>(null);
  const [reviewStatus, setReviewStatus] = useState<SiwesApplicationStatus>("submitted");
  const [reviewerNote, setReviewerNote] = useState("");
  const [documentLoading, setDocumentLoading] = useState(false);
  const [applicationReloadKey, setApplicationReloadKey] = useState(0);

  const opportunityById = useMemo(
    () => new Map(opportunities.map((item) => [item.id, item])),
    [opportunities],
  );

  const reload = useCallback(() => setReloadKey((key) => key + 1), []);
  const reviewChanged = Boolean(selectedApplication) &&
    (reviewStatus !== selectedApplication?.status || reviewerNote !== selectedApplication?.reviewer_note);

  useEffect(() => {
    let active = true;
    let controller: AbortController | null = null;
    const load = async () => {
      controller?.abort();
      const currentController = new AbortController();
      controller = currentController;
      try {
        const result = await listManagedOpportunities({
          ...(filterType ? { type: filterType } : {}),
          ...(filterStatus ? { status: filterStatus } : {}),
          limit: opportunityPageSize,
          offset: opportunityOffset,
        }, currentController.signal);
        if (active) {
          setOpportunities(result.opportunities);
          setOpportunityTotal(result.page.total);
          setLoadError(null);
        }
      } catch (error) {
        if (active && !currentController.signal.aborted) {
          setLoadError(formatOpportunityError(error, "Unable to load opportunities."));
        }
      } finally {
        if (active) setOpportunitiesLoading(false);
      }
    };
    void load();
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void load();
    }, 30_000);
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") void load();
    };
    window.addEventListener("focus", refreshWhenVisible);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(interval);
      window.removeEventListener("focus", refreshWhenVisible);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
    };
  }, [filterStatus, filterType, opportunityOffset, reloadKey]);

  useEffect(() => {
    if (!canReviewSiwes) {
      setApplications([]);
      setApplicationsLoading(false);
      return;
    }
    let active = true;
    let controller: AbortController | null = null;
    const load = async () => {
      controller?.abort();
      const currentController = new AbortController();
      controller = currentController;
      try {
        const result = await listSiwesApplications({
          ...(applicationFilter ? { status: applicationFilter } : {}),
          limit: applicationPageSize,
          offset: applicationOffset,
        }, currentController.signal);
        if (active) {
          setApplications(result.applications);
          setApplicationTotal(result.page.total);
          setApplicationLoadError(null);
        }
      } catch (error) {
        if (active && !currentController.signal.aborted) {
          setApplicationLoadError(formatOpportunityError(error, "Unable to load SIWES applications."));
        }
      } finally {
        if (active) setApplicationsLoading(false);
      }
    };
    void load();
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void load();
    }, 30_000);
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") void load();
    };
    window.addEventListener("focus", refreshWhenVisible);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(interval);
      window.removeEventListener("focus", refreshWhenVisible);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
    };
  }, [applicationFilter, applicationOffset, applicationReloadKey, canReviewSiwes]);

  useEffect(() => {
    if (!selectedApplicationId) {
      setSelectedApplication(null);
      return;
    }
    const controller = new AbortController();
    setApplicationLoading(true);
    setApplicationError(null);
    void getSiwesApplication(selectedApplicationId, controller.signal)
      .then((item) => {
        if (!controller.signal.aborted) {
          setSelectedApplication(item);
          setReviewStatus(item.status);
          setReviewerNote(item.reviewer_note);
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setApplicationError(formatOpportunityError(error, "Unable to load application details."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setApplicationLoading(false);
      });
    return () => controller.abort();
  }, [selectedApplicationId]);

  function openCreateForm() {
    setEditing(null);
    setForm(emptyForm);
    setFormOpen(true);
    setSaveError(null);
  }

  function openEditForm(item: Opportunity) {
    setEditing(item);
    setForm(opportunityForm(item));
    setFormOpen(true);
    setSaveError(null);
  }

  function openApplicationReview(applicationId: string) {
    setSelectedApplication(null);
    setApplicationError(null);
    setApplicationLoading(true);
    setSelectedApplicationId(applicationId);
  }

  async function submitOpportunity(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canPublish) return;
    setSaving(true);
    setSaveError(null);
    setNotice(null);
    try {
      if (editing) {
        await updateOpportunity(editing.id, toOpportunityInput(form));
        setNotice("Draft opportunity updated.");
      } else {
        await createOpportunity(toOpportunityInput(form));
        setNotice("Opportunity saved as a draft.");
      }
      setEditing(null);
      setForm(emptyForm);
      setFormOpen(false);
      reload();
    } catch (error) {
      setSaveError(formatOpportunityError(error, "Unable to save the opportunity."));
    } finally {
      setSaving(false);
    }
  }

  async function changeStatus(item: Opportunity, action: "publish" | "unpublish" | "close") {
    setSaving(true);
    setSaveError(null);
    setNotice(null);
    try {
      const updated = await changeOpportunityStatus(item.id, action);
      setNotice(`Opportunity ${updated.status === "draft" ? "moved to drafts" : updated.status}.`);
      reload();
    } catch (error) {
      setSaveError(formatOpportunityError(error, "Unable to change opportunity status."));
    } finally {
      setSaving(false);
    }
  }

  async function saveReview() {
    if (!selectedApplication) return;
    setSaving(true);
    setApplicationError(null);
    setNotice(null);
    try {
      const updated = await updateSiwesApplication(selectedApplication.id, {
        status: reviewStatus === selectedApplication.status ? undefined : reviewStatus,
        reviewer_note: reviewerNote === selectedApplication.reviewer_note ? undefined : reviewerNote,
      });
      setSelectedApplication(updated);
      setReviewStatus(updated.status);
      setReviewerNote(updated.reviewer_note);
      setNotice("Application review saved.");
      setApplicationReloadKey((key) => key + 1);
    } catch (error) {
      setApplicationError(formatOpportunityError(error, "Unable to save the application review."));
    } finally {
      setSaving(false);
    }
  }

  async function downloadDocument() {
    if (!selectedApplication) return;
    setDocumentLoading(true);
    setApplicationError(null);
    try {
      const result = await downloadSiwesDocument(selectedApplication.id);
      downloadBlob(result.blob, result.fileName);
    } catch (error) {
      setApplicationError(formatOpportunityError(error, "Unable to download the document."));
    } finally {
      setDocumentLoading(false);
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
        description="Manage published opportunities and review live SIWES applications."
        actions={canPublish ? <button type="button" className="button-primary" onClick={openCreateForm}>Create opportunity</button> : null}
      />
      {notice ? <p role="status" className="access-notice">{notice}</p> : null}
      {saveError ? <p role="alert" className="field-error">{saveError}</p> : null}
      {loadError ? (
        <div role="alert" className="error-banner">
          <p className="text-sm">{loadError}</p>
          <button type="button" className="button-secondary" onClick={reload}>Retry</button>
        </div>
      ) : null}

      <Panel title="Opportunities">
        <div className="grid gap-3 p-4 sm:grid-cols-3">
          <label className="workspace-field">
            <span className="field-label">Type</span>
            <GlassSelect label="Opportunity type filter" value={filterType} placeholder="All types" options={[
              { value: "", label: "All types" },
              { value: "job", label: "Job openings" },
              { value: "siwes", label: "SIWES placements" },
            ]} onChange={(value) => {
              setOpportunityOffset(0);
              setOpportunitiesLoading(true);
              setFilterType(value as OpportunityType | "");
            }} />
          </label>
          {canPublish ? (
            <label className="workspace-field">
              <span className="field-label">Status</span>
              <GlassSelect label="Opportunity status filter" value={filterStatus} placeholder="All statuses" options={[
                { value: "", label: "All statuses" },
                { value: "draft", label: "Draft" },
                { value: "published", label: "Published" },
                { value: "closed", label: "Closed" },
              ]} onChange={(value) => {
                setOpportunityOffset(0);
                setOpportunitiesLoading(true);
                setFilterStatus(value as OpportunityStatus | "");
              }} />
            </label>
          ) : null}
        </div>
        {opportunitiesLoading ? (
          <p role="status" className="p-5 text-sm text-[var(--muted)]">Loading opportunities…</p>
        ) : !opportunities.length ? (
          <EmptyState>No opportunities are available for this view.</EmptyState>
        ) : (
          <DataTable columns={["Opportunity", "Type", "Deadline", "Status", "Actions"]}>
            {opportunities.map((item) => (
              <tr key={item.id}>
                <td>
                  <span className="block font-medium">{item.title}</span>
                  <span className="text-xs text-[var(--muted)]">{item.location} · {item.work_mode}</span>
                </td>
                <td>{displayType(item.type)}</td>
                <td className="whitespace-nowrap">{formatDate(item.application_deadline)}</td>
                <td>{displayStatus(item.status)}</td>
                <td className="flex flex-wrap justify-end gap-2">
                  {canPublish && item.status === "draft" ? (
                    <>
                      <button type="button" className="button-secondary" disabled={saving} onClick={() => openEditForm(item)}>Edit</button>
                      <button type="button" className="button-primary" disabled={saving} onClick={() => void changeStatus(item, "publish")}>Publish</button>
                    </>
                  ) : null}
                  {canPublish && item.status === "published" ? (
                    <>
                      <button type="button" className="button-secondary" disabled={saving} onClick={() => void changeStatus(item, "unpublish")}>Unpublish</button>
                      <button type="button" className="button-secondary" disabled={saving} onClick={() => void changeStatus(item, "close")}>Close applications</button>
                    </>
                  ) : null}
                </td>
              </tr>
            ))}
          </DataTable>
        )}
        {!opportunitiesLoading && opportunityTotal > opportunityPageSize ? (
          <div className="flex items-center justify-between gap-3 border-t border-slate-200 p-4 text-sm">
            <span>Showing {opportunityOffset + 1}–{Math.min(opportunityOffset + opportunities.length, opportunityTotal)} of {opportunityTotal}</span>
            <div className="flex gap-2">
              <button type="button" className="button-secondary" disabled={opportunityOffset === 0} onClick={() => {
                setOpportunitiesLoading(true);
                setOpportunityOffset((offset) => Math.max(0, offset - opportunityPageSize));
              }}>Previous</button>
              <button type="button" className="button-secondary" disabled={opportunityOffset + opportunityPageSize >= opportunityTotal} onClick={() => {
                setOpportunitiesLoading(true);
                setOpportunityOffset((offset) => offset + opportunityPageSize);
              }}>Next</button>
            </div>
          </div>
        ) : null}
      </Panel>

      {canReviewSiwes ? (
        <Panel title="SIWES applications">
          <div className="grid gap-3 p-4 sm:grid-cols-2">
            <label className="workspace-field">
              <span className="field-label">Application status</span>
              <GlassSelect label="Application status filter" value={applicationFilter} placeholder="All statuses" options={[
                { value: "", label: "All statuses" },
                ...applicationStatuses.map((status) => ({ value: status, label: applicationStatusLabels[status] })),
              ]} onChange={(value) => {
                setApplicationOffset(0);
                setApplicationsLoading(true);
                setApplicationFilter(value as SiwesApplicationStatus | "");
              }} />
            </label>
          </div>
          {applicationLoadError ? (
            <div role="alert" className="error-banner">
              <p className="text-sm">{applicationLoadError}</p>
              <button type="button" className="button-secondary" onClick={() => setApplicationReloadKey((key) => key + 1)}>Retry</button>
            </div>
          ) : null}
          {applicationsLoading ? (
            <p role="status" className="p-5 text-sm text-[var(--muted)]">Loading SIWES applications…</p>
          ) : !applications.length ? (
            <EmptyState>No SIWES applications match this view.</EmptyState>
          ) : (
            <DataTable columns={["Applicant", "Placement", "Institution", "Document", "Status", "Submitted", ""]}>
              {applications.map((application) => (
                <tr key={application.id}>
                  <td>
                    <span className="block font-medium">{application.full_name}</span>
                    <span className="text-xs text-[var(--muted)]">{application.email}</span>
                  </td>
                  <td>{opportunityById.get(application.opportunity_id)?.title ?? "SIWES placement"}</td>
                  <td>
                    <span className="block">{application.institution}</span>
                    <span className="text-xs text-[var(--muted)]">{application.course}</span>
                  </td>
                  <td>{application.document.file_name}</td>
                  <td>{applicationStatusLabels[application.status]}</td>
                  <td className="whitespace-nowrap">{formatTimestamp(application.submitted_at)}</td>
                  <td className="text-right">
                    <button type="button" className="button-secondary" onClick={() => openApplicationReview(application.id)}>Review</button>
                  </td>
                </tr>
              ))}
            </DataTable>
          )}
          {!applicationsLoading && applicationTotal > applicationPageSize ? (
            <div className="flex items-center justify-between gap-3 border-t border-slate-200 p-4 text-sm">
              <span>Showing {applicationOffset + 1}–{Math.min(applicationOffset + applications.length, applicationTotal)} of {applicationTotal}</span>
              <div className="flex gap-2">
                <button type="button" className="button-secondary" disabled={applicationOffset === 0} onClick={() => {
                  setApplicationsLoading(true);
                  setApplicationOffset((offset) => Math.max(0, offset - applicationPageSize));
                }}>Previous</button>
                <button type="button" className="button-secondary" disabled={applicationOffset + applicationPageSize >= applicationTotal} onClick={() => {
                  setApplicationsLoading(true);
                  setApplicationOffset((offset) => offset + applicationPageSize);
                }}>Next</button>
              </div>
            </div>
          ) : null}
        </Panel>
      ) : null}

      {canReviewApplications && !canReviewSiwes ? (
        <p className="rounded-lg border border-slate-200 bg-white p-4 text-sm text-[var(--muted)]">
          Job applications are not collected on this site; candidates are directed to Contact. No job-application records are available to review.
        </p>
      ) : null}

      {canPublish ? (
        <Dialog
          open={formOpen}
          title={editing ? "Edit draft opportunity" : "Create an opportunity"}
          onClose={() => {
            setEditing(null);
            setForm(emptyForm);
            setFormOpen(false);
          }}
          actions={(
            <>
              <button type="button" className="button-secondary" disabled={saving} onClick={() => {
                setEditing(null);
                setForm(emptyForm);
                setFormOpen(false);
              }}>Cancel</button>
              <button type="submit" form="opportunity-form" className="button-primary" disabled={saving}>
                {saving ? "Saving…" : editing ? "Save changes" : "Save as draft"}
              </button>
            </>
          )}
        >
          <form id="opportunity-form" className="grid gap-4" onSubmit={(event) => void submitOpportunity(event)}>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="workspace-field">
                <span className="field-label">Opportunity type</span>
                <GlassSelect label="Opportunity type" value={form.type} placeholder="Job opening" options={[
                  { value: "job", label: "Job opening" },
                  { value: "siwes", label: "SIWES / industrial training placement" },
                ]} onChange={(value) => setForm({ ...form, type: value as OpportunityType })} disabled={Boolean(editing) || saving} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Title</span>
                <input className="form-input" required maxLength={160} value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Location</span>
                <input className="form-input" required maxLength={120} value={form.location} onChange={(event) => setForm({ ...form, location: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Work arrangement</span>
                <input className="form-input" required maxLength={80} value={form.work_mode} onChange={(event) => setForm({ ...form, work_mode: event.target.value })} />
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
                <span className="field-label">Requirements (optional)</span>
                <textarea className="form-input form-textarea" maxLength={4000} rows={3} value={form.requirements} onChange={(event) => setForm({ ...form, requirements: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Application deadline</span>
                <input className="form-input" type="date" required value={form.application_deadline} onChange={(event) => setForm({ ...form, application_deadline: event.target.value })} />
              </label>
            </div>
            {saveError ? <p role="alert" className="field-error">{saveError}</p> : null}
          </form>
        </Dialog>
      ) : null}

      <Dialog
        open={Boolean(selectedApplicationId)}
        title="SIWES application review"
        onClose={() => {
          setSelectedApplicationId(null);
          setSelectedApplication(null);
        }}
        actions={(
          <>
            <button type="button" className="button-secondary" disabled={saving || documentLoading} onClick={() => setSelectedApplicationId(null)}>Close</button>
            {selectedApplication?.document.download_available ? (
              <button type="button" className="button-secondary" disabled={documentLoading || applicationLoading} onClick={() => void downloadDocument()}>
                {documentLoading ? "Preparing download…" : "Download document"}
              </button>
            ) : null}
            <button type="button" className="button-primary" disabled={saving || applicationLoading || !selectedApplication || !reviewChanged} onClick={() => void saveReview()}>
              {saving ? "Saving…" : "Save review"}
            </button>
          </>
        )}
      >
        {applicationLoading ? <p role="status">Loading application details…</p> : null}
        {applicationError ? <p role="alert" className="field-error">{applicationError}</p> : null}
        {selectedApplication ? (
          <div className="grid gap-4">
            <dl className="grid gap-3 sm:grid-cols-2">
              <div><dt className="field-label">Applicant</dt><dd>{selectedApplication.full_name}</dd></div>
              <div><dt className="field-label">Email</dt><dd><a href={`mailto:${selectedApplication.email}`}>{selectedApplication.email}</a></dd></div>
              <div><dt className="field-label">Phone</dt><dd><a href={`tel:${selectedApplication.phone}`}>{selectedApplication.phone}</a></dd></div>
              <div><dt className="field-label">Placement</dt><dd>{opportunityById.get(selectedApplication.opportunity_id)?.title ?? "SIWES placement"}</dd></div>
              <div><dt className="field-label">Institution</dt><dd>{selectedApplication.institution}</dd></div>
              <div><dt className="field-label">Course</dt><dd>{selectedApplication.course}</dd></div>
              <div><dt className="field-label">Matriculation number</dt><dd>{selectedApplication.matric_number}</dd></div>
              <div><dt className="field-label">Submitted</dt><dd>{formatTimestamp(selectedApplication.submitted_at)}</dd></div>
              <div className="sm:col-span-2"><dt className="field-label">Message</dt><dd>{selectedApplication.message || "No message provided."}</dd></div>
              <div className="sm:col-span-2"><dt className="field-label">Document</dt><dd>{selectedApplication.document.file_name} · {(selectedApplication.document.size_bytes / 1024).toFixed(0)} KiB</dd></div>
            </dl>
            <label className="workspace-field">
              <span className="field-label">Review status</span>
              <GlassSelect
                label="Review status"
                value={reviewStatus}
                placeholder="Select status"
                options={[
                  { value: selectedApplication.status, label: applicationStatusLabels[selectedApplication.status] },
                  ...validNextApplicationStatuses[selectedApplication.status].map((status) => ({
                    value: status,
                    label: applicationStatusLabels[status],
                  })),
                ]}
                onChange={(value) => setReviewStatus(value as SiwesApplicationStatus)}
                disabled={saving}
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">Reviewer note (optional)</span>
              <textarea className="form-input form-textarea" rows={3} maxLength={2000} value={reviewerNote} onChange={(event) => setReviewerNote(event.target.value)} disabled={saving} />
            </label>
          </div>
        ) : null}
      </Dialog>
    </div>
  );
}
