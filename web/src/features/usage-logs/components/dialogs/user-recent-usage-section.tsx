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
import { Loader2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { FlowGroupUsageSummary } from '@/features/dashboard/lib'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import { UserInfoItem } from './user-info-item'

interface UserRecentUsageSectionProps {
  summary: FlowGroupUsageSummary
  loading: boolean
}

export function UserRecentUsageSection(props: UserRecentUsageSectionProps) {
  const { t } = useTranslation()

  let content: ReactNode
  if (props.loading) {
    content = (
      <div className='flex items-center justify-center py-6'>
        <Loader2 className='text-muted-foreground size-5 animate-spin' />
      </div>
    )
  } else if (props.summary.groups.length === 0) {
    content = (
      <div className='text-muted-foreground text-sm'>
        {t('No usage in the last 24 hours')}
      </div>
    )
  } else {
    content = (
      <div className='space-y-3'>
        <div className='grid grid-cols-2 gap-4'>
          <UserInfoItem
            label={t('Last 24h Used Quota')}
            value={formatQuota(props.summary.totalQuota)}
          />
          <UserInfoItem
            label={t('Last 24h Requests')}
            value={formatCompactNumber(props.summary.totalCount)}
          />
        </div>
        <div className='border-border max-h-56 overflow-auto rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Group')}</TableHead>
                <TableHead className='w-[110px] text-right'>
                  {t('Used Quota')}
                </TableHead>
                <TableHead className='w-[90px] text-right'>
                  {t('Requests')}
                </TableHead>
                <TableHead className='w-[70px] text-right'>
                  {t('Share')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.summary.groups.map((row) => (
                <TableRow key={row.name}>
                  <TableCell className='font-medium'>
                    <span className='select-all'>{row.name}</span>
                  </TableCell>
                  <TableCell className='text-right font-semibold tabular-nums'>
                    {formatQuota(row.quota)}
                  </TableCell>
                  <TableCell className='text-muted-foreground text-right tabular-nums'>
                    {formatCompactNumber(row.count)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {`${Math.round(row.share * 1000) / 10}%`}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>
    )
  }

  return (
    <div className='space-y-2'>
      <Label className='text-sm font-medium'>
        {t('Usage in the last 24 hours')}
      </Label>
      <p className='text-muted-foreground text-xs'>
        {t('Total consumption and breakdown by group for the past 24 hours.')}
      </p>
      {content}
    </div>
  )
}
