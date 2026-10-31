import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[#071535] px-6 text-white">
      <div className="text-center">
        <p className="text-sm uppercase tracking-[0.24em] text-blue-200">404</p>
        <h1 className="mt-4 font-serif text-5xl">Page not found</h1>
        <p className="mt-4 text-slate-300">The page you requested could not be located.</p>
        <Link href="/" className="mt-8 inline-flex rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white">
          Return home
        </Link>
      </div>
    </main>
  );
}
