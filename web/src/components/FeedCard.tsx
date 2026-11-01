import { useState } from 'react';
import type { FeedItem, Rating } from '../api/types';
import { hostOf, safeUrl, timeAgo } from '../lib/format';
import { Badge, Card, Chip, Icon, cx } from '../ui';

export function FeedCard({ item, now, onRate }: { item: FeedItem; now: number; onRate: (id: string, rating: Rating | null) => void }) {
  const [why, setWhy] = useState(false);
  const url = safeUrl(item.url);
  const host = hostOf(url);
  return (
    <Card className={cx('feed-item', item.severity === 'high' && 'alerted', item.severity === 'notable' && 'notable')} aria-label={item.title}>
      <div className="row wrap small">
        <Chip source={item.source}>{item.source}</Chip>
        {item.entities.slice(0, 3).map((e) => <Badge key={e}>{e}</Badge>)}
        {item.severity !== 'info' && <Badge tone={item.severity} icon="zap">{item.severity}</Badge>}
        <span className="faint">{timeAgo(item.created_at, now)}</span>
      </div>
      <h3>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{item.title}</a> : item.title}</h3>
      {item.body && <p className="feed-body">{item.body}</p>}
      <div className="feed-actions">
        <button type="button" className="rate up" aria-pressed={item.feedback === 'useful'} aria-label="Mark as useful" onClick={() => onRate(item.id, item.feedback === 'useful' ? null : 'useful')}><Icon name="up" size={15} />Useful</button>
        <button type="button" className="rate down" aria-pressed={item.feedback === 'not_useful'} aria-label="Mark as not useful" onClick={() => onRate(item.id, item.feedback === 'not_useful' ? null : 'not_useful')}><Icon name="down" size={15} />Not useful</button>
        <button type="button" className="rate" aria-expanded={why} onClick={() => setWhy((w) => !w)}><Icon name="info" size={15} />Why this</button>
        {url && <a className="rate" href={url} target="_blank" rel="noopener noreferrer"><Icon name="external" size={15} />Open{host ? ` · ${host}` : ''}</a>}
      </div>
      {why && <div className="why" role="note"><b>Why you see this:</b> {item.reason}</div>}
    </Card>
  );
}
