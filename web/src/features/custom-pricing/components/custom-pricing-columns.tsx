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
import { Loader2, Pencil, PowerOff } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { StaticDataTableColumn } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import type { CustomPricingUserItem } from '../../users/types'

/** Max group badges rendered inline before collapsing into a "+N" badge. */
const MAX_VISIBLE_GROUP_BADGES = 6

export type CustomPricingUserTarget = Pick<
  CustomPricingUserItem,
  'id' | 'username' | 'display_name'
>

/** Compact ratio display for list badges (e.g. 0.2, 1.5). */
function formatConfiguredRatio(ratio: number): string {
  if (!Number.isFinite(ratio)) return String(ratio)
  // Trim trailing zeros while keeping enough precision for billing ratios.
  return Number(ratio.toPrecision(6)).toString()
}

interface UseCustomPricingColumnsOptions {
  onConfigure: (user: CustomPricingUserTarget) => void
  onDisable: (user: CustomPricingUserTarget) => void
  disablePending: boolean
}

export function useCustomPricingColumns(
  options: UseCustomPricingColumnsOptions
): StaticDataTableColumn<CustomPricingUserItem>[] {
  const { t } = useTranslation()
  const { onConfigure, onDisable, disablePending } = options

  return useMemo(
    () => [
      {
        id: 'user',
        header: t('User'),
        cell: (user) => (
          <div className='min-w-0'>
            <div className='truncate font-medium'>
              {user.display_name || user.username}
            </div>
            <div className='text-muted-foreground truncate text-xs'>
              {user.username} · ID {user.id}
            </div>
          </div>
        ),
      },
      {
        id: 'group',
        header: t('Group'),
        className: 'w-[140px]',
        cell: (user) => (
          <StatusBadge label={user.group} autoColor={user.group} copyable />
        ),
      },
      {
        id: 'configuredGroups',
        header: t('Configured Groups'),
        cellClassName: 'min-w-[280px]',
        cell: (user) => {
          const groups = user.groups ?? []
          if (groups.length === 0) {
            return (
              <span className='text-muted-foreground text-sm'>
                {t('No groups configured')}
              </span>
            )
          }
          const visible = groups.slice(0, MAX_VISIBLE_GROUP_BADGES)
          return (
            <div className='flex min-w-0 flex-wrap gap-1.5'>
              {visible.map((group) => (
                <Badge
                  key={group.name}
                  variant='outline'
                  className='max-w-full gap-1 font-normal'
                >
                  <span className='truncate'>{group.name}</span>
                  <span className='text-muted-foreground'>·</span>
                  <span className='text-foreground font-medium tabular-nums'>
                    {formatConfiguredRatio(group.ratio)}
                  </span>
                </Badge>
              ))}
              {groups.length > visible.length && (
                <Badge variant='secondary'>
                  +{groups.length - visible.length}
                </Badge>
              )}
            </div>
          )
        },
      },
      {
        id: 'actions',
        header: t('Actions'),
        className: 'w-[190px] text-right',
        cellClassName: 'text-right',
        cell: (user) => {
          const target: CustomPricingUserTarget = {
            id: user.id,
            username: user.username,
            display_name: user.display_name,
          }
          return (
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => onConfigure(target)}
              >
                <Pencil />
                {t('Configure')}
              </Button>
              <Button
                variant='destructive'
                size='sm'
                onClick={() => onDisable(target)}
                disabled={disablePending}
              >
                {disablePending ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <PowerOff />
                )}
                {t('Disable')}
              </Button>
            </div>
          )
        },
      },
    ],
    [t, onConfigure, onDisable, disablePending]
  )
}
