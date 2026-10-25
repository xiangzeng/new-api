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
import {
  Ban,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Unlock,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  deleteUserGroupBlocks,
  getGroupBlockUsers,
  searchUsers,
} from '../users/api'
import { UserGroupBlocksDialog } from '../users/components/dialogs/user-group-blocks-dialog'
import { formatGroupBlockExpiry, formatGroupBlockRemaining } from '../users/lib'
import type { GroupBlockUserItem, User } from '../users/types'

type GroupBlockUserTarget = Pick<User, 'id' | 'username' | 'display_name'>

const groupBlockQueryKeys = {
  users: ['group-blocks', 'users'] as const,
  search: (keyword: string) => ['group-blocks', 'search', keyword] as const,
}

export function GroupBlocks() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [blocksDialogOpen, setBlocksDialogOpen] = useState(false)
  const [editingUser, setEditingUser] = useState<GroupBlockUserTarget | null>(
    null
  )
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('')
  const [selectedUser, setSelectedUser] = useState<GroupBlockUserTarget | null>(
    null
  )

  useEffect(() => {
    const timeout = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm.trim())
    }, 250)
    return () => clearTimeout(timeout)
  }, [searchTerm])

  useEffect(() => {
    if (!addDialogOpen) {
      setSearchTerm('')
      setDebouncedSearchTerm('')
      setSelectedUser(null)
    }
  }, [addDialogOpen])

  const groupBlockUsersQuery = useQuery({
    queryKey: groupBlockQueryKeys.users,
    queryFn: async () => {
      const result = await getGroupBlockUsers()
      if (!result.success) {
        throw new Error(
          result.message || t('Failed to load group restrictions')
        )
      }
      return result.data ?? []
    },
    // Expired restrictions drop off the list without a manual refresh.
    refetchInterval: 60_000,
  })

  const groupBlockUsers = useMemo(
    () => groupBlockUsersQuery.data ?? [],
    [groupBlockUsersQuery.data]
  )
  const restrictedUserIds = useMemo(
    () => new Set(groupBlockUsers.map((user) => user.id)),
    [groupBlockUsers]
  )

  const searchUsersQuery = useQuery({
    queryKey: groupBlockQueryKeys.search(debouncedSearchTerm),
    enabled: addDialogOpen && debouncedSearchTerm.length > 0,
    queryFn: async () => {
      const result = await searchUsers({
        keyword: debouncedSearchTerm,
        page_size: 20,
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to search users'))
      }
      return result.data?.items ?? []
    },
  })

  const searchResults = (searchUsersQuery.data ?? []).filter(
    (user) => !restrictedUserIds.has(user.id)
  )

  const invalidateGroupBlockUsers = async () => {
    await queryClient.invalidateQueries({
      queryKey: groupBlockQueryKeys.users,
    })
  }

  const openBlocksDialog = (user: GroupBlockUserTarget) => {
    setEditingUser(user)
    setBlocksDialogOpen(true)
  }

  const liftMutation = useMutation({
    mutationFn: async (user: GroupBlockUserTarget) => {
      const result = await deleteUserGroupBlocks(user.id)
      if (!result.success) {
        throw new Error(
          result.message || t('Failed to lift group restrictions')
        )
      }
      return user
    },
    onSuccess: async () => {
      toast.success(t('All group restrictions lifted'))
      await invalidateGroupBlockUsers()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to lift group restrictions')
      )
    },
  })

  const columns = useMemo<StaticDataTableColumn<GroupBlockUserItem>[]>(
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
        id: 'restrictedGroups',
        header: t('Restricted Groups'),
        cellClassName: 'min-w-[280px]',
        cell: (user) => (
          <div className='flex min-w-0 flex-wrap gap-1.5'>
            {(user.blocks ?? []).map((block) => {
              const permanent = block.expires_at === 0
              const tooltip = [
                permanent
                  ? t('Permanent')
                  : formatGroupBlockExpiry(block.expires_at),
                block.note,
              ]
                .filter(Boolean)
                .join(' · ')
              return (
                <Badge
                  key={block.group}
                  variant='outline'
                  className='max-w-full gap-1 font-normal'
                  title={tooltip}
                >
                  <span className='truncate'>{block.group}</span>
                  <span className='text-muted-foreground'>·</span>
                  <span
                    className={cn(
                      'tabular-nums font-medium',
                      permanent ? 'text-destructive' : 'text-foreground'
                    )}
                  >
                    {permanent
                      ? t('Permanent')
                      : formatGroupBlockRemaining(
                          block.expires_at,
                          i18n.language
                        )}
                  </span>
                </Badge>
              )
            })}
          </div>
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        className: 'w-[190px] text-right',
        cellClassName: 'text-right',
        cell: (user) => {
          const targetUser = {
            id: user.id,
            username: user.username,
            display_name: user.display_name,
          }
          return (
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => {
                  setEditingUser(targetUser)
                  setBlocksDialogOpen(true)
                }}
              >
                <Pencil />
                {t('Configure')}
              </Button>
              <Button
                variant='destructive'
                size='sm'
                onClick={() => liftMutation.mutate(targetUser)}
                disabled={liftMutation.isPending}
              >
                {liftMutation.isPending ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <Unlock />
                )}
                {t('Lift')}
              </Button>
            </div>
          )
        },
      },
    ],
    [liftMutation, t, i18n.language]
  )

  let groupBlocksContent = (
    <StaticDataTable
      data={groupBlockUsers}
      columns={columns}
      getRowKey={(user) => user.id}
      emptyContent={
        <EmptyState
          icon={Ban}
          title={t('No restricted users')}
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
  if (groupBlockUsersQuery.isLoading) {
    groupBlocksContent = <LoadingState />
  } else if (groupBlockUsersQuery.isError) {
    groupBlocksContent = (
      <EmptyState
        icon={Ban}
        title={t('Failed to load group restrictions')}
        action={
          <Button onClick={() => groupBlockUsersQuery.refetch()}>
            <RefreshCw />
            {t('Retry')}
          </Button>
        }
      />
    )
  }

  let searchContent = (
    <div className='space-y-1'>
      {searchResults.map((user) => {
        const selected = selectedUser?.id === user.id
        return (
          <button
            key={user.id}
            type='button'
            className={cn(
              'hover:bg-muted focus-visible:border-ring focus-visible:ring-ring/50 flex w-full items-center justify-between gap-3 rounded-md border border-transparent px-2.5 py-2 text-left outline-none focus-visible:ring-3',
              selected && 'border-primary bg-primary/5'
            )}
            onClick={() =>
              setSelectedUser({
                id: user.id,
                username: user.username,
                display_name: user.display_name,
              })
            }
          >
            <span className='min-w-0'>
              <span className='block truncate text-sm font-medium'>
                {user.display_name || user.username}
              </span>
              <span className='text-muted-foreground block truncate text-xs'>
                {user.username} · ID {user.id}
              </span>
            </span>
            <StatusBadge
              label={user.group}
              autoColor={user.group}
              copyable={false}
            />
          </button>
        )
      })}
    </div>
  )
  if (searchUsersQuery.isFetching) {
    searchContent = <LoadingState inline message={t('Searching...')} />
  } else if (debouncedSearchTerm.length === 0) {
    searchContent = (
      <div className='text-muted-foreground flex min-h-[200px] items-center justify-center text-sm'>
        {t('Search users to restrict groups')}
      </div>
    )
  } else if (searchUsersQuery.isError) {
    searchContent = (
      <div className='text-destructive flex min-h-[200px] items-center justify-center text-sm'>
        {t('Failed to search users')}
      </div>
    )
  } else if (searchResults.length === 0) {
    searchContent = (
      <div className='text-muted-foreground flex min-h-[200px] items-center justify-center text-sm'>
        {t('No matching users')}
      </div>
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Group Restrictions')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => groupBlockUsersQuery.refetch()}
            disabled={groupBlockUsersQuery.isFetching}
          >
            <RefreshCw
              className={cn(groupBlockUsersQuery.isFetching && 'animate-spin')}
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
              {groupBlocksContent}
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        title={
          <span className='flex items-center gap-2'>
            <Search className='size-5' />
            {t('Add Restricted User')}
          </span>
        }
        description={t('Select a user to configure group restrictions.')}
        contentClassName='sm:max-w-xl'
        footer={
          <>
            <Button variant='outline' onClick={() => setAddDialogOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={() => {
                if (!selectedUser) return
                setAddDialogOpen(false)
                openBlocksDialog(selectedUser)
              }}
              disabled={!selectedUser}
            >
              <Ban />
              {t('Next')}
            </Button>
          </>
        }
      >
        <div className='space-y-3'>
          <Input
            value={searchTerm}
            onChange={(event) => {
              setSearchTerm(event.target.value)
              setSelectedUser(null)
            }}
            placeholder={t('Search by ID, username, display name, or email')}
          />

          <div className='border-border bg-muted/20 min-h-[220px] rounded-lg border p-2'>
            {searchContent}
          </div>
        </div>
      </Dialog>

      <UserGroupBlocksDialog
        open={blocksDialogOpen}
        onOpenChange={setBlocksDialogOpen}
        user={editingUser}
        onSuccess={invalidateGroupBlockUsers}
      />
    </>
  )
}
