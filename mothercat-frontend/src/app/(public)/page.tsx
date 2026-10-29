import Link from "next/link";
import Image from "next/image";
import { ArrowRight, ArrowUpRight, Building2, Cog, Compass, Landmark, Layers, Leaf, Trees, type LucideIcon } from "lucide-react";
import { siteContent } from "@/content/site";
import { MobileMenu } from "@/components/public/mobile-menu";

/* Use the project’s real, existing branded assets so the home page resolves reliably. */
const LOGO_SRC = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";
const HERO_IMAGE = "/mothercat-welcome-hero.jpeg";

const NAV = [
  { href: "/", label: "Home" },
  { href: "/services", label: "Services" },
  { href: "/projects", label: "Projects" },
  { href: "/about", label: "About" },
];

const serviceIcons: LucideIcon[] = [Landmark, Building2, Trees];
const expertiseIcons: LucideIcon[] = [Cog, Compass, Leaf, Layers];

const serviceImages = [
  "/images/Infrastructure-Development.jpeg",
  "/images/Commercial%20Buildings.webp",
  "/images/Urban%20Solutions.jpeg",
];
const projectImages = [
  "/images/Skyline%20Tower.webp",
  "/images/Corporate%20Headquarters.jpeg",
  "/images/International%20Business%20Center.jpeg",
];

const FEATURED_INTRO_FALLBACK =
  "A selection of our engineering and construction work across infrastructure, commercial and urban development.";

const focus =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#2d8fd8]";

