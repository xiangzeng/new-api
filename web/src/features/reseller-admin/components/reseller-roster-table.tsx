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

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { ResellerPage } from '@/features/reseller/types'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ResellerRosterItem } from '../types'
import { DataSection } from './data-section'

export function ResellerRosterTable(props: {
  page: ResellerPage<ResellerRosterItem>
  selectedUserId: number
  searchInput: React.ReactNode
  onSelect: (reseller: ResellerRosterItem) => void
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <DataSection
      title={t('All resellers')}
      count={props.page.total}
      action={props.searchInput}
      empty={props.page.items.length === 0}
      emptyText={t('Nobody has opened a reseller center yet.')}
      page={props.page}
      onPageChange={props.onPageChange}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Reseller')}</TableHead>
            <TableHead>{t('Customers')}</TableHead>
            <TableHead>{t('Commission')}</TableHead>
            <TableHead>{t('Opened')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.page.items.map((reseller) => (
            <TableRow
              key={reseller.user_id}
              className={cn(
                reseller.user_id === props.selectedUserId && 'bg-muted/50'
              )}
            >
              <TableCell>
                <div className='font-medium'>
                  {reseller.display_name || reseller.username}
                  {reseller.status !== 'active' ? (
                    <Badge variant='destructive' className='ml-2'>
                      {t('Frozen')}
                    </Badge>
                  ) : null}
                  {reseller.user_status !== 1 ? (
                    <Badge variant='destructive' className='ml-2'>
                      {t('Disabled')}
                    </Badge>
                  ) : null}
                </div>
                <div className='text-muted-foreground text-xs'>
                  #{reseller.user_id} · {reseller.username}
                </div>
              </TableCell>
              <TableCell className='tabular-nums'>
                {reseller.customer_count}
              </TableCell>
              <TableCell className='tabular-nums'>
                <div>{formatQuota(reseller.available_commission_quota)}</div>
                <div className='text-muted-foreground mt-1 text-xs'>
                  {t('{{quota}} pending', {
                    quota: formatQuota(reseller.pending_commission_quota),
                  })}
                </div>
              </TableCell>
              <TableCell>
                {formatTimestampToDate(reseller.created_at)}
              </TableCell>
              <TableCell className='text-right'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => props.onSelect(reseller)}
                >
                  {t('View customers')}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataSection>
  )
}
