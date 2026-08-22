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

import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { UserInfoItem } from '@/features/usage-logs/components/dialogs/user-info-item'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import { USER_STATUS, USER_STATUSES, isUserDeleted } from '../../constants'
import type { UserUsageSummary } from '../../lib/user-usage'
import type { User } from '../../types'

interface UserUsageSummaryPanelProps {
  user?: User
  summary: UserUsageSummary
  profileLoading: boolean
  usageLoading: boolean
}

function StatTile(props: { label: string; value: string; loading: boolean }) {
  return (
    <div className='bg-card rounded-lg border px-3 py-2.5'>
      <Label className='text-muted-foreground text-xs'>{props.label}</Label>
      {props.loading ? (
        <Skeleton className='mt-1.5 h-6 w-24' />
      ) : (
        <div className='mt-1 text-lg font-semibold tabular-nums'>
          {props.value}
        </div>
      )}
    </div>
  )
}

export function UserUsageSummaryPanel(props: UserUsageSummaryPanelProps) {
  const { t } = useTranslation()
  const user = props.user

  if (props.profileLoading || !user) {
    return (
      <div className='space-y-3'>
        <Skeleton className='h-24 w-full rounded-lg' />
        <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
          <Skeleton className='h-16 rounded-lg' />
          <Skeleton className='h-16 rounded-lg' />
          <Skeleton className='h-16 rounded-lg' />
        </div>
      </div>
    )
  }

  const statusKey = isUserDeleted(user) ? USER_STATUS.DELETED : user.status
  const statusMeta = USER_STATUSES[statusKey as keyof typeof USER_STATUSES]

  return (
    <div className='space-y-3'>
      <div className='bg-card grid grid-cols-2 gap-4 rounded-lg border p-4 lg:grid-cols-4'>
        <UserInfoItem
          label={t('Username')}
          value={user.username}
          copyable
          copyTooltip={t('Copy username')}
          copiedTooltip={t('Copied!')}
          copyAriaLabel={t('Copy username')}
        />
        <UserInfoItem label={t('User ID')} value={user.id} />
        <UserInfoItem label={t('Balance')} value={formatQuota(user.quota)} />
        <UserInfoItem
          label={t('Used Quota')}
          value={formatQuota(user.used_quota)}
        />
        <UserInfoItem
          label={t('Request Count')}
          value={formatCompactNumber(user.request_count)}
        />
        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-xs'>
            {t('User Group')}
          </Label>
          <div>
            <GroupBadge group={user.group} copyable />
          </div>
        </div>
        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-xs'>{t('Status')}</Label>
          <div>
            {statusMeta && (
              <StatusBadge
                label={t(statusMeta.labelKey)}
                variant={statusMeta.variant}
                copyable={false}
              />
            )}
          </div>
        </div>
        {user.display_name && user.display_name !== user.username ? (
          <UserInfoItem label={t('Display Name')} value={user.display_name} />
        ) : (
          <UserInfoItem label={t('Remark')} value={user.remark || '-'} />
        )}
      </div>

      <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
        <StatTile
          label={t('Quota used in range')}
          value={formatQuota(props.summary.totalQuota)}
          loading={props.usageLoading}
        />
        <StatTile
          label={t('Requests in range')}
          value={formatCompactNumber(props.summary.totalCount)}
          loading={props.usageLoading}
        />
        <StatTile
          label={t('Tokens in range')}
          value={formatCompactNumber(props.summary.totalTokens)}
          loading={props.usageLoading}
        />
      </div>
    </div>
  )
}
