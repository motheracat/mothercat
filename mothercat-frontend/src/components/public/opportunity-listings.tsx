"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ArrowRight, BriefcaseBusiness, GraduationCap, MapPin } from "lucide-react";
import Link from "next/link";

import { Dialog } from "@/components/workspace/dialog";
import {
  createDemoApplication,
  opportunityStoreEvent,
  readOpportunityDemoData,
  writeOpportunityDemoData,
  type Opportunity,
  type OpportunityDemoData,
} from "@/lib/opportunities/demo-store";

type ApplicationForm = {
  fullName: string;
  email: string;
  phone: string;
  institution: string;
  course: string;
  matricNumber: string;
  message: string;
};

const emptyApplication: ApplicationForm = {
  fullName: "",
  email: "",
  phone: "",
  institution: "",
  course: "",
  matricNumber: "",
  message: "",
};

const allowedDocumentTypes = new Set([
  "application/pdf",
  "application/msword",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
]);
const maxDemoDocumentBytes = 10 * 1024 * 1024;

function isAllowedDocument(file: File) {
  const extensionAllowed = /\.(pdf|doc|docx)$/i.test(file.name);
  return extensionAllowed && (!file.type || allowedDocumentTypes.has(file.type));
}

export function OpportunityListings() {
  const [data, setData] = useState<OpportunityDemoData | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selectedOpportunity, setSelectedOpportunity] = useState<Opportunity | null>(null);
  const [application, setApplication] = useState(emptyApplication);
  const [document, setDocument] = useState<File | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);

  const refresh = useCallback(() => {
    try {
      setData(readOpportunityDemoData());
      setLoadError(null);
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "Unable to load current opportunities.");
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

  function openApplication(opportunity: Opportunity) {
    setSelectedOpportunity(opportunity);
    setApplication(emptyApplication);
    setDocument(null);
    setSubmitted(false);
    setFormError(null);
  }

  function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedOpportunity || !data || !document) {
      setFormError("Select your CV or SIWES introduction letter before submitting.");
      return;
    }
    if (!isAllowedDocument(document)) {
      setFormError("Choose a PDF, DOC, or DOCX document.");
      return;
    }
    if (document.size === 0 || document.size > maxDemoDocumentBytes) {
      setFormError("The document must be non-empty and no larger than 10 MB.");
      return;
    }

    try {
      const newApplication = createDemoApplication({
        opportunityId: selectedOpportunity.id,
        opportunityTitle: selectedOpportunity.title,
        type: selectedOpportunity.type,
        ...application,
        fileName: document.name,
        fileType: document.type,
        fileSize: document.size,
      });
      writeOpportunityDemoData({
        ...data,
        applications: [newApplication, ...data.applications],
      });
      setSubmitted(true);
      setFormError(null);
    } catch {
      setFormError("Unable to save this demo application in browser storage.");
    }
  }

  const published = data?.opportunities.filter((opportunity) => opportunity.status === "published") ?? [];
  const jobs = published.filter((opportunity) => opportunity.type === "job");
  const placements = published.filter((opportunity) => opportunity.type === "siwes");

  function renderCards(opportunities: Opportunity[], type: Opportunity["type"]) {
    if (!opportunities.length) {
      return (
        <p className="rounded-xl border border-slate-200 bg-white px-5 py-8 text-sm leading-6 text-slate-600">
          {type === "job" ? "There are no open job opportunities right now." : "There are no SIWES placements open right now."}
        </p>
      );
    }
    return (
      <div className="grid gap-4">
        {opportunities.map((opportunity) => (
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
                <p><span className="font-semibold text-[#0f1b33]">Requirements:</span> {opportunity.requirements}</p>
              </div>
            </details>
            <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4 text-sm sm:grid-cols-2">
              <p><span className="font-semibold text-[#0f1b33]">Work arrangement:</span> {opportunity.workMode}</p>
              <p><span className="font-semibold text-[#0f1b33]">Apply by:</span> {opportunity.deadline}</p>
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
      <div role="note" className="mb-6 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-5 text-amber-950">
        Demo listings: applications and document uploads are stored only in this browser and are not sent to Mothercat.
      </div>
      {loadError ? (
        <div role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">
          <p>{loadError}</p>
          <button type="button" className="mt-2 font-semibold underline" onClick={refresh}>Try again</button>
        </div>
      ) : !data ? (
        <p role="status" className="py-8 text-sm text-slate-600">Loading opportunities…</p>
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
        onClose={() => setSelectedOpportunity(null)}
        actions={(
          <button type="button" className="button-secondary" onClick={() => setSelectedOpportunity(null)}>
            {submitted ? "Done" : "Cancel"}
          </button>
        )}
      >
        {submitted ? (
          <div role="status" className="space-y-3">
            <p className="font-semibold text-[var(--text)]">Demo application submitted.</p>
            <p className="text-sm leading-6 text-[var(--muted)]">
              Your application and the selected filename are saved locally in this browser for demonstration only. The document has not been uploaded to a server.
            </p>
          </div>
        ) : selectedOpportunity ? (
          <form className="grid gap-4" onSubmit={submitApplication}>
            <div role="note" className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-950">
              Demo only. Do not enter sensitive personal information; data and the document are not transmitted to Mothercat.
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="workspace-field">
                <span className="field-label">Full name</span>
                <input className="form-input" required maxLength={160} value={application.fullName} onChange={(event) => setApplication({ ...application, fullName: event.target.value })} />
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
                <input className="form-input" required maxLength={80} value={application.matricNumber} onChange={(event) => setApplication({ ...application, matricNumber: event.target.value })} />
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Supporting document (CV or SIWES introduction letter)</span>
                <input
                  className="form-input !h-auto py-2"
                  type="file"
                  accept=".pdf,.doc,.docx,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
                  required
                  onChange={(event) => {
                    setDocument(event.target.files?.[0] ?? null);
                    setFormError(null);
                  }}
                />
                <span className="text-xs text-[var(--muted)]">PDF, DOC, or DOCX; up to 10 MB for this demo.</span>
              </label>
              <label className="workspace-field sm:col-span-2">
                <span className="field-label">Message (optional)</span>
                <textarea className="form-input form-textarea" rows={3} maxLength={2000} value={application.message} onChange={(event) => setApplication({ ...application, message: event.target.value })} />
              </label>
            </div>
            {formError ? <p role="alert" className="field-error">{formError}</p> : null}
            <div className="flex justify-end">
              <button type="submit" className="button-primary">Submit demo application</button>
            </div>
          </form>
        ) : null}
      </Dialog>
    </>
  );
}
