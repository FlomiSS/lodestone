import Link from "next/link";

export default function NotFound() {
  return (
    <main className="grid min-h-screen place-items-center px-4 text-center">
      <div>
        <p className="text-sm text-muted">404</p>
        <h1 className="mt-1 text-2xl font-semibold">Seite nicht gefunden</h1>
        <Link href="/" className="mt-5 inline-block text-sm text-accent hover:underline">
          Zur Übersicht
        </Link>
      </div>
    </main>
  );
}
