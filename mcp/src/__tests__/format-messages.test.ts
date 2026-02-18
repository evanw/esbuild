import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';

describe('esbuild_format_messages', () => {
  it('formats error messages', async () => {
    const messages: esbuild.Message[] = [{
      text: 'test error',
      location: null,
      notes: [],
      id: '',
      pluginName: '',
      detail: undefined,
    }];
    const formatted = await esbuild.formatMessages(messages, { kind: 'error' });
    expect(formatted.length).toBeGreaterThan(0);
    expect(formatted[0]).toContain('test error');
  });

  it('formats warning messages', async () => {
    const messages: esbuild.Message[] = [{
      text: 'test warning',
      location: null,
      notes: [],
      id: '',
      pluginName: '',
      detail: undefined,
    }];
    const formatted = await esbuild.formatMessages(messages, { kind: 'warning' });
    expect(formatted.length).toBeGreaterThan(0);
  });

  it('handles empty message array', async () => {
    const formatted = await esbuild.formatMessages([], { kind: 'error' });
    expect(formatted).toEqual([]);
  });
});
