"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";

import { formatAuthError, logout } from "@/lib/auth/client";
import type { AuthSession } from "@/lib/auth/types";

type SidebarItem = {
  href: string;
  label: string;
  permissions?: string[];
};

const items: SidebarItem[] = [
  { href: "/dashboard", label: "Dashboard" },
  { href: "/staff", label: "Staff directory", permissions: ["staff.read", "*"] },
  { href: "/staff/imports", label: "Staff imports", permissions: ["staff.import", "*"] },
  { href: "/departments", label: "Departments", permissions: ["departments.manage", "staff.read", "*"] },
  { href: "/roles", label: "Roles & permissions", permissions: ["roles.read", "roles.manage", "*"] },
  { href: "/audit", label: "Audit log", permissions: ["audit.read", "*"] },
  { href: "/account", label: "Password & security" },
];

export function WorkspaceSidebar({ session }: { session: AuthSession | null }) {
  const pathname = usePathname();
  const router = useRouter();
  const [logoutError, setLogoutError] = useState<string | null>(null);
  const [isLoggingOut, setIsLoggingOut] = useState(false);

  if (!session) return null;

  async function handleLogout() {
    setIsLoggingOut(true);
    setLogoutError(null);
    try {
      await logout();
      router.replace("/login");
    } catch (error) {
      setLogoutError(formatAuthError(error, "Unable to sign out. Please try again."));
    } finally {
      setIsLoggingOut(false);
    }
  }

  const perms = session.permissions ?? [];
  const hasWildcard = perms.includes("*");

  // If user has no permissions or only empty permissions
  if (!hasWildcard && (perms.length === 0 || (perms.length === 1 && perms[0] === ""))) {
    return (
      <nav className="space-y-2 p-4 text-sm">
        <Link href="/dashboard" className="block rounded-md bg-white/5 px-3 py-2 hover:bg-white/10">Dashboard</Link>
        <div className="mt-4 px-3 py-2 text-xs text-white/70">Your role has no workspace permissions yet</div>
        <Link href="/account" className="block rounded-md bg-white/5 px-3 py-2 hover:bg-white/10">Password & security</Link>
        <button type="button" onClick={handleLogout} disabled={isLoggingOut} className="w-full rounded-md bg-white/5 px-3 py-2 text-left hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60">
          {isLoggingOut ? "Signing out..." : "Sign out"}
        </button>
        {logoutError ? <p role="alert" className="px-3 py-2 text-xs text-red-200">{logoutError}</p> : null}
      </nav>
    );
  }

  const visibleItems = items.filter((item) => {
    if (!item.permissions || item.permissions.length === 0) {
      return true; // anyone signed in
    }
    if (hasWildcard) return true;
    return item.permissions.some((p) => perms.includes(p));
  });

  return (
    <nav className="space-y-2 p-4 text-sm">
      {visibleItems.map((item) => (
        <Link
          key={item.href}
          href={item.href}
          className={`block rounded-md px-3 py-2 hover:bg-white/10 ${
            pathname === item.href || pathname.startsWith(item.href + "/") ? "bg-white/10" : "bg-white/5"
          }`}
        >
          {item.label}
        </Link>
      ))}
      <button type="button" onClick={handleLogout} disabled={isLoggingOut} className="w-full rounded-md bg-white/5 px-3 py-2 text-left hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60">
        {isLoggingOut ? "Signing out..." : "Sign out"}
      </button>
      {logoutError ? <p role="alert" className="px-3 py-2 text-xs text-red-200">{logoutError}</p> : null}
    </nav>
  );
}
