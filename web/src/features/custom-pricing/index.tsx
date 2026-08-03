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
import { Plus, RefreshCw, SlidersHorizontal } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import {
  deleteUserCustomPricing,
  getCustomPricingUsers,
  updateUserCustomPricing,
} from '../users/api'
import { UserCustomPricingDialog } from '../users/components/dialogs/user-custom-pricing-dialog'
import { AddCustomPricingUserDialog } from './components/add-custom-pricing-user-dialog'
import {
  useCustomPricingColumns,
  type CustomPricingUserTarget,
} from './components/custom-pricing-columns'

const CUSTOM_PRICING_USERS_QUERY_KEY = ['custom-pricing', 'users'] as const

export function CustomPricing() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pricingDialogOpen, setPricingDialogOpen] = useState(false)
  const [editingUser, setEditingUser] =
    useState<CustomPricingUserTarget | null>(null)
  const [addDialogOpen, setAddDialogOpen] = useState(false)

  const customPricingUsersQuery = useQuery({
    queryKey: CUSTOM_PRICING_USERS_QUERY_KEY,
    queryFn: async () => {
      const result = await getCustomPricingUsers()
      if (!result.success) {
        throw new Error(
          result.message || t('Failed to load custom pricing users')
        )
      }
      return result.data ?? []
    },
  })

  const customPricingUsers = useMemo(
    () => customPricingUsersQuery.data ?? [],
    [customPricingUsersQuery.data]
  )
  const enabledUserIds = useMemo(
    () => new Set(customPricingUsers.map((user) => user.id)),
    [customPricingUsers]
  )

  const invalidateCustomPricingUsers = useCallback(async () => {
    await queryClient.invalidateQueries({
      queryKey: CUSTOM_PRICING_USERS_QUERY_KEY,
    })
  }, [queryClient])

  const enableMutation = useMutation({
    mutationFn: async (user: CustomPricingUserTarget) => {
      const result = await updateUserCustomPricing(user.id, {
        enabled: true,
        groups: {},
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to enable custom pricing'))
      }
      return user
    },
    onSuccess: async (user) => {
      toast.success(t('Custom pricing enabled'))
      await invalidateCustomPricingUsers()
      setAddDialogOpen(false)
      setEditingUser(user)
      setPricingDialogOpen(true)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to enable custom pricing')
      )
    },
  })

  const disableMutation = useMutation({
    mutationFn: async (user: CustomPricingUserTarget) => {
      const result = await deleteUserCustomPricing(user.id)
      if (!result.success) {
        throw new Error(result.message || t('Failed to disable custom pricing'))
      }
      return user
    },
    onSuccess: async () => {
      toast.success(t('Custom pricing disabled'))
      await invalidateCustomPricingUsers()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to disable custom pricing')
      )
    },
  })

  const handleConfigure = useCallback((user: CustomPricingUserTarget) => {
    setEditingUser(user)
    setPricingDialogOpen(true)
  }, [])

  const handleDisable = useCallback(
    (user: CustomPricingUserTarget) => disableMutation.mutate(user),
    [disableMutation]
  )

  const columns = useCustomPricingColumns({
    onConfigure: handleConfigure,
    onDisable: handleDisable,
    disablePending: disableMutation.isPending,
  })

  let customPricingContent = (
    <StaticDataTable
      data={customPricingUsers}
      columns={columns}
      getRowKey={(user) => user.id}
      emptyContent={
        <EmptyState
          icon={SlidersHorizontal}
          title={t('No custom pricing users')}
          action={
            <Button onClick={() => setAddDialogOpen(true)}>
              <Plus />
              {t('Add User')}
            </Button>
          }
          className='min-h-[220px]'
        />
      }
    />
  )
  if (customPricingUsersQuery.isLoading) {
    customPricingContent = <LoadingState />
  } else if (customPricingUsersQuery.isError) {
    customPricingContent = (
      <EmptyState
        icon={SlidersHorizontal}
        title={t('Failed to load custom pricing users')}
        action={
          <Button onClick={() => customPricingUsersQuery.refetch()}>
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
        <SectionPageLayout.Title>{t('Custom Pricing')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => customPricingUsersQuery.refetch()}
            disabled={customPricingUsersQuery.isFetching}
          >
            <RefreshCw
              className={cn(
                customPricingUsersQuery.isFetching && 'animate-spin'
              )}
            />
            {t('Refresh')}
          </Button>
          <Button onClick={() => setAddDialogOpen(true)}>
            <Plus />
            {t('Add User')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Card className='h-full min-h-0 rounded-lg'>
            <CardContent className='min-h-0 flex-1 overflow-auto'>
              {customPricingContent}
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <AddCustomPricingUserDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        enabledUserIds={enabledUserIds}
        onEnable={(user) => enableMutation.mutate(user)}
        enablePending={enableMutation.isPending}
      />

      <UserCustomPricingDialog
        open={pricingDialogOpen}
        onOpenChange={setPricingDialogOpen}
        user={editingUser}
        onSuccess={invalidateCustomPricingUsers}
      />
    </>
  )
}
