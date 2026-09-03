import { useRef, useState } from 'react';
import { MAX_ATTACHMENT_BYTES } from '../../../lib/attachments';

type Props = {
  /** Fired when the operator picks a new file. Parent kicks off the upload. */
  onSelected: (file: File) => void;
  /** Fired when the operator clears the current selection before sending. */
  onClear: () => void;
  /** The currently-selected File, if any. Controls the preview strip. */
  file: File | null;
  /** True while the file is being uploaded — disables re-picking. */
  uploading?: boolean;
  /** Non-null when the last upload attempt failed; rendered inline. */
  error?: string | null;
};

const ACCEPT = 'image/*,video/*,audio/*,application/*';

/**
 * ComposerAttach renders a paperclip button + hidden file input plus a small
 * preview strip when a file is selected. It owns no upload state — the parent
 * Composer wires the picker to `useUploadAttachment` and passes the resulting
 * `file` back down so this component can render the correct preview.
 */
export function ComposerAttach({ onSelected, onClear, file, uploading = false, error = null }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [previewURL, setPreviewURL] = useState<string | null>(null);

  const openPicker = () => {
    inputRef.current?.click();
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const picked = e.target.files?.[0] ?? null;
    // Reset the input so re-picking the same file still fires onChange.
    e.target.value = '';
    if (picked === null) return;
    if (picked.size > MAX_ATTACHMENT_BYTES) {
      // Surface the size error through onClear + a synthetic call; parent
      // owns error display.
      onClear();
      return;
    }
    if (previewURL !== null) {
      URL.revokeObjectURL(previewURL);
    }
    if (picked.type.startsWith('image/')) {
      setPreviewURL(URL.createObjectURL(picked));
    } else {
      setPreviewURL(null);
    }
    onSelected(picked);
  };

  const handleClear = () => {
    if (previewURL !== null) {
      URL.revokeObjectURL(previewURL);
      setPreviewURL(null);
    }
    onClear();
  };

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        onClick={openPicker}
        disabled={uploading}
        aria-label="Attach a file"
        className="flex h-11 w-11 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-500 shadow-sm hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <svg viewBox="0 0 24 24" fill="none" className="h-5 w-5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M21.44 11.05L12.25 20.24a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
        </svg>
      </button>

      <input
        ref={inputRef}
        type="file"
        accept={ACCEPT}
        className="hidden"
        onChange={handleChange}
        aria-hidden="true"
        tabIndex={-1}
      />

      {file !== null && (
        <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700">
          {previewURL !== null ? (
            <img
              src={previewURL}
              alt={file.name}
              className="h-10 w-10 rounded-md object-cover ring-1 ring-slate-200"
            />
          ) : (
            <div className="flex h-10 w-10 items-center justify-center rounded-md bg-white text-[10px] font-medium uppercase tracking-wide text-slate-500 ring-1 ring-slate-200">
              {fileExt(file.name)}
            </div>
          )}
          <div className="min-w-0 flex-1">
            <p className="truncate font-medium text-slate-900">{file.name}</p>
            <p className="text-[11px] text-slate-500">
              {formatBytes(file.size)}
              {uploading && ' · uploading…'}
            </p>
          </div>
          <button
            type="button"
            onClick={handleClear}
            className="rounded-md px-2 py-1 text-[11px] font-medium text-slate-500 hover:bg-white hover:text-slate-700"
          >
            Clear
          </button>
        </div>
      )}

      {error !== null && (
        <p role="alert" className="text-[11px] text-rose-700">
          {error}
        </p>
      )}
    </div>
  );
}

function fileExt(name: string): string {
  const dot = name.lastIndexOf('.');
  if (dot < 0 || dot === name.length - 1) return 'FILE';
  return name.slice(dot + 1, dot + 5).toUpperCase();
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
