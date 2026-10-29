"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowRight, Phone } from "lucide-react";
import { siteContent } from "@/content/site";
import { MobileMenu } from "@/components/mobile-menu";

const logoSrc = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";
const navItems = [
  { href: "/", label: "Home" },
  { href: "/services", label: "Services" },
  { href: "/projects", label: "Projects" },
  { href: "/about", label: "About" },
];

const footerLink =
  "rounded transition hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70";

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isHome = pathname === "/";

  return (
    <div className="min-h-screen bg-white text-slate-900">
      {!isHome && (
        <header className="sticky top-0 z-40 px-3 pt-3 sm:px-4">
          <div className="mx-auto max-w-6xl rounded-full border border-slate-200 bg-white/90 px-2 py-2 shadow-[0_12px_30px_rgba(15,23,42,0.08)] backdrop-blur-md sm:px-3">
            <div className="flex items-center justify-between gap-2">
              <Link href="/" className="flex items-center" aria-label="Mothercat home">
                <span className="flex h-11 w-11 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-white p-1 sm:h-12 sm:w-12">
                  <Image
                    src={logoSrc}
                    alt="Mothercat logo"
                    width={160}
                    height={160}
                    className="h-full w-full rounded-full object-contain"
                  />
                </span>
              </Link>

              {/* Desktop nav */}
              <nav aria-label="Main navigation" className="hidden items-center gap-1 text-sm font-semibold md:flex">
                {navItems.map((item) => (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={pathname === item.href ? "page" : undefined}
                    className={`rounded-full px-4 py-2 text-[#0a4a8a] transition ${
                      pathname === item.href ? "bg-[#eaf1ff]" : "hover:bg-slate-100"
                    }`}
                  >
                    {item.label}
                  </Link>
                ))}
              </nav>

              {/* Desktop actions: secondary + primary */}
              <div className="hidden items-center gap-2 md:flex">
                <Link
                  href="/login"
                  className="inline-flex h-10 items-center rounded-full border border-slate-200 px-5 text-sm font-semibold text-[#0a4a8a] transition hover:bg-slate-100"
                >
                  Sign in
                </Link>
                <Link
                  href="/contact"
                  className="inline-flex h-10 items-center gap-2 rounded-full bg-[#1f63d8] px-5 text-sm font-semibold text-white transition hover:bg-[#1855b4]"
                >
                  Contact
                  <ArrowRight size={16} aria-hidden="true" />
                </Link>
              </div>

              {/* Mobile: one clean row, menu holds everything else */}
              <div className="flex items-center gap-2 md:hidden">
                <MobileMenu logoSrc={logoSrc} navItems={navItems} />
              </div>
            </div>
          </div>
        </header>
      )}

      {!isHome && (
        <Link
          href="/contact"
          aria-label="Contact Mothercat"
          className="fixed bottom-[calc(env(safe-area-inset-bottom)+1rem)] right-4 z-30 flex h-14 w-14 items-center justify-center rounded-full bg-[#1f63d8] text-white shadow-[0_12px_26px_rgba(31,99,216,0.35)] transition active:scale-95 md:hidden"
        >
          <Phone size={22} aria-hidden="true" />
        </Link>
      )}

      <div>{children}</div>

      <footer className="bg-[#071535] text-slate-300">
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
              <li><Link className={`${footerLink} inline-block py-1.5`} href="/careers">Careers</Link></li>
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
