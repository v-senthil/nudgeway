import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Spinner } from './Spinner';

type Variant = 'primary' | 'secondary' | 'ghost';

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  loading?: boolean;
  children: ReactNode;
};

const base =
  'inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2 text-sm font-medium transition ' +
  'focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2 ' +
  'disabled:cursor-not-allowed';

const variants: Record<Variant, string> = {
  primary:
    'bg-emerald-600 text-white shadow-sm hover:bg-emerald-500 disabled:bg-slate-300 disabled:text-slate-500',
  secondary:
    'bg-white text-slate-700 border border-slate-200 shadow-sm hover:bg-slate-50 disabled:bg-slate-50 disabled:text-slate-400',
  ghost:
    'bg-transparent text-slate-700 hover:bg-slate-100 disabled:text-slate-400',
};

export function Button({
  variant = 'primary',
  loading = false,
  disabled,
  className = '',
  children,
  type,
  ...rest
}: Props) {
  return (
    <button
      type={type ?? 'button'}
      className={`${base} ${variants[variant]} ${className}`}
      disabled={disabled === true || loading}
      {...rest}
    >
      {loading && <Spinner className="h-4 w-4" />}
      {children}
    </button>
  );
}
