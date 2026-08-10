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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plug, Plus, RefreshCw } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import {
  createOpenApp,
  deleteOpenApp,
  getOpenApps,
  resetOpenAppSecret,
  updateOpenApp,
} from './api'
import { OpenApiSettingsCard } from './components/open-api-settings-card'
import { OpenAppFormDialog } from './components/open-app-form-dialog'
import { OpenAppSecretDialog } from './components/open-app-secret-dialog'
import { useOpenAppsColumns } from './components/open-apps-columns'
import type { OpenApp, OpenAppRequest, OpenAppSecretPayload } from './types'

const OPEN_APPS_QUERY_KEY = ['open-apps'] as const

export function OpenApps() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editingApp, setEditingApp] = useState<OpenApp | null>(null)
  const [secretPayload, setSecretPayload] =
    useState<OpenAppSecretPayload | null>(null)
  const [resetTarget, setResetTarget] = useState<OpenApp | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<OpenApp | null>(null)

  const openAppsQuery = useQuery({
    queryKey: OPEN_APPS_QUERY_KEY,
    queryFn: async () => {
      const result = await getOpenApps()
      if (!result.success) {
        throw new Error(result.message || t('Failed to load applications'))
      }
      return result.data ?? []
    },
  })

  const invalidateOpenApps = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: OPEN_APPS_QUERY_KEY })
  }, [queryClient])

  const reportError = useCallback((error: unknown, fallback: string) => {
    toast.error(error instanceof Error ? error.message : fallback)
  }, [])

  const saveMutation = useMutation({
    mutationFn: async (values: OpenAppRequest) => {
      if (editingApp) {
        const result = await updateOpenApp(editingApp.id, values)
        if (!result.success) {
          throw new Error(result.message || t('Failed to save application'))
        }
        return null
      }
      const result = await createOpenApp(values)
      if (!result.success || !result.data) {
        throw new Error(result.message || t('Failed to save application'))
      }
      return result.data
    },
    onSuccess: async (payload) => {
      toast.success(t('Application saved'))
      setFormOpen(false)
      setEditingApp(null)
      await invalidateOpenApps()
      if (payload) setSecretPayload(payload)
    },
    onError: (error) => reportError(error, t('Failed to save application')),
  })

  const resetSecretMutation = useMutation({
    mutationFn: async (app: OpenApp) => {
      const result = await resetOpenAppSecret(app.id)
      if (!result.success || !result.data) {
        throw new Error(result.message || t('Failed to reset the secret'))
      }
      return result.data
    },
    onSuccess: async (payload) => {
      toast.success(t('Secret reset'))
      setResetTarget(null)
      await invalidateOpenApps()
      setSecretPayload(payload)
    },
    onError: (error) => reportError(error, t('Failed to reset the secret')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (app: OpenApp) => {
      const result = await deleteOpenApp(app.id)
      if (!result.success) {
        throw new Error(result.message || t('Failed to delete the application'))
      }
    },
    onSuccess: async () => {
      toast.success(t('Application deleted'))
      setDeleteTarget(null)
      await invalidateOpenApps()
    },
    onError: (error) =>
      reportError(error, t('Failed to delete the application')),
  })

  const handleCreate = useCallback(() => {
    setEditingApp(null)
    setFormOpen(true)
  }, [])

  const handleEdit = useCallback((app: OpenApp) => {
    setEditingApp(app)
    setFormOpen(true)
  }, [])

  const columns = useOpenAppsColumns({
    onEdit: handleEdit,
    onResetSecret: setResetTarget,
    onDelete: setDeleteTarget,
    actionsDisabled: saveMutation.isPending || deleteMutation.isPending,
  })

  let content = (
    <StaticDataTable
      data={openAppsQuery.data ?? []}
      columns={columns}
      getRowKey={(app) => app.id}
      emptyContent={
        <EmptyState
          icon={Plug}
          title={t('No applications yet')}
          description={t(
            'Issue credentials to a partner site so it can query balances on behalf of its users.'
          )}
          action={
            <Button onClick={handleCreate}>
              <Plus />
              {t('New Application')}
            </Button>
          }
          className='min-h-[220px]'
        />
      }
    />
  )
  if (openAppsQuery.isLoading) {
    content = <LoadingState />
  } else if (openAppsQuery.isError) {
    content = (
      <EmptyState
        icon={Plug}
        title={t('Failed to load applications')}
        action={
          <Button onClick={() => openAppsQuery.refetch()}>
            <RefreshCw />
            {t('Retry')}
          </Button>
        }
      />
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Balance Open API')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => openAppsQuery.refetch()}
            disabled={openAppsQuery.isFetching}
          >
            <RefreshCw
              className={cn(openAppsQuery.isFetching && 'animate-spin')}
            />
            {t('Refresh')}
          </Button>
          <Button onClick={handleCreate}>
            <Plus />
            {t('New Application')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto'>
            <OpenApiSettingsCard />
            <Card className='min-h-0 rounded-lg'>
              <CardContent className='min-h-0 flex-1 overflow-auto'>
                {content}
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <OpenAppFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        app={editingApp}
        onSubmit={(values) => saveMutation.mutate(values)}
        pending={saveMutation.isPending}
      />

      <OpenAppSecretDialog
        open={secretPayload !== null}
        onOpenChange={(open) => {
          if (!open) setSecretPayload(null)
        }}
        payload={secretPayload}
      />

      <AlertDialog
        open={resetTarget !== null}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Reset secret')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'A new secret will be issued and every credential this application currently holds will be revoked. Its users must authorize again.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={resetSecretMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={resetSecretMutation.isPending}
              onClick={() =>
                resetTarget && resetSecretMutation.mutate(resetTarget)
              }
            >
              {t('Reset secret')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete application')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This removes the application and revokes every credential issued under it. This cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteMutation.isPending}
              onClick={() =>
                deleteTarget && deleteMutation.mutate(deleteTarget)
              }
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
