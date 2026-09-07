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
import { Link } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import dayjs from '@/lib/dayjs'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import { getChannelUserUsage } from '../../api'
import type { ChannelUserUsage } from '../../types'
import { useChannels } from '../channels-provider'

type ChannelUserUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** 时间预设：整天取边界，与卡片「最近 N 天消耗」的分日口径一致 */
const RANGE_PRESETS = [
  { key: 'today', days: 1, labelKey: 'Today' },
  { key: '3d', days: 3, labelKey: 'Last 3 days' },
  { key: '7d', days: 7, labelKey: 'Last 7 days' },
  { key: '30d', days: 30, labelKey: 'Last 30 days' },
] as const

type RangePresetKey = (typeof RANGE_PRESETS)[number]['key']

/** [start, end] 为 unix 秒，含今天在内共 days 个自然日 */
function buildPresetRange(days: number, now: Date = new Date()) {
  const today = dayjs(now)
  return {
    start: today
      .subtract(days - 1, 'day')
      .startOf('day')
      .unix(),
    end: today.endOf('day').unix(),
  }
}

type RankedRow = ChannelUserUsage & {
  rank: number
  /** 该用户占本渠道区间总额度的比例（0-1） */
  share: number
}

// 「这条渠道的钱是谁花的」：按用户汇总的消耗排名，点用户名跳到该用户的用量详情页
// 并预设「按模型 × 本渠道」筛选，接着就能看到 TA 在这条渠道上跑的是什么模型。
export function ChannelUserUsageDialog({
  open,
  onOpenChange,
}: ChannelUserUsageDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const [preset, setPreset] = useState<RangePresetKey>('3d')

  const presetDays =
    RANGE_PRESETS.find((item) => item.key === preset)?.days ?? 3
  // 边界只随预设变化；对话框重开时按当前时刻重算
  const range = useMemo(
    () => buildPresetRange(presetDays),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [presetDays, open]
  )

  const channelId = currentRow?.id ?? 0
  const usageQuery = useQuery({
    queryKey: ['channels', 'user-usage', channelId, range.start, range.end],
    enabled: open && channelId > 0,
    staleTime: 60_000,
    queryFn: async () => {
      const res = await getChannelUserUsage(channelId, {
        start_timestamp: range.start,
        end_timestamp: range.end,
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to load usage data'))
      }
      return res.data ?? []
    },
  })

  const rows = useMemo<RankedRow[]>(() => {
    const data = usageQuery.data ?? []
    const totalQuota = data.reduce((sum, row) => sum + row.quota, 0)
    return data.map((row, index) => ({
      ...row,
      rank: index + 1,
      share: totalQuota > 0 ? row.quota / totalQuota : 0,
    }))
  }, [usageQuery.data])

  const totals = useMemo(
    () =>
      rows.reduce(
        (acc, row) => ({
          quota: acc.quota + row.quota,
          count: acc.count + row.count,
          tokens: acc.tokens + row.token_used,
        }),
        { quota: 0, count: 0, tokens: 0 }
      ),
    [rows]
  )

  if (!currentRow) return null

  const columns = [
    {
      id: 'rank',
      className: 'w-[56px]',
      cellClassName: 'text-muted-foreground tabular-nums',
      header: t('Rank'),
      cell: (row: RankedRow) => row.rank,
    },
    {
      id: 'user',
      header: t('User'),
      cell: (row: RankedRow) => (
        <Link
          to='/users/$userId'
          params={{ userId: String(row.user_id) }}
          search={{
            start: range.start,
            end: range.end,
            dim: 'model',
            channel: String(currentRow.id),
          }}
          title={t('Usage details')}
          className='hover:text-primary font-medium break-all transition-colors hover:underline'
        >
          {row.username || `#${row.user_id}`}
          <span className='text-muted-foreground ml-1 text-xs font-normal'>
            #{row.user_id}
          </span>
        </Link>
      ),
    },
    {
      id: 'quota',
      className: 'w-[120px] text-right',
      cellClassName: 'text-right font-semibold tabular-nums',
      header: <span className='block text-right'>{t('Used Quota')}</span>,
      cell: (row: RankedRow) => formatQuota(row.quota),
    },
    {
      id: 'share',
      className: 'w-[140px]',
      header: t('Share'),
      cell: (row: RankedRow) => (
        <div className='flex items-center gap-2'>
          <span className='bg-muted block h-1.5 w-16 shrink-0 overflow-hidden rounded-full'>
            <span
              className='bg-primary block h-full rounded-full'
              style={{ width: `${Math.min(100, row.share * 100)}%` }}
            />
          </span>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {`${Math.round(row.share * 1000) / 10}%`}
          </span>
        </div>
      ),
    },
    {
      id: 'count',
      className: 'w-[90px] text-right',
      cellClassName: 'text-muted-foreground text-right tabular-nums',
      header: <span className='block text-right'>{t('Requests')}</span>,
      cell: (row: RankedRow) => formatCompactNumber(row.count),
    },
    {
      id: 'tokens',
      className: 'w-[90px] text-right',
      cellClassName: 'text-muted-foreground text-right tabular-nums',
      header: <span className='block text-right'>{t('Tokens')}</span>,
      cell: (row: RankedRow) => formatCompactNumber(row.token_used),
    },
  ]

  let body = (
    <StaticDataTable
      columns={columns}
      data={rows}
      getRowKey={(row) => row.user_id}
      emptyContent={t('No usage data in the selected period')}
    />
  )
  if (usageQuery.isLoading) {
    body = (
      <div className='flex items-center justify-center rounded-lg border py-10'>
        <Loader2 className='text-muted-foreground size-5 animate-spin' />
      </div>
    )
  } else if (usageQuery.isError) {
    body = (
      <p className='text-destructive rounded-lg border py-6 text-center text-sm'>
        {usageQuery.error instanceof Error
          ? usageQuery.error.message
          : t('Failed to load usage data')}
      </p>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User usage ranking')}
      description={
        <>
          <strong>{currentRow.name}</strong> #{currentRow.id} ·{' '}
          {t(
            'Users ranked by consumption on this channel in the selected period; click a user to open their usage details filtered to this channel.'
          )}
        </>
      }
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-3'
      footer={
        <Button variant='outline' onClick={() => onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <Tabs
          value={preset}
          onValueChange={(value) => setPreset(value as RangePresetKey)}
        >
          <TabsList>
            {RANGE_PRESETS.map((item) => (
              <TabsTrigger key={item.key} value={item.key}>
                {t(item.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {dayjs.unix(range.start).format('YYYY-MM-DD')} ~{' '}
          {dayjs.unix(range.end).format('YYYY-MM-DD')}
        </span>
      </div>

      {body}

      {rows.length > 0 && (
        <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-1 text-xs'>
          <span>
            {t('Total')} · {t('Users')} {rows.length}
          </span>
          <span className='tabular-nums'>
            {t('Used Quota')}{' '}
            <span className='text-foreground font-semibold'>
              {formatQuota(totals.quota)}
            </span>
            {' · '}
            {t('Requests')} {formatCompactNumber(totals.count)}
            {' · '}
            {t('Tokens')} {formatCompactNumber(totals.tokens)}
          </span>
        </div>
      )}
    </Dialog>
  )
}
