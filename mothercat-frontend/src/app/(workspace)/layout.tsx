import { isDevOpen } from "@/lib/auth/mode";
import { getCurrentSession } from "@/lib/auth/session";
import { WorkspaceShell } from "@/components/workspace/workspace-shell";

export default async function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const devOpen = isDevOpen();
  const session = await getCurrentSession();

  if (!devOpen && !session) {
    const { redirect } = await import("next/navigation");
    redirect(`/login?redirect=${encodeURIComponent("/dashboard")}`);
  }

  return <WorkspaceShell session={session} devOpen={devOpen}>{children}</WorkspaceShell>;
}
