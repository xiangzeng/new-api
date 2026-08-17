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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import { getCascadeHealthEvents } from '../api'
import type { CascadeHealthEventType } from '../types'

const EVENT_STYLES: Record<
  CascadeHealthEventType,
  { dot: string; labelKey: string }
> = {
  trip: { dot: 'bg-red-500', labelKey: 'Tripped' },
  restore: { dot: 'bg-emerald-500', labelKey: 'Restored' },
  manual_reset: { dot: 'bg-blue-500', labelKey: 'Manual restore' },
}

function formatEventTime(ts: number): string {
  const d = new Date(ts * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

type HealthEventsDialogProps = {
  channelId: number
  channelName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// 渠道健康事件时间线弹窗：近 7d 事件一次拉全，24h/7d 切换为纯客户端过滤
export function HealthEventsDialog({
  channelId,
  channelName,
  open,
  onOpenChange,
}: HealthEventsDialogProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState<'24h' | '7d'>('24h')

  const { data, isLoading } = useQuery({
    queryKey: ['cascade-health-events', channelId],
    queryFn: () => getCascadeHealthEvents(channelId),
    enabled: open,
    staleTime: 30_000,
  })

  const events = data?.data?.events ?? []
  const dayStart = Date.now() / 1000 - 86400
  const shown =
    range === '24h' ? events.filter((e) => e.created_at >= dayStart) : events

  // 三态渲染拆成早返回，避免嵌套三元（oxlint no-nested-ternary）
  const renderTimeline = () => {
    if (isLoading) {
      return (
        <div className='flex justify-center py-8'>
          <Spinner />
        </div>
      )
    }
    if (shown.length === 0) {
      return (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('No events in this range')}
        </div>
      )
    }
    return (
      <div className='max-h-80 overflow-y-auto'>
        {shown.map((event) => {
          const style = EVENT_STYLES[event.event]
          return (
            <div
              key={event.id}
              className='flex items-start gap-2 border-b py-1.5 text-xs last:border-b-0'
            >
              <span
                className={cn('mt-1 size-2 shrink-0 rounded-full', style.dot)}
              />
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  <span className='font-medium'>{t(style.labelKey)}</span>
                  <span className='text-muted-foreground'>
                    {formatEventTime(event.created_at)}
                  </span>
                </div>
                {event.reason && (
                  <div className='text-muted-foreground break-all'>
                    {event.reason}
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle className='truncate'>
            {t('Health Timeline')} · #{channelId} {channelName}
          </DialogTitle>
        </DialogHeader>
        <div className='text-muted-foreground flex items-center justify-between gap-2 text-xs'>
          <span>
            {t('{{count}} trips in last 24h', {
              count: data?.data?.trip_count_24h ?? 0,
            })}
            {' · '}
            {t('{{count}} trips in last 7d', {
              count: data?.data?.trip_count_7d ?? 0,
            })}
          </span>
          <Tabs
            value={range}
            onValueChange={(v) => setRange(v as '24h' | '7d')}
          >
            <TabsList className='h-7'>
              <TabsTrigger value='24h' className='h-5 px-2 text-xs'>
                {t('Last 24h')}
              </TabsTrigger>
              <TabsTrigger value='7d' className='h-5 px-2 text-xs'>
                {t('Last 7d')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        {renderTimeline()}
      </DialogContent>
    </Dialog>
  )
}
