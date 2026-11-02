"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Bell, ChevronDown, Search } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { formatAuthError, logout } from "@/lib/auth/client";
import type { AuthSession } from "@/lib/auth/types";
import { WorkspaceSidebar } from "@/components/workspace/sidebar";

const breadcrumbLabels: Record<string, string> = {
  dashboard: "Dashboard",
  staff: "Staff directory",
  imports: "Staff imports",
  departments: "Departments",
  roles: "Roles & permissions",
  audit: "Audit log",
  notifications: "Notifications",
  account: "Password & security",
  settings: "Settings",
};

function getInitials(name: string) {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("") || "U";
}

export function WorkspaceShell({
  children,
  session,
  devOpen,
}: {
  children: React.ReactNode;
  session: AuthSession | null;
  devOpen: boolean;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [signOutError, setSignOutError] = useState<string | null>(null);
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef<HTMLDivElement>(null);
  const userMenuButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!userMenuOpen) return;
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setUserMenuOpen(false);
        userMenuButtonRef.current?.focus();
      }
    }
    function closeOnOutsideClick(event: PointerEvent) {
      if (event.target instanceof Node && !userMenuRef.current?.contains(event.target)) {
        setUserMenuOpen(false);
      }
    }
    window.addEventListener("keydown", closeOnEscape);
    document.addEventListener("pointerdown", closeOnOutsideClick);
    return () => {
      window.removeEventListener("keydown", closeOnEscape);
      document.removeEventListener("pointerdown", closeOnOutsideClick);
    };
  }, [userMenuOpen]);

  async function handleSignOut() {
    setIsSigningOut(true);
    setSignOutError(null);
    try {
      await logout();
      router.replace("/login");
    } catch (error) {
      setSignOutError(formatAuthError(error, "Unable to sign out. Please try again."));
    } finally {
      setIsSigningOut(false);
    }
  }

  const segments = pathname.split("/").filter(Boolean);
  const pageLabel = segments.map((segment) => breadcrumbLabels[segment] ?? segment).join(" / ");
  const signOutButton = (
    <button
      type="button"
      onClick={handleSignOut}
      disabled={isSigningOut}
      className="flex h-10 w-full items-center rounded-lg px-3 text-left text-sm text-white/70 transition-colors duration-150 hover:bg-white/5 hover:text-white disabled:opacity-60"
    >
      {isSigningOut ? "Signing out…" : "Sign out"}
    </button>
  );

  return (
    <div className="workspace-shell">
      {devOpen ? (
        <div className="border-b border-amber-200 bg-amber-100 px-4 py-2 text-center text-xs font-medium text-amber-900">
          Development navigation mode. You are not signed in and no data is real.
        </div>
      ) : null}
      <WorkspaceSidebar
        session={session}
        signOut={signOutButton}
        footer={signOutError ? <p role="alert" className="px-3 pt-2 text-xs text-red-200">{signOutError}</p> : null}
      />

      <div className="workspace-main">
        <header className="workspace-topbar">
          <div className="flex min-w-0 items-center gap-3">
            <nav aria-label="Breadcrumb" className="truncate text-sm">
              <Link href="/dashboard" className="text-[var(--muted)] hover:text-[var(--text)]">Dashboard</Link>
              {pathname !== "/dashboard" ? (
                <>
                  <span className="mx-2 text-slate-300">/</span>
                  <span aria-current="page" className="font-medium text-[var(--text)]">{pageLabel}</span>
                </>
              ) : null}
            </nav>
          </div>

          <div className="hidden min-w-0 flex-1 justify-center px-4 md:flex">
            <label className="relative block w-full max-w-[360px]">
              <span className="sr-only">Search workspace</span>
              <Search size={16} aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted)]" />
              <input className="form-input !h-9 !pl-9" placeholder="Search" />
            </label>
          </div>

          <div className="flex shrink-0 items-center gap-3">
            <Link
              href="/notifications"
              aria-label="Notifications"
              aria-current={pathname === "/notifications" ? "page" : undefined}
              className="relative rounded-lg p-2 text-[var(--muted)] hover:bg-[var(--page-bg)] hover:text-[var(--text)]"
            >
              <Bell size={18} aria-hidden="true" />
            </Link>
            {session ? (
              <div className="user-menu" ref={userMenuRef}>
                <button
                  type="button"
                  ref={userMenuButtonRef}
                  className="user-menu-trigger"
                  aria-expanded={userMenuOpen}
                  aria-controls="workspace-user-menu"
                  aria-label={`User menu for ${session.displayName}`}
                  onClick={() => setUserMenuOpen((open) => !open)}
                >
                  <span className="flex h-8 w-8 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-700">{getInitials(session.displayName)}</span>
                  <span className="hidden max-w-36 sm:block">
                    <span className="block truncate text-xs font-medium text-[var(--text)]">{session.displayName}</span>
                    <span className="block truncate text-[11px] text-[var(--muted)]">{session.role}</span>
                  </span>
                  <ChevronDown size={15} aria-hidden="true" className="user-menu-chevron text-[var(--muted)]" />
                </button>
                <div
                  id="workspace-user-menu"
                  className="user-menu-panel"
                  data-open={userMenuOpen}
                  aria-hidden={!userMenuOpen}
                >
                  <p className="border-b border-[var(--border)] px-2 py-2 text-xs text-[var(--muted)]">{session.displayName}</p>
                  <Link href="/account" tabIndex={userMenuOpen ? 0 : -1} onClick={() => setUserMenuOpen(false)}>Password &amp; security</Link>
                  <button type="button" tabIndex={userMenuOpen ? 0 : -1} disabled={isSigningOut} onClick={handleSignOut}>{isSigningOut ? "Signing out…" : "Sign out"}</button>
                  {signOutError ? <p role="alert" className="px-2 py-1 text-xs text-[var(--danger-text)]">{signOutError}</p> : null}
                </div>
              </div>
            ) : null}
          </div>
        </header>

        <main className="workspace-content">
          <div className="workspace-content-inner">{children}</div>
        </main>
      </div>
    </div>
  );
}
