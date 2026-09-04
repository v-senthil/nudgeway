# Recording and transcript

When a call ends, Nudgeway downloads the recording and (if you enabled it) the transcript, then makes both available on the call detail page.

## Turning on recording and transcription

- **For inbound calls** — defaults come from the integration's [Call settings](#/integrations/call-settings). Turn them on there once and every inbound call is recorded.
- **For outbound calls** — the call composer has two toggles right below the phone number: **Record call** and **Transcribe**. Tick them before clicking Call.

## How to access after a call

1. Open the conversation in the Inbox.
2. Scroll to the info message that says **Completed**, **Missed**, or **Failed** for the call in question.
3. Click it. The call detail page opens with:
   - An audio player at the top. Press play to hear the call.
   - A transcript below the player, if transcription was enabled.
   - A download button to save the audio file locally.

Recordings take a few seconds to appear after the call ends (Nudgeway downloads them from Meta first). Transcripts take longer — typically 30 to 90 seconds.

## Storage

Recordings are stored inside Nudgeway. The customer's browser never sees Meta's URLs. Duplicate audio (for example, the same voicemail replayed) is stored once.

## Troubleshooting

- **The player shows "Transcript not available yet"** — Meta is still transcribing. Refresh the page every 15 to 30 seconds; it usually arrives within a minute.
- **The player shows "Recording not available"** — either recording wasn't enabled for this call, or Meta hasn't delivered the file yet. Wait 30 seconds and refresh. If it's still missing after a minute, check the integration's [Call settings](#/integrations/call-settings) — recording may be off by default.
- **The audio plays as static or won't play at all** — click **Download** and try opening the file in another player (VLC, Quicktime). Meta occasionally serves the audio with an unhelpful content type; the file itself is usually fine.
- **"Call not found"** — the call belongs to another organization, or the URL was mistyped. Go back to the Inbox and click through from the info message.
- **"Integration missing"** — the WhatsApp integration was deleted, so Nudgeway can't fetch the file. Reconnect the integration and re-fire the call from Meta (rare — most calls download successfully on the first try).

## Related

- [Calls overview](#/calls/overview) — lifecycle and dedup rules.
- [Inbound calls](#/calls/inbound-call) — accept flow.
- [Outbound calls](#/calls/outbound-call) — set recording on before you dial.
