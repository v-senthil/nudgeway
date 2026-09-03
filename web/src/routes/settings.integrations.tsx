import { useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { IntegrationCard } from '../features/settings/IntegrationCard';
import { ComingSoonModal } from '../features/settings/ComingSoonModal';

type Integration = {
  name: string;
  description: string;
  initials: string;
  accentClass: string;
};

type Category = {
  title: string;
  description: string;
  items: Integration[];
};

const categories: Category[] = [
  {
    title: 'Channels',
    description: 'Connect messaging channels to receive and send conversations.',
    items: [
      {
        name: 'WhatsApp',
        description: 'Meta WhatsApp Business Cloud API.',
        initials: 'WA',
        accentClass: 'bg-emerald-600',
      },
    ],
  },
  {
    title: 'Ticketing',
    description: 'Sync conversations with your ticketing system.',
    items: [
      {
        name: 'Zoho Desk',
        description: 'Two-way sync of tickets and conversations.',
        initials: 'ZD',
        accentClass: 'bg-red-500',
      },
    ],
  },
  {
    title: 'AI & Bots',
    description: 'Route messages through AI providers for automation.',
    items: [
      {
        name: 'OpenAI',
        description: 'GPT models for replies, summaries and routing.',
        initials: 'AI',
        accentClass: 'bg-slate-900',
      },
      {
        name: 'Anthropic',
        description: 'Claude models for agents and workflows.',
        initials: 'AN',
        accentClass: 'bg-amber-600',
      },
      {
        name: 'Zoho Zia',
        description: 'Zoho’s built-in AI assistant.',
        initials: 'ZI',
        accentClass: 'bg-indigo-600',
      },
    ],
  },
];

function IntegrationsPage() {
  const [modalFor, setModalFor] = useState<string | null>(null);

  return (
    <div className="mx-auto max-w-4xl space-y-8">
      <header>
        <h1 className="text-2xl font-semibold text-slate-900">Integrations</h1>
        <p className="mt-1 text-sm text-slate-500">
          Connect channels, ticketing systems and AI providers to your workspace.
        </p>
      </header>

      {categories.map((category) => (
        <section key={category.title} className="space-y-3">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">{category.title}</h2>
            <p className="text-xs text-slate-500">{category.description}</p>
          </div>
          <div className="space-y-2">
            {category.items.map((item) => (
              <IntegrationCard
                key={item.name}
                name={item.name}
                description={item.description}
                initials={item.initials}
                accentClass={item.accentClass}
                onConnect={() => setModalFor(item.name)}
              />
            ))}
          </div>
        </section>
      ))}

      <ComingSoonModal
        open={modalFor !== null}
        onClose={() => setModalFor(null)}
        integration={modalFor}
      />
    </div>
  );
}

export const settingsIntegrationsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/integrations',
  component: IntegrationsPage,
});

export const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/',
  component: IntegrationsPage,
});
