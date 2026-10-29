"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Menu, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

type MobileMenuProps = {
  logoSrc: string;
  navItems: Array<{ href: string; label: string }>;
};

const ring =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a]";

export function MobileMenu({ logoSrc, navItems }: MobileMenuProps) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);
  const previousPathnameRef = useRef(pathname);

  // Close when the route changes
  useEffect(() => {
    if (!open || previousPathnameRef.current === pathname) {
      previousPathnameRef.current = pathname;
      return;
    }

    previousPathnameRef.current = pathname;
    const timeoutId = window.setTimeout(() => setOpen(false), 0);

    return () => window.clearTimeout(timeoutId);
  }, [open, pathname]);

  // Scroll lock, focus trap, Escape, resize
  useEffect(() => {
    if (!open) return;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        return;
      }

      if (event.key !== "Tab" || !panelRef.current) return;

      const focusable = panelRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'
      );

      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }

      const first = focusable[0];
      const last = focusable[focusable.length - 1];

      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    const handleResize = () => {
      if (window.innerWidth >= 768) setOpen(false);
    };

    const trigger = triggerRef.current;

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("resize", handleResize);
    closeRef.current?.focus();

    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("resize", handleResize);
      // Return focus to the trigger only when the menu closes.
      // (The old separate effect also stole focus on first page load.)
      trigger?.focus();
    };
  }, [open]);

  return (
    <>
      <style>{`
        @keyframes mc-slide-in { from { transform: translateX(100%); } to { transform: translateX(0); } }
        @keyframes mc-fade-in { from { opacity: 0; } to { opacity: 1; } }
        @media (prefers-reduced-motion: no-preference) {
          .mc-panel { animation: mc-slide-in 220ms cubic-bezier(0.22, 1, 0.36, 1); }
          .mc-backdrop { animation: mc-fade-in 180ms ease-out; }
        }
      `}</style>

      <button
        ref={triggerRef}
        type="button"
        aria-label={open ? "Close menu" : "Open menu"}
        aria-expanded={open}
        aria-controls="mobile-menu"
        onClick={() => setOpen((value) => !value)}
        className="flex h-11 w-11 items-center justify-center rounded-xl border border-white/25 bg-white/10 text-white transition hover:bg-white/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#2d8fd8] md:hidden"
      >
        {open ? <X size={20} aria-hidden="true" /> : <Menu size={20} aria-hidden="true" />}
      </button>

      {open && (
        <div
          className="mc-backdrop fixed inset-0 z-50 bg-black/60 backdrop-blur-sm"
          onClick={() => setOpen(false)}
        >
          <div
            id="mobile-menu"
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-label="Main menu"
            onClick={(event) => event.stopPropagation()}
            className="mc-panel ml-auto flex h-dvh w-[85%] max-w-[360px] flex-col overflow-y-auto overscroll-contain bg-[#0a4a8a] shadow-2xl"
          >
            {/* Header */}
            <div className="flex items-center justify-between border-b border-white/15 px-4 py-3.5">
              <Link
                href="/"
                onClick={() => setOpen(false)}
                aria-label="Mothercat home"
                className={`flex items-center gap-3 rounded-full ${ring}`}
              >
                <Image
                  src={logoSrc}
                  alt="Mothercat"
                  width={40}
                  height={40}
                  className="h-10 w-10 rounded-full bg-white object-cover shadow-md"
                />
              </Link>

              <button
                ref={closeRef}
                type="button"
                aria-label="Close menu"
                onClick={() => setOpen(false)}
                className={`flex h-11 w-11 items-center justify-center rounded-xl border border-white/25 bg-white/10 text-white transition hover:bg-white/20 ${ring}`}
              >
                <X size={20} aria-hidden="true" />
              </button>
            </div>

            {/* Navigation */}
            <nav className="flex flex-col gap-1 px-3 py-4" aria-label="Mobile menu">
              {navItems.map((item) => {
                const isActive = pathname === item.href;

                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    onClick={() => setOpen(false)}
                    aria-current={isActive ? "page" : undefined}
                    className={`relative flex min-h-[52px] items-center rounded-xl px-4 text-base font-semibold transition ${ring} ${
                      isActive
                        ? "bg-white/15 text-white before:absolute before:left-0 before:top-3 before:bottom-3 before:w-1 before:rounded-full before:bg-white"
                        : "text-white/85 hover:bg-white/10 hover:text-white active:bg-white/15"
                    }`}
                  >
                    {item.label}
                  </Link>
                );
              })}
            </nav>

            {/* Actions */}
            <div className="mt-auto flex flex-col gap-3 border-t border-white/15 px-4 pb-[calc(env(safe-area-inset-bottom)+1.25rem)] pt-5">
              <Link
                href="/contact"
                onClick={() => setOpen(false)}
                className={`flex min-h-[52px] w-full items-center justify-center rounded-xl bg-white px-4 text-base font-semibold text-[#0a4a8a] shadow-sm transition hover:bg-slate-100 active:bg-slate-200 ${ring}`}
              >
                Contact us
              </Link>

              <Link
                href="/login"
                onClick={() => setOpen(false)}
                className={`flex min-h-[52px] w-full items-center justify-center rounded-xl border border-white/40 px-4 text-base font-semibold text-white transition hover:bg-white/10 active:bg-white/15 ${ring}`}
              >
                Sign in
              </Link>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
