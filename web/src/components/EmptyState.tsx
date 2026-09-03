import type { ReactNode } from 'react';

type Props = {
  icon?: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
};

export function EmptyState({ icon, title, description, action, className = '' }: Props) {
  return (
    <div
      className={`flex flex-col items-center justify-center gap-3 px-6 py-10 text-center ${className}`}
    >
      {icon !== undefined && (
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-slate-100 text-slate-400">
          {icon}
        </div>
      )}
      <div className="space-y-1">
        <p className="text-sm font-medium text-slate-700">{title}</p>
        {description !== undefined && (
          <p className="text-xs text-slate-500 max-w-xs">{description}</p>
        )}
      </div>
      {action !== undefined && <div className="pt-1">{action}</div>}
    </div>
  );
}
