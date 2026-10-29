import Image from "next/image";
import Link from "next/link";

import { isDevOpen } from "@/lib/auth/mode";
import { getCurrentSession } from "@/lib/auth/session";
import { WorkspaceSidebar } from "@/components/workspace/sidebar";

const logoSrc = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";

export default async function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const devOpen = isDevOpen();
  const session = await getCurrentSession();

  if (!devOpen && !session) {
    const { redirect } = await import("next/navigation");
    redirect(`/login?redirect=${encodeURIComponent("/dashboard")}`);
  }

  return (
    <div className="min-h-screen bg-[#f4f7fc] text-[#0f1b33]">
      {devOpen && (
        <div className="border-b border-amber-200 bg-amber-100 px-4 py-3 text-center text-sm font-medium text-amber-900">
          Development navigation mode. You are not signed in and no data is real.
        </div>
      )}

      <div className="flex min-h-[calc(100vh-52px)]">
        <aside className="w-64 border-r border-slate-200 bg-[#0b1f4d] text-white">
          <div className="border-b border-white/10 px-5 py-5">
            <Link href="/dashboard" className="flex items-center">
              <span className="flex h-16 w-16 items-center justify-center overflow-hidden rounded-full border border-white/15 bg-white/90 p-1 shadow-[0_10px_30px_rgba(15,26,46,0.35)] sm:h-20 sm:w-20">
                <Image src={logoSrc} alt="Mothercat logo" width={160} height={160} className="h-full w-full rounded-full object-contain brightness-125" />
              </span>
            </Link>
          </div>
          <WorkspaceSidebar session={session} />
        </aside>

        <div className="flex-1">
          <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.22em] text-slate-500">Workspace</p>
              <h1 className="mt-2 font-serif text-4xl text-[#0f1b33]">Dashboard</h1>
            </div>
            <div className="text-sm text-slate-600">{session?.displayName ?? "Not signed in"}</div>
          </header>
          <main className="p-6">{children}</main>
        </div>
      </div>
    </div>
  );
}

        <div className="flex-1">
          <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.22em] text-slate-500">Workspace</p>
              <h1 className="mt-2 font-serif text-4xl text-[#0f1b33]">Dashboard</h1>
            </div>
            <div className="text-sm text-slate-600">{session?.displayName ?? "Not signed in"}</div>
          </header>
          <main className="p-6">{children}</main>
        </div>
      </div>
    </div>
  );
}
