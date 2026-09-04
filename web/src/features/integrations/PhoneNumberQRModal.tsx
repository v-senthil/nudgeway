import { useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { Modal } from '../../components/Modal';

/** PhoneNumberQRModal renders a scannable QR that encodes the wa.me
 * deep link for the integration's phone number. Operators drop this on
 * marketing collateral or business cards so customers can start a chat
 * without typing a phone number.
 *
 * The E.164 input is normalised to digits-only for the wa.me URL — Meta
 * accepts only digits after the domain segment. */
export function PhoneNumberQRModal({
  open,
  onClose,
  phone,
}: {
  open: boolean;
  onClose: () => void;
  phone: string;
}) {
  const [copied, setCopied] = useState(false);
  const digits = phone.replace(/[^0-9]/g, '');
  const url = `https://wa.me/${digits}`;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard blocked (insecure origin / permission); the URL is
      // already visible for manual copy so we swallow silently.
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="WhatsApp chat QR">
      <div className="flex flex-col items-center gap-4">
        {digits.length > 0 ? (
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <QRCodeSVG value={url} size={220} level="M" includeMargin={false} />
          </div>
        ) : (
          <div className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700">
            No phone number available to encode.
          </div>
        )}
        <p className="text-xs text-slate-500">
          Scan this QR to open a WhatsApp chat with this business number.
        </p>
        <div className="w-full">
          <label className="text-[11px] font-medium uppercase tracking-wide text-slate-500">
            wa.me link
          </label>
          <div className="mt-1 flex items-center gap-2">
            <div className="flex-1 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-xs text-slate-800 break-all">
              {url}
            </div>
            <button
              type="button"
              onClick={() => void copy()}
              className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-emerald-700 shadow-sm hover:bg-slate-50"
            >
              {copied ? 'Copied' : 'Copy link'}
            </button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
