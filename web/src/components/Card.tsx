import type { HTMLAttributes, ReactNode } from 'react';

type Props = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
};

export function Card({ children, className = '', ...rest }: Props) {
  return (
    <div
      className={`rounded-xl border border-slate-200 bg-white shadow-sm ${className}`}
      {...rest}
    >
      {children}
    </div>
  );
}
