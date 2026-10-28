export default function DashboardPage() {
  return (
    <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
      <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <p className="text-sm text-slate-500">Active staff</p>
        <p className="mt-3 text-3xl font-semibold text-[#0f1b33]">—</p>
      </div>
      <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <p className="text-sm text-slate-500">Pending setup</p>
        <p className="mt-3 text-3xl font-semibold text-[#0f1b33]">—</p>
      </div>
      <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <p className="text-sm text-slate-500">Departments</p>
        <p className="mt-3 text-3xl font-semibold text-[#0f1b33]">—</p>
      </div>
      <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <p className="text-sm text-slate-500">Recent activity</p>
        <p className="mt-3 text-3xl font-semibold text-[#0f1b33]">—</p>
      </div>
    </div>
  );
}
