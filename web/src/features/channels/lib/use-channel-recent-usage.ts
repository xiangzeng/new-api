/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getChannelDailyUsage } from '../api'
import {
  buildRecentUsageDateRange,
  mergeRecentUsageDays,
  type RecentUsageDisplayRow,
} from './channel-recent-usage'

/**
 * Lazy "recent days usage" query shared by the channel list balance cell and the
 * cascade channel card. Both call sites use the same query key, so hovering one
 * warms the other; the request only fires while a tooltip is open.
 */
export function useChannelRecentUsageQuery(
  channelId: number,
  options: { enabled: boolean; dayCount?: number }
) {
  const { t } = useTranslation()
  const range = buildRecentUsageDateRange(new Date(), options.dayCount ?? 3)

  return useQuery<RecentUsageDisplayRow[]>({
    queryKey: [
      'channels',
      'daily-usage',
      channelId,
      range.startDate,
      range.endDate,
    ],
    enabled: options.enabled,
    staleTime: 60_000,
    queryFn: async () => {
      const res = await getChannelDailyUsage(channelId, {
        start_date: range.startDate,
        end_date: range.endDate,
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to load recent usage'))
      }
      return mergeRecentUsageDays(range.days, res.data)
    },
  })
}
