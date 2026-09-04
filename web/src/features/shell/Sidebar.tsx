import { useEffect, useState, type ComponentType, type SVGProps } from 'react';
import { Link, useRouterState } from '@tanstack/react-router';
import {
  BarChart3,
  Bot,
  ChevronLeft,
  ChevronRight,
  Contact,
  FileText,
  Key,
  LayoutDashboard,
  Megaphone,
  MessageSquare,
  PhoneCall,
  ScrollText,
  Shield,
  ShieldCheck,
  Users,
  Webhook,
  Workflow,
  Zap,
} from 'lucide-react';

type IconType = ComponentType<SVGProps<SVGSVGElement>>;

type Item = {
  label: string;
  icon: IconType;
  to?: string;
  comingSoon?: boolean;
};

type Section = {
  label: string;
  items: Item[];
};

// Whatomate-style sidebar section layout adapted to the routes Nudgeway has
// today. Items without a live route are rendered as "Coming soon" and are
// not click-through.
const TOP_SECTIONS: Section[] = [
  {
    label: 'Main',
    items: [
      { label: 'Dashboard', icon: LayoutDashboard, to: '/dashboard', comingSoon: true },
      { label: 'Inbox', icon: MessageSquare, to: '/inbox' },
      { label: 'Contacts', icon: Contact, comingSoon: true },
    ],
  },
  {
    label: 'Messaging',
    items: [
      { label: 'Templates', icon: FileText, to: '/settings/templates' },
      { label: 'Groups', icon: Users, to: '/settings/groups' },
      { label: 'Campaigns', icon: Megaphone, comingSoon: true },
      { label: 'Chatbot', icon: Bot, comingSoon: true },
      { label: 'Flows', icon: Workflow, comingSoon: true },
    ],
  },
  {
    label: 'Calling',
    items: [{ label: 'Call Logs', icon: PhoneCall, to: '/calls' }],
  },
  {
    label: 'Analytics',
    items: [
      { label: 'Analytics', icon: BarChart3, to: '/analytics' },
      { label: 'Meta API Logs', icon: ScrollText, to: '/settings/provider-calls' },
    ],
  },
];

const SETTINGS_SECTION: Section = {
  label: 'Settings',
  items: [
    { label: 'Integrations', icon: Zap, to: '/settings/integrations' },
    { label: 'Audit Log', icon: ShieldCheck, to: '/settings/audit' },
    { label: 'Users & Roles', icon: Shield, comingSoon: true },
    { label: 'API Keys', icon: Key, comingSoon: true },
    { label: 'Webhooks', icon: Webhook, comingSoon: true },
  ],
};

const STORAGE_KEY = 'nudgeway.sidebar.collapsed';

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

// isActive returns true when the current path starts with the item's `to`.
// This matches the whatomate rule that "child of /settings/templates" still
// highlights the parent nav row.
function isActive(currentPath: string, to: string): boolean {
  if (to === '/') return currentPath === '/';
  if (currentPath === to) return true;
  return currentPath.startsWith(to + '/');
}

