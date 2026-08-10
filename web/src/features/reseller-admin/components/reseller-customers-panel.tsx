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
import { Plus } from 'lucide-react'
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
import type { ResellerCustomer, ResellerPage } from '@/features/reseller/types'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import type { ResellerRosterItem } from '../types'
import { DataSection } from './data-section'

/**
 * Where the ownership edge came from, as recorded on the binding row. Same
 * wording as the users-list binding dialog so one relationship never reads as
 * two different things depending on which screen the operator opened.
 */
const SOURCE_LABELS: Record<string, string> = {
  primary: 'Primary site',
  reseller: 'Reseller invitation',
  admin: 'Admin binding',
  legacy_unknown: 'Legacy',
}

export function ResellerCustomersPanel(props: {
  reseller: ResellerRosterItem
  page: ResellerPage<ResellerCustomer>
  onPageChange: (page: number) => void
  onAdd: () => void
  onUnbind: (customer: ResellerCustomer) => void
  unbindingId: number
}) {
  const { t } = useTranslation()
  return (
    <DataSection
      title={t('Customers of {{reseller}}', {
        reseller: props.reseller.display_name || props.reseller.username,
      })}
      count={props.page.total}
      action={
        <Button size='sm' onClick={props.onAdd}>
          <Plus />
          {t('Add a direct customer')}
        </Button>
      }
      empty={props.page.items.length === 0}
      emptyText={t(
        'This reseller owns nobody yet. Use "Add a direct customer" to move an existing account under it.'
      )}
      page={props.page}
      onPageChange={props.onPageChange}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Balance')}</TableHead>
            <TableHead>{t('Current price')}</TableHead>
            <TableHead>{t('Bound')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.page.items.map((customer) => (
            <TableRow key={customer.binding_id}>
              <TableCell>
                <div className='font-medium'>
                  {customer.display_name || customer.username}
                  {customer.status !== 1 ? (
                    <Badge variant='destructive' className='ml-2'>
                      {t('Disabled')}
                    </Badge>
                  ) : null}
                </div>
                <div className='text-muted-foreground text-xs'>
                  #{customer.customer_id} · {customer.username}
                </div>
              </TableCell>
              <TableCell className='tabular-nums'>
                <div>{formatQuota(customer.quota)}</div>
                <div className='text-muted-foreground mt-1 text-xs'>
                  {t('{{quota}} used', {
                    quota: formatQuota(customer.used_quota),
                  })}
                </div>
              </TableCell>
              <TableCell className='tabular-nums'>
                {(customer.current_multiplier_bps / 10000).toFixed(4)}x
              </TableCell>
              <TableCell>
                <div>{formatTimestampToDate(customer.bound_at)}</div>
                <div className='text-muted-foreground mt-1 text-xs'>
                  {t(SOURCE_LABELS[customer.registration_source] ?? 'Legacy')}
                </div>
              </TableCell>
              <TableCell className='text-right'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={props.unbindingId === customer.customer_id}
                  onClick={() => props.onUnbind(customer)}
                >
                  {t('Unbind')}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataSection>
  )
}
