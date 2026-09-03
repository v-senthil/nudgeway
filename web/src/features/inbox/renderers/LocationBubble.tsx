import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

/**
 * LocationBubble renders a shared-location message: a small pin, an
 * optional venue name / address, and an "Open in Maps" link.
 *
 * Behaves the same on inbound and outbound; the caller decides
 * background colour via the enclosing bubble wrapper.
 */
export function LocationBubble({ msg, footer }: Props) {
  const loc = msg.location;
  if (loc === undefined) {
    return (
      <p className="italic opacity-70">[location payload missing]</p>
    );
  }
  const q = `${loc.latitude},${loc.longitude}`;
  const mapsUrl = `https://www.google.com/maps?q=${encodeURIComponent(q)}`;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-start gap-2">
        <span aria-hidden="true" className="mt-0.5 text-lg leading-none">📍</span>
        <div className="min-w-0 flex-1">
          {loc.name !== undefined && loc.name !== '' && (
            <p className="truncate font-semibold">{loc.name}</p>
          )}
          {loc.address !== undefined && loc.address !== '' && (
            <p className="truncate text-xs opacity-80">{loc.address}</p>
          )}
          <p className="text-[11px] opacity-70">
            {loc.latitude.toFixed(5)}, {loc.longitude.toFixed(5)}
          </p>
          <a
            href={mapsUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-1 inline-block text-xs font-medium underline"
          >
            Open in Maps
          </a>
        </div>
      </div>
      {footer}
    </div>
  );
}
