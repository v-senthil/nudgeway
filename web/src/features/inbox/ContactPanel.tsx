export function ContactPanel() {
  return (
    <aside className="flex w-[320px] flex-shrink-0 flex-col border-l border-slate-200 bg-white">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Contact</h2>
      </div>
      <div className="flex flex-1 items-center justify-center px-6">
        <p className="text-center text-sm text-slate-500">
          Select a conversation to see contact details.
        </p>
      </div>
    </aside>
  );
}
