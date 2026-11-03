"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowRight, BriefcaseBusiness, Building2, House, Info, Layers } from "lucide-react";
import { siteContent } from "@/content/site";

const logoSrc = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";
const navItems = [
  { href: "/", label: "Home" },
  { href: "/services", label: "Services" },
  { href: "/projects", label: "Projects" },
  { href: "/opportunities", label: "Opportunities" },
  { href: "/about", label: "About" },
];
const mobileNavItems = [
  { ...navItems[0], icon: House },
  { ...navItems[1], icon: Building2 },
  { ...navItems[2], icon: Layers },
  { ...navItems[3], icon: BriefcaseBusiness },
  { ...navItems[4], icon: Info },
];

const footerLink =
  "rounded transition hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70";

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isHome = pathname === "/";

  return (
    <div className="relative min-h-screen bg-white text-slate-900">
      <header className={`fixed inset-x-0 top-3 z-40 px-3 ${isHome ? "sm:px-6" : "sm:px-4"}`}>
        <div
          className={`public-header-surface mx-auto max-w-7xl rounded-[28px] border px-2 py-2 sm:rounded-[32px] sm:px-4 ${isHome ? "public-header-surface-home" : ""}`}
        >
          <div className="flex h-14 items-center justify-between gap-2 sm:h-16">
            <Link href="/" className="flex shrink-0 items-center gap-2 rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#0a4a8a] focus-visible:ring-offset-2" aria-label="Mothercat home">
              <span className="flex h-10 w-10 items-center justify-center overflow-hidden rounded-full border border-white/40 bg-white/90 p-1 shadow-[0_2px_8px_rgba(0,0,0,0.15)] sm:h-12 sm:w-12">
                <Image
                  src={logoSrc}
                  alt="Mothercat logo"
                  width={160}
                  height={160}
                  className="h-full w-full rounded-full object-contain"
                />
              </span>
              <span className={`hidden text-sm font-extrabold uppercase sm:block sm:text-base ${isHome ? "text-white" : "text-[#071535]"}`}>Mothercat</span>
            </Link>

            <nav aria-label="Main navigation" className="hidden items-center gap-1 text-sm font-semibold md:flex">
              {navItems.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={pathname === item.href ? "page" : undefined}
                  className={`rounded-full px-3 py-2 transition focus-visible:outline-none focus-visible:ring-2 ${
                    isHome
                      ? "text-white hover:bg-white/15 focus-visible:ring-white"
                      : "text-[#0f1b33] hover:bg-slate-100 focus-visible:ring-[#0a4a8a]"
                  } ${
                    pathname === item.href ? (isHome ? "bg-white/15" : "bg-[#eaf1ff]") : ""
                  }`}
                >
                  {item.label}
                </Link>
              ))}
            </nav>

            <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
              <Link
                href="/login"
                className="inline-flex h-9 items-center rounded-full border border-[#071535]/20 bg-white px-2.5 text-xs font-semibold text-[#071535] shadow-[0_2px_10px_rgba(0,0,0,0.08),inset_0_1px_1px_rgba(255,255,255,0.6)] backdrop-blur-[14px] transition hover:bg-[#1f5fd0] hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2 sm:h-10 sm:px-4 sm:text-sm"
              >
                Staff Login
              </Link>
              <Link
                href="/contact"
                className="inline-flex h-9 items-center gap-1 rounded-full border border-white/25 bg-[#071535]/75 px-2.5 text-xs font-semibold text-white shadow-[0_2px_10px_rgba(0,0,0,0.08),inset_0_1px_1px_rgba(255,255,255,0.3)] backdrop-blur-[14px] backdrop-saturate-150 transition hover:bg-[#1f5fd0] hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#1f5fd0] focus-visible:ring-offset-2 sm:h-10 sm:gap-2 sm:px-4 sm:text-sm"
              >
                Contact Us
                <ArrowRight size={15} aria-hidden="true" />
              </Link>
            </div>
          </div>
        </div>
      </header>
      <div aria-hidden="true" className={isHome ? "h-0" : "h-[84px] sm:h-[92px]"} />

      <nav
        aria-label="Mobile navigation"
        className="public-mobile-navigation fixed inset-x-0 bottom-0 z-50 border-t px-1 pt-2 pb-[calc(env(safe-area-inset-bottom)+0.5rem)] md:hidden"
      >
        <ul className="mx-auto grid max-w-lg grid-cols-5">
          {mobileNavItems.map(({ href, label, icon: Icon }) => {
            const isActive = pathname === href || (href !== "/" && pathname.startsWith(`${href}/`));

            return (
              <li key={href}>
                <Link
                  href={href}
                  aria-current={isActive ? "page" : undefined}
                  className={`mx-0.5 flex min-h-12 flex-col items-center justify-center gap-0.5 rounded-lg text-[10px] font-semibold leading-tight transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#0a4a8a] ${
                    isActive ? "text-[#0a4a8a]" : "text-slate-600 hover:bg-white/70 hover:text-[#0f1b33]"
                  }`}
                >
                  <Icon size={18} aria-hidden="true" />
                  <span>{label}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      <div id="main-content" tabIndex={-1} className="pb-[calc(4.5rem+env(safe-area-inset-bottom))] md:pb-0">{children}</div>

      <footer className="bg-[#071535] pb-[calc(4.5rem+env(safe-area-inset-bottom))] text-slate-300 md:pb-0">
        <div className="mx-auto grid max-w-6xl gap-10 px-5 py-14 sm:px-6 sm:grid-cols-2 lg:grid-cols-[1.2fr_0.8fr_0.8fr_1fr] lg:px-8">
          <div className="sm:col-span-2 lg:col-span-1">
            <span className="mb-5 flex h-16 w-16 items-center justify-center overflow-hidden rounded-full border border-white/20 bg-white/90 p-1 sm:h-20 sm:w-20">
              <Image
                src={logoSrc}
                alt="Mothercat logo"
                width={160}
                height={160}
                className="h-full w-full rounded-full object-contain"
              />
            </span>
            <p className="max-w-xs text-sm leading-6">{siteContent.company.footerBlurb}</p>
          </div>

          <nav aria-label="Company">
            <h3 className="mb-4 text-sm font-semibold text-white">Company</h3>
            <ul className="space-y-1 text-sm">
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/about">About</Link></li>
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/projects">Projects</Link></li>
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/opportunities">Opportunities</Link></li>
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/contact">Contact</Link></li>
            </ul>
          </nav>

          <nav aria-label="Services">
            <h3 className="mb-4 text-sm font-semibold text-white">Services</h3>
            <ul className="space-y-1 text-sm">
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/services#infrastructure-development">Infrastructure</Link></li>
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/services#commercial-buildings">Commercial</Link></li>
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/services#urban-solutions">Urban Solutions</Link></li>
            </ul>
          </nav>

          <div>
            <h3 className="mb-4 text-sm font-semibold text-white">Contact</h3>
            <ul className="space-y-1 text-sm">
              <li className="py-1.5">hello@mothercat.example</li>
              <li className="py-1.5">+27 11 000 0000</li>
              <li className="py-1.5">Johannesburg, South Africa</li>
              <li><Link href="/login" className={`${footerLink} inline-block py-1.5 text-blue-300`}>Staff login</Link></li>
            </ul>
          </div>
        </div>

        <div className="border-t border-white/10 px-5 py-5 text-center text-xs text-slate-400">
          © {new Date().getFullYear()} Mothercat. All rights reserved.
        </div>
      </footer>
    </div>
  );
}
