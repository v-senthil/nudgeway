# Inbound calls

When a WhatsApp user calls your business number, a popup appears in the bottom-right of the Inbox with the caller's name and Accept / Reject buttons.

## The popup

- **Accept** — negotiates the audio session in your browser. Your microphone is used, so grant mic permission the first time you accept a call.
- **Reject** — declines the call. The caller hears the WhatsApp "declined" tone.
- **Auto-dismiss** — if the caller hangs up before you click either button, the popup closes automatically and the thread shows a **Missed call** info message.

Multiple tabs open? Only the tab that clicks Accept first wins; the other tabs' popups close automatically.

## What happens after Accept

1. Your browser asks for microphone permission the first time. Click **Allow**.
2. Audio connects within a few seconds. You'll see a live call bar at the top of the middle pane with a mute button and a hang-up button.
3. When either side hangs up, the bar closes and a **Completed** info message appears in the thread. Click the info message to open the call detail page with the recording and transcript.

## Troubleshooting

- **The popup never appears when a customer calls** — your real-time connection dropped. Refresh the page. If it still doesn't appear after a refresh, ask an admin to verify webhook subscriptions include call events via [Push webhook to Meta](#/integrations/webhook-setup).
- **You click Accept but the audio never connects** — refresh the tab and answer the next call. If it's happening every time, an admin should check the call adapter's health.
- **You can hear the caller but they can't hear you** — your microphone is muted at the operating-system level. On macOS, open System Settings → Privacy and Security → Microphone and enable your browser. On Windows, Settings → Privacy → Microphone.
- **The browser threw "microphone permission denied"** — click the padlock in the address bar, grant microphone access, then refresh the tab.
- **The call rings but auto-declines after a few seconds** — either you took too long to click Accept (Meta has a ring timeout) or Meta had a delay reaching Nudgeway. Answer faster; if it keeps happening, ask an admin.
- **Calls work on your local machine but not on the deployed site** — the deployed URL must be HTTPS. Ask an admin to configure TLS.

## Related

- [Calls overview](#/calls/overview) — full lifecycle.
- [Outbound calls](#/calls/outbound-call) — place a call from your side.
- [Recording and transcript](#/calls/recording-transcript) — access after the call ends.
