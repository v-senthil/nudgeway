# Calls troubleshooting

Common call problems and what you can do to fix them yourself. When the on-screen error isn't enough, an admin can open the [Provider calls log](#/audit-telemetry/provider-calls) for the full Meta response.

## Permission denied

- **Call fails with "No permission from recipient"** — the customer hasn't granted call permission. Click **Request permission** on the Call button and wait for them to tap Accept. See [Call permissions](#/calls/call-permissions).
- **Call button is greyed out** — the recipient has No permission. Use the **Request permission** chip on the button.
- **Permanent permission suddenly shows No permission** — the customer revoked it in their WhatsApp Privacy settings, or blocked your business number. There's nothing to fix on your side.
- **"Permission denied — call management"** — your role doesn't allow placing calls. Ask an admin.
- **"Permission denied — call read"** — your role doesn't allow reading call state. Ask an admin.

## Audio problems

- **You clicked Accept but the audio never connects** — refresh the tab and take the next call. If it keeps happening, ask an admin to check the call adapter.
- **One-way audio (you hear them but they don't hear you)** — your microphone is blocked at the operating-system level.
  - **macOS**: System Settings → Privacy and Security → Microphone → enable your browser.
  - **Windows**: Settings → Privacy → Microphone → allow apps and your browser.
  - Then refresh the tab.
- **Browser threw "microphone permission denied"** — click the padlock in the address bar, allow microphone access, refresh.
- **Calls work locally but not on the deployed site** — the deployed URL must be HTTPS. Microphone access is blocked on plain HTTP outside of `localhost`. Ask an admin to configure TLS.
- **Call connects but drops after a few seconds** — a corporate firewall may be blocking the audio stream. Ask your IT team to allow UDP outbound to Meta's calling servers.

## Recording missing

- **Call ended but recording is empty** — either recording wasn't enabled (check the integration's [Call settings](#/integrations/call-settings)), or Meta hasn't delivered the file yet. Wait 30 seconds and refresh.
- **"Call not found"** — the call belongs to another organization, or the URL was mistyped.
- **"Provider error" when playing the recording** — Meta's temporary URL expired. Refresh the page — Nudgeway will retry automatically.
- **Recording plays as silence** — the recipient never fully joined (dropped mid-connect). Duration will be near zero; there's nothing to recover.
- **"Transcript not available yet"** — transcription is still running. Refresh every 15 to 30 seconds.

## Call state stuck

- **Info message stuck on "Ringing"** — Meta didn't push the terminate event. Click **End call** if the live call bar is still open; otherwise refresh the thread — the missing update will backfill within a few minutes.
- **Two info messages for the same call** — refresh once; report it if it persists.
- **A "ghost" popup for a call that's already over** — your real-time connection dropped and reconnected out of order. Refresh.

## Related

- [Calls overview](#/calls/overview) — full lifecycle.
- [Call permissions](#/calls/call-permissions) — check and request.
- [Recording and transcript](#/calls/recording-transcript) — how the download works.
