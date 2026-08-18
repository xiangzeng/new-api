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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Spinner } from '@/components/ui/spinner'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useChannels } from '@/features/channels/components/channels-provider'
import { useChannelRecentUsageQuery } from '@/features/channels/lib'
import type { Channel } from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'

/** 超过这个字符数的金额转紧凑记数（如 $28万），精确值留给 tooltip */
const MAX_INLINE_USED_CHARS = 10
const SENSITIVE_MASK = '••••'

// 卡片「已使用量」：与渠道号同行右对齐，口径与渠道列表的已用量徽标一致。
// 只有金额本身是悬停触发区（标签与周围空白不触发），且悬停弹窗不可停留——
// 鼠标一离开金额就收起，不挡住卡片上的其他操作。
export function UsedQuotaRow({ fullChannel }: { fullChannel?: Channel }) {
  const { t, i18n } = useTranslation()
  const { sensitiveVisible } = useChannels()
  // 悬停即预取，比 tooltip 打开早一步，避免弹出后还要空转一次 loading
  const [hovered, setHovered] = useState(false)

  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
  const withSuffix = (value: string) =>
    tokenSuffix && value !== '-' ? `${value}${tokenSuffix}` : value
  const formatAmount = (value: number) =>
    withSuffix(
      formatQuotaWithCurrency(value, {
        digitsLarge: 2,
        digitsSmall: 4,
        abbreviate: true,
      })
    )

  const recentUsageQuery = useChannelRecentUsageQuery(fullChannel?.id ?? 0, {
    enabled: hovered && sensitiveVisible && Boolean(fullChannel),
  })

  // 渠道全量数据还没到（编排 overview 比渠道列表快），这一格先留空
  if (!fullChannel) return null

  const usedQuota = fullChannel.used_quota || 0
  const usedFull = formatAmount(usedQuota)
  const usedDisplay =
    usedFull.length > MAX_INLINE_USED_CHARS
      ? withSuffix(
          formatQuotaWithCurrency(usedQuota, { compact: true, locale })
        )
      : usedFull

  let recentContent = (
    <div className='space-y-1'>
      {(recentUsageQuery.data ?? []).map((item) => (
        <div
          key={item.date}
          className='flex items-center justify-between gap-4 text-xs'
        >
          <span className='text-background/70 flex items-center gap-1'>
            <span>{t(item.labelKey)}</span>
            <span className='font-mono'>{item.date}</span>
          </span>
          <span className='font-medium'>{formatAmount(item.quota_used)}</span>
        </div>
      ))}
    </div>
  )
  if (recentUsageQuery.isLoading) {
    recentContent = (
      <div className='flex items-center justify-center py-2'>
        <Spinner className='text-background size-4' />
      </div>
    )
  } else if (recentUsageQuery.isError) {
    recentContent = (
      <p className='text-background/70 py-1 text-xs'>
        {recentUsageQuery.error instanceof Error
          ? recentUsageQuery.error.message
          : t('Failed to load recent usage')}
      </p>
    )
  }

  return (
    <span className='flex min-w-0 items-center gap-1'>
      <span className='shrink-0'>{t('Used')}</span>
      <Tooltip disableHoverablePopup>
        <TooltipTrigger
          render={
            <span
              className='text-foreground cursor-help truncate font-medium'
              onMouseEnter={() => setHovered(true)}
              onFocus={() => setHovered(true)}
            />
          }
        >
          {sensitiveVisible ? usedDisplay : SENSITIVE_MASK}
        </TooltipTrigger>
        <TooltipContent className='min-w-56 items-stretch'>
          {sensitiveVisible ? (
            <div className='w-full space-y-2'>
              <p className='font-medium'>
                {t('Used:')} {usedFull}
              </p>
              <div className='border-background/20 space-y-1 border-t pt-2'>
                <p className='text-background/70 text-[11px] font-medium'>
                  {t('Recent 3 days usage')}
                </p>
                {recentContent}
              </div>
            </div>
          ) : (
            <p>
              {t('Used:')} {SENSITIVE_MASK}
            </p>
          )}
        </TooltipContent>
      </Tooltip>
    </span>
  )
}
