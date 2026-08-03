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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useIsAdmin } from '@/hooks/use-admin'

import { deleteErrorLogs } from '../api'

/**
 * Admin-only toolbar action that drops every error-type log in one call.
 * Consumption logs and other log types are left untouched by the backend.
 */
export function ClearErrorLogsButton() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const clearErrorLogs = useMutation({
    mutationFn: deleteErrorLogs,
    onSuccess: (data) => {
      // Business failures are already surfaced by the axios response interceptor
      if (!data.success) return

      setConfirmOpen(false)
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
      toast.success(
        t('Cleared {{value}} error logs', { value: data.data ?? 0 })
      )
    },
  })

  if (!isAdmin) return null

  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              onClick={() => setConfirmOpen(true)}
              aria-label={t('Clear error logs')}
              className='text-muted-foreground hover:text-destructive size-7'
            />
          }
        >
          <Trash2 />
        </TooltipTrigger>
        <TooltipContent>{t('Clear error logs')}</TooltipContent>
      </Tooltip>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Clear error logs')}
        desc={t(
          'This deletes every error-type log. Consumption logs are not affected. This action cannot be undone.'
        )}
        destructive
        confirmText={t('Clear error logs')}
        isLoading={clearErrorLogs.isPending}
        handleConfirm={() => clearErrorLogs.mutate()}
      />
    </>
  )
}
