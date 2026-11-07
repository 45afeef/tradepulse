import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { RobotsOverrideModal } from './RobotsOverride';

describe('RobotsOverrideModal', () => {
  it('needs the acknowledgement and a real reason before it can be applied, and trims the reason', () => {
    const onApply = vi.fn();
    render(<RobotsOverrideModal onApply={onApply} onClose={() => {}} />);
    const apply = screen.getByRole('button', { name: /override for this source/i });
    expect(apply.hasAttribute('disabled')).toBe(true);
    fireEvent.change(screen.getByLabelText(/why is this acceptable/i), { target: { value: '  Permission from the publisher  ' } });
    expect(apply.hasAttribute('disabled')).toBe(true); // reason alone is not enough
    fireEvent.click(screen.getByRole('checkbox'));
    expect(apply.hasAttribute('disabled')).toBe(false);
    fireEvent.click(apply);
    expect(onApply).toHaveBeenCalledWith('Permission from the publisher');
  });
  it('refuses a too-short reason even when acknowledged', () => {
    render(<RobotsOverrideModal onApply={() => {}} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText(/why is this acceptable/i), { target: { value: 'because' } });
    fireEvent.click(screen.getByRole('checkbox'));
    expect(screen.getByRole('button', { name: /override for this source/i }).hasAttribute('disabled')).toBe(true);
  });
});
