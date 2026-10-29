"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowRight, BriefcaseBusiness, House, Info, Phone, SquareStack } from "lucide-react";
import { siteContent } from "@/content/site";

const logoSrc = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";
const navItems = [
  { href: "/", label: "Home", icon: House },
  { href: "/services", label: "Services", icon: SquareStack },
  { href: "/projects", label: "Projects", icon: BriefcaseBusiness },
  { href: "/about", label: "About", icon: Info },
];

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isHome = pathname === "/";

  return (
    <div className="min-h-screen bg-white text-slate-900">
      {!isHome && (
        <header className="sticky top-0 z-50 px-3 pt-3 sm:px-4">
          <div className="mx-auto max-w-6xl rounded-full border border-slate-200 bg-white/90 px-2 py-2 shadow-[0_12px_30px_rgba(15,23,42,0.08)] backdrop-blur-md sm:px-3">
            <div className="flex items-center justify-between gap-2">
              <Link href="/" className="flex items-center" aria-label="Mothercat home">
                <span className="flex h-11 w-11 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-white p-1 shadow-[0_8px_20px_rgba(15,23,42,0.06)] sm:h-12 sm:w-12">
                  <Image
                    src={logoSrc}
                    alt="Mothercat logo"
                    width={160}
                    height={160}
                    className="h-full w-full rounded-full object-contain"
                  />
                </span>
              </Link>

              <nav aria-label="Main navigation" className="hidden items-center gap-2 text-[13px] font-semibold md:flex">
                {navItems.map((item) => (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={`rounded-full px-3 py-2 transition ${pathname === item.href ? "bg-[#eaf1ff] text-[#0a4a8a]" : "text-[#0a4a8a] hover:bg-slate-100"}`}
                  >
                    {item.label}
                  </Link>
                ))}
              </nav>

              <div className="hidden items-center gap-2 md:flex">
                <Link
                  href="/login"
                  className="inline-flex h-10 items-center gap-2 rounded-full bg-[#1f63d8] px-[18px] text-sm font-semibold text-white transition hover:bg-[#1855b4]"
                >
                  Sign in
                </Link>
                <Link
                  href="/contact"
                  className="inline-flex h-10 items-center gap-2 rounded-full bg-[#1f63d8] px-[18px] text-sm font-semibold text-white shadow-[0_10px_18px_rgba(31,99,216,0.25)] transition hover:bg-[#1855b4]"
                >
                  Contact
                  <ArrowRight size={16} />
                </Link>
              </div>

              <div className="flex items-center gap-2 md:hidden">
                <Link
                  href="/login"
                  className="inline-flex h-9 shrink-0 items-center justify-center rounded-full bg-[#1f63d8] px-3 text-[11px] font-semibold leading-none text-white shadow-[0_8px_16px_rgba(31,99,216,0.25)] transition hover:bg-[#1855b4]"
                >
                  Sign in
                </Link>
              </div>
            </div>

            <div className="mt-2 overflow-x-auto pb-1 md:hidden">
              <nav aria-label="Mobile navigation" className="flex min-w-max items-center gap-2">
                {navItems.map((item) => {
                  const Icon = item.icon;
                  const isActive = pathname === item.href;

                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-2 text-[10px] font-semibold leading-none tracking-tight transition ${
                        isActive
                          ? "border-[#cfe0ff] bg-[#eaf1ff] text-[#0a4a8a]"
                          : "border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50"
                      }`}
                    >
                      <Icon size={13} />
                      <span className="whitespace-nowrap">{item.label}</span>
                    </Link>
                  );
                })}
              </nav>
            </div>
          </div>
        </header>
      )}

      {!isHome && (
        <>
          <Link
            href="/contact"
            aria-label="Contact Mothercat"
            className="fixed bottom-20 right-4 z-50 flex h-14 w-14 items-center justify-center rounded-full bg-[#1f63d8] text-white shadow-[0_12px_26px_rgba(31,99,216,0.35)] md:hidden"
          >
            <Phone size={22} />
          </Link>
        </>
      )}

      <div className={isHome ? "" : "pb-20 md:pb-0"}>{children}</div>

      <footer className="bg-[#071535] text-slate-200">
        <div className="mx-auto grid max-w-6xl gap-10 px-4 py-16 sm:px-6 lg:grid-cols-[1.2fr_0.8fr_0.8fr_1fr] lg:px-8">
          <div>
            <div className="mb-5 flex items-center">
              <span className="flex h-16 w-16 items-center justify-center overflow-hidden rounded-full border border-white/20 bg-white/90 p-1 shadow-[0_10px_30px_rgba(15,26,46,0.45)] sm:h-20 sm:w-20">
                <Image
                  src={logoSrc}
                  alt="Mothercat logo"
                  width={160}
                  height={160}
                  className="h-full w-full rounded-full object-contain"
                />
              </span>
            </div>
            <p className="max-w-xs text-sm leading-6 text-slate-300">{siteContent.company.footerBlurb}</p>
          </div>

          <div>
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-[0.18em] text-white">Company</h3>
            <ul className="space-y-3 text-sm text-slate-300">
              <li><Link href="/about">About</Link></li>
              <li><Link href="/projects">Projects</Link></li>
              <li><Link href="/careers">Careers</Link></li>
              <li><Link href="/contact">Contact</Link></li>
            </ul>
          </div>

          <div>
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-[0.18em] text-white">Services</h3>
            <ul className="space-y-3 text-sm text-slate-300">
              <li><Link href="/services#infrastructure-development">Infrastructure</Link></li>
              <li><Link href="/services#commercial-buildings">Commercial</Link></li>
              <li><Link href="/services#urban-solutions">Urban Solutions</Link></li>
            </ul>
          </div>

          <div>
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-[0.18em] text-white">Contact</h3>
            <ul className="space-y-3 text-sm text-slate-300">
              <li>hello@mothercat.example</li>
              <li>+27 11 000 0000</li>
              <li>Johannesburg, South Africa</li>
              <li><Link href="/login" className="text-blue-300">Staff Login</Link></li>
            </ul>
          </div>
        </div>
      </footer>
    </div>
  );
}
