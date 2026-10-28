import Image from "next/image";
import { siteContent } from "@/content/site";

export default function AboutPage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8">
      <p className="text-sm font-semibold uppercase tracking-[0.24em] text-blue-600">About</p>
      <h1 className="mt-3 font-serif text-5xl text-[#0f1b33]">Built on expertise and trust</h1>

      <div className="mt-10 grid gap-8 lg:grid-cols-[1.05fr_0.95fr]">
        <div className="space-y-6 text-base leading-7 text-slate-600">
          <p>
            <strong>Mothercat Limited</strong> is an international civil engineering and construction contractor part of the C.A.T. Group, originally founded with roots connected to Lebanon and a long-standing presence in the Middle East and West Africa.
          </p>
          <p>
            The company brings together regional insight, engineering discipline, and delivery-focused execution to support critical infrastructure, commercial assets, and urban development projects across complex operating environments.
          </p>
          <p>
            Our approach is grounded in practical project management, quality control, and strong stakeholder coordination—delivering lasting value through disciplined execution and trusted partnerships.
          </p>
        </div>

        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_10px_30px_rgba(15,27,51,0.05)]">
          <div className="relative h-[360px] w-full">
            <Image src="/images/about-us.jpeg" alt="Mothercat construction and engineering work" fill className="object-cover" sizes="(max-width: 1024px) 100vw, 50vw" />
          </div>
        </div>
      </div>
    </main>
  );
}
