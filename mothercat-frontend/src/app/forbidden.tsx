import Link from "next/link";

export default function ForbiddenPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[#071535] px-6 text-white">
      <div className="text-center">
        <p className="text-sm uppercase tracking-[0.24em] text-blue-200">403</p>
        <h1 className="mt-4 font-serif text-5xl">Forbidden</h1>
        <p className="mt-4 text-slate-300">You do not have permission to view this page.</p>
        <Link href="/dashboard" className="mt-8 inline-flex rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white">
          Return to dashboard
        </Link>
      </div>
    </main>
  );
}
