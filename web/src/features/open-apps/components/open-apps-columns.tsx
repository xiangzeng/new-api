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
import { KeyRound, Pencil, Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { StaticDataTableColumn } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import dayjs from '@/lib/dayjs'

import { OPEN_APP_STATUS, type OpenApp } from '../types'

interface UseOpenAppsColumnsOptions {
  onEdit: (app: OpenApp) => void
  onResetSecret: (app: OpenApp) => void
  onDelete: (app: OpenApp) => void
  actionsDisabled: boolean
}

export function useOpenAppsColumns(
  options: UseOpenAppsColumnsOptions
): StaticDataTableColumn<OpenApp>[] {
  const { t } = useTranslation()
  const { onEdit, onResetSecret, onDelete, actionsDisabled } = options

  return useMemo(
    () => [
      {
        id: 'name',
        header: t('Application'),
        cell: (app) => (
          <div className='min-w-0'>
            <div className='truncate font-medium'>{app.name}</div>
            <div className='text-muted-foreground truncate font-mono text-xs'>
              {app.app_id}
            </div>
          </div>
        ),
      },
      {
        id: 'secret',
        header: t('Secret'),
        className: 'w-[180px]',
        cell: (app) => (
          <span className='text-muted-foreground font-mono text-xs'>
            {app.secret_hint || '—'}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('Status'),
        className: 'w-[120px]',
        cell: (app) => (
          <StatusBadge
            label={
              app.status === OPEN_APP_STATUS.ENABLED
                ? t('Enabled')
                : t('Disabled')
            }
            variant={
              app.status === OPEN_APP_STATUS.ENABLED ? 'success' : 'neutral'
            }
            copyable={false}
          />
        ),
      },
      {
        id: 'restrictions',
        header: t('Restrictions'),
        cellClassName: 'min-w-[220px]',
        cell: (app) => {
          const allowedIps = app.allowed_ips
            .split('\n')
            .map((entry) => entry.trim())
            .filter(Boolean)
          return (
            <div className='flex flex-wrap items-center gap-1'>
              <Badge variant='outline'>
                {app.exchange_rate_limit > 0
                  ? t('{{limit}} exchanges/min', {
                      limit: app.exchange_rate_limit,
                    })
                  : t('Global rate limit')}
              </Badge>
              {allowedIps.length === 0 ? (
                <Badge variant='outline'>{t('Any source IP')}</Badge>
              ) : (
                allowedIps.map((entry) => (
                  <Badge key={entry} variant='outline' className='font-mono'>
                    {entry}
                  </Badge>
                ))
              )}
            </div>
          )
        },
      },
      {
        id: 'lastUsed',
        header: t('Last used'),
        className: 'w-[170px]',
        cell: (app) =>
          app.last_used_time > 0 ? (
            <span className='text-muted-foreground text-sm'>
              {dayjs.unix(app.last_used_time).format('YYYY-MM-DD HH:mm')}
            </span>
          ) : (
            <span className='text-muted-foreground text-sm'>{t('Never')}</span>
          ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        className: 'w-[220px]',
        cell: (app) => (
          <div className='flex items-center gap-1'>
            <Button
              variant='ghost'
              size='sm'
              disabled={actionsDisabled}
              onClick={() => onEdit(app)}
            >
              <Pencil />
              {t('Edit')}
            </Button>
            <Button
              variant='ghost'
              size='sm'
              disabled={actionsDisabled}
              onClick={() => onResetSecret(app)}
            >
              <KeyRound />
              {t('Reset secret')}
            </Button>
            <Button
              variant='ghost'
              size='sm'
              className='text-destructive'
              disabled={actionsDisabled}
              onClick={() => onDelete(app)}
            >
              <Trash2 />
            </Button>
          </div>
        ),
      },
    ],
    [actionsDisabled, onDelete, onEdit, onResetSecret, t]
  )
}