function ItemLink({
  item,
  collapsed,
  currentPath,
}: {
  item: Item;
  collapsed: boolean;
  currentPath: string;
}) {
  const Icon = item.icon;

  if (item.comingSoon === true || item.to === undefined) {
    return (
      <div
        title={collapsed ? `${item.label} (coming soon)` : undefined}
        aria-disabled="true"
        className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-slate-300 cursor-not-allowed dark:text-slate-600 ${
          collapsed ? 'justify-center px-2' : ''
        }`}
      >
        <Icon className="h-5 w-5 flex-shrink-0" aria-hidden="true" />
        {!collapsed && (
          <>
            <span className="truncate">{item.label}</span>
            <span className="ml-auto rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-slate-400 dark:bg-slate-800 dark:text-slate-500">
              Soon
            </span>
          </>
        )}
      </div>
    );
  }

  const active = isActive(currentPath, item.to);
  const base = `flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors ${
    collapsed ? 'justify-center px-2' : ''
  }`;
  const activeCls =
    'bg-emerald-50 text-emerald-700 border-r-2 border-emerald-500 dark:bg-emerald-950/40 dark:text-emerald-300 dark:border-emerald-400';
  const idleCls =
    'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-slate-100';

  return (
    <Link
      to={item.to}
      title={collapsed ? item.label : undefined}
      className={`${base} ${active ? activeCls : idleCls}`}
    >
      <Icon className="h-5 w-5 flex-shrink-0" aria-hidden="true" />
      {!collapsed && <span className="truncate">{item.label}</span>}
    </Link>
  );
}

function SectionBlock({
  section,
  collapsed,
  currentPath,
}: {
  section: Section;
  collapsed: boolean;
  currentPath: string;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      {!collapsed && (
        <p className="px-3 pb-1 pt-3 text-xs font-medium uppercase tracking-wide text-slate-400 dark:text-slate-500">
          {section.label}
        </p>
      )}
      {collapsed && <div className="mx-2 my-2 h-px bg-slate-100 dark:bg-slate-800" />}
      {section.items.map((item) => (
        <ItemLink
          key={item.label}
          item={item}
          collapsed={collapsed}
          currentPath={currentPath}
        />
      ))}
    </div>
  );
}

export function Sidebar() {
  const [collapsed, setCollapsed] = useState<boolean>(() => readCollapsed());
  const routerState = useRouterState();
  const currentPath = routerState.location.pathname;

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, collapsed ? '1' : '0');
    } catch {
      // ignore
    }
  }, [collapsed]);

  return (
    <aside
      className={`flex h-full flex-col border-r border-slate-200 bg-white transition-all duration-200 dark:border-slate-800 dark:bg-slate-900 ${
        collapsed ? 'w-16' : 'w-64'
      }`}
      aria-label="Primary navigation"
    >
      <div
        className={`flex h-14 flex-shrink-0 items-center border-b border-slate-200 px-3 dark:border-slate-800 ${
          collapsed ? 'justify-center' : 'justify-between'
        }`}
      >
        {!collapsed && (
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-600 text-white">
              <span className="text-[11px] font-bold">fW</span>
            </div>
            <span className="text-sm font-semibold tracking-tight text-slate-900 dark:text-slate-100">
              Nudgeway
            </span>
          </div>
        )}
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 hover:text-slate-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-200"
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          ) : (
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          )}
        </button>
      </div>

      <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-2 py-2">
        {TOP_SECTIONS.map((section) => (
          <SectionBlock
            key={section.label}
            section={section}
            collapsed={collapsed}
            currentPath={currentPath}
          />
        ))}
        <div className="mt-auto">
          <SectionBlock
            section={SETTINGS_SECTION}
            collapsed={collapsed}
            currentPath={currentPath}
          />
        </div>
      </nav>
    </aside>
  );
}

// findBreadcrumb resolves the current path to a display label + optional
// parent. Exported so TopBar can render "Settings › Templates" without
// re-declaring the section table.
export function findBreadcrumb(currentPath: string): { section?: string; page: string } {
  const all: Array<{ section: string; item: Item }> = [];
  for (const s of TOP_SECTIONS) {
    for (const it of s.items) {
      all.push({ section: s.label, item: it });
    }
  }
  for (const it of SETTINGS_SECTION.items) {
    all.push({ section: SETTINGS_SECTION.label, item: it });
  }

  // Prefer the most specific (longest) match so /settings/templates wins
  // over /settings/*.
  let best: { section: string; item: Item } | null = null;
  for (const entry of all) {
    if (entry.item.to === undefined) continue;
    if (!isActive(currentPath, entry.item.to)) continue;
    if (best === null || entry.item.to.length > (best.item.to?.length ?? 0)) {
      best = entry;
    }
  }
  if (best === null) return { page: '' };
  if (best.section === 'Main' || best.section === 'Calling' || best.section === 'Analytics') {
    return { page: best.item.label };
  }
  return { section: best.section, page: best.item.label };
}
