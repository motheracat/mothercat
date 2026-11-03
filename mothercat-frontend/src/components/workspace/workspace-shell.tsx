"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Bell, ChevronDown, Search } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { formatAuthError, logout } from "@/lib/auth/client";
import type { AuthSession } from "@/lib/auth/types";
import { listStaff, type StaffMember } from "@/lib/organization/api";
import { WorkspaceModalMenu } from "@/components/workspace/modal-menu";
import { WorkspaceSidebar } from "@/components/workspace/sidebar";
import { WorkspaceContextProvider } from "@/components/workspace/workspace-context";

const profileUpdatedEvent = "mothercat:staff-profile-updated";

const breadcrumbLabels: Record<string, string> = {
  dashboard: "Dashboard",
  staff: "Staff directory",
  imports: "Staff imports",
  departments: "Departments",
  roles: "Roles & permissions",
  audit: "Audit log",
  notifications: "Notifications",
  profile: "My profile",
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

function UserAvatar({
  name,
  imageUrl,
  className,
}: {
  name: string;
  imageUrl: string | null;
  className: string;
}) {
  const [failedImageUrl, setFailedImageUrl] = useState<string | null>(null);
  const usableImageUrl = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <span className={className} aria-hidden="true">
      {usableImageUrl && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          width={40}
          height={40}
          unoptimized
          className="workspace-user-avatar-image"
          onError={() => setFailedImageUrl(imageUrl)}
        />
      ) : (
        getInitials(name)
      )}
    </span>
  );
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
  const [profileImage, setProfileImage] = useState<{ staffId: string; url: string | null } | null>(null);
  const staffNumber = session?.staffId;
  const profileImageUrl = profileImage && profileImage.staffId === staffNumber ? profileImage.url : null;
  const userMenuButtonRef = useRef<HTMLButtonElement>(null);
  const closeUserMenu = useCallback(() => setUserMenuOpen(false), []);

  useEffect(() => {
    if (!staffNumber || devOpen) return;

    let active = true;
    void listStaff({ limit: 100, offset: 0, search: staffNumber })
      .then(({ staff }) => {
        if (!active) return;
        const currentStaff = staff.find((member) => member.staff_id === staffNumber);
        setProfileImage({ staffId: staffNumber, url: currentStaff?.profile_image_url ?? null });
      })
      .catch(() => {
        if (active) setProfileImage({ staffId: staffNumber, url: null });
      });

    return () => {
      active = false;
    };
  }, [staffNumber, devOpen]);

  useEffect(() => {
    if (!staffNumber) return;

    function handleStaffProfileUpdated(event: Event) {
      const staff = (event as CustomEvent<StaffMember>).detail;
      if (staff?.staff_id !== staffNumber) return;
      setProfileImage({ staffId: staffNumber, url: staff.profile_image_url });
    }

    window.addEventListener(profileUpdatedEvent, handleStaffProfileUpdated);
    return () => window.removeEventListener(profileUpdatedEvent, handleStaffProfileUpdated);
  }, [staffNumber]);

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
  const pageLabel = segments[0] === "staff" && segments.length > 1
    ? "Staff directory / Staff profile"
    : segments.map((segment) => breadcrumbLabels[segment] ?? segment).join(" / ");
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
    <WorkspaceContextProvider session={session} devOpen={devOpen}>
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
              <div className="user-menu">
                <button
                  type="button"
                  ref={userMenuButtonRef}
                  className="user-menu-trigger"
                  aria-expanded={userMenuOpen}
                  aria-haspopup="dialog"
                  aria-label={`User menu for ${session.displayName}`}
                  onClick={() => setUserMenuOpen((open) => !open)}
                >
                  <UserAvatar
                    name={session.displayName}
                    imageUrl={profileImageUrl}
                    className="workspace-user-avatar user-menu-avatar"
                  />
                  <span className="hidden max-w-36 sm:block">
                    <span className="block truncate text-xs font-medium text-[var(--text)]">{session.displayName}</span>
                    <span className="block truncate text-[11px] text-[var(--muted)]">{session.role}</span>
                  </span>
                  <ChevronDown size={15} aria-hidden="true" className="user-menu-chevron text-[var(--muted)]" />
                </button>
              </div>
            ) : null}
          </div>
        </header>

        <main id="main-content" tabIndex={-1} className="workspace-content">
          <div className="workspace-content-inner">{children}</div>
        </main>
      </div>
      {session ? (
        <WorkspaceModalMenu
          open={userMenuOpen}
          onClose={closeUserMenu}
          triggerRef={userMenuButtonRef}
          title="Account menu"
          placement="top"
        >
          <div className="workspace-modal-user-row">
            <UserAvatar
              name={session.displayName}
              imageUrl={profileImageUrl}
              className="workspace-user-avatar workspace-modal-avatar"
            />
            <p className="workspace-modal-user">{session.displayName}<span>{session.role}</span></p>
          </div>
          <Link
            href="/profile"
            className="workspace-modal-link"
            data-menu-initial-focus=""
            onClick={closeUserMenu}
          >
            My profile
          </Link>
          <Link
            href="/account"
            className="workspace-modal-link"
            onClick={closeUserMenu}
          >
            Password &amp; security
          </Link>
          <button
            type="button"
            className="workspace-modal-link"
            disabled={isSigningOut}
            onClick={() => {
              closeUserMenu();
              void handleSignOut();
            }}
          >
            {isSigningOut ? "Signing out…" : "Sign out"}
          </button>
          {signOutError ? <p role="alert" className="workspace-modal-error">{signOutError}</p> : null}
        </WorkspaceModalMenu>
      ) : null}
    </div>
    </WorkspaceContextProvider>
  );
}
