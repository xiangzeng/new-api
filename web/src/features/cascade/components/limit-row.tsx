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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useChannels } from '@/features/channels/components/channels-provider'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'
import { cn } from '@/lib/utils'

import { saveCascadeWatermark } from '../api'
import {
  type CascadeLimitMode,
  getCascadeLimitMode,
  isDailyQuotaReached,
} from '../lib/limits'
import type { CascadeChannel } from '../types'

const SENSITIVE_MASK = '••••'

// 负载条配色：接近上限转黄，打满转红
function loadBarClass(ratio: number): string {
  if (ratio >= 1) return 'bg-red-500'
  if (ratio >= 0.7) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function LoadBar({ ratio }: { ratio: number }) {
  return (
    <span className='bg-muted mt-1 block h-1 w-full overflow-hidden rounded-full'>
      <span
        className={cn(
          'block h-full rounded-full transition-all',
          loadBarClass(ratio)
        )}
        style={{ width: `${Math.min(100, ratio * 100)}%` }}
      />
    </span>
  )
}

/** 额度回填到输入框：按显示货币换算，保留两位小数 */
function quotaToDraft(quota: number): string {
  return String(Math.round(quotaUnitsToDollars(quota) * 100) / 100)
}

// 限流行：RPM 水位线与每日额度上限二选一，点开可就地切换方式并修改数值。
// 水位线模式：本分组近 60 秒请求数 / 本分组水位线，达到后本分组流量溢出到下一个渠道
// （软限流；同一渠道在各分组独立设置、独立计数）；
// 额度模式：今日累计消耗 / 上限，满额后该渠道等同禁用到次日 0 点（硬限流，渠道级、各分组共用）。
export function LimitRow({
  channel,
  group,
}: {
  channel: CascadeChannel
  group: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { sensitiveVisible } = useChannels()
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<CascadeLimitMode>('rpm')
  const [draft, setDraft] = useState('')

  const currentMode = getCascadeLimitMode(channel)
  const rpm = channel.rpm ?? 0
  const rpmTotal = channel.rpm_total ?? 0
  const watermark = channel.rpm_watermark ?? 0
  const quotaCap = channel.daily_quota_cap ?? 0
  const todayUsed = channel.today_used_quota ?? 0
  const rpmFull = watermark > 0 && rpm >= watermark
  const quotaReached = isDailyQuotaReached(channel)

  const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
  const formatAmount = (value: number) => {
    if (!sensitiveVisible) return SENSITIVE_MASK
    const amount = formatQuotaWithCurrency(value, {
      digitsLarge: 2,
      digitsSmall: 2,
      abbreviate: true,
    })
    return tokenSuffix && amount !== '-' ? `${amount}${tokenSuffix}` : amount
  }

  const draftFor = (next: CascadeLimitMode) => {
    if (next === 'daily_quota') {
      return quotaCap > 0 ? quotaToDraft(quotaCap) : ''
    }
    return watermark > 0 ? String(watermark) : ''
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      mode: CascadeLimitMode
      rpm: number
      dailyQuota: number
    }) => {
      const res = await saveCascadeWatermark([
        {
          channel_id: channel.id,
          group,
          rpm: payload.rpm,
          daily_quota: payload.dailyQuota,
        },
      ])
      if (!res.success) throw new Error(res.message || t('Save failed'))
      return payload.mode
    },
    onSuccess: (savedMode) => {
      toast.success(
        savedMode === 'daily_quota'
          ? t('Daily quota cap saved')
          : t('Watermark saved')
      )
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
      setOpen(false)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    },
  })

  const submit = () => {
    const raw = draft.trim()
    if (mode === 'daily_quota') {
      const amount = Number.parseFloat(raw)
      saveMutation.mutate({
        mode,
        rpm: 0,
        dailyQuota:
          Number.isFinite(amount) && amount > 0
            ? parseQuotaFromDollars(amount)
            : 0,
      })
      return
    }
    const parsed = Number.parseInt(raw, 10)
    saveMutation.mutate({
      mode,
      rpm: Number.isInteger(parsed) && parsed > 0 ? parsed : 0,
      dailyQuota: 0,
    })
  }

  return (
    // 操作区屏蔽拖拽，避免点开限流编辑时误触发卡片排序
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
          if (next) {
            setMode(currentMode)
            setDraft(draftFor(currentMode))
          }
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
          {currentMode === 'daily_quota' ? (
            <>
              <span className='flex items-center justify-between gap-2 text-xs'>
                <span
                  className={cn(
                    'truncate',
                    quotaReached
                      ? 'text-red-600 dark:text-red-400'
                      : 'text-muted-foreground'
                  )}
                >
                  {t('Today')}{' '}
                  <span className='font-medium'>
                    {formatAmount(todayUsed)}/{formatAmount(quotaCap)}
                  </span>
                </span>
                <span className='text-muted-foreground shrink-0'>
                  {t('RPM')} {rpm}
                </span>
              </span>
              <LoadBar ratio={todayUsed / quotaCap} />
            </>
          ) : (
            <>
              <span
                className={cn(
                  'text-xs',
                  rpmFull
                    ? 'text-red-600 dark:text-red-400'
                    : 'text-muted-foreground'
                )}
              >
                {t('RPM')}{' '}
                <span className='font-medium'>
                  {rpm}/{watermark > 0 ? watermark : '∞'}
                </span>
              </span>
              {watermark > 0 && <LoadBar ratio={rpm / watermark} />}
            </>
          )}
        </PopoverTrigger>
        <PopoverContent align='start' className='w-72'>
          <ToggleGroup
            value={[mode]}
            onValueChange={(value) => {
              const next = value.find((item) => item !== mode)
              if (next === 'rpm' || next === 'daily_quota') {
                setMode(next)
                setDraft(draftFor(next))
              }
            }}
            aria-label={t('Limit mode')}
            variant='outline'
            size='sm'
            className='grid w-full grid-cols-2'
          >
            <ToggleGroupItem value='rpm'>{t('RPM watermark')}</ToggleGroupItem>
            <ToggleGroupItem value='daily_quota'>
              {t('Daily quota cap')}
            </ToggleGroupItem>
          </ToggleGroup>
          <Label htmlFor={`cascade-limit-${channel.id}`}>
            {mode === 'daily_quota'
              ? `${t('Daily quota cap')} (${getCurrencyLabel()})`
              : t('RPM watermark')}
          </Label>
          <Input
            id={`cascade-limit-${channel.id}`}
            inputMode={mode === 'daily_quota' ? 'decimal' : 'numeric'}
            autoFocus
            value={draft}
            placeholder='0'
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submit()
            }}
          />
          <p className='text-muted-foreground text-xs'>
            {mode === 'daily_quota'
              ? t(
                  'Spend since 00:00 today (server time), same basis as Used. Once the cap is reached the channel is skipped like a disabled one until tomorrow. 0 = unlimited.'
                )
              : t(
                  "Requests from this group in the last 60s; once the watermark is reached this group's traffic spills to the next channel. Each group is set and counted separately. 0 = unlimited."
                )}
          </p>
          {mode === 'daily_quota' ? (
            <>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'The daily quota cap applies to the channel across all groups; saving it clears the RPM watermarks in every group.'
                )}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('Used today')}{' '}
                <span className='text-foreground font-medium'>
                  {formatAmount(todayUsed)}
                </span>
              </p>
            </>
          ) : (
            // 其他分组也在用这个渠道时，给出上游实际承受的合计 RPM
            rpmTotal > rpm && (
              <p className='text-muted-foreground text-xs'>
                {t('All groups combined')}{' '}
                <span className='text-foreground font-medium'>
                  {t('RPM')} {rpmTotal}
                </span>
              </p>
            )
          )}
          <div className='flex justify-end gap-2'>
            <Button
              variant='ghost'
              size='sm'
              disabled={saveMutation.isPending}
              onClick={() =>
                saveMutation.mutate({ mode, rpm: 0, dailyQuota: 0 })
              }
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
