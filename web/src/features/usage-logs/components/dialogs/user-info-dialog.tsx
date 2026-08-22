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
import { Link } from '@tanstack/react-router'
import { ArrowRight, Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Label } from '@/components/ui/label'
import { getFlowQuotaDates } from '@/features/dashboard/api'
import {
  EMPTY_FLOW_GROUP_USAGE,
  aggregateFlowGroupUsage,
  type FlowGroupUsageSummary,
} from '@/features/dashboard/lib'
import { formatQuota, formatCompactNumber } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getUserInfo } from '../../api'
import type { UserInfo } from '../../types'
import { UserInfoItem } from './user-info-item'
import { UserRecentUsageSection } from './user-recent-usage-section'

/** Lookback window for the recent consumption breakdown. */
const RECENT_USAGE_LOOKBACK_SECONDS = 24 * 60 * 60

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function UserInfoBody(props: {
  userInfo: UserInfo
  usageSummary: FlowGroupUsageSummary
  usageLoading: boolean
  usageRange: { start: number; end: number } | null
  onNavigate: () => void
}) {
  const { t } = useTranslation()
  const userInfo = props.userInfo
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? ROLE.USER) >= ROLE.ADMIN
  )

  return (
    <div className='space-y-5 py-2'>
      <div className='grid grid-cols-2 gap-4'>
        <UserInfoItem
          label={t('Username')}
          value={userInfo.username}
          copyable
          copyTooltip={t('Copy username')}
          copiedTooltip={t('Copied!')}
          copyAriaLabel={t('Copy username')}
        />
        {userInfo.display_name ? (
          <UserInfoItem
            label={t('Display Name')}
            value={userInfo.display_name}
          />
        ) : (
          <UserInfoItem label={t('User ID')} value={userInfo.id} />
        )}
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <UserInfoItem
          label={t('Balance')}
          value={formatQuota(userInfo.quota)}
        />
        <UserInfoItem
          label={t('Used Quota')}
          value={formatQuota(userInfo.used_quota)}
        />
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <UserInfoItem
          label={t('Request Count')}
          value={formatCompactNumber(userInfo.request_count)}
        />
        {userInfo.group && (
          <div className='space-y-1.5'>
            <Label className='text-muted-foreground text-xs'>
              {t('User Group')}
            </Label>
            <div>
              <StatusBadge
                label={userInfo.group}
                autoColor={userInfo.group}
                copyable
              />
            </div>
          </div>
        )}
      </div>

      <UserRecentUsageSection
        summary={props.usageSummary}
        loading={props.usageLoading}
      />

      {isAdmin && props.usageRange && (
        <Link
          to='/users/$userId'
          params={{ userId: String(userInfo.id) }}
          search={{ start: props.usageRange.start, end: props.usageRange.end }}
          onClick={props.onNavigate}
          className='text-primary inline-flex items-center gap-1 text-sm hover:underline'
        >
          {t('View full usage')}
          <ArrowRight className='size-3.5' aria-hidden='true' />
        </Link>
      )}

      {(userInfo.aff_code ||
        userInfo.aff_count !== undefined ||
        (userInfo.aff_quota !== undefined && userInfo.aff_quota > 0)) && (
        <>
          <div className='grid grid-cols-2 gap-4'>
            {userInfo.aff_code && (
              <UserInfoItem
                label={t('Invitation Code')}
                value={userInfo.aff_code}
              />
            )}
            {userInfo.aff_count !== undefined && (
              <UserInfoItem
                label={t('Invited Users')}
                value={formatCompactNumber(userInfo.aff_count)}
              />
            )}
          </div>

          {userInfo.aff_quota !== undefined && userInfo.aff_quota > 0 && (
            <UserInfoItem
              label={t('Invitation Quota')}
              value={formatQuota(userInfo.aff_quota)}
            />
          )}
        </>
      )}

      {userInfo.remark && (
        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-xs'>{t('Remark')}</Label>
          <div className='text-sm leading-relaxed font-semibold break-words'>
            {userInfo.remark}
          </div>
        </div>
      )}
    </div>
  )
}

export function UserInfoDialog({
  userId,
  open,
  onOpenChange,
}: UserInfoDialogProps) {
  const { t } = useTranslation()
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [usageSummary, setUsageSummary] = useState<FlowGroupUsageSummary>(
    EMPTY_FLOW_GROUP_USAGE
  )
  const [isLoading, setIsLoading] = useState(false)
  const [usageLoading, setUsageLoading] = useState(false)
  const [usageRange, setUsageRange] = useState<{
    start: number
    end: number
  } | null>(null)

  const fetchUserInfo = useCallback(
    async (id: number) => {
      setIsLoading(true)
      setUsageLoading(true)
      setUserInfo(null)
      setUsageSummary(EMPTY_FLOW_GROUP_USAGE)
      setUsageRange(null)
      try {
        const userResult = await getUserInfo(id)
        if (userResult.success) {
          setUserInfo(userResult.data || null)
        } else {
          toast.error(
            userResult.message || t('Failed to fetch user information')
          )
        }
        setIsLoading(false)

        const username = userResult.data?.username
        if (!username) {
          setUsageLoading(false)
          return
        }

        const end = Math.floor(Date.now() / 1000)
        const start = end - RECENT_USAGE_LOOKBACK_SECONDS
        setUsageRange({ start, end })
        try {
          const flowResult = await getFlowQuotaDates(
            {
              start_timestamp: start,
              end_timestamp: end,
              username,
            },
            true
          )
          if (flowResult.success) {
            setUsageSummary(aggregateFlowGroupUsage(flowResult.data))
          }
        } catch (error) {
          // eslint-disable-next-line no-console
          console.error('Failed to fetch recent usage:', error)
        } finally {
          setUsageLoading(false)
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to fetch user info:', error)
        toast.error(t('Failed to fetch user information'))
        setIsLoading(false)
        setUsageLoading(false)
      }
    },
    [t]
  )

  useEffect(() => {
    if (open && userId) {
      fetchUserInfo(userId)
    } else if (!open) {
      setUserInfo(null)
      setUsageSummary(EMPTY_FLOW_GROUP_USAGE)
      setUsageRange(null)
    }
  }, [open, userId, fetchUserInfo])

  let body: ReactNode = (
    <div className='text-muted-foreground py-8 text-center text-sm'>
      {t('No user information available')}
    </div>
  )
  if (isLoading) {
    body = (
      <div className='flex items-center justify-center py-8'>
        <Loader2 className='text-muted-foreground size-6 animate-spin' />
      </div>
    )
  } else if (userInfo) {
    body = (
      <UserInfoBody
        userInfo={userInfo}
        usageSummary={usageSummary}
        usageLoading={usageLoading}
        usageRange={usageRange}
        onNavigate={() => onOpenChange(false)}
      />
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User Information')}
      description={t(
        'View detailed information about this user including balance, usage statistics, and invitation details.'
      )}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {body}
    </Dialog>
  )
}
