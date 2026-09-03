import { Button } from '../../components/Button';

type Props = {
  name: string;
  description: string;
  initials: string;
  accentClass?: string;
  onConnect: () => void;
};

export function IntegrationCard({ name, description, initials, accentClass, onConnect }: Props) {
  return (
    <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="flex items-center gap-3">
        <div
          className={`flex h-10 w-10 items-center justify-center rounded-xl text-sm font-bold text-white ${
            accentClass ?? 'bg-slate-500'
          }`}
          aria-hidden="true"
        >
          {initials}
        </div>
        <div>
          <p className="text-sm font-semibold text-slate-900">{name}</p>
          <p className="text-xs text-slate-500">{description}</p>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <span className="text-xs font-medium text-slate-500">Not connected</span>
        <Button variant="secondary" onClick={onConnect}>
          Connect
        </Button>
      </div>
    </div>
  );
}
