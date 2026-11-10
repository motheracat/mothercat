import Image from "next/image";
import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { OpportunityListings } from "@/components/public/opportunity-listings";

export default function OpportunitiesPage() {
  return (
    <main className="text-[#0f1b33]">
      <section className="border-b border-slate-200 bg-[#f4f7fc] px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        <div className="mx-auto grid max-w-6xl items-center gap-8 lg:grid-cols-[1.1fr_0.9fr]">
          <div>
            <p className="text-sm font-semibold uppercase text-[#1f5fd0]">Careers at Mothercat</p>
            <h1 className="mt-4 max-w-3xl font-serif text-4xl leading-tight sm:text-5xl">Build the places and infrastructure people rely on.</h1>
            <p className="mt-5 max-w-2xl text-base leading-7 text-slate-600 sm:text-lg">
              Mothercat Limited brings regional insight, engineering discipline, and delivery-focused execution to infrastructure, commercial, and urban development projects.
            </p>
          </div>
          <div className="relative min-h-[220px] overflow-hidden rounded-lg border border-white shadow-[0_12px_30px_rgba(15,27,51,0.12)] sm:min-h-[300px]">
            <Image
              src="/images/Blueprint%20Construction%20Skyline.png"
              alt="Mothercat engineering and construction project"
              fill
              priority
              className="object-cover"
              sizes="(max-width: 1024px) 100vw, 42vw"
            />
          </div>
        </div>
      </section>

      <section aria-label="Career opportunities" className="mx-auto grid max-w-6xl gap-10 px-4 py-14 sm:px-6 sm:py-16 lg:grid-cols-[minmax(0,1fr)_320px] lg:px-8">
        <OpportunityListings />
        <aside className="h-fit border-l-2 border-[#1f5fd0] bg-[#f4f7fc] p-6 sm:p-7">
          <h2 className="font-serif text-2xl">Already part of the team?</h2>
          <p className="mt-3 text-sm leading-6 text-slate-600">Access the staff workspace using your staff ID and password.</p>
          <Link
            href="/login"
            className="mt-6 inline-flex min-h-11 items-center gap-2 rounded-md bg-[#071535] px-5 text-sm font-semibold text-white transition hover:bg-[#1f5fd0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2"
          >
            Staff Login
            <ArrowRight size={16} aria-hidden="true" />
          </Link>
        </aside>
      </section>
    </main>
  );
}
