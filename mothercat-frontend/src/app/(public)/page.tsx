import Link from "next/link";
import Image from "next/image";
import { ArrowRight, Building2, Cog, Compass, Landmark, Layers, Leaf, Trees, type LucideIcon } from "lucide-react";
import { siteContent } from "@/content/site";

const HERO_IMAGE = "/mothercat-welcome-hero.jpeg";

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
      {/* ───────────── Hero: full-width photo, solid overlay, clear headline ───────────── */}
      <section aria-labelledby="hero-title" className="relative isolate overflow-hidden bg-[#0a2a52]">
        <Image
          src={HERO_IMAGE}
          alt=""
          fill
          priority
          sizes="100vw"
          className="-z-20 object-cover"
        />
        {/* Simple dark overlay for text legibility */}
        <div className="absolute inset-0 -z-10 bg-gradient-to-r from-[#071e3d]/90 via-[#071e3d]/70 to-[#071e3d]/30" aria-hidden="true" />

        <div className="mx-auto flex min-h-[560px] max-w-6xl flex-col justify-center px-4 pb-20 pt-32 sm:px-6 sm:pt-36 lg:min-h-[640px] lg:px-8 lg:pt-40">
          <div className="max-w-2xl motion-safe:animate-fade-up">
            <span className="mb-5 block h-1 w-16 bg-white" aria-hidden="true" />
            <h1 id="hero-title" className="text-4xl font-bold leading-tight text-white sm:text-5xl lg:text-6xl">
              Advancing Innovative Engineering &amp; Construction
            </h1>
            <p className="mt-5 max-w-xl text-base leading-7 text-slate-200 sm:text-lg">
              Reliable infrastructure, commercial and urban projects, delivered with quality, safety and integrity.
            </p>
            <div className="mt-8 flex flex-wrap gap-4">
              <Link
                href="/projects"
                className="inline-flex items-center gap-2 rounded-md bg-white px-6 py-3 text-sm font-bold text-[#0a2a52] transition-colors duration-200 hover:bg-black hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a2a52]"
              >
                View Our Projects
                <ArrowRight size={16} aria-hidden="true" />
              </Link>
              <Link
                href="/contact"
                className="inline-flex items-center rounded-md border border-white bg-white px-6 py-3 text-sm font-semibold text-[#0a2a52] transition-colors duration-200 hover:bg-black hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#0a2a52]"
              >
                Get in Touch
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Stats strip */}
      <section aria-label="Company at a glance" className="bg-[#0a4a8a]">
        <dl className="mx-auto grid max-w-6xl grid-cols-3 divide-x divide-white/25 px-4 py-6 text-center text-white sm:px-6 lg:px-8">
          {stats.slice(0, 3).map((s) => (
            <div key={s.label} className="px-2">
              <dd className="text-3xl font-bold leading-none sm:text-4xl">{s.value}</dd>
              <dt className="mt-2 text-xs font-medium uppercase tracking-wide text-white/85 sm:text-sm">{s.label}</dt>
            </div>
          ))}
        </dl>
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