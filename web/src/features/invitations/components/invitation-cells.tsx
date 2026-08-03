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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import type { InvitationPeriodStats } from '../types'

interface InvitationUserCellProps {
  id: number
  username: string
  displayName?: string
  email?: string
  deleted?: boolean
}

/** Identity cell shared by the inviter summary table and the invitee panel. */
export function InvitationUserCell(props: InvitationUserCellProps) {
  const { t } = useTranslation()
  const secondary =
    props.displayName && props.displayName !== props.username
      ? props.displayName
      : props.email

  return (
    <div className='flex min-w-0 flex-col gap-1'>
      <div className='flex min-w-0 items-center gap-1.5'>
        <span className='truncate font-medium'>{props.username || '-'}</span>
        {props.deleted && (
          <StatusBadge label={t('Deleted')} variant='danger' copyable={false} />
        )}
      </div>
      <div className='text-muted-foreground truncate text-xs'>
        {secondary || `ID ${props.id}`}
      </div>
    </div>
  )
}

interface QuotaWithRequestsCellProps {
  quota: number
  requestCount: number
}

/** Quota headline with the matching request count underneath. */
export function QuotaWithRequestsCell(props: QuotaWithRequestsCellProps) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-col items-start gap-0.5'>
      <span className='font-medium tabular-nums'>
        {formatQuota(props.quota)}
      </span>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {t('{{count}} requests', {
          count: props.requestCount,
        })}
      </span>
    </div>
  )
}

/** Period consumption cell — token breakdown lives in the tooltip. */
export function PeriodUsageCell(props: { stats: InvitationPeriodStats }) {
  const { t } = useTranslation()
  const stats = props.stats

  return (
    <Tooltip>
      <TooltipTrigger render={<div className='w-fit cursor-default' />}>
        <QuotaWithRequestsCell
          quota={stats.period_quota}
          requestCount={stats.period_request_count}
        />
      </TooltipTrigger>
      <TooltipContent>
        <div className='space-y-0.5'>
          <div>
            {t('Prompt Tokens')}:{' '}
            {formatCompactNumber(stats.period_prompt_tokens)}
          </div>
          <div>
            {t('Completion Tokens')}:{' '}
            {formatCompactNumber(stats.period_completion_tokens)}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
