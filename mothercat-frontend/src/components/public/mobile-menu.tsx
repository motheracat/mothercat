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

export function MobileMenu({ logoSrc, navItems }: MobileMenuProps) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!open) return;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        return;
      }

      if (event.key !== "Tab" || !panelRef.current) {
        return;
      }

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
      if (window.innerWidth >= 768) {
        setOpen(false);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("resize", handleResize);
    closeRef.current?.focus();

    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("resize", handleResize);
      triggerRef.current?.focus();
    };
  }, [open]);

  useEffect(() => {
    if (!open) {
      triggerRef.current?.focus();
    }
  }, [open]);

  return (
    <>
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
          className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"
          onClick={() => setOpen(false)}
        >
          <div
            id="mobile-menu"
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-label="Main menu"
            onClick={(event) => event.stopPropagation()}
            className="ml-auto flex h-dvh w-[85%] max-w-[360px] flex-col overflow-y-auto bg-gradient-to-b from-[#0a4a8a] via-[#0c5ea8] to-[#eaf3ff] shadow-2xl motion-safe:transition-transform motion-safe:duration-200 motion-safe:translate-x-0 overscroll-contain pb-[env(safe-area-inset-bottom)]"
          >
            <div className="flex items-center justify-between border-b border-white/10 px-4 py-3.5">
              <Link href="/" onClick={() => setOpen(false)} aria-label="Mothercat home" className="flex items-center gap-3 rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a]">
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
                className="flex h-11 w-11 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white transition hover:bg-white/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a]"
              >
                <X size={20} aria-hidden="true" />
              </button>
            </div>

            <nav className="flex flex-col gap-2 px-4 py-5" aria-label="Mobile menu">
              {navItems.map((item) => {
                const isActive = pathname === item.href;

                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    onClick={() => setOpen(false)}
                    className={`flex min-h-[48px] items-center justify-start whitespace-nowrap rounded-xl border px-4 text-[15px] font-semibold leading-none transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a] ${
                      isActive
                        ? "border-white/25 bg-white/15 text-white"
                        : "border-white/15 bg-white/5 text-slate-100 hover:bg-white/10"
                    }`}
                  >
                    {item.label}
                  </Link>
                );
              })}
            </nav>

            <div className="mx-4 border-t border-white/10" />

            <div className="mt-auto space-y-3 px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] pt-4">
              <Link
                href="/login"
                onClick={() => setOpen(false)}
                className="flex min-h-[48px] w-full items-center justify-center whitespace-nowrap rounded-xl border border-black/10 bg-white/90 px-4 text-[15px] font-semibold leading-none !text-black backdrop-blur-sm transition hover:bg-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a]"
              >
                Sign In
              </Link>

              <Link
                href="/contact"
                onClick={() => setOpen(false)}
                className="flex min-h-[48px] w-full items-center justify-center whitespace-nowrap rounded-xl bg-white px-4 text-[15px] font-semibold leading-none !text-black shadow-sm transition hover:bg-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a4a8a]"
              >
                Contact Us
              </Link>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
