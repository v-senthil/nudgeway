// _nav-provider-calls.ts — sidebar entry for the Meta API execution logs
// viewer. Exported as a plain descriptor so the settings sidebar can pull
// it in without touching this file's route definition directly.
//
// The wire-up commit (settings.tsx sidebar) will read this object and
// add it to the `sections` array. Keeping the descriptor as a separate
// module means parallel agents don't step on each other's route lists.

export type SettingsNavEntry = {
  label: string;
  to: string;
};

export const providerCallsNavEntry: SettingsNavEntry = {
  label: 'Meta API logs',
  to: '/settings/provider-calls',
};
