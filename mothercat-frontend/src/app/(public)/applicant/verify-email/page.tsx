import { Suspense } from "react";

import ApplicantEmailVerification from "./verification";

export default function ApplicantEmailVerificationPage() {
  return (
    <Suspense fallback={<p className="mx-auto max-w-xl px-4 py-20 text-sm text-slate-600">Loading verification…</p>}>
      <ApplicantEmailVerification />
    </Suspense>
  );
}
