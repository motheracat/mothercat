import type { Metadata } from "next";
import { Barlow_Condensed, Cormorant_Garamond, Inter } from "next/font/google";
import "./globals.css";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
});

const cormorant = Cormorant_Garamond({
  variable: "--font-cormorant",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const barlowCondensed = Barlow_Condensed({
  variable: "--font-barlow-condensed",
  subsets: ["latin"],
  weight: ["500", "600", "700"],
});

export const metadata: Metadata = {
  title: "Mothercat",
  description: "Construction and civil engineering management platform.",
  icons: {
    icon: [
      { url: "/Mothercat%20Logo%20Variations%20Sheet%20%28Edited%202%29.png", type: "image/png" },
      { url: "/Mothercat%20Logo%20Variations%20Sheet%20%28Edited%29.png", type: "image/png" },
    ],
    shortcut: "/Mothercat%20Logo%20Variations%20Sheet%20%28Edited%202%29.png",
    apple: "/Mothercat%20Logo%20Variations%20Sheet%20%28Edited%202%29.png",
  },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${inter.variable} ${cormorant.variable} ${barlowCondensed.variable} antialiased`}>
      <body className="min-h-full bg-white text-slate-900">{children}</body>
    </html>
  );
}
