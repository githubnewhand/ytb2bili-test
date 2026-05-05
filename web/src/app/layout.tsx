import type { Metadata } from "next";
import "./globals.css";
import { APP_TITLE } from '@/lib/appVersion';

export const metadata: Metadata = {
  title: APP_TITLE,
  description: "Web platform for managing Bilibili video uploads and subtitle processing.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen bg-gray-50">
        {children}
      </body>
    </html>
  );
}
