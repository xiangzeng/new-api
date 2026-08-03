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
import type { ColumnDef } from '@tanstack/react-table'
import { Users } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import type { InvitationSummary } from '../types'
import {
  InvitationUserCell,
  PeriodUsageCell,
  QuotaWithRequestsCell,
} from './invitation-cells'

interface UseInvitationsColumnsOptions {
  onViewInvitees: (summary: InvitationSummary) => void
}

export function useInvitationsColumns(
  options: UseInvitationsColumnsOptions
): ColumnDef<InvitationSummary, unknown>[] {
  const { t } = useTranslation()
  const onViewInvitees = options.onViewInvitees

  return useMemo(
    () => [
      {
        id: 'inviter',
        accessorKey: 'inviter_username',
        header: t('Inviter'),
        meta: { mobileTitle: true },
        size: 220,
        cell: ({ row }) => (
          <InvitationUserCell
            id={row.original.inviter_id}
            username={row.original.inviter_username}
            displayName={row.original.inviter_display_name}
            email={row.original.inviter_email}
            deleted={row.original.inviter_deleted}
          />
        ),
      },
      {
        id: 'inviter_id',
        accessorKey: 'inviter_id',
        header: t('ID'),
        meta: { mobileHidden: true },
        size: 80,
        cell: ({ row }) => <TableId value={row.original.inviter_id} />,
      },
      {
        id: 'aff_code',
        accessorKey: 'aff_code',
        header: t('Invitation Code'),
        size: 140,
        cell: ({ row }) =>
          row.original.aff_code ? (
            <StatusBadge label={row.original.aff_code} copyable />
          ) : (
            <span className='text-muted-foreground'>-</span>
          ),
      },
      {
        id: 'invitee_count',
        accessorKey: 'invitee_count',
        header: t('Invited Users'),
        size: 120,
        cell: ({ row }) => (
          <StatusBadge
            label={formatCompactNumber(row.original.invitee_count)}
            variant='blue'
            copyable={false}
          />
        ),
      },
      {
        id: 'invitee_total_used_quota',
        accessorKey: 'invitee_total_used_quota',
        header: t('Invitee Total Usage'),
        size: 170,
        cell: ({ row }) => (
          <QuotaWithRequestsCell
            quota={row.original.invitee_total_used_quota}
            requestCount={row.original.invitee_total_request_count}
          />
        ),
      },
      {
        id: 'period_quota',
        accessorKey: 'period_quota',
        header: t('Usage In Range'),
        size: 170,
        cell: ({ row }) => <PeriodUsageCell stats={row.original} />,
      },
      {
        id: 'aff_history_quota',
        accessorKey: 'aff_history_quota',
        header: t('Invitation Rewards'),
        size: 160,
        cell: ({ row }) => (
          <div className='flex flex-col items-start gap-0.5'>
            <span className='font-medium tabular-nums'>
              {formatQuota(row.original.aff_history_quota)}
            </span>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('Transferable')}: {formatQuota(row.original.aff_quota)}
            </span>
          </div>
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        size: 150,
        cell: ({ row }) => (
          <Button
            variant='outline'
            size='sm'
            disabled={row.original.invitee_count <= 0}
            onClick={() => onViewInvitees(row.original)}
          >
            <Users />
            {t('Invitees')}
          </Button>
        ),
      },
    ],
    [t, onViewInvitees]
  )
}
