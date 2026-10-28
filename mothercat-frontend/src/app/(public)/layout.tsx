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
        <header className="sticky top-0 z-50 border-b border-[#d6dfef] bg-white">
          <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6 lg:px-8">
            <Link href="/" className="flex items-center" aria-label="Mothercat home">
              <span className="flex h-14 w-14 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-white p-1 shadow-[0_10px_25px_rgba(15,23,42,0.08)] sm:h-14 sm:w-14">
                <Image
                  src={logoSrc}
                  alt="Mothercat logo"
                  width={160}
                  height={160}
                  className="h-full w-full rounded-full object-contain"
                />
              </span>
            </Link>

            <nav aria-label="Main navigation" className="hidden items-center gap-8 text-[14px] font-semibold md:flex">
              {navItems.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className="pb-1 text-[#0a4a8a] transition hover:border-b-2 hover:border-[#0a4a8a]"
                >
                  {item.label}
                </Link>
              ))}
            </nav>

            <div className="hidden items-center md:flex">
              <Link
                href="/contact"
                className="inline-flex h-10 items-center gap-2 rounded-[3px] bg-[#1f63d8] px-[22px] text-sm font-semibold text-white shadow-[0_10px_18px_rgba(31,99,216,0.25)] transition hover:bg-[#1855b4]"
              >
                Contact
                <ArrowRight size={16} />
              </Link>
            </div>
          </div>
        </header>
      )}

      {!isHome && (
        <>
          <nav aria-label="Bottom navigation" className="fixed inset-x-0 bottom-0 z-50 border-t border-[#d6dfef] bg-white/95 px-3 py-2 shadow-[0_-10px_25px_rgba(15,23,42,0.08)] backdrop-blur-md md:hidden">
            <div className="mx-auto grid max-w-md grid-cols-5 gap-3">
              {navItems.map((item) => {
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className="flex flex-col items-center justify-center rounded-xl px-1 py-2 text-[10px] font-semibold text-[#0a4a8a] transition hover:bg-slate-100"
                  >
                    <Icon size={18} className="mb-1" />
                    <span>{item.label}</span>
                  </Link>
                );
              })}
            </div>
          </nav>

          <Link
            href="/contact"
            aria-label="Contact Mothercat"
            className="fixed bottom-20 right-4 z-50 flex h-14 w-14 items-center justify-center rounded-full bg-[#1f63d8] text-white shadow-[0_12px_26px_rgba(31,99,216,0.35)] md:hidden"
          >
            <Phone size={22} />
          </Link>
        </>
      )}

      <div className={isHome ? "" : "pb-24 md:pb-0"}>{children}</div>

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
