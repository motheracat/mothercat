"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowRight, BriefcaseBusiness, GraduationCap, MapPin } from "lucide-react";
import Link from "next/link";

import {
  formatOpportunityError,
  listPublicOpportunities,
  type Opportunity,
} from "@/lib/opportunities/api";
const refreshIntervalMs = 60_000;

function formatDeadline(value: string | null) {
  if (!value) return "No deadline";
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

export function OpportunityListings() {
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
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
                <Link
                  href={`/opportunities/${encodeURIComponent(opportunity.id)}/apply`}
                  className="inline-flex min-h-11 items-center gap-2 rounded-md bg-[#071535] px-5 text-sm font-semibold text-white transition hover:bg-[#1f5fd0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2"
                >
                  Apply for this placement
                  <ArrowRight size={16} aria-hidden="true" />
                </Link>
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
    </>
  );
}
