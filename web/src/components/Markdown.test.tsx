import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Markdown } from './Markdown';

const r = (source: string, skipTitle = false) => render(<MemoryRouter><Markdown source={source} skipTitle={skipTitle} /></MemoryRouter>);

describe('Markdown component', () => {
  it('renders structure, ids and safe links', () => {
    r('# Title\n\n## Why paused {#why}\n\nSee [docs](https://example.com/x) and [alerts](/alerts).\n\n- a\n- b\n');
    expect(screen.getByRole('heading', { level: 1, name: 'Title' })).toBeTruthy();
    expect(document.getElementById('why')?.tagName).toBe('H2');
    const ext = screen.getByRole('link', { name: 'docs' });
    expect(ext.getAttribute('rel')).toBe('noopener noreferrer');
    expect(ext.getAttribute('target')).toBe('_blank');
    expect(screen.getByRole('link', { name: 'alerts' }).getAttribute('href')).toBe('/alerts');
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });
  it('can hide the page title (the page renders its own)', () => {
    r('# Title\n\nBody', true);
    expect(screen.queryByRole('heading', { level: 1 })).toBeNull();
    expect(screen.getByText('Body')).toBeTruthy();
  });
  it('never turns source text into elements: HTML stays visible text, unsafe links become plain text', () => {
    const { container } = r('<script>alert(1)</script> [x](javascript:alert(1))\n\n```\n<img src=x onerror=alert(1)>\n```');
    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent).toContain('<script>alert(1)</script>');
  });
});
