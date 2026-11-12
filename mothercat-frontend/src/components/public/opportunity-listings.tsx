"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowRight, BriefcaseBusiness, GraduationCap, MapPin } from "lucide-react";
import Link from "next/link";

import { Dialog } from "@/components/workspace/dialog";
import {
  formatOpportunityError,
  listPublicOpportunities,
  submitSiwesApplication,
  type Opportunity,
} from "@/lib/opportunities/api";

type ApplicationForm = {
  full_name: string;
  email: string;
  phone: string;
  institution: string;
  course: string;
  matric_number: string;
  message: string;
};

const emptyApplication: ApplicationForm = {
  full_name: "",
  email: "",
  phone: "",
  institution: "",
  course: "",
  matric_number: "",
  message: "",
};

const allowedDocumentTypes = new Set([
  "application/pdf",
  "application/msword",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
]);
const maxDocumentBytes = 10 * 1024 * 1024;
const refreshIntervalMs = 60_000;

function isAllowedDocument(file: File) {
  return /\.(pdf|doc|docx)$/i.test(file.name) && (!file.type || allowedDocumentTypes.has(file.type));
}

function formatDeadline(value: string | null) {
  if (!value) return "No deadline";
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

export function OpportunityListings() {
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selectedOpportunity, setSelectedOpportunity] = useState<Opportunity | null>(null);
  const [application, setApplication] = useState(emptyApplication);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const requestRef = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    try {
      const result = await listPublicOpportunities({ limit: 100 }, controller.signal);
      if (!controller.signal.aborted) {
        setOpportunities(result.opportunities.filter((item) => item.status === "published"));
        setLoadError(null);
      }
    } catch (error) {
      if (!controller.signal.aborted) setLoadError(formatOpportunityError(error, "Unable to load current opportunities."));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void refresh();
    }, refreshIntervalMs);
    function refreshWhenVisible() {
      if (document.visibilityState === "visible") void refresh();
    }
    window.addEventListener("focus", refreshWhenVisible);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", refreshWhenVisible);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      requestRef.current?.abort();
    };
  }, [refresh]);

  function openApplication(opportunity: Opportunity) {
    setSelectedOpportunity(opportunity);
    setApplication(emptyApplication);
    setSelectedFile(null);
    setSubmitted(false);
    setFormError(null);
  }

  async function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedOpportunity || !selectedFile) {
      setFormError("Select your SIWES introduction letter before submitting.");
      return;
    }
    if (!isAllowedDocument(selectedFile)) {
      setFormError("Choose a PDF, DOC, or DOCX document.");
      return;
    }
    if (selectedFile.size === 0 || selectedFile.size > maxDocumentBytes) {
      setFormError("The document must be non-empty and no larger than 10 MiB.");
      return;
    }

    setSubmitting(true);
    setFormError(null);
    try {
      await submitSiwesApplication(selectedOpportunity.id, { ...application, document: selectedFile });
      setSubmitted(true);
      void refresh();
    } catch (error) {
      setFormError(formatOpportunityError(error, "Unable to submit your application."));
    } finally {
      setSubmitting(false);
    }
  }

  const jobs = opportunities.filter((opportunity) => opportunity.type === "job");
  const placements = opportunities.filter((opportunity) => opportunity.type === "siwes");

  function renderCards(items: Opportunity[], type: Opportunity["type"]) {
    if (!items.length) {
      return (
        <p className="rounded-xl border border-slate-200 bg-white px-5 py-8 text-sm leading-6 text-slate-600">
          {type === "job" ? "There are no open job opportunities right now." : "There are no SIWES placements open right now."}
        </p>
      );
    }
    return (
      <div className="grid gap-4">
        {items.map((opportunity) => (
          <article key={opportunity.id} className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <span className="inline-flex items-center gap-1.5 rounded-full bg-blue-50 px-3 py-1 text-xs font-semibold text-[#1f5fd0]">
                  {type === "job" ? <BriefcaseBusiness size={14} aria-hidden="true" /> : <GraduationCap size={15} aria-hidden="true" />}
                  {type === "job" ? "Job opening" : "SIWES / industrial training"}
                </span>
                <h3 className="mt-3 text-xl font-semibold text-[#0f1b33]">{opportunity.title}</h3>
              </div>
              <span className="inline-flex items-center gap-1.5 text-sm text-slate-600">
                <MapPin size={15} aria-hidden="true" />
                {opportunity.location}
              </span>
            </div>
            <p className="mt-3 text-sm leading-6 text-slate-600">{opportunity.summary}</p>
            <details className="mt-4 rounded-lg bg-slate-50 px-4 py-3">
              <summary className="cursor-pointer text-sm font-semibold text-[#0f1b33] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0]">
                Role details and requirements
              </summary>
              <div className="mt-3 space-y-3 text-sm leading-6 text-slate-600">
                <p>{opportunity.description}</p>
                <p><span className="font-semibold text-[#0f1b33]">Requirements:</span> {opportunity.requirements || "No additional requirements listed."}</p>
              </div>
            </details>
            <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4 text-sm sm:grid-cols-2">
              <p><span className="font-semibold text-[#0f1b33]">Work arrangement:</span> {opportunity.work_mode}</p>
              <p><span className="font-semibold text-[#0f1b33]">Apply by:</span> {formatDeadline(opportunity.application_deadline)}</p>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              {type === "siwes" ? (
                <button
                  type="button"
                  className="inline-flex min-h-11 items-center gap-2 rounded-md bg-[#071535] px-5 text-sm font-semibold text-white transition hover:bg-[#1f5fd0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2"
                  onClick={() => openApplication(opportunity)}
                >
                  Apply for this placement
                  <ArrowRight size={16} aria-hidden="true" />
                </button>
              ) : (
                <Link
                  href="/contact"
                  className="inline-flex min-h-11 items-center gap-2 rounded-md border border-[#071535]/20 px-5 text-sm font-semibold text-[#071535] transition hover:bg-slate-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2"
                >
                  Contact recruitment
                  <ArrowRight size={16} aria-hidden="true" />
                </Link>
              )}
            </div>
          </article>
        ))}
      </div>
    );
  }

  return (
    <>
      {loadError ? (
        <div role="alert" className="mb-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">
          <p>{loadError}</p>
          <button type="button" className="mt-2 font-semibold underline" onClick={() => void refresh()}>Try again</button>
        </div>
      ) : null}
      {loading ? (
        <p role="status" className="py-8 text-sm text-slate-600">Loading opportunities…</p>
      ) : !opportunities.length && !loadError ? (
        <p className="py-8 text-sm text-slate-600">No opportunities are currently open. Please check back soon.</p>
      ) : (
        <div className="space-y-12">
          <section aria-labelledby="job-openings-title">
            <p className="text-sm font-semibold uppercase text-slate-500">Current opportunities</p>
            <h2 id="job-openings-title" className="mt-3 font-serif text-3xl">Open roles</h2>
            <div className="mt-6">{renderCards(jobs, "job")}</div>
          </section>
          <section aria-labelledby="siwes-openings-title">
            <p className="text-sm font-semibold uppercase text-slate-500">Students and recent graduates</p>
            <h2 id="siwes-openings-title" className="mt-3 font-serif text-3xl">IT / SIWES placements</h2>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600">
              Explore industrial training placements and apply with your student details and supporting document.
            </p>
            <div className="mt-6">{renderCards(placements, "siwes")}</div>
          </section>
        </div>
      )}

      <Dialog
        open={selectedOpportunity !== null}
        title={selectedOpportunity ? `Apply: ${selectedOpportunity.title}` : "Apply for SIWES"}
        onClose={() => {
          if (!submitting) setSelectedOpportunity(null);
        }}
        actions={(
          <button type="button" className="button-secondary" disabled={submitting} onClick={() => setSelectedOpportunity(null)}>
            {submitted ? "Done" : "Cancel"}
          </button>
        )}
      >
        {submitted ? (
          <div role="status" className="space-y-3">
            <p className="font-semibold text-[var(--text)]">Your application was submitted securely.</p>
            <p className="text-sm leading-6 text-[var(--muted)]">Mothercat’s recruitment team can now review your application. Your personal details and document were sent to the application service, not saved in this browser.</p>
          </div>
        ) : selectedOpportunity ? (
          <form className="grid gap-4" onSubmit={(event) => void submitApplication(event)}>
            <p className="rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-xs leading-5 text-blue-950">
              Your details and SIWES introduction letter are submitted securely for recruitment review. Documents are checked before being stored.
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="workspace-field">
                <span className="field-label">Full name</span>
                <input className="form-input" required maxLength={160} value={application.full_name} onChange={(event) => setApplication({ ...application, full_name: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Email</span>
                <input className="form-input" type="email" required maxLength={254} value={application.email} onChange={(event) => setApplication({ ...application, email: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Phone</span>
                <input className="form-input" type="tel" required maxLength={40} value={application.phone} onChange={(event) => setApplication({ ...application, phone: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Institution</span>
                <input className="form-input" required maxLength={180} value={application.institution} onChange={(event) => setApplication({ ...application, institution: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Course / programme</span>
                <input className="form-input" required maxLength={160} value={application.course} onChange={(event) => setApplication({ ...application, course: event.target.value })} />
              </label>
              <label className="workspace-field">
                <span className="field-label">Matriculation number</span>
                <input className="form-input" required maxLength={80} value={application.matric_number} onChange={(event) => setApplication({ ...application, matric_number: event.target.value })} />
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Supporting document (CV or SIWES introduction letter)</span>
                <input
                  className="form-input !h-auto py-2"
                  type="file"
                  accept=".pdf,.doc,.docx,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
                  required
                  disabled={submitting}
                  onChange={(event) => {
                    setSelectedFile(event.target.files?.[0] ?? null);
                    setFormError(null);
                  }}
                />
                <span className="text-xs text-[var(--muted)]">PDF, DOC, or DOCX; maximum 10 MiB. The server verifies file content and scans the document.</span>
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Message (optional)</span>
                <textarea className="form-input form-textarea" rows={3} maxLength={2000} value={application.message} onChange={(event) => setApplication({ ...application, message: event.target.value })} />
              </label>
            </div>
            {formError ? <p role="alert" className="field-error">{formError}</p> : null}
            <div className="flex justify-end">
              <button type="submit" className="button-primary" disabled={submitting}>
                {submitting ? "Submitting securely…" : "Submit application"}
              </button>
            </div>
          </form>
        ) : null}
      </Dialog>
    </>
  );
}
