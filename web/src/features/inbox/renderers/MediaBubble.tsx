import { useState } from 'react';
import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  /** Optional footer slot the driver uses to append timestamps / status ticks. */
  footer?: React.ReactNode;
};

/**
 * MediaBubble renders one inbound-or-outbound media message. It dispatches
 * on `msg.type` and picks the smallest correct browser primitive:
 *
 *  - image   → `<img>`   with a rounded thumbnail cap
 *  - video   → `<video controls>` at intrinsic aspect ratio
 *  - audio   → `<audio controls>` — always full-width
 *  - document → styled link with filename + download affordance
 *  - sticker → `<img>` fixed at 128 x 128
 *
 * `msg.media_url` is expected to be a same-origin URL (usually
 * `/api/v1/media/<key>`) so the browser reuses the session cookie for
 * auth. Absent bytes surface a subtle "Attachment unavailable" fallback
 * so the thread never renders as broken chrome — the message row still
 * carries provenance (`provider_message_id`, timestamps, caption).
 *
 * The component is self-contained: the parent (Thread.tsx) composes it
 * inside the standard chat bubble shell and passes the `footer` slot for
 * meta.
 */
export function MediaBubble({ msg, footer }: Props) {
  const url = msg.media_url;
  const caption = msg.media_caption ?? msg.text;

  if (url === undefined || url === '') {
    return <UnavailableAttachment kind={msg.type} caption={caption} footer={footer} />;
  }

  switch (msg.type) {
    case 'image':
      return <ImageBubble url={url} caption={caption} footer={footer} />;
    case 'video':
      return <VideoBubble url={url} caption={caption} footer={footer} />;
    case 'audio':
      return <AudioBubble url={url} caption={caption} footer={footer} />;
    case 'sticker':
      return <StickerBubble url={url} footer={footer} />;
    case 'document':
      return (
        <DocumentBubble
          url={url}
          filename={caption ?? 'document'}
          contentType={msg.content_type}
          footer={footer}
        />
      );
    default:
      return <UnavailableAttachment kind={msg.type} caption={caption} footer={footer} />;
  }
}

// --- Sub-components ---------------------------------------------------------

type SubProps = { url: string; caption?: string | undefined; footer?: React.ReactNode };

function ImageBubble({ url, caption, footer }: SubProps) {
  const [state, setState] = useState<'loading' | 'loaded' | 'error'>('loading');
  return (
    <div className="flex flex-col gap-1">
      <div className="relative overflow-hidden rounded-xl bg-slate-100">
        {state === 'loading' && <Skeleton />}
        {state !== 'error' && (
          <img
            src={url}
            alt={caption ?? 'image'}
            loading="lazy"
            decoding="async"
            onLoad={() => setState('loaded')}
            onError={() => setState('error')}
            className={
              'block max-h-72 max-w-xs object-cover ' + (state === 'loaded' ? '' : 'invisible')
            }
          />
        )}
        {state === 'error' && <BrokenBadge />}
      </div>
      {caption !== undefined && caption !== '' && <Caption text={caption} />}
      {footer}
    </div>
  );
}

function VideoBubble({ url, caption, footer }: SubProps) {
  const [errored, setErrored] = useState(false);
  return (
    <div className="flex flex-col gap-1">
      {errored ? (
        <div className="rounded-xl bg-slate-100 p-6 text-center">
          <BrokenBadge />
        </div>
      ) : (
        <video
          controls
          preload="metadata"
          onError={() => setErrored(true)}
          className="block max-h-72 max-w-xs rounded-xl bg-black"
        >
          <source src={url} />
          Your browser cannot play this video.
        </video>
      )}
      {caption !== undefined && caption !== '' && <Caption text={caption} />}
      {footer}
    </div>
  );
}

function AudioBubble({ url, caption, footer }: SubProps) {
  const [errored, setErrored] = useState(false);
  return (
    <div className="flex flex-col gap-1">
      {errored ? (
        <BrokenBadge />
      ) : (
        <audio
          controls
          preload="metadata"
          onError={() => setErrored(true)}
          className="block w-64 max-w-full"
        >
          <source src={url} />
        </audio>
      )}
      {caption !== undefined && caption !== '' && <Caption text={caption} />}
      {footer}
    </div>
  );
}

function StickerBubble({ url, footer }: SubProps) {
  const [errored, setErrored] = useState(false);
  return (
    <div className="flex flex-col gap-1">
      {errored ? (
        <BrokenBadge />
      ) : (
        <img
          src={url}
          alt="sticker"
          width={128}
          height={128}
          loading="lazy"
          decoding="async"
          onError={() => setErrored(true)}
          className="block h-32 w-32 object-contain"
        />
      )}
      {footer}
    </div>
  );
}

type DocProps = {
  url: string;
  filename: string;
  contentType?: string | undefined;
  footer?: React.ReactNode;
};

function DocumentBubble({ url, filename, contentType, footer }: DocProps) {
  return (
    <div className="flex flex-col gap-1">
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        download={filename}
        className="flex items-center gap-3 rounded-xl bg-slate-100 px-3 py-2 text-sm ring-1 ring-inset ring-slate-200 hover:bg-slate-200"
      >
        <DownloadIcon />
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-medium text-slate-900">{filename}</span>
          {contentType !== undefined && contentType !== '' && (
            <span className="truncate text-[11px] uppercase tracking-wide text-slate-500">
              {contentType}
            </span>
          )}
        </span>
      </a>
      {footer}
    </div>
  );
}

// --- Shared UI atoms --------------------------------------------------------

function Caption({ text }: { text: string }) {
  return <p className="whitespace-pre-wrap break-words text-sm">{text}</p>;
}

function Skeleton() {
  return (
    <div
      aria-hidden="true"
      className="h-40 w-64 animate-pulse bg-gradient-to-br from-slate-200 to-slate-100"
    />
  );
}

function BrokenBadge() {
  return (
    <p className="italic text-slate-500">Attachment unavailable</p>
  );
}

function UnavailableAttachment({
  kind,
  caption,
  footer,
}: {
  kind: string;
  caption?: string | undefined;
  footer?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[11px] uppercase tracking-wide opacity-70">{kind}</p>
      <BrokenBadge />
      {caption !== undefined && caption !== '' && <Caption text={caption} />}
      {footer}
    </div>
  );
}

function DownloadIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className="h-5 w-5 flex-none text-slate-500"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M12 3v12" />
      <path d="M7 10l5 5 5-5" />
      <path d="M5 21h14" />
    </svg>
  );
}
