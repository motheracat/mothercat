import Link from "next/link";
import Image from "next/image";

const logoSrc = "/Mothercat%20Logo%20Variations%20Sheet%20(Edited%202).png";

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-slate-100">
      <div className="grid min-h-screen lg:grid-cols-[1fr_560px]">
        <aside className="relative hidden overflow-hidden lg:block">
          <Image src="/images/Infrastructure-Development.jpeg" alt="Infrastructure development" fill priority quality={100} className="h-full w-full object-cover" sizes="50vw" />
          <div className="relative z-10 flex h-full flex-col justify-end p-12 text-white">
            <p className="text-sm uppercase tracking-[0.26em] text-blue-100">Mothercat</p>
            <h1 className="mt-4 max-w-md font-serif text-5xl leading-none">Building a stronger future.</h1>
            <p className="mt-5 max-w-sm text-base text-slate-200">Secure operations, trusted teams, and disciplined execution for every project stage.</p>
          </div>
        </aside>

        <main className="flex items-center justify-center bg-white p-6 sm:p-10">
          <div className="w-full max-w-md">
            <div className="mb-8 flex items-center">
              <Link href="/" className="flex items-center text-[#0f1b33]">
                <span className="flex h-16 w-16 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-white p-1 shadow-[0_10px_25px_rgba(15,23,42,0.08)] sm:h-20 sm:w-20">
                  <Image src={logoSrc} alt="Mothercat logo" width={160} height={160} className="h-full w-full rounded-full object-contain" />
                </span>
              </Link>
            </div>
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}
