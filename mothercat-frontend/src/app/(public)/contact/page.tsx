import Link from "next/link";

export default function ContactPage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8">
      <p className="text-sm font-semibold uppercase tracking-[0.24em] text-blue-600">Contact</p>
      <h1 className="mt-3 font-serif text-5xl text-[#0f1b33]">Let’s talk</h1>

      <div className="mt-10 grid gap-8 lg:grid-cols-[0.9fr_1.1fr]">
        <div className="rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
          <h2 className="text-xl font-semibold">Get in touch</h2>
          <ul className="mt-6 space-y-4 text-sm leading-6 text-slate-600">
            <li>Email: hello@mothercat.example</li>
            <li>Phone: +27 11 000 0000</li>
            <li>Address: Johannesburg, South Africa</li>
          </ul>
          <div className="mt-8 text-sm text-slate-600">
            <Link href="/login" className="font-semibold text-blue-600">Staff Login</Link>
          </div>
        </div>

        <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
          <p className="text-base leading-7 text-slate-600">
            The enquiry form is intentionally disabled until a backend endpoint is available. Submit requests through the approved channel while the API is being finalized.
          </p>
        </div>
      </div>
    </main>
  );
}
