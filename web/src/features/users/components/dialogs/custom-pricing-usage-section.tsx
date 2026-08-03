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
import { ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { FlowGroupUsageRow } from '@/features/dashboard/lib'
import { formatCompactNumber, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

const TOP_USAGE_PREVIEW = 3

interface CustomPricingUsageSectionProps {
  groups: FlowGroupUsageRow[]
  loading: boolean
  expanded: boolean
  onExpandedChange: (expanded: boolean) => void
}

export function CustomPricingUsageSection(
  props: CustomPricingUsageSectionProps
) {
  const { t } = useTranslation()
  const hasMore = props.groups.length > TOP_USAGE_PREVIEW
  const visible = props.expanded
    ? props.groups
    : props.groups.slice(0, TOP_USAGE_PREVIEW)

  let body
  if (props.loading) {
    body = (
      <div className='flex items-center justify-center py-4'>
        <Loader2 className='text-muted-foreground h-5 w-5 animate-spin' />
      </div>
    )
  } else if (props.groups.length === 0) {
    body = (
      <p className='text-muted-foreground text-sm'>
        {t('No group usage in the last 7 days')}
      </p>
    )
  } else {
    const expandLabel = props.expanded
      ? t('Show less')
      : t('Show more groups ({{count}})', {
          count: props.groups.length - TOP_USAGE_PREVIEW,
        })
    body = (
      <div className='space-y-2'>
        <div className='border-border max-h-52 overflow-auto rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='w-[44px]'>{t('Rank')}</TableHead>
                <TableHead>{t('Group')}</TableHead>
                <TableHead className='w-[110px] text-right'>
                  {t('Used Quota')}
                </TableHead>
                <TableHead className='w-[80px] text-right'>
                  {t('Requests')}
                </TableHead>
                <TableHead className='w-[64px] text-right'>
                  {t('Share')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((row, index) => (
                <TableRow key={row.name}>
                  <TableCell className='text-muted-foreground tabular-nums'>
                    {index + 1}
                  </TableCell>
                  <TableCell className='font-medium'>{row.name}</TableCell>
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
        {hasMore ? (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='h-7 px-2 text-xs'
            onClick={() => props.onExpandedChange(!props.expanded)}
          >
            {expandLabel}
            <ChevronDown
              className={cn(
                'ml-1 h-3.5 w-3.5 transition-transform',
                props.expanded && 'rotate-180'
              )}
            />
          </Button>
        ) : null}
      </div>
    )
  }

  return (
    <div className='border-border space-y-2 rounded-lg border p-3'>
      <div className='space-y-1'>
        <p className='text-sm font-medium'>
          {t('Recent group usage (last 7 days)')}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Top groups by consumption — use this to decide preferential ratios.'
          )}
        </p>
      </div>
      {body}
    </div>
  )
}
