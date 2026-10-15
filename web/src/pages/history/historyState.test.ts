import { describe, expect, it } from 'vitest';
import {
  canContributeToHistory,
  historyContributionStatusColor,
  historyContributionStatusText,
} from './historyState';

describe('history contribution state mapping', () => {
  it('maps every private contribution state to a visible label and status color', () => {
    expect(historyContributionStatusText).toEqual({
      draft: '草稿',
      pending: '审核中',
      returned: '已退回',
      approved: '已通过',
      rejected: '已驳回',
    });
    expect(historyContributionStatusColor).toEqual({
      draft: undefined,
      pending: 'processing',
      returned: 'warning',
      approved: 'success',
      rejected: 'error',
    });
  });

  it('allows only alumni users to create and view their own contributions', () => {
    expect(canContributeToHistory('alumni')).toBe(true);
    expect(canContributeToHistory('admin')).toBe(false);
    expect(canContributeToHistory('super_admin')).toBe(false);
    expect(canContributeToHistory(undefined)).toBe(false);
  });
});
