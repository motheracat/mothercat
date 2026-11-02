"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import {
  Building2,
  Bell,
  CircleUserRound,
  FileUp,
  Gauge,
  History,
  Plus,
  ShieldCheck,
  UsersRound,
  type LucideIcon,
} from "lucide-react";
import type { CSSProperties, ReactNode } from "react";

import type { AuthSession } from "@/lib/auth/types";

type SidebarItem = {
  href: string;
  label: string;
  icon: LucideIcon;
  permissions?: string[];
};

const items: SidebarItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: Gauge },
  { href: "/staff", label: "Staff directory", icon: UsersRound, permissions: ["staff.read", "*"] },
  { href: "/staff/imports", label: "Staff imports", icon: FileUp, permissions: ["staff.import", "*"] },
  { href: "/departments", label: "Departments", icon: Building2, permissions: ["departments.manage", "staff.read", "*"] },
  { href: "/roles", label: "Roles & permissions", icon: ShieldCheck, permissions: ["roles.read", "roles.manage", "*"] },
  { href: "/audit", label: "Audit log", icon: History, permissions: ["audit.read", "*"] },
  { href: "/notifications", label: "Notifications", icon: Bell },
  { href: "/account", label: "Password & security", icon: CircleUserRound },
];

const mobilePrimaryHrefs = ["/dashboard", "/staff", "/departments", "/account"];

export function WorkspaceSidebar({
  session,
  signOut,
  footer,
}: {
  session: AuthSession | null;
  signOut: ReactNode;
  footer?: ReactNode;
}) {
  const pathname = usePathname();
  const [moreState, setMoreState] = useState<{ pathname: string; open: boolean } | null>(null);
  const moreContainerRef = useRef<HTMLDivElement>(null);
  const moreButtonRef = useRef<HTMLButtonElement>(null);
  const moreOpen = moreState?.pathname === pathname && moreState.open;

  const permissions = session?.permissions ?? [];
  const hasWildcard = permissions.includes("*");
  const hasNoPermissions = !hasWildcard && (permissions.length === 0 || (permissions.length === 1 && permissions[0] === ""));
  const visibleItems = hasNoPermissions
    ? items.filter((item) => item.href === "/dashboard" || item.href === "/account")
    : items.filter((item) => !item.permissions || hasWildcard || item.permissions.some((permission) => permissions.includes(permission)));
  const primaryItems = mobilePrimaryHrefs
    .map((href) => visibleItems.find((item) => item.href === href))
    .filter((item): item is SidebarItem => item !== undefined);
  const moreItems = visibleItems.filter((item) => !mobilePrimaryHrefs.includes(item.href));

  useEffect(() => {
    if (!moreOpen) return;
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setMoreState({ pathname, open: false });
        moreButtonRef.current?.focus();
      }
    }
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [moreOpen, pathname]);

  useEffect(() => {
    if (!moreOpen) return;
    function closeOnOutsideClick(event: PointerEvent) {
      if (event.target instanceof Node && !moreContainerRef.current?.contains(event.target)) {
        setMoreState({ pathname, open: false });
      }
    }
    document.addEventListener("pointerdown", closeOnOutsideClick);
    return () => document.removeEventListener("pointerdown", closeOnOutsideClick);
  }, [moreOpen, pathname]);

  if (!session) return null;

  return (
    <>
      <aside className="workspace-sidebar">
        <div className="flex h-16 shrink-0 items-center gap-3 border-b border-white/15 px-5">
          <Image
            src="/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png"
            alt=""
            width={32}
            height={32}
            className="h-8 w-8 rounded-md bg-white object-contain p-0.5"
          />
          <span className="text-sm font-semibold tracking-wide text-white">Mothercat</span>
        </div>

        <nav aria-label="Workspace navigation" className="flex-1 space-y-1 overflow-y-auto p-3">
          {visibleItems.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-current={pathname === item.href ? "page" : undefined}
                className={`relative flex h-10 items-center rounded-lg px-3 text-sm transition-colors duration-150 ${
                  active ? "bg-white/10 text-white" : "text-white/70 hover:bg-white/5 hover:text-white"
                }`}
              >
                {active ? <span aria-hidden="true" className="absolute inset-y-2 left-0 w-[3px] rounded-r bg-[var(--accent)]" /> : null}
                <span className="pl-1">{item.label}</span>
              </Link>
            );
          })}
          {hasNoPermissions ? <p className="px-3 py-3 text-xs leading-5 text-white/60">Your role has no workspace permissions yet.</p> : null}
        </nav>

        <div className="shrink-0 border-t border-white/15 p-3">
          {signOut}
          <p className="mt-3 px-3 text-xs text-white/45">v0.1.0</p>
          {footer}
        </div>
      </aside>
      <nav aria-label="Workspace navigation" className="workspace-bottom-nav">
        <div className="workspace-bottom-nav-bar">
          {primaryItems.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-label={item.label}
                title={item.label}
                aria-current={pathname === item.href ? "page" : undefined}
                className={`workspace-bottom-nav-link${active ? " is-active" : ""}`}
              >
                <Icon size={21} strokeWidth={active ? 2.2 : 1.8} aria-hidden="true" />
              </Link>
            );
          })}
        </div>
        {moreItems.length > 0 ? (
          <div className="workspace-bottom-more" ref={moreContainerRef}>
            <div
              id="workspace-bottom-more-menu"
              className="workspace-bottom-more-menu"
              data-open={moreOpen}
              aria-hidden={!moreOpen}
            >
              {moreItems.map((item, index) => {
                const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-label={item.label}
                    aria-current={pathname === item.href ? "page" : undefined}
                    tabIndex={moreOpen ? 0 : -1}
                    className="workspace-bottom-more-link"
                    data-active={active}
                    style={{ "--menu-order": index } as CSSProperties}
                  >
                    <Icon size={19} aria-hidden="true" />
                    <span>{item.label}</span>
                  </Link>
                );
              })}
            </div>
            <button
              type="button"
              ref={moreButtonRef}
              className="workspace-bottom-more-button"
              aria-label={moreOpen ? "Close more navigation" : "More navigation"}
              aria-expanded={moreOpen}
              aria-controls="workspace-bottom-more-menu"
              onClick={() => setMoreState({ pathname, open: !moreOpen })}
            >
              <Plus size={23} aria-hidden="true" />
            </button>
          </div>
        ) : null}
      </nav>
    </>
  );
}
