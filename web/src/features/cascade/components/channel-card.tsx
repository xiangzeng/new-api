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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { GripVertical, History, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ChannelRowActionsLayoutContext } from '@/features/channels/components/channel-row-actions-context'
import { ChannelRowActions } from '@/features/channels/components/data-table-row-actions'
import type { Channel } from '@/features/channels/types'
import { cn } from '@/lib/utils'

import { saveCascadeWatermark } from '../api'
import { formatMs } from '../lib/format'
import type { CascadeChannel, CascadeChannelMetrics } from '../types'
import { HealthEventsDialog } from './health-events-dialog'
import { UsedQuotaRow } from './used-quota-row'

const CHANNEL_STATUS_ENABLED = 1

function formatErrorRate(rate: number): string {
  return `${(rate * 100).toFixed(rate >= 0.1 ? 0 : 1)}%`
}

function errorRateClass(rate: number): string {
  if (rate >= 0.2) return 'text-red-600 dark:text-red-400'
  if (rate >= 0.05) return 'text-amber-600 dark:text-amber-400'
  return ''
}

// 近 1h 紧凑指标行；1h/24h 全量明细在「健康时间线」弹窗里看，
// 这里不再挂 tooltip——卡片本就窄，悬停弹窗会糊住上半张卡。无指标数据时不渲染。
function MetricsRow({ metrics }: { metrics?: CascadeChannelMetrics }) {
  const { t } = useTranslation()
  if (!metrics) return null
  const hour = metrics['1h']

  return (
    <div className='text-muted-foreground mt-1.5 text-xs'>
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
    </div>
  )
}

// 负载条配色：接近水位线转黄，打满转红
function watermarkBarClass(ratio: number): string {
  if (ratio >= 1) return 'bg-red-500'
  if (ratio >= 0.7) return 'bg-amber-500'
  return 'bg-emerald-500'
}

// RPM 水位线行：近 60 秒请求数 / 水位线 + 负载条，点开可就地改水位线。
// 达到水位线的渠道会被级联选择器跳过，流量自然溢出到下一个渠道。
function WatermarkRow({ channel }: { channel: CascadeChannel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState('')

  const rpm = channel.rpm ?? 0
  const watermark = channel.rpm_watermark ?? 0
  const ratio = watermark > 0 ? rpm / watermark : 0
  const full = watermark > 0 && rpm >= watermark

  const saveMutation = useMutation({
    mutationFn: async (value: number) => {
      const res = await saveCascadeWatermark([
        { channel_id: channel.id, rpm: value },
      ])
      if (!res.success) throw new Error(res.message || t('Save failed'))
    },
    onSuccess: () => {
      toast.success(t('Watermark saved'))
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
      setOpen(false)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    },
  })

  const submit = () => {
    const parsed = Number.parseInt(draft.trim(), 10)
    saveMutation.mutate(Number.isInteger(parsed) && parsed > 0 ? parsed : 0)
  }

  return (
    // 操作区屏蔽拖拽，避免点开水位线编辑时误触发卡片排序
    <span
      draggable
      onDragStart={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
      className='mt-2 block cursor-default'
    >
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (next) setDraft(watermark > 0 ? String(watermark) : '')
        }}
      >
        <PopoverTrigger
          render={
            <button
              type='button'
              className='hover:bg-accent/50 -mx-1 block w-[calc(100%+0.5rem)] cursor-pointer rounded px-1 py-0.5 text-left'
            />
          }
        >
          <span
            className={cn(
              'text-xs',
              full ? 'text-red-600 dark:text-red-400' : 'text-muted-foreground'
            )}
          >
            {t('RPM')}{' '}
            <span className='font-medium'>
              {rpm}/{watermark > 0 ? watermark : '∞'}
            </span>
          </span>
          {watermark > 0 && (
            <span className='bg-muted mt-1 block h-1 w-full overflow-hidden rounded-full'>
              <span
                className={cn(
                  'block h-full rounded-full transition-all',
                  watermarkBarClass(ratio)
                )}
                style={{ width: `${Math.min(100, ratio * 100)}%` }}
              />
            </span>
          )}
        </PopoverTrigger>
        <PopoverContent align='start' className='w-64'>
          <Label htmlFor={`cascade-watermark-${channel.id}`}>
            {t('RPM watermark')}
          </Label>
          <Input
            id={`cascade-watermark-${channel.id}`}
            inputMode='numeric'
            autoFocus
            value={draft}
            placeholder='0'
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submit()
            }}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Requests in the last 60s; once the watermark is reached traffic spills to the next channel. 0 = unlimited.'
            )}
          </p>
          <div className='flex justify-end gap-2'>
            <Button
              variant='ghost'
              size='sm'
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate(0)}
            >
              {t('Clear')}
            </Button>
            <Button
              size='sm'
              disabled={saveMutation.isPending}
              onClick={submit}
            >
              {t('Save')}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
    </span>
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
    // 卡片内的 tooltip 统一走 delay 0（Base UI 默认 600ms，悬停要空等半秒），
    // 与渠道列表的手感对齐；Provider 不渲染任何 DOM，不影响泳道布局
    <TooltipProvider>
      <div
        draggable
        onDragStart={onDragStart}
        onDragEnter={onDragEnter}
        onDragOver={(e) => e.preventDefault()}
        onDragEnd={onDragEnd}
        className={cn(
          'bg-card w-64 shrink-0 cursor-grab rounded-lg border p-3.5 transition-opacity',
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
        {/* 编排顺序已与渠道优先级解耦，卡片只显示序号圆标与渠道号；
            已使用量与渠道号同行右对齐，省一行高度 */}
        <div className='text-muted-foreground mt-2 flex items-center justify-between gap-2 text-xs'>
          <span>#{channel.id}</span>
          <UsedQuotaRow fullChannel={fullChannel} />
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
        <WatermarkRow channel={channel} />
        <MetricsRow metrics={channel.metrics} />
        {eventsOpen && (
          <HealthEventsDialog
            channelId={channel.id}
            channelName={channel.name}
            metrics={channel.metrics}
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
          className='mt-2 block cursor-default'
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
    </TooltipProvider>
  )
}
