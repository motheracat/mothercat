import Image from "next/image";
import Link from "next/link";
import { ArrowRight, CalendarRange } from "lucide-react";

import { NewsletterSignup } from "@/components/public/newsletter-signup";
import { siteContent } from "@/content/site";

export default function NewsPage() {
  const articles = siteContent.news ?? [];
  const [featuredArticle, ...remainingArticles] = articles;

  return (
    <main className="mx-auto max-w-6xl px-4 py-20 text-[#0f1b33] sm:px-6 lg:px-8">
      <section className="flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-blue-600">News &amp; insights</p>
          <h1 className="mt-3 max-w-3xl font-serif text-5xl text-[#0f1b33]">Updates from the field and the workplace.</h1>
        </div>
        <Link
          href="/contact"
          className="inline-flex items-center gap-2 self-start rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-semibold text-[#0f1b33] transition hover:border-[#1f5fd0] hover:text-[#1f5fd0]"
        >
          Request a briefing
          <ArrowRight size={16} aria-hidden="true" />
        </Link>
      </section>

      {articles.length === 0 ? (
        <div className="mt-10 rounded-xl border border-dashed border-slate-300 bg-slate-50 p-12 text-center text-slate-600">
          News content will be published here once the editorial calendar is approved.
        </div>
      ) : (
        <div className="mt-12 grid gap-8 lg:grid-cols-[1.25fr_0.75fr]">
          <div className="space-y-6">
            <article className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-[0_15px_30px_rgba(15,27,51,0.05)]">
              <div className="relative h-72 w-full overflow-hidden">
                <Image
                  src={featuredArticle.image}
                  alt={featuredArticle.title}
                  fill
                  className="object-cover"
                  sizes="(max-width: 1024px) 100vw, 62vw"
                />
              </div>

              <div className="p-6 sm:p-8">
                <div className="flex flex-wrap items-center gap-3 text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">
                  <span>{featuredArticle.category}</span>
                  <span className="text-slate-300">•</span>
                  <span className="flex items-center gap-1.5 text-slate-500">
                    <CalendarRange size={12} aria-hidden="true" />
                    {featuredArticle.date}
                  </span>
                </div>

                <h2 className="mt-4 font-serif text-3xl text-[#0f1b33] sm:text-4xl">{featuredArticle.title}</h2>
                <p className="mt-4 max-w-2xl text-base leading-7 text-slate-600">{featuredArticle.summary}</p>

                <div className="mt-6 flex items-center justify-between gap-4 border-t border-slate-200 pt-5">
                  <span className="text-sm text-slate-500">{featuredArticle.readTime}</span>
                  <Link
                    href={featuredArticle.href}
                    className="inline-flex items-center gap-2 text-sm font-semibold text-[#1f5fd0] transition hover:text-[#0a2a52]"
                  >
                    Read article
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                </div>
              </div>
            </article>

            <div className="grid gap-5 md:grid-cols-2">
              {remainingArticles.map((article) => (
                <article key={article.title} className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
                  <div className="relative h-48 w-full overflow-hidden">
                    <Image
                      src={article.image}
                      alt={article.title}
                      fill
                      className="object-cover"
                      sizes="(max-width: 768px) 100vw, 50vw"
                    />
                  </div>
                  <div className="p-5">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-blue-600">{article.category}</p>
                    <h3 className="mt-3 font-serif text-2xl text-[#0f1b33]">{article.title}</h3>
                    <p className="mt-3 text-sm leading-6 text-slate-600">{article.summary}</p>
                    <div className="mt-5 flex items-center justify-between gap-2 border-t border-slate-200 pt-4 text-xs text-slate-500">
                      <span>{article.date}</span>
                      <span>{article.readTime}</span>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          </div>

          <NewsletterSignup />
        </div>
      )}
    </main>
  );
}
