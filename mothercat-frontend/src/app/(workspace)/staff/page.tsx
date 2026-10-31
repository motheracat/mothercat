export default function StaffPage() {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div className="mb-5 flex items-center justify-between">
        <h2 className="font-serif text-4xl text-[#0f1b33]">Staff</h2>
        <button className="rounded-md bg-[#1f5fd0] px-4 py-2 text-sm font-semibold text-white">Add staff</button>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-slate-50 text-slate-600">
            <tr>
              <th className="px-4 py-3">Staff ID</th>
              <th className="px-4 py-3">Full name</th>
              <th className="px-4 py-3">Department</th>
              <th className="px-4 py-3">Job title</th>
              <th className="px-4 py-3">Status</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-t border-slate-200 text-slate-700">
              <td className="px-4 py-3">—</td>
              <td className="px-4 py-3">No staff records available</td>
              <td className="px-4 py-3">—</td>
              <td className="px-4 py-3">—</td>
              <td className="px-4 py-3">—</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
