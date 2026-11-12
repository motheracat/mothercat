"use client";

import Link from "next/link";
import { ArrowLeft, CheckCircle2 } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";

import {
  formatOpportunityError,
  getPublicOpportunity,
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
  full_name: "", email: "", phone: "", institution: "", course: "", matric_number: "", message: "",
};
const allowedDocumentTypes = new Set([
  "application/pdf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
]);
const maxDocumentBytes = 10 * 1024 * 1024;

function isAllowedDocument(file: File) {
  return /\.(pdf|doc|docx)$/i.test(file.name) && (!file.type || allowedDocumentTypes.has(file.type));
}

export function OpportunityApplication({ opportunityId }: { opportunityId: string }) {
  const [opportunity, setOpportunity] = useState<Opportunity | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [application, setApplication] = useState(emptyApplication);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void getPublicOpportunity(opportunityId, controller.signal)
      .then((item) => {
        if (item.type !== "siwes" || item.status !== "published") throw new Error("unavailable");
        setOpportunity(item);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setLoadError(
          error instanceof Error && error.message === "unavailable"
            ? "This placement is not open for applications."
            : formatOpportunityError(error, "Unable to load this placement."),
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [opportunityId]);

  async function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) {
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
      await submitSiwesApplication(opportunityId, { ...application, document: selectedFile });
      setSubmitted(true);
    } catch (error) {
      setFormError(formatOpportunityError(error, "Unable to submit your application."));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="bg-[#f4f7fc] px-4 py-10 text-[#0f1b33] sm:px-6 sm:py-14 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <Link href="/opportunities" className="inline-flex items-center gap-2 text-sm font-semibold text-[#1f5fd0] hover:text-[#071535] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0]">
          <ArrowLeft size={16} aria-hidden="true" /> Back to opportunities
        </Link>
        {loading ? <p role="status" className="mt-8 text-sm text-slate-600">Loading placement…</p> : null}
        {loadError ? <div role="alert" className="mt-8 rounded-xl border border-red-200 bg-red-50 p-5 text-sm text-red-900">{loadError}</div> : null}
        {opportunity ? (
          <section className="mt-6 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8">
            {submitted ? (
              <div role="status" className="space-y-4 py-4 text-center">
                <CheckCircle2 className="mx-auto text-[#1f5fd0]" size={42} aria-hidden="true" />
                <h1 className="font-serif text-3xl">Application submitted</h1>
                <p className="mx-auto max-w-xl text-sm leading-6 text-slate-600">Your application for {opportunity.title} was submitted securely. Mothercat’s recruitment team can now review it.</p>
                <Link href="/opportunities" className="button-primary inline-flex">View other opportunities</Link>
              </div>
            ) : (
              <>
                <p className="text-sm font-semibold uppercase text-[#1f5fd0]">SIWES application</p>
                <h1 className="mt-2 font-serif text-3xl sm:text-4xl">Apply for {opportunity.title}</h1>
                <p className="mt-3 text-sm leading-6 text-slate-600">{opportunity.location} · {opportunity.work_mode}</p>
                <form className="mt-8 grid gap-4" onSubmit={(event) => void submitApplication(event)}>
                  <p className="rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-xs leading-5 text-blue-950">Your details and SIWES introduction letter are submitted securely for recruitment review. Documents are checked before being stored.</p>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field label="Full name"><input className="form-input" required maxLength={160} value={application.full_name} onChange={(event) => setApplication({ ...application, full_name: event.target.value })} /></Field>
                    <Field label="Email"><input className="form-input" type="email" required maxLength={254} value={application.email} onChange={(event) => setApplication({ ...application, email: event.target.value })} /></Field>
                    <Field label="Phone"><input className="form-input" type="tel" required maxLength={40} value={application.phone} onChange={(event) => setApplication({ ...application, phone: event.target.value })} /></Field>
                    <Field label="Institution"><input className="form-input" required maxLength={180} value={application.institution} onChange={(event) => setApplication({ ...application, institution: event.target.value })} /></Field>
                    <Field label="Course / programme"><input className="form-input" required maxLength={160} value={application.course} onChange={(event) => setApplication({ ...application, course: event.target.value })} /></Field>
                    <Field label="Matriculation number"><input className="form-input" required maxLength={80} value={application.matric_number} onChange={(event) => setApplication({ ...application, matric_number: event.target.value })} /></Field>
                    <label className="workspace-field sm:col-span-2"><span className="field-label">Supporting document (CV or SIWES introduction letter)</span><input className="form-input !h-auto py-2" type="file" accept=".pdf,.doc,.docx,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document" required disabled={submitting} onChange={(event) => { setSelectedFile(event.target.files?.[0] ?? null); setFormError(null); }} /><span className="text-xs text-[var(--muted)]">PDF, DOC, or DOCX; maximum 10 MiB. The server verifies file content and scans the document.</span></label>
                    <label className="workspace-field sm:col-span-2"><span className="field-label">Message (optional)</span><textarea className="form-input form-textarea" rows={3} maxLength={2000} value={application.message} onChange={(event) => setApplication({ ...application, message: event.target.value })} /></label>
                  </div>
                  {formError ? <p role="alert" className="field-error">{formError}</p> : null}
                  <div className="flex justify-end"><button type="submit" className="button-primary" disabled={submitting}>{submitting ? "Submitting securely…" : "Submit application"}</button></div>
                </form>
              </>
            )}
          </section>
        ) : null}
      </div>
    </main>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="workspace-field"><span className="field-label">{label}</span>{children}</label>;
}
