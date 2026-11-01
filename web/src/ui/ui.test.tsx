import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { FeedCard } from '../components/FeedCard';
import { ChipInput, Modal, Tabs } from './index';
import type { FeedItem } from '../api/types';

const base: FeedItem = { id: 'i1', cursor: 'c1_1', event_id: 'e', event_type: 't', source: 'cnbc', entities: ['oil'], title: 'Oil jumps', body: 'Brent rose', url: 'https://example.com/a', severity: 'notable', reason: 'Matched your source subscription "cnbc"', created_at: '2026-10-30T11:00:00Z' };

describe('FeedCard', () => {
  it('links out safely and explains why on demand', () => {
    render(<FeedCard item={base} now={Date.parse('2026-10-30T12:00:00Z')} onRate={() => {}} />);
    const link = screen.getByRole('link', { name: 'Oil jumps' });
    expect(link.getAttribute('rel')).toContain('noopener'); expect(link.getAttribute('target')).toBe('_blank');
    expect(screen.queryByText(/Matched your source/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /why this/i }));
    expect(screen.getByText(/Matched your source/)).toBeTruthy();
  });
  it('never renders a javascript: link', () => {
    render(<FeedCard item={{ ...base, url: 'javascript:alert(1)' }} now={0} onRate={() => {}} />);
    expect(screen.queryByRole('link', { name: 'Oil jumps' })).toBeNull();
    expect(screen.getByText('Oil jumps')).toBeTruthy();
  });
  it('rates and un-rates', () => {
    const onRate = vi.fn();
    const { rerender } = render(<FeedCard item={base} now={0} onRate={onRate} />);
    fireEvent.click(screen.getByRole('button', { name: /mark as useful/i }));
    expect(onRate).toHaveBeenCalledWith('i1', 'useful');
    rerender(<FeedCard item={{ ...base, feedback: 'useful' }} now={0} onRate={onRate} />);
    expect(screen.getByRole('button', { name: /mark as useful/i }).getAttribute('aria-pressed')).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: /mark as useful/i }));
    expect(onRate).toHaveBeenLastCalledWith('i1', null);
    fireEvent.click(screen.getByRole('button', { name: /mark as not useful/i }));
    expect(onRate).toHaveBeenLastCalledWith('i1', 'not_useful');
  });
});

describe('ChipInput', () => {
  it('adds on Enter and on comma, de-duplicating case-insensitively', () => {
    const onChange = vi.fn();
    render(<ChipInput label="Topics" value={['Oil']} onChange={onChange} />);
    const input = screen.getByLabelText('Topics');
    fireEvent.change(input, { target: { value: 'rates' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenLastCalledWith(['Oil', 'rates']);
    fireEvent.change(input, { target: { value: 'oil, gold,' } });
    expect(onChange).toHaveBeenLastCalledWith(['Oil', 'gold']);
  });
  it('removes the last chip on Backspace and respects the maximum', () => {
    const onChange = vi.fn();
    render(<ChipInput label="Topics" value={['a', 'b']} onChange={onChange} max={2} />);
    const input = screen.getByLabelText('Topics');
    fireEvent.keyDown(input, { key: 'Backspace' });
    expect(onChange).toHaveBeenLastCalledWith(['a']);
    fireEvent.change(input, { target: { value: 'c' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenLastCalledWith(['a', 'b']);
  });
});

describe('Modal', () => {
  it('is an accessible dialog that closes on Escape and on backdrop click', () => {
    const onClose = vi.fn();
    render(<Modal title="Hello" onClose={onClose}><button>Inside</button></Modal>);
    const dialog = screen.getByRole('dialog');
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(screen.getByRole('heading', { name: 'Hello' })).toBeTruthy();
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.mouseDown(dialog.parentElement as HTMLElement);
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

describe('Tabs', () => {
  it('moves with the arrow keys', () => {
    const onChange = vi.fn();
    render(<Tabs value="a" onChange={onChange} items={[{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }]} />);
    fireEvent.keyDown(screen.getByRole('tablist'), { key: 'ArrowRight' });
    expect(onChange).toHaveBeenCalledWith('b');
    expect(screen.getByRole('tab', { name: 'A' }).getAttribute('aria-selected')).toBe('true');
  });
});
