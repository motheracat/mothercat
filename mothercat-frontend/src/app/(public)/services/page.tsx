import Image from "next/image";
import Link from "next/link";

const serviceSections = [
  {
    id: "infrastructure-development",
    title: "Infrastructure Development",
    image: "/images/Infrastructure-Development.jpeg",
    intro: "We deliver complex infrastructure programs that support resilient growth, dependable service delivery, and long-term asset performance across civil, utility, and public works environments.",
    points: [
      "Civil works and strategic site preparation",
      "Utilities, drainage, and essential network coordination",
      "Program planning that keeps delivery efficient and accountable",
    ],
  },
  {
    id: "commercial-buildings",
    title: "Commercial Buildings",
    image: "/images/Commercial%20Buildings.webp",
    intro: "Our commercial building work prioritizes productivity, flexibility, and operational efficiency to create spaces where businesses can perform at their best.",
    points: [
      "Workplace environments designed for collaboration and performance",
      "Robust construction management from pre-construction to handover",
      "Quality-driven delivery that balances schedule, cost, and resilience",
    ],
  },
  {
    id: "urban-solutions",
    title: "Urban Solutions",
    image: "/images/Urban%20Solutions.jpeg",
    intro: "We shape urban spaces that connect communities, infrastructure, and commerce through integrated planning, coordinated execution, and future-ready design thinking.",
    points: [
      "Connected city infrastructure and precinct planning",
      "Context-aware execution for public and private urban projects",
      "Sustainable solutions that strengthen long-term urban value",
    ],
  },
];

export default function ServicesPage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8">
      <div className="mb-12">
        <p className="text-sm font-semibold uppercase tracking-[0.24em] text-blue-600">Services</p>
        <h1 className="mt-3 font-serif text-5xl text-[#0f1b33]">Building lasting value</h1>
      </div>

      <div className="space-y-10">
        {serviceSections.map((service, index) => (
          <section
            key={service.id}
            id={service.id}
            className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_10px_30px_rgba(15,27,51,0.04)]"
          >
            <div className="grid gap-0 lg:grid-cols-[1.08fr_0.92fr]">
              <div className="order-2 p-8 lg:order-1 lg:p-10">
                <p className="text-sm font-semibold uppercase tracking-[0.22em] text-blue-600">
                  {String(index + 1).padStart(2, "0")}
                </p>
                <h2 className="mt-3 font-serif text-3xl text-[#0f1b33] sm:text-4xl">{service.title}</h2>
                <p className="mt-5 max-w-xl text-base leading-7 text-slate-600">{service.intro}</p>

                <ul className="mt-6 space-y-3">
                  {service.points.map((point) => (
                    <li key={point} className="flex items-start gap-3 text-sm leading-6 text-slate-700">
                      <span className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full bg-[#1f5fd0]" aria-hidden="true" />
                      <span>{point}</span>
                    </li>
                  ))}
                </ul>

                <Link href="/contact" className="mt-8 inline-flex items-center rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white shadow-[0_10px_24px_rgba(31,95,208,0.22)] transition hover:bg-[#2f74e6]">
                  Discuss a project
                </Link>
              </div>

              <div className="order-1 relative min-h-[280px] lg:order-2">
                <Image
                  src={service.image}
                  alt={service.title}
                  fill
                  className="object-cover"
                  sizes="(max-width: 1024px) 100vw, 50vw"
                />
              </div>
            </div>
          </section>
        ))}
      </div>
    </main>
  );
}
