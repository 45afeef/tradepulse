import type { IconName } from '../ui/Icon';
import { outline } from '../lib/markdown';
import quickStart from './quick-start.md?raw';
import feed from './feed.md?raw';
import alerts from './alerts.md?raw';
import delivery from './delivery.md?raw';
import account from './account.md?raw';
import faq from './faq.md?raw';
import glossary from './glossary.md?raw';
import dataUse from './data-use.md?raw';
import operatorHandbook from './operator-handbook.md?raw';
import sourcesGuide from './sources-guide.md?raw';

export type Audience = 'member' | 'operator';
export interface Topic { slug: string; audience: Audience; icon: IconName; md: string; title: string; summary: string; text: string }

// Order here is the order shown in the help centre. Add a page: create the .md file, import it above, list it here
// (a test fails if a .md file is not listed, or if a link points nowhere).
const defs: { slug: string; audience: Audience; icon: IconName; md: string }[] = [
  { slug: 'quick-start', audience: 'member', icon: 'zap', md: quickStart },
  { slug: 'feed', audience: 'member', icon: 'feed', md: feed },
  { slug: 'alerts', audience: 'member', icon: 'bell', md: alerts },
  { slug: 'delivery', audience: 'member', icon: 'send', md: delivery },
  { slug: 'account', audience: 'member', icon: 'key', md: account },
  { slug: 'faq', audience: 'member', icon: 'info', md: faq },
  { slug: 'glossary', audience: 'member', icon: 'list', md: glossary },
  { slug: 'data-use', audience: 'member', icon: 'shield', md: dataUse },
  { slug: 'operator-handbook', audience: 'operator', icon: 'activity', md: operatorHandbook },
  { slug: 'sources-guide', audience: 'operator', icon: 'database', md: sourcesGuide },
];

export const TOPICS: Topic[] = defs.map((d) => ({ ...d, ...outline(d.md) }));
export const topicBySlug = (slug: string | undefined) => TOPICS.find((t) => t.slug === slug);
