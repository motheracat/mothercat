"use client";

export default function ErrorBoundary({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[#071535] px-6 text-white">
      <div className="max-w-lg text-center">
        <p className="text-sm uppercase tracking-[0.24em] text-blue-200">Error</p>
        <h1 className="mt-4 font-serif text-5xl">Something went wrong</h1>
        <p className="mt-4 text-slate-300">Reference: {error.name || "unknown"}</p>
        <button onClick={() => reset()} className="mt-8 rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white">
          Try again
        </button>
      </div>
    </main>
  );
}
