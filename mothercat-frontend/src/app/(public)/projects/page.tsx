import Link from "next/link";
import Image from "next/image";
import { siteContent } from "@/content/site";

const projectImages = [
  "/images/Skyline%20Tower.webp",
  "/images/Corporate%20Headquarters.jpeg",
  "/images/International%20Business%20Center.jpeg",
];

export default function ProjectsPage() {
  const projects = siteContent.home.projects;

  return (
    <main className="mx-auto max-w-6xl px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8">
      <div className="mb-8 flex items-end justify-between gap-4">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-blue-600">Projects</p>
          <h1 className="mt-3 font-serif text-5xl text-[#0f1b33]">Featured work</h1>
        </div>
        <Link href="/contact" className="hidden rounded-md border border-slate-300 px-4 py-2 text-sm font-semibold text-[#0f1b33] md:inline-flex">
          Enquire
        </Link>
      </div>

      {projects.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 bg-slate-50 p-10 text-center text-slate-600">
          No project data is available yet.
        </div>
      ) : (
        <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project, index) => (
            <article key={project.title} className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <div className="relative h-56 overflow-hidden">
                <Image
                  src={projectImages[index] ?? projectImages[0]}
                  alt={project.title}
                  fill
                  className="object-cover"
                  sizes="(max-width: 768px) 100vw, 33vw"
                />
              </div>
              <div className="p-5">
                <h2 className="font-serif text-3xl text-[#0f1b33]">{project.title}</h2>
                <p className="mt-3 text-sm leading-6 text-slate-600">{project.description}</p>
              </div>
            </article>
          ))}
        </div>
      )}
    </main>
  );
}
