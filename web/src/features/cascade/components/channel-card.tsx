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
import { GripVertical, History, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ChannelRowActionsLayoutContext } from '@/features/channels/components/channel-row-actions-context'
import { ChannelRowActions } from '@/features/channels/components/data-table-row-actions'
import type { Channel } from '@/features/channels/types'
import { cn } from '@/lib/utils'

import type {
  CascadeChannel,
  CascadeChannelMetrics,
  CascadeChannelMetricsWindow,
} from '../types'
import { HealthEventsDialog } from './health-events-dialog'

const CHANNEL_STATUS_ENABLED = 1

function formatMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${ms}ms`
}

function formatErrorRate(rate: number): string {
  return `${(rate * 100).toFixed(rate >= 0.1 ? 0 : 1)}%`
}

function errorRateClass(rate: number): string {
  if (rate >= 0.2) return 'text-red-600 dark:text-red-400'
  if (rate >= 0.05) return 'text-amber-600 dark:text-amber-400'
  return ''
}

function MetricsDetailLine({
  label,
  window,
}: {
  label: string
  window: CascadeChannelMetricsWindow
}) {
  const { t } = useTranslation()
  return (
    <div>
      <span className='font-medium'>{label}</span>
      {': '}
      {t('Attempts')} {window.attempts} · {t('Faults')} {window.faults} ·{' '}
      {t('Trips')} {window.trips} · {t('Restores')} {window.restores}
      {window.avg_latency_ms > 0 &&
        ` · ${t('Avg latency')} ${formatMs(window.avg_latency_ms)}`}
      {window.avg_ttft_ms > 0 &&
        ` · ${t('Avg TTFT')} ${formatMs(window.avg_ttft_ms)}`}
    </div>
  )
}

// 近 1h 紧凑指标行；hover 出 1h/24h 全量明细。无任何指标数据时不渲染。
function MetricsRow({ metrics }: { metrics?: CascadeChannelMetrics }) {
  const { t } = useTranslation()
  if (!metrics) return null
  const hour = metrics['1h']

  return (
    <Tooltip>
      <TooltipTrigger
        render={<div className='text-muted-foreground mt-1 text-xs' />}
      >
        {hour.attempts === 0 && hour.trips === 0 && hour.restores === 0 ? (
          <span>{t('No traffic in last hour')}</span>
        ) : (
          <span>
            {t('Err')}{' '}
            <span className={errorRateClass(hour.error_rate)}>
              {formatErrorRate(hour.error_rate)}
            </span>
            {hour.trips > 0 && ` · ${t('Trips')} ${hour.trips}`}
            {hour.avg_ttft_ms > 0 &&
              ` · ${t('TTFT')} ${formatMs(hour.avg_ttft_ms)}`}
            {hour.avg_ttft_ms === 0 &&
              hour.avg_latency_ms > 0 &&
              ` · ${t('Latency')} ${formatMs(hour.avg_latency_ms)}`}
          </span>
        )}
      </TooltipTrigger>
      <TooltipContent className='max-w-sm space-y-1'>
        <MetricsDetailLine label={t('Last 1h')} window={metrics['1h']} />
        <MetricsDetailLine label={t('Last 24h')} window={metrics['24h']} />
      </TooltipContent>
    </Tooltip>
  )
}

type ChannelCardProps = {
  channel: CascadeChannel
  fullChannel?: Channel
  index: number
  recoveryTarget: number
  isDragging: boolean
  onDragStart: () => void
  onDragEnter: () => void
  onDragEnd: () => void
  onRestore: (channelId: number) => void
  restorePending: boolean
}

function HealthBadge({
  channel,
  recoveryTarget,
}: {
  channel: CascadeChannel
  recoveryTarget: number
}) {
  const { t } = useTranslation()

  if (channel.status !== CHANNEL_STATUS_ENABLED) {
    return (
      <Badge variant='outline' className='text-muted-foreground'>
        {t('Disabled')}
      </Badge>
    )
  }
  const health = channel.health
  if (health?.state === 'cooling') {
    return (
      <Badge
        variant='outline'
        className='border-red-500/40 text-red-600 dark:text-red-400'
      >
        {t('Cooling')}
        {typeof health.cooldown_remaining === 'number'
          ? ` · ${health.cooldown_remaining}s`
          : ''}
      </Badge>
    )
  }
  if (health?.state === 'probing') {
    return (
      <Badge
        variant='outline'
        className='border-amber-500/40 text-amber-600 dark:text-amber-400'
      >
        {t('Recovering')} {health.recovery_successes}/{recoveryTarget}
      </Badge>
    )
  }
  return (
    <Badge
      variant='outline'
      className='border-emerald-500/40 text-emerald-600 dark:text-emerald-400'
    >
      {t('Healthy')}
    </Badge>
  )
}

export function ChannelCard({
  channel,
  fullChannel,
  index,
  recoveryTarget,
  isDragging,
  onDragStart,
  onDragEnter,
  onDragEnd,
  onRestore,
  restorePending,
}: ChannelCardProps) {
  const { t } = useTranslation()
  const [eventsOpen, setEventsOpen] = useState(false)
  const tripped =
    channel.health?.state === 'cooling' || channel.health?.state === 'probing'

  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragEnter={onDragEnter}
      onDragOver={(e) => e.preventDefault()}
      onDragEnd={onDragEnd}
      className={cn(
        'bg-card w-56 shrink-0 cursor-grab rounded-lg border p-3 transition-opacity',
        isDragging && 'opacity-40',
        tripped && 'border-red-500/40',
        channel.status !== CHANNEL_STATUS_ENABLED && 'opacity-60'
      )}
    >
      <div className='flex items-center gap-2'>
        <span className='bg-primary text-primary-foreground flex size-5 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
          {index + 1}
        </span>
        <Tooltip>
          <TooltipTrigger
            render={
              <span className='min-w-0 flex-1 truncate text-sm font-medium' />
            }
          >
            {channel.name}
          </TooltipTrigger>
          <TooltipContent>
            #{channel.id} {channel.name}
          </TooltipContent>
        </Tooltip>
        <GripVertical className='text-muted-foreground size-4 shrink-0' />
      </div>
      {/* 编排顺序已与渠道优先级解耦，卡片只显示序号圆标与渠道号 */}
      <div className='text-muted-foreground mt-2 flex items-center gap-2 text-xs'>
        <span>#{channel.id}</span>
      </div>
      <div className='mt-2 flex items-center justify-between gap-2'>
        <HealthBadge channel={channel} recoveryTarget={recoveryTarget} />
        <div className='flex shrink-0 items-center'>
          {tripped && (
            <Button
              variant='ghost'
              size='sm'
              className='h-6 px-2 text-xs'
              disabled={restorePending}
              onClick={() => onRestore(channel.id)}
            >
              <RotateCcw className='size-3' />
              {t('Restore Now')}
            </Button>
          )}
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='sm'
                  className='size-6 p-0'
                  onClick={() => setEventsOpen(true)}
                />
              }
            >
              <History className='size-3' />
            </TooltipTrigger>
            <TooltipContent>{t('Health Timeline')}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <MetricsRow metrics={channel.metrics} />
      {eventsOpen && (
        <HealthEventsDialog
          channelId={channel.id}
          channelName={channel.name}
          open={eventsOpen}
          onOpenChange={setEventsOpen}
        />
      )}
      {/* 操作区屏蔽拖拽，避免误触发卡片排序 */}
      <span
        draggable
        onDragStart={(e) => {
          e.preventDefault()
          e.stopPropagation()
        }}
        className='mt-1 block cursor-default'
      >
        {fullChannel ? (
          <ChannelRowActionsLayoutContext.Provider value='card'>
            <ChannelRowActions channel={fullChannel} />
          </ChannelRowActionsLayoutContext.Provider>
        ) : (
          <span className='block h-8' />
        )}
      </span>
      {(channel.health?.consecutive_failures ?? 0) > 0 && (
        <div className='text-muted-foreground mt-1 text-xs'>
          {t('Consecutive failures')}: {channel.health?.consecutive_failures}
        </div>
      )}
      {channel.health?.last_error && (
        <Tooltip>
          <TooltipTrigger
            render={
              <div className='text-muted-foreground mt-1 truncate text-xs' />
            }
          >
            {channel.health.last_error}
          </TooltipTrigger>
          <TooltipContent className='max-w-sm break-all'>
            {channel.health.last_error}
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  )
}
