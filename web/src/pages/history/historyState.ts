import type { UserRole } from '../../types/auth';
import type { HistoryContributionStatus } from '../../types/history';

export const historyContributionStatusText: Record<HistoryContributionStatus, string> = {
  draft: '草稿',
  pending: '审核中',
  returned: '已退回',
  approved: '已通过',
  rejected: '已驳回',
};

export const historyContributionStatusColor: Record<HistoryContributionStatus, string | undefined> =
  {
    draft: undefined,
    pending: 'processing',
    returned: 'warning',
    approved: 'success',
    rejected: 'error',
  };

export const canContributeToHistory = (role: UserRole | undefined) => role === 'alumni';