export default function HomePage() {
  const { services, expertise, projects } = siteContent.home;
  const featuredIntro = (siteContent.home as { projectsIntro?: string }).projectsIntro ?? FEATURED_INTRO_FALLBACK;

  /*
    "Impact" figures must be REAL. Until the company supplies official numbers
    (add `stats: [{ value, label }]` to siteContent.home), show counts derived
    from the content itself — never invented statistics.
  */
  const stats =
    (siteContent.home as { stats?: { value: string; label: string }[] }).stats ?? [
      { value: String(services.length), label: "Core Services" },
      { value: String(expertise.length), label: "Areas of Expertise" },
      { value: String(projects.length), label: "Featured Projects" },
    ];

  return (
    <main className="bg-white text-slate-900">
      {/* ───────────── Hero: framed card ───────────── */}
      <section
        aria-labelledby="hero-title"
        className="bg-[#0a4a8a] p-3 sm:p-5 lg:p-6"
      >
        <div className="relative isolate mx-auto flex min-h-[780px] max-w-[1400px] flex-col overflow-hidden rounded-[22px] bg-[#0a4a8a] shadow-[0_30px_80px_rgba(12,56,118,0.35)] lg:min-h-[740px]">
          {/* Photo: right side, fades into the blue on the left (no recolouring of the image) */}
          <div className="pointer-events-none absolute inset-y-0 right-0 -z-10 w-full opacity-60 lg:w-[66%] lg:opacity-100">
            <Image
              src={HERO_IMAGE}
              alt=""
              fill
              priority
              sizes="(max-width: 1024px) 100vw, 66vw"
              className="object-cover grayscale-0 [-webkit-mask-image:linear-gradient(to_right,transparent,black_38%)] [mask-image:linear-gradient(to_right,transparent,black_38%)]"
            />
          </div>
          {/* soft sky glow + water-line haze */}
          <div className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(900px_420px_at_20%_0%,rgba(255,255,255,0.20),transparent_68%)]" aria-hidden="true" />

          {/* Nav (inside the card) */}
          <header className="relative z-30 border-b border-white/25">
            <div className="grid h-[76px] grid-cols-[auto_1fr_auto] items-center gap-4 px-5 sm:px-8">
              <Link href="/" aria-label="Mothercat home" className={`flex items-center gap-3 rounded-full ${focus}`}>
                <Image src={LOGO_SRC} alt="Mothercat" width={48} height={48} className="h-12 w-12 rounded-full bg-white object-cover shadow-md" />
              </Link>

              <nav aria-label="Primary" className="hidden justify-center gap-9 text-[13px] font-medium text-white md:flex">
                {NAV.map((item) => (
                  <Link key={item.href} href={item.href} className={`whitespace-nowrap rounded px-1 py-1 opacity-90 transition hover:opacity-100 ${focus}`}>
                    {item.label}
                  </Link>
                ))}
              </nav>

              <div className="flex items-center justify-end gap-3">
                <Link
                  href="/login"
                  className="hidden whitespace-nowrap rounded-lg bg-white/10 px-4 py-2.5 text-[13px] font-semibold !text-black transition hover:bg-white/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#2d8fd8] md:inline-flex"
                >
                  Sign in
                </Link>
                <Link
                  href="/contact"
                  className="hidden whitespace-nowrap rounded-lg bg-white px-5 py-2.5 text-[13px] font-semibold !text-black shadow-sm transition hover:bg-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#2d8fd8] md:inline-flex"
                >
                  Contact Us
                </Link>

                <MobileMenu logoSrc={LOGO_SRC} navItems={NAV} />
              </div>
            </div>
          </header>

          {/* Headline block */}
          <div className="relative z-20 flex-1 px-5 pb-52 pt-12 sm:px-8 lg:pb-44 lg:pt-16">
            <div className="max-w-[560px] motion-safe:animate-fade-up">
              <h1 id="hero-title" className="font-serif text-5xl font-semibold leading-[1.05] tracking-tight text-white sm:text-6xl lg:text-7xl">
                <span className="block">Advancing</span>
                <span className="block">Innovative</span>
                <span className="block">Engineering &amp;</span>
                <span className="block">Construction</span>
              </h1>
           
            </div>
          </div>

          {/* Bottom glass bar */}
          <div className="absolute inset-x-0 bottom-0 z-20 border-t border-white/25 bg-gradient-to-t from-[#1d6fb0]/70 to-[#3a9ad8]/10 backdrop-blur-[3px]">
            <div className="flex flex-col gap-6 px-5 py-6 sm:px-8 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex items-center gap-5">
                {/* Rotating "start the journey" badge */}
                <Link href="#about" aria-label="Start the journey: scroll to About" className={`relative block h-[84px] w-[84px] shrink-0 rounded-full ${focus}`}>
                  <svg viewBox="0 0 100 100" className="absolute inset-0 h-full w-full motion-safe:animate-[spin_22s_linear_infinite]" aria-hidden="true">
                    <defs>
                      <path id="circ" d="M50,50 m-38,0 a38,38 0 1,1 76,0 a38,38 0 1,1 -76,0" />
                    </defs>
                    <text fill="white" fontSize="10.5" letterSpacing="2.2" fontWeight="500">
                      <textPath href="#circ">START THE JOURNEY • START THE JOURNEY •</textPath>
                    </text>
                  </svg>
                  <ArrowUpRight className="absolute inset-0 m-auto text-white" size={22} aria-hidden="true" />
                </Link>
              
              </div>

              {/* Impact card */}
              <div className="w-full rounded-2xl border border-white/30 bg-white/15 p-5 backdrop-blur-md lg:w-[420px]">
                <p className="text-[13px] font-medium text-white/90">Our impact in numbers</p>
                <dl className="mt-3 grid grid-cols-3 divide-x divide-white/25 text-center text-white">
                  {stats.slice(0, 3).map((s) => (
                    <div key={s.label} className="px-2">
                      <dt className="sr-only">{s.label}</dt>
                      <dd className="text-[34px] font-light leading-none" style={{ fontFamily: "var(--font-barlow-condensed)" }}>
                        {s.value}
                      </dd>
                      <p className="mt-2 text-[11px] text-white/85" aria-hidden="true">{s.label}</p>
                    </div>
                  ))}
                </dl>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ───────────── Services ───────────── */}
      <section aria-label="Our services" className="mx-auto max-w-6xl px-4 pb-4 pt-16 sm:px-6 lg:px-8">
        <div className="grid gap-5 md:grid-cols-3">
          {services.map((service, index) => {
            const Icon = serviceIcons[index] ?? Landmark;
            return (
              <article
                key={service.slug}
                className="reveal-card group overflow-hidden rounded-[12px] border border-slate-200 bg-[#102a63] shadow-[0_12px_28px_rgba(3,10,30,0.18)] transition duration-200 motion-safe:hover:-translate-y-1 hover:shadow-[0_18px_34px_rgba(31,95,208,0.22)]"
              >
                <div className="relative h-[220px] w-full overflow-hidden">
                  <Image
                    src={serviceImages[index] ?? serviceImages[0]}
                    alt={service.title}
                    fill
                    className="object-cover transition duration-300 motion-safe:group-hover:scale-105"
                    sizes="(max-width: 768px) 100vw, 33vw"
                  />
                  <div className="absolute inset-0 bg-[linear-gradient(180deg,rgba(7,21,53,0.20),rgba(7,21,53,0.78))]" />
                  <div className="absolute inset-x-0 bottom-0 flex items-end justify-between gap-4 p-5">
                    <div>
                      <h3 className="text-xl font-semibold text-white">{service.title}</h3>
                      <p className="mt-2 max-w-xs text-sm leading-6 text-slate-100">{service.description}</p>
                    </div>
                    <span className="mt-1 flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-white/15 bg-white/10 text-blue-100 backdrop-blur-[2px]">
                      <Icon size={18} aria-hidden="true" />
                    </span>
                  </div>
                </div>
                <div className="px-5 pb-5 pt-4">
                  <Link
                    href={service.href}
                    aria-label={`Discover more about ${service.title}`}
                    className="inline-flex items-center gap-2 rounded-md border border-white/15 bg-white px-4 py-2 text-sm font-semibold text-[#0f1b33] transition hover:bg-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-300"
                  >
                    Discover More
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                </div>
              </article>
            );
          })}
        </div>
      </section>

      {/* ───────────── About + Expertise ───────────── */}
      <section
        id="about"
        aria-labelledby="about-title"
        className="reveal-section scroll-mt-24 bg-[#f4f7fc] px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8"
      >
        <div className="mx-auto grid max-w-6xl gap-10 lg:grid-cols-2">
          <div className="reveal-card">
            <h2 id="about-title" className="font-serif text-4xl">About Us</h2>
            <div className="relative mt-8 h-[340px] w-full overflow-hidden rounded-[4px] border border-slate-200">
              <Image
                src="/images/about-us.jpeg"
                alt="Modern glass and steel building"
                fill
                className="object-cover"
                sizes="(max-width: 1024px) 100vw, 50vw"
              />
            </div>
          </div>

          <div className="reveal-card">
            <h2 className="font-serif text-4xl">Our Expertise</h2>
            <div className="mt-8 grid gap-4 sm:grid-cols-2">
              {expertise.map((item, index) => {
                const Icon = expertiseIcons[index % expertiseIcons.length];
                return (
                  <div key={item.title} className="flex gap-4 rounded-md border border-slate-200 bg-white p-4 shadow-sm">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-blue-200 bg-blue-50 text-[#1f5fd0]">
                      <Icon size={18} aria-hidden="true" />
                    </div>
                    <div>
                      <h3 className="font-semibold">{item.title}</h3>
                      <p className="mt-2 text-sm leading-6 text-slate-600">{item.description}</p>
                    </div>
                  </div>
                );
              })}
            </div>
            <div className="mt-8">
              <Link
                href="/about"
                className="inline-flex items-center gap-2 rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white shadow-[0_10px_30px_rgba(31,95,208,0.2)] transition hover:bg-[#2f74e6] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-2"
              >
                Global Reach
                <ArrowRight size={16} aria-hidden="true" />
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* ───────────── Featured Projects ───────────── */}
      <section aria-labelledby="featured-title" className="reveal-section bg-[#071535] px-4 py-20 sm:px-6 lg:px-8">
        <div className="mx-auto max-w-6xl">
          <div className="reveal-card mb-10 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <h2 id="featured-title" className="font-serif text-4xl text-white">Featured Projects</h2>
            <p className="max-w-xl text-sm leading-6 text-slate-300">{featuredIntro}</p>
          </div>

          <div className="grid gap-6 md:grid-cols-3">
            {projects.map((project, index) => (
              <article
                key={project.title}
                className="reveal-card group overflow-hidden rounded-[10px] border border-white/10 bg-[#102a63] shadow-[0_10px_30px_rgba(3,10,30,0.35)] transition duration-200 motion-safe:hover:-translate-y-1 hover:shadow-[0_16px_40px_rgba(31,95,208,0.18)]"
              >
                <div className="relative h-56 w-full overflow-hidden">
                  <Image
                    src={projectImages[index] ?? projectImages[0]}
                    alt={project.title}
                    fill
                    className="object-cover transition duration-300 motion-safe:group-hover:scale-105"
                    sizes="(max-width: 768px) 100vw, 33vw"
                  />
                </div>
                <div className="p-5">
                  <h3 className="text-xl font-semibold text-white">{project.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-slate-300">{project.description}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>
    </main>
  );
}